package testutil

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/luoyif/memory-harness/internal/app"
	"github.com/luoyif/memory-harness/internal/config"
)

func Open(t *testing.T) (*app.App, config.Config) {
	t.Helper()
	cfg, err := config.Resolve(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	a, err := app.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	return a, cfg
}

func Evidence(t *testing.T, id, source, session, role, when, text string) []byte {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, when)
	if err != nil {
		t.Fatal(err)
	}
	v := map[string]any{"schema_version": "0.1", "evidence_id": id, "source_system": source, "external_conversation_id": session, "role": role, "observed_at": ts.Format(time.RFC3339), "captured_at": ts.Add(time.Minute).Format(time.RFC3339), "content": []map[string]any{{"type": "text", "text": text}}, "provenance": map[string]any{"capture_method": "test"}, "scope_hints": []string{"test"}}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
