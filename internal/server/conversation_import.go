package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/connectors"
	"github.com/luoyif/memory-harness/internal/contracts"
	"github.com/luoyif/memory-harness/internal/sourcearchive"
)

func (s *Server) importConversations(w http.ResponseWriter, r *http.Request) {
	rawBody, err := io.ReadAll(io.LimitReader(r.Body, (64<<20)+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_conversation_import", err.Error())
		return
	}
	if len(rawBody) > 64<<20 {
		writeErr(w, http.StatusRequestEntityTooLarge, "import_too_large", "conversation import exceeds 64 MiB")
		return
	}
	var request struct {
		Format         string          `json:"format"`
		ProjectID      string          `json:"project_id"`
		ConnectorID    string          `json:"connector_id"`
		IdempotencyKey string          `json:"idempotency_key"`
		DryRun         bool            `json:"dry_run"`
		Payload        json.RawMessage `json:"payload"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(rawBody)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_conversation_import", err.Error())
		return
	}
	project, exact, err := s.app.Portfolio.Resolve(r.Context(), request.ProjectID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_conversation_import", err.Error())
		return
	}
	if !exact {
		writeErr(w, http.StatusBadRequest, "bad_conversation_import", "project_id must identify a registered project")
		return
	}
	conversations, preview, err := connectors.Parse(request.Format, request.Payload)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "parse_failed", err.Error())
		return
	}
	rawHash := contracts.HashBytes(request.Payload)
	if request.DryRun {
		writeJSON(w, http.StatusOK, map[string]any{"dry_run": true, "project_id": project.ProjectID, "source_sha256": rawHash, "preview": preview})
		return
	}
	if strings.TrimSpace(request.IdempotencyKey) == "" {
		request.IdempotencyKey = "conversation:" + strings.ToLower(strings.TrimSpace(request.Format)) + ":" + rawHash
	}
	batch, duplicate, err := s.app.Portfolio.BeginImportBatch(r.Context(), request.ConnectorID, project.ProjectID, request.IdempotencyKey)
	if err != nil {
		writePortfolioError(w, "import_batch_failed", err)
		return
	}
	if duplicate && batch.Status == "completed" {
		writeJSON(w, http.StatusOK, map[string]any{"duplicate": true, "batch": batch, "preview": preview})
		return
	}
	archive, err := sourcearchive.Preserve(s.app.Config.SourcesDir(), request.Payload)
	if err != nil {
		_, _ = s.app.Portfolio.CompleteImportBatch(r.Context(), batch.BatchID, nil, err.Error())
		writeErr(w, http.StatusInternalServerError, "archive_failed", err.Error())
		return
	}

	parserName := strings.ToLower(strings.TrimSpace(request.Format))
	parserVersion := connectors.ParserVersion
	batchID := batch.BatchID
	evidenceIDs := []string{}
	evidenceBySession := map[string][]string{}
	for _, conversation := range conversations {
		sessionID := parserName + ":" + conversation.ExternalID
		idMap := map[string]string{}
		for _, message := range conversation.Messages {
			idMap[message.ExternalID] = "import_" + contracts.HashBytes([]byte(parserName + "\x00" + conversation.ExternalID + "\x00" + message.ExternalID + "\x00" + message.Text))[:24]
		}
		previousEvidenceID := ""
		for _, message := range conversation.Messages {
			evidenceID := idMap[message.ExternalID]
			observed := time.Now().UTC()
			if message.ObservedAt != "" {
				observed, _ = time.Parse(time.RFC3339, message.ObservedAt)
			} else if conversation.CreatedAt != "" {
				observed, _ = time.Parse(time.RFC3339, conversation.CreatedAt)
			}
			captured := time.Now().UTC()
			if prior, ok, receiptErr := s.app.Control.Receipt(r.Context(), evidenceID); receiptErr == nil && ok {
				if parsed, parseErr := time.Parse(time.RFC3339Nano, prior.ObservedAt); parseErr == nil {
					observed = parsed
				}
				if parsed, parseErr := time.Parse(time.RFC3339Nano, prior.CapturedAt); parseErr == nil {
					captured = parsed
				}
			}
			parent := previousEvidenceID
			if mapped := idMap[message.ParentID]; mapped != "" {
				parent = mapped
			}
			role := message.Role
			externalID := message.ExternalID
			envelope := contracts.EvidenceEnvelope{
				SchemaVersion:          "0.1",
				EvidenceID:             evidenceID,
				SourceSystem:           parserName,
				ExternalConversationID: &sessionID,
				ExternalMessageID:      &externalID,
				Role:                   &role,
				ObservedAt:             &observed,
				CapturedAt:             captured,
				Content:                []contracts.ContentBlock{{Type: "text", Text: message.Text}},
				Provenance:             contracts.Provenance{CaptureMethod: "conversation_export", RawBatchID: &batchID, RawHash: &archive.Hash, RawPath: &archive.RelPath, Parser: &parserName, ParserVersion: &parserVersion},
				ScopeHints:             []string{"project:" + project.Slug},
				Visibility:             "private",
				ParseWarnings:          message.Warnings,
			}
			if parent != "" {
				envelope.ParentEvidenceID = &parent
			}
			raw, _ := json.Marshal(envelope)
			result, appendErr := s.app.Ledger.Append(r.Context(), raw)
			if appendErr != nil {
				_, _ = s.app.Portfolio.CompleteImportBatch(r.Context(), batch.BatchID, evidenceIDs, appendErr.Error())
				writeErr(w, http.StatusBadRequest, "conversation_import_failed", appendErr.Error())
				return
			}
			if err := s.app.Portfolio.LinkRecord(r.Context(), "evidence", result.EvidenceID, project.ProjectID, true); err != nil {
				_, _ = s.app.Portfolio.CompleteImportBatch(r.Context(), batch.BatchID, evidenceIDs, err.Error())
				writeErr(w, http.StatusInternalServerError, "conversation_route_failed", err.Error())
				return
			}
			evidenceIDs = append(evidenceIDs, result.EvidenceID)
			evidenceBySession[sessionID] = append(evidenceBySession[sessionID], result.EvidenceID)
			previousEvidenceID = result.EvidenceID
		}
	}
	autoProcess, processingMode, err := s.projectAutoProcesses(r.Context(), project.ProjectID)
	if err != nil {
		_, _ = s.app.Portfolio.CompleteImportBatch(r.Context(), batch.BatchID, evidenceIDs, err.Error())
		writeErr(w, http.StatusInternalServerError, "conversation_process_failed", err.Error())
		return
	}
	if autoProcess {
		for sessionID, ids := range evidenceBySession {
			_, processErr := s.growSession(r.Context(), project.ProjectID, sessionID, ids, true, false)
			if processErr != nil {
				_, _ = s.app.Portfolio.CompleteImportBatch(r.Context(), batch.BatchID, evidenceIDs, processErr.Error())
				writeErr(w, http.StatusInternalServerError, "conversation_process_failed", processErr.Error())
				return
			}
		}
	}
	batch, err = s.app.Portfolio.CompleteImportBatch(r.Context(), batch.BatchID, evidenceIDs, "")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "import_batch_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"project_id": project.ProjectID, "preview": preview, "source_archive": archive, "batch": batch, "processing_mode": processingMode})
}
