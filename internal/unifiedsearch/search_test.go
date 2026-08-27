package unifiedsearch_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/luoyif/memory-harness/internal/localembedding"
	"github.com/luoyif/memory-harness/internal/store"
	"github.com/luoyif/memory-harness/internal/unifiedsearch"
)

func TestSearchRanksByDistanceToHistoricalAnchor(t *testing.T) {
	searchStore, err := store.OpenSearch(filepath.Join(t.TempDir(), "search.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer searchStore.Close()
	ctx := t.Context()
	for _, doc := range []store.IndexedDocument{
		{DocKey: "old", Kind: "memory", SourceID: "old", ProjectID: "project-personal", Title: "deployment plan", Body: "deployment plan", Status: "active", ObservedAt: "2026-01-01T00:00:00Z", MetadataJSON: "{}"},
		{DocKey: "new", Kind: "memory", SourceID: "new", ProjectID: "project-personal", Title: "deployment plan", Body: "deployment plan", Status: "active", ObservedAt: "2026-08-20T00:00:00Z", MetadataJSON: "{}"},
	} {
		if err := searchStore.UpsertDocument(ctx, doc); err != nil {
			t.Fatal(err)
		}
	}
	engine := unifiedsearch.New(searchStore)
	result, err := engine.Search(ctx, unifiedsearch.Query{Text: "deployment", ProjectID: "project-personal", AsOf: "2026-01-02T00:00:00Z", IncludeHistory: true, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 2 {
		t.Fatalf("hits=%#v", result.Hits)
	}
	if result.Hits[0].ResultID != "old" {
		t.Fatalf("historical anchor ignored: %#v", result.Hits)
	}
	if result.Hits[0].TemporalRank != 1 || result.Hits[0].TemporalRelevance <= result.Hits[1].TemporalRelevance {
		t.Fatalf("bad temporal metadata %#v", result.Hits)
	}
}

func TestSearchUsesLocalEmbeddingForFuzzyRetrieval(t *testing.T) {
	searchStore, err := store.OpenSearch(filepath.Join(t.TempDir(), "search.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer searchStore.Close()
	ctx := t.Context()
	for _, doc := range []store.IndexedDocument{
		{DocKey: "related", Kind: "memory", SourceID: "related", ProjectID: "project-personal", Title: "socket recovery", Body: "automatic reconnection retries after a dropped socket", Status: "active", ObservedAt: "2026-08-20T00:00:00Z", MetadataJSON: "{}"},
		{DocKey: "unrelated", Kind: "memory", SourceID: "unrelated", ProjectID: "project-personal", Title: "finance", Body: "quarterly budget and expense review", Status: "active", ObservedAt: "2026-08-20T00:00:00Z", MetadataJSON: "{}"},
	} {
		if err := searchStore.UpsertDocument(ctx, doc); err != nil {
			t.Fatal(err)
		}
	}
	result, err := unifiedsearch.New(searchStore).Search(ctx, unifiedsearch.Query{Text: "reconnect", ProjectID: "project-personal", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) == 0 || result.Hits[0].ResultID != "related" {
		t.Fatalf("fuzzy embedding result missing: %#v", result.Hits)
	}
	if result.Hits[0].LexicalRank != 0 || result.Hits[0].VectorRank != 1 || result.Hits[0].VectorSimilarity <= localembedding.MinimumSimilarity {
		t.Fatalf("expected vector-only hit metadata: %#v", result.Hits[0])
	}
	if result.Embedding == "" || result.Dimensions == 0 || !strings.Contains(result.Backend, "local-feature-embedding") {
		t.Fatalf("embedding provenance missing: %#v", result)
	}
}

func TestIncludeHistoryWithoutAsOfIncludesExpiredIntervals(t *testing.T) {
	searchStore, err := store.OpenSearch(filepath.Join(t.TempDir(), "search.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer searchStore.Close()
	ctx := t.Context()
	if err := searchStore.UpsertDocument(ctx, store.IndexedDocument{DocKey: "expired", Kind: "fact", SourceID: "expired", ProjectID: "project-personal", Title: "legacy policy", Body: "legacy policy", Status: "superseded", ObservedAt: "2025-01-01T00:00:00Z", ValidFrom: "2025-01-01T00:00:00Z", ValidUntil: "2025-02-01T00:00:00Z", MetadataJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	engine := unifiedsearch.New(searchStore)
	result, err := engine.Search(ctx, unifiedsearch.Query{Text: "legacy", ProjectID: "project-personal", IncludeHistory: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 1 || result.Hits[0].ResultID != "expired" {
		t.Fatalf("history missing %#v", result.Hits)
	}
}
