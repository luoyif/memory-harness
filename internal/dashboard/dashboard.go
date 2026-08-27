package dashboard

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/luoyif/memory-harness/internal/app"
)

const recentActivityLimit = 8

type Activity struct {
	EvidenceID   string `json:"evidence_id"`
	SourceSystem string `json:"source_system"`
	SessionID    string `json:"session_id"`
	Role         string `json:"role,omitempty"`
	ObservedAt   string `json:"observed_at"`
	CapturedAt   string `json:"captured_at"`
	Preview      string `json:"preview"`
}

type Today struct {
	Date           string     `json:"date"`
	Timezone       string     `json:"timezone"`
	UTCOffset      string     `json:"utc_offset"`
	Evidence       int        `json:"evidence"`
	Sessions       int        `json:"sessions"`
	Sources        int        `json:"sources"`
	LastCapturedAt string     `json:"last_captured_at,omitempty"`
	Recent         []Activity `json:"recent"`
}

type Totals struct {
	Evidence int `json:"evidence"`
	Sessions int `json:"sessions"`
	Sources  int `json:"sources"`
}

type Search struct {
	Status         string `json:"status"`
	IndexedTurns   int    `json:"indexed_turns"`
	UnicodeFTSRows int    `json:"unicode_fts_rows"`
	TrigramFTSRows int    `json:"trigram_fts_rows"`
	Consistent     bool   `json:"consistent"`
}

type Jobs struct {
	Total          int            `json:"total"`
	Pending        int            `json:"pending"`
	Running        int            `json:"running"`
	Failed         int            `json:"failed"`
	Completed      int            `json:"completed"`
	NeedsAttention int            `json:"needs_attention"`
	ByStatus       map[string]int `json:"by_status"`
}

type Source struct {
	Name           string `json:"name"`
	Evidence       int    `json:"evidence"`
	Sessions       int    `json:"sessions"`
	LastCapturedAt string `json:"last_captured_at,omitempty"`
}

type Layer struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Enabled bool   `json:"enabled"`
}

type Memory struct {
	Stage   string  `json:"stage"`
	Enabled bool    `json:"enabled"`
	Message string  `json:"message"`
	Layers  []Layer `json:"layers"`
}

type Assets struct {
	Stage   string  `json:"stage"`
	Enabled bool    `json:"enabled"`
	Message string  `json:"message"`
	Types   []Layer `json:"types"`
}

type Snapshot struct {
	GeneratedAt string   `json:"generated_at"`
	Today       Today    `json:"today"`
	Totals      Totals   `json:"totals"`
	Search      Search   `json:"search"`
	Jobs        Jobs     `json:"jobs"`
	Memory      Memory   `json:"memory"`
	Assets      Assets   `json:"assets"`
	Sources     []Source `json:"sources"`
}

func Build(ctx context.Context, a *app.App, now time.Time) (Snapshot, error) {
	today, err := ReadToday(ctx, a, now)
	if err != nil {
		return Snapshot{}, err
	}
	totals, err := ReadTotals(ctx, a)
	if err != nil {
		return Snapshot{}, err
	}
	search, err := ReadSearch(ctx, a, totals.Evidence)
	if err != nil {
		return Snapshot{}, err
	}
	jobs, err := ReadJobs(ctx, a)
	if err != nil {
		return Snapshot{}, err
	}
	sources, err := ReadSources(ctx, a)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		GeneratedAt: now.UTC().Format(time.RFC3339Nano),
		Today:       today,
		Totals:      totals,
		Search:      search,
		Jobs:        jobs,
		Memory:      MemoryStatus(),
		Assets:      AssetStatus(),
		Sources:     sources,
	}, nil
}

func ReadToday(ctx context.Context, a *app.App, now time.Time) (Today, error) {
	localNow := now.In(time.Local)
	startLocal := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, localNow.Location())
	start := startLocal.UTC().Format(time.RFC3339Nano)
	end := startLocal.AddDate(0, 0, 1).UTC().Format(time.RFC3339Nano)
	zone, offset := localNow.Zone()
	if strings.TrimSpace(zone) == "" {
		zone = "Local"
	}
	t := Today{Date: localNow.Format("2006-01-02"), Timezone: zone, UTCOffset: formatOffset(offset), Recent: []Activity{}}
	err := a.Control.DB.QueryRowContext(ctx, `SELECT count(*), count(DISTINCT session_id), count(DISTINCT source_system), coalesce(max(captured_at), '') FROM evidence_receipts WHERE captured_at >= ? AND captured_at < ?`, start, end).
		Scan(&t.Evidence, &t.Sessions, &t.Sources, &t.LastCapturedAt)
	if err != nil {
		return Today{}, err
	}
	rows, err := a.SearchStore.DB.QueryContext(ctx, `SELECT evidence_id, source_system, session_id, coalesce(role, ''), observed_at, captured_at, body FROM turns WHERE captured_at >= ? AND captured_at < ? ORDER BY captured_at DESC, id DESC LIMIT ?`, start, end, recentActivityLimit)
	if err != nil {
		return Today{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item Activity
		var body string
		if err := rows.Scan(&item.EvidenceID, &item.SourceSystem, &item.SessionID, &item.Role, &item.ObservedAt, &item.CapturedAt, &body); err != nil {
			return Today{}, err
		}
		item.Preview = preview(body, 160)
		t.Recent = append(t.Recent, item)
	}
	return t, rows.Err()
}

func ReadTotals(ctx context.Context, a *app.App) (Totals, error) {
	var out Totals
	err := a.Control.DB.QueryRowContext(ctx, `SELECT count(*), count(DISTINCT session_id), count(DISTINCT source_system) FROM evidence_receipts`).
		Scan(&out.Evidence, &out.Sessions, &out.Sources)
	return out, err
}

func ReadSearch(ctx context.Context, a *app.App, evidenceTotal int) (Search, error) {
	var out Search
	if err := a.SearchStore.DB.QueryRowContext(ctx, `SELECT count(*) FROM turns`).Scan(&out.IndexedTurns); err != nil {
		return Search{}, err
	}
	if err := a.SearchStore.DB.QueryRowContext(ctx, `SELECT count(*) FROM turns_fts`).Scan(&out.UnicodeFTSRows); err != nil {
		return Search{}, err
	}
	if err := a.SearchStore.DB.QueryRowContext(ctx, `SELECT count(*) FROM turns_tri`).Scan(&out.TrigramFTSRows); err != nil {
		return Search{}, err
	}
	out.Consistent = evidenceTotal == out.IndexedTurns && out.IndexedTurns == out.UnicodeFTSRows && out.IndexedTurns == out.TrigramFTSRows
	out.Status = "healthy"
	if !out.Consistent {
		out.Status = "needs_rebuild"
	}
	return out, nil
}

func ReadJobs(ctx context.Context, a *app.App) (Jobs, error) {
	out := Jobs{ByStatus: map[string]int{}}
	rows, err := a.Control.DB.QueryContext(ctx, `SELECT lower(status), count(*) FROM jobs GROUP BY lower(status)`)
	if err != nil {
		return Jobs{}, err
	}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			rows.Close()
			return Jobs{}, err
		}
		out.ByStatus[status] = count
		out.Total += count
		switch status {
		case "queued", "pending", "retry", "retrying":
			out.Pending += count
		case "running", "processing":
			out.Running += count
		case "failed", "error":
			out.Failed += count
		case "complete", "completed", "done", "succeeded", "success":
			out.Completed += count
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Jobs{}, err
	}
	rows.Close()
	if err := a.Control.DB.QueryRowContext(ctx, `SELECT
  (SELECT count(*) FROM operation_receipts WHERE lower(status) IN ('pending', 'review_required', 'awaiting_review', 'blocked')) +
  (SELECT count(*) FROM memory_operations WHERE lower(status) IN ('proposed', 'review_required'))`).Scan(&out.NeedsAttention); err != nil {
		return Jobs{}, err
	}
	out.NeedsAttention += out.Failed
	return out, nil
}

func ReadSources(ctx context.Context, a *app.App) ([]Source, error) {
	rows, err := a.Control.DB.QueryContext(ctx, `SELECT source_system, count(*), count(DISTINCT session_id), coalesce(max(captured_at), '') FROM evidence_receipts GROUP BY source_system ORDER BY count(*) DESC, source_system ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Source{}
	for rows.Next() {
		var source Source
		if err := rows.Scan(&source.Name, &source.Evidence, &source.Sessions, &source.LastCapturedAt); err != nil {
			return nil, err
		}
		out = append(out, source)
	}
	return out, rows.Err()
}

func MemoryStatus() Memory {
	names := []string{"Identity Core", "Episodic", "Semantic", "Procedural", "Working Context"}
	layers := make([]Layer, 0, len(names))
	for _, name := range names {
		layers = append(layers, Layer{Name: name, Status: "governed", Enabled: true})
	}
	return Memory{Stage: "memory_growth_active", Enabled: true, Message: "Evidence is compiled into traceable Knowledge Units, Episodes and governed Memory Records.", Layers: layers}
}

func AssetStatus() Assets {
	names := []string{"Prompt", "Skill", "Rule", "Constraint", "Procedure", "Workflow", "Tool Recipe", "MCP Profile", "Template", "Eval"}
	types := make([]Layer, 0, len(names))
	for _, name := range names {
		types = append(types, Layer{Name: name, Status: "protected", Enabled: true})
	}
	return Assets{Stage: "protected_registry_active", Enabled: true, Message: "Procedural memories may propose versioned AgentAsset candidates; activation remains manual.", Types: types}
}

func formatOffset(seconds int) string {
	sign := "+"
	if seconds < 0 {
		sign = "-"
		seconds = -seconds
	}
	return fmt.Sprintf("UTC%s%02d:%02d", sign, seconds/3600, seconds%3600/60)
}

func preview(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return string(runes[:limit-1]) + "…"
}
