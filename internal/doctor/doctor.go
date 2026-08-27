package doctor

import (
	"context"
	"fmt"

	"github.com/luoyif/memory-harness/internal/app"
	"github.com/luoyif/memory-harness/internal/contracts"
	"github.com/luoyif/memory-harness/internal/localembedding"
)

type Report struct {
	Status               string   `json:"status"`
	LedgerRecords        int      `json:"ledger_records"`
	ControlReceipts      int      `json:"control_receipts"`
	SearchTurns          int      `json:"search_turns"`
	FTSUnicodeRows       int      `json:"fts_unicode_rows"`
	FTSTrigramRows       int      `json:"fts_trigram_rows"`
	Episodes             int      `json:"episodes"`
	KnowledgeUnits       int      `json:"knowledge_units"`
	MemoryRecords        int      `json:"memory_records"`
	MemoryOperations     int      `json:"memory_operations"`
	LivingViews          int      `json:"living_views"`
	AgentAssets          int      `json:"agent_assets"`
	Projects             int      `json:"projects"`
	TemporalFacts        int      `json:"temporal_facts"`
	Goals                int      `json:"goals"`
	Risks                int      `json:"risks"`
	FinanceEntries       int      `json:"finance_entries"`
	Connectors           int      `json:"connectors"`
	AgentPrincipals      int      `json:"agent_principals"`
	AgentAuditEvents     int      `json:"agent_audit_events"`
	ModelProviders       int      `json:"model_providers"`
	UnifiedDocuments     int      `json:"unified_documents"`
	UnifiedUnicodeRows   int      `json:"unified_unicode_rows"`
	UnifiedTrigramRows   int      `json:"unified_trigram_rows"`
	UnifiedEmbeddingRows int      `json:"unified_embedding_rows"`
	EmbeddingAlgorithm   string   `json:"embedding_algorithm"`
	EmbeddingDimensions  int      `json:"embedding_dimensions"`
	Errors               []string `json:"errors,omitempty"`
	Warnings             []string `json:"warnings,omitempty"`
}

type seenRecord struct {
	Hash, Rel string
	Ordinal   int
}

func Run(ctx context.Context, a *app.App) (Report, error) {
	r := Report{Status: "pass", EmbeddingAlgorithm: localembedding.Algorithm, EmbeddingDimensions: localembedding.Dimensions}
	seen := map[string]seenRecord{}
	err := a.Ledger.Walk(ctx, func(rel string, ordinal int, e contracts.EvidenceEnvelope, compact []byte) error {
		r.LedgerRecords++
		h := contracts.HashBytes(compact)
		if old, ok := seen[e.EvidenceID]; ok {
			r.Errors = append(r.Errors, fmt.Sprintf("duplicate evidence_id %s at %s:%d and %s:%d", e.EvidenceID, old.Rel, old.Ordinal, rel, ordinal))
		} else {
			seen[e.EvidenceID] = seenRecord{h, rel, ordinal}
		}
		receipt, ok, err := a.Control.Receipt(ctx, e.EvidenceID)
		if err != nil {
			return err
		}
		if !ok {
			r.Errors = append(r.Errors, "missing control receipt for "+e.EvidenceID)
			return nil
		}
		if receipt.LineHash != h {
			r.Errors = append(r.Errors, "receipt hash mismatch for "+e.EvidenceID)
		}
		if receipt.LedgerRelPath != rel || receipt.Ordinal != ordinal {
			r.Errors = append(r.Errors, "receipt locator mismatch for "+e.EvidenceID)
		}
		return nil
	})
	if err != nil {
		return r, err
	}
	r.ControlReceipts, err = a.Control.CountReceipts(ctx)
	if err != nil {
		return r, err
	}
	r.SearchTurns, err = a.SearchStore.CountTurns(ctx)
	if err != nil {
		return r, err
	}
	rows, err := a.SearchStore.DB.QueryContext(ctx, `SELECT evidence_id,line_hash,ledger_rel_path,ordinal FROM turns`)
	if err != nil {
		return r, err
	}
	for rows.Next() {
		var id, hash, rel string
		var ordinal int
		if err := rows.Scan(&id, &hash, &rel, &ordinal); err != nil {
			rows.Close()
			return r, err
		}
		sr, ok := seen[id]
		if !ok {
			r.Errors = append(r.Errors, "search index contains non-ledger evidence "+id)
			continue
		}
		if sr.Hash != hash || sr.Rel != rel || sr.Ordinal != ordinal {
			r.Errors = append(r.Errors, "search locator/hash mismatch for "+id)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return r, err
	}
	rows.Close()
	if err := a.SearchStore.DB.QueryRowContext(ctx, `SELECT count(*) FROM turns_fts`).Scan(&r.FTSUnicodeRows); err != nil {
		return r, err
	}
	if err := a.SearchStore.DB.QueryRowContext(ctx, `SELECT count(*) FROM turns_tri`).Scan(&r.FTSTrigramRows); err != nil {
		return r, err
	}
	if r.ControlReceipts != r.LedgerRecords {
		r.Errors = append(r.Errors, fmt.Sprintf("control receipt count %d != ledger count %d", r.ControlReceipts, r.LedgerRecords))
	}
	if r.SearchTurns != r.LedgerRecords {
		r.Errors = append(r.Errors, fmt.Sprintf("search turn count %d != ledger count %d", r.SearchTurns, r.LedgerRecords))
	}
	if r.FTSUnicodeRows != r.SearchTurns || r.FTSTrigramRows != r.SearchTurns {
		r.Errors = append(r.Errors, "FTS row counts do not match turns")
	}
	for _, count := range []struct {
		name   string
		query  string
		target *int
	}{
		{"episodes", `SELECT count(*) FROM episodes`, &r.Episodes},
		{"knowledge_units", `SELECT count(*) FROM knowledge_units`, &r.KnowledgeUnits},
		{"memory_records", `SELECT count(*) FROM memory_records`, &r.MemoryRecords},
		{"memory_operations", `SELECT count(*) FROM memory_operations`, &r.MemoryOperations},
		{"living_views", `SELECT count(*) FROM living_views`, &r.LivingViews},
		{"agent_assets", `SELECT count(*) FROM agent_assets`, &r.AgentAssets},
		{"projects", `SELECT count(*) FROM projects`, &r.Projects},
		{"temporal_facts", `SELECT count(*) FROM temporal_facts`, &r.TemporalFacts},
		{"goals", `SELECT count(*) FROM project_goals`, &r.Goals},
		{"risks", `SELECT count(*) FROM project_risks`, &r.Risks},
		{"finance_entries", `SELECT count(*) FROM finance_entries`, &r.FinanceEntries},
		{"connectors", `SELECT count(*) FROM connectors`, &r.Connectors},
		{"agent_principals", `SELECT count(*) FROM agent_principals`, &r.AgentPrincipals},
		{"agent_audit_log", `SELECT count(*) FROM agent_audit_log`, &r.AgentAuditEvents},
		{"model_providers", `SELECT count(*) FROM model_providers`, &r.ModelProviders},
	} {
		if err := a.Control.DB.QueryRowContext(ctx, count.query).Scan(count.target); err != nil {
			return r, fmt.Errorf("count %s: %w", count.name, err)
		}
	}
	var orphanUnits, orphanOperations int
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_units u LEFT JOIN evidence_receipts e ON e.evidence_id=u.evidence_id WHERE e.evidence_id IS NULL`).Scan(&orphanUnits); err != nil {
		return r, err
	}
	if orphanUnits > 0 {
		r.Errors = append(r.Errors, fmt.Sprintf("%d knowledge units reference missing Evidence", orphanUnits))
	}
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT count(*) FROM memory_operations o LEFT JOIN memory_records m ON m.memory_id=o.target_memory_id WHERE o.target_memory_id IS NOT NULL AND m.memory_id IS NULL`).Scan(&orphanOperations); err != nil {
		return r, err
	}
	if orphanOperations > 0 {
		r.Errors = append(r.Errors, fmt.Sprintf("%d memory operations reference missing records", orphanOperations))
	}
	if err := a.SearchStore.DB.QueryRowContext(ctx, `SELECT count(*) FROM documents`).Scan(&r.UnifiedDocuments); err != nil {
		return r, err
	}
	if err := a.SearchStore.DB.QueryRowContext(ctx, `SELECT count(*) FROM documents_fts`).Scan(&r.UnifiedUnicodeRows); err != nil {
		return r, err
	}
	if err := a.SearchStore.DB.QueryRowContext(ctx, `SELECT count(*) FROM documents_tri`).Scan(&r.UnifiedTrigramRows); err != nil {
		return r, err
	}
	if err := a.SearchStore.DB.QueryRowContext(ctx, `SELECT count(*) FROM document_embeddings`).Scan(&r.UnifiedEmbeddingRows); err != nil {
		return r, err
	}
	if r.UnifiedDocuments != r.UnifiedUnicodeRows || r.UnifiedDocuments != r.UnifiedTrigramRows || r.UnifiedDocuments != r.UnifiedEmbeddingRows {
		r.Errors = append(r.Errors, "unified search projection counts do not match documents")
	}
	var invalidEmbeddings int
	if err := a.SearchStore.DB.QueryRowContext(ctx, `SELECT count(*) FROM document_embeddings WHERE algorithm<>? OR dimensions<>? OR length(vector)<>?`, localembedding.Algorithm, localembedding.Dimensions, localembedding.Dimensions*4).Scan(&invalidEmbeddings); err != nil {
		return r, err
	}
	if invalidEmbeddings > 0 {
		r.Errors = append(r.Errors, fmt.Sprintf("%d unified embeddings have an invalid algorithm, dimension or payload", invalidEmbeddings))
	}
	var unscopedEvidence, invalidRecordProjects int
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT count(*) FROM evidence_receipts e WHERE NOT EXISTS(SELECT 1 FROM record_projects rp WHERE rp.record_type='evidence' AND rp.record_id=e.evidence_id)`).Scan(&unscopedEvidence); err != nil {
		return r, err
	}
	if unscopedEvidence > 0 {
		r.Errors = append(r.Errors, fmt.Sprintf("%d Evidence records have no project scope", unscopedEvidence))
	}
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT count(*) FROM record_projects rp LEFT JOIN projects p ON p.project_id=rp.project_id WHERE p.project_id IS NULL`).Scan(&invalidRecordProjects); err != nil {
		return r, err
	}
	if invalidRecordProjects > 0 {
		r.Errors = append(r.Errors, fmt.Sprintf("%d record-project links reference missing projects", invalidRecordProjects))
	}
	foreignRows, err := a.Control.DB.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return r, err
	}
	if foreignRows.Next() {
		r.Errors = append(r.Errors, "control sqlite foreign_key_check failed")
	}
	foreignRows.Close()
	for _, db := range []struct {
		name  string
		query string
		scan  func(*string) error
	}{
		{"control", `PRAGMA quick_check`, func(s *string) error { return a.Control.DB.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(s) }},
		{"search", `PRAGMA quick_check`, func(s *string) error { return a.SearchStore.DB.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(s) }},
	} {
		var status string
		if err := db.scan(&status); err != nil {
			return r, err
		}
		if status != "ok" {
			r.Errors = append(r.Errors, db.name+" sqlite quick_check: "+status)
		}
	}
	if len(r.Errors) > 0 {
		r.Status = "fail"
	}
	return r, nil
}
