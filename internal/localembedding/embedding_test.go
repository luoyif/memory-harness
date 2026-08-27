package localembedding_test

import (
	"testing"

	"github.com/luoyif/memory-harness/internal/localembedding"
)

func TestEmbeddingRoundTripAndSimilarity(t *testing.T) {
	query := localembedding.Encode("automatic reconnect")
	related := localembedding.Encode("automatic reconnection retries")
	unrelated := localembedding.Encode("quarterly finance budget")
	decoded, err := localembedding.Unmarshal(localembedding.Marshal(related), localembedding.Dimensions)
	if err != nil {
		t.Fatal(err)
	}
	if got := localembedding.Similarity(query, decoded); got <= localembedding.Similarity(query, unrelated) {
		t.Fatalf("related similarity did not win: related=%f unrelated=%f", got, localembedding.Similarity(query, unrelated))
	}
	if got := localembedding.Similarity(localembedding.Encode("reconnect"), localembedding.Encode("socket recovery\nautomatic reconnection retries after a dropped socket")); got <= localembedding.MinimumSimilarity {
		t.Fatalf("word-form similarity fell below retrieval threshold: %f", got)
	}
	if got := localembedding.Similarity(localembedding.Encode("A-PRIVATE-MARKER"), localembedding.Encode("Approved team result\nAPPROVED-DURABLE-MARKER release is ready")); got >= localembedding.MinimumSimilarity {
		t.Fatalf("generic suffix collision crossed retrieval threshold: %f", got)
	}
}

func TestChineseFeaturesAreStable(t *testing.T) {
	left := localembedding.Encode("长期记忆检索")
	right := localembedding.Encode("长期记忆搜索与回看")
	if similarity := localembedding.Similarity(left, right); similarity <= 0.15 {
		t.Fatalf("expected useful CJK overlap, got %f", similarity)
	}
}
