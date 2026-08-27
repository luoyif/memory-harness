package builtin_test

import (
	"testing"

	"github.com/luoyif/memory-harness/internal/builtin"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestBootstrapIsIdempotentAcrossCanonicalFormatChanges(t *testing.T) {
	a, _ := testutil.Open(t)
	if _, err := a.Control.DB.ExecContext(t.Context(), `UPDATE harness_pipeline_versions SET content_hash='sha256:legacy-canonical' WHERE pipeline_id='builtin.opportunity-intelligence.qualify' AND version='1.0.0'`); err != nil {
		t.Fatal(err)
	}
	if err := builtin.Bootstrap(t.Context(), builtin.Services{Harness: a.Harness, Pipelines: a.Pipelines, Plugins: a.Plugins}); err != nil {
		t.Fatalf("existing built-in versions must not be republished: %v", err)
	}
	version, err := a.Pipelines.Version(t.Context(), "builtin.opportunity-intelligence.qualify", "1.0.0")
	if err != nil || version.ContentHash != "sha256:legacy-canonical" {
		t.Fatalf("bootstrap mutated an immutable existing version: %#v err=%v", version, err)
	}
}
