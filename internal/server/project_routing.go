package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/luoyif/memory-harness/internal/contracts"
	"github.com/luoyif/memory-harness/internal/growth"
	"github.com/luoyif/memory-harness/internal/memory"
	"github.com/luoyif/memory-harness/internal/portfolio"
)

func (s *Server) sessionRoute(ctx context.Context, sessionID string) (string, []string, error) {
	rows, err := s.app.SearchStore.DB.QueryContext(ctx, `SELECT evidence_id FROM turns WHERE session_id=? ORDER BY observed_at,id`, strings.TrimSpace(sessionID))
	if err != nil {
		return "", nil, err
	}
	evidenceIDs := []string{}
	for rows.Next() {
		var evidenceID string
		if err := rows.Scan(&evidenceID); err != nil {
			rows.Close()
			return "", nil, err
		}
		evidenceIDs = append(evidenceIDs, evidenceID)
	}
	if err := rows.Close(); err != nil {
		return "", nil, err
	}
	if len(evidenceIDs) == 0 {
		return "", nil, sql.ErrNoRows
	}
	for _, evidenceID := range evidenceIDs {
		projectID, err := s.app.Portfolio.ProjectForRecord(ctx, "evidence", evidenceID)
		if err == nil {
			return projectID, evidenceIDs, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", nil, err
		}
	}
	return portfolio.InboxProjectID, evidenceIDs, nil
}

func (s *Server) growSession(ctx context.Context, projectID, sessionID string, evidenceIDs []string, primary, force bool) (memory.RunResult, error) {
	callerType, callerID := "system", "memory-growth"
	if principal, ok := ownerFromContext(ctx); ok {
		callerType, callerID = "owner", principal.SessionID
	}
	result, err := s.app.Growth.Process(ctx, growth.ProcessInput{
		ProjectID: projectID, SessionID: sessionID, EvidenceIDs: evidenceIDs, Primary: primary, Force: force,
		CallerType: callerType, CallerID: callerID, Channel: "default-import",
	})
	return result.Compilation, err
}

func (s *Server) projectForEvidence(ctx context.Context, raw []byte, explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		project, exact, err := s.app.Portfolio.Resolve(ctx, explicit)
		if err != nil {
			return "", err
		}
		if exact {
			return project.ProjectID, nil
		}
	}
	envelope, _, err := contracts.ParseEvidence(raw)
	if err != nil {
		return "", err
	}
	for _, hint := range envelope.ScopeHints {
		value := strings.TrimSpace(hint)
		if strings.HasPrefix(strings.ToLower(value), "project:") {
			value = strings.TrimSpace(value[len("project:"):])
		}
		project, exact, err := s.app.Portfolio.Resolve(ctx, value)
		if err != nil {
			return "", err
		}
		if exact {
			return project.ProjectID, nil
		}
	}
	return portfolio.InboxProjectID, nil
}

func evidenceIDFromRaw(raw json.RawMessage) string {
	var head struct {
		EvidenceID string `json:"evidence_id"`
	}
	_ = json.Unmarshal(raw, &head)
	return head.EvidenceID
}
