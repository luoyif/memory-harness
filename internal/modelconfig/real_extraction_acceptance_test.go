package modelconfig_test

import (
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/luoyif/memory-harness/internal/app"
	"github.com/luoyif/memory-harness/internal/config"
	"github.com/luoyif/memory-harness/internal/memory"
)

// Opt-in diagnostic for the configured real provider. The prompt is synthetic
// and does not read or mutate Evidence or derived memory.
func TestRealProviderReturnsOneValidStructuredCandidate(t *testing.T) {
	home := os.Getenv("MEMORYOS_REAL_MODEL_HOME")
	if home == "" {
		t.Skip("set MEMORYOS_REAL_MODEL_HOME to test the configured provider")
	}
	cfg, err := config.Resolve(home, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	a, err := app.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	result, err := a.Models.Extract(t.Context(), memory.ExtractionRequest{SessionID: "synthetic-provider-contract", Participants: []memory.ParticipantIdentity{{ParticipantID: "participant:li-ming", DisplayName: "李明", Role: "first_person_speaker", Aliases: []string{"我"}}}, Turns: []memory.ExtractionTurn{{EvidenceID: "synthetic-evidence", SourceSystem: "acceptance-test", Role: "speaker", Text: "我决定下周在上海完成项目验收，并要求所有测试通过后才能发布。", ObservedAt: "2026-08-22T00:00:00Z"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) == 0 || result.Compiler == "" || result.Degraded {
		t.Fatalf("incomplete real provider result: %#v", result)
	}
}

func realWindows(text string, maxRunes int) []string {
	runes := []rune(text)
	windows := []string{}
	for start := 0; start < len(runes); {
		end := start + maxRunes
		if end >= len(runes) {
			end = len(runes)
		} else {
			floor := start + maxRunes/2
			for cursor := end; cursor > floor; cursor-- {
				switch runes[cursor-1] {
				case '。', '！', '？', '.', '!', '?', '\n':
					end = cursor
					cursor = floor
				}
			}
		}
		if part := strings.TrimSpace(string(runes[start:end])); part != "" {
			windows = append(windows, part)
		}
		start = end
	}
	return windows
}

// This opt-in diagnostic reads one real transcript but never logs its content
// and never writes memory. It isolates each provider window so failures report
// their bounded shape rather than collapsing into one partial-batch count.
func TestRealProviderCompletesEveryEvidenceWindow(t *testing.T) {
	home := os.Getenv("MEMORYOS_REAL_MODEL_HOME")
	if home == "" {
		t.Skip("set MEMORYOS_REAL_MODEL_HOME to test real evidence windows")
	}
	cfg, err := config.Resolve(home, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	a, err := app.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	var evidenceID, role, body, observedAt string
	if err := a.SearchStore.DB.QueryRowContext(t.Context(), `SELECT evidence_id,role,body,observed_at FROM turns ORDER BY id LIMIT 1`).Scan(&evidenceID, &role, &body, &observedAt); err != nil {
		t.Fatal(err)
	}
	windows := realWindows(body, 4000)
	type outcome struct {
		candidates int
		err        error
	}
	results := make([]outcome, len(windows))
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	for index, text := range windows {
		index, text := index, text
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			result, err := a.Models.Extract(t.Context(), memory.ExtractionRequest{SessionID: "real-window-diagnostic", Turns: []memory.ExtractionTurn{{EvidenceID: evidenceID, SourceSystem: "real-window-diagnostic", Role: role, Text: text, ObservedAt: observedAt}}})
			results[index] = outcome{candidates: len(result.Candidates), err: err}
		}()
	}
	wg.Wait()
	failed := 0
	for index, result := range results {
		if result.err != nil {
			failed++
			t.Logf("window %d/%d failed: %v", index+1, len(results), result.err)
		} else {
			t.Logf("window %d/%d passed: candidates=%d", index+1, len(results), result.candidates)
		}
	}
	if failed > 0 {
		t.Fatalf("%d of %d provider windows failed", failed, len(results))
	}
}
