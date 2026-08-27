package search

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/luoyif/memory-harness/internal/store"
)

type Query struct {
	Text              string   `json:"query"`
	Limit             int      `json:"limit,omitempty"`
	SourceSystem      string   `json:"source_system,omitempty"`
	SessionID         string   `json:"session_id,omitempty"`
	Scope             string   `json:"scope,omitempty"`
	DateFrom          string   `json:"date_from,omitempty"`
	DateTo            string   `json:"date_to,omitempty"`
	NeighborTurns     int      `json:"neighbor_turns,omitempty"`
	SessionFusion     bool     `json:"session_fusion,omitempty"`
	ExcludeSessionIDs []string `json:"exclude_session_ids,omitempty"`
}

type Turn struct {
	EvidenceID   string   `json:"evidence_id"`
	SessionID    string   `json:"session_id"`
	SourceSystem string   `json:"source_system"`
	ObservedAt   string   `json:"observed_at"`
	Role         string   `json:"role,omitempty"`
	ScopeHints   []string `json:"scope_hints,omitempty"`
	Ordinal      int      `json:"ordinal"`
	Body         string   `json:"body"`
	LedgerPath   string   `json:"ledger_path"`
}

type Hit struct {
	Turn         Turn    `json:"turn"`
	LexicalScore float64 `json:"lexical_score"`
	TurnRank     int     `json:"turn_rank"`
	SessionRank  int     `json:"session_rank"`
	Score        float64 `json:"score"`
	Context      []Turn  `json:"context,omitempty"`
}

type Result struct {
	Query          string `json:"query"`
	Backend        string `json:"backend"`
	Hits           []Hit  `json:"hits"`
	CandidateCount int    `json:"candidate_count"`
	TookMS         int64  `json:"took_ms"`
}

type Engine struct{ store *store.SearchStore }

func New(s *store.SearchStore) *Engine { return &Engine{store: s} }

type candidate struct {
	id int64
	Turn
	lexical     float64
	turnRank    int
	sessionRank int
	score       float64
}

func containsHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func runeCountNonSpace(s string) int {
	n := 0
	for _, r := range s {
		if !unicode.IsSpace(r) {
			n++
		}
	}
	return n
}

func quoteFTS(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

func unicodeQuery(text string) string {
	fields := strings.Fields(text)
	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimFunc(f, func(r rune) bool { return unicode.IsPunct(r) || unicode.IsSymbol(r) })
		if f != "" {
			terms = append(terms, quoteFTS(f))
		}
	}
	if len(terms) == 0 {
		return quoteFTS(strings.TrimSpace(text))
	}
	return strings.Join(terms, " OR ")
}

func normalizeBound(v string, end bool) (string, error) {
	if strings.TrimSpace(v) == "" {
		return "", nil
	}
	v = strings.TrimSpace(v)
	if t, err := time.Parse("2006-01-02", v); err == nil {
		if end {
			t = t.Add(24*time.Hour - time.Nanosecond)
		}
		return t.UTC().Format(time.RFC3339Nano), nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return "", fmt.Errorf("invalid time bound %q", v)
	}
	return t.UTC().Format(time.RFC3339Nano), nil
}

func (e *Engine) Search(ctx context.Context, q Query) (Result, error) {
	started := time.Now()
	q.Text = strings.TrimSpace(q.Text)
	if q.Text == "" {
		return Result{}, errors.New("query is required")
	}
	if q.Limit <= 0 {
		q.Limit = 5
	}
	if q.Limit > 50 {
		q.Limit = 50
	}
	if q.NeighborTurns < 0 {
		q.NeighborTurns = 0
	}
	if q.NeighborTurns > 10 {
		q.NeighborTurns = 10
	}
	from, err := normalizeBound(q.DateFrom, false)
	if err != nil {
		return Result{}, err
	}
	to, err := normalizeBound(q.DateTo, true)
	if err != nil {
		return Result{}, err
	}
	if from != "" && to != "" && from > to {
		return Result{}, errors.New("date_from is after date_to")
	}

	backend := "unicode61-bm25"
	useLike := runeCountNonSpace(q.Text) < 3
	table := "turns_fts"
	match := unicodeQuery(q.Text)
	if containsHan(q.Text) && !useLike {
		backend = "trigram-bm25"
		table = "turns_tri"
		match = quoteFTS(q.Text)
	}
	if useLike {
		backend = "substring"
	}

	where := []string{"1=1"}
	args := []any{}
	if q.SourceSystem != "" {
		where = append(where, "t.source_system=?")
		args = append(args, q.SourceSystem)
	}
	if q.SessionID != "" {
		where = append(where, "t.session_id=?")
		args = append(args, q.SessionID)
	}
	if q.Scope != "" {
		where = append(where, "EXISTS (SELECT 1 FROM json_each(t.scope_json) WHERE value=?)")
		args = append(args, q.Scope)
	}
	if from != "" {
		where = append(where, "t.observed_at>=?")
		args = append(args, from)
	}
	if to != "" {
		where = append(where, "t.observed_at<=?")
		args = append(args, to)
	}
	for _, sid := range q.ExcludeSessionIDs {
		if sid != "" {
			where = append(where, "t.session_id<>?")
			args = append(args, sid)
		}
	}
	capN := q.Limit * 10
	if capN < 50 {
		capN = 50
	}
	if capN > 500 {
		capN = 500
	}

	var rows *sql.Rows
	if useLike {
		sqlText := `SELECT t.id,t.evidence_id,t.session_id,t.source_system,t.observed_at,coalesce(t.role,''),t.scope_json,t.ordinal,t.body,t.ledger_rel_path,1.0 FROM turns t WHERE ` + strings.Join(where, " AND ") + ` AND t.body LIKE ? ORDER BY t.observed_at DESC LIMIT ?`
		args = append(args, "%"+q.Text+"%", capN)
		rows, err = e.store.DB.QueryContext(ctx, sqlText, args...)
	} else {
		sqlText := `SELECT t.id,t.evidence_id,t.session_id,t.source_system,t.observed_at,coalesce(t.role,''),t.scope_json,t.ordinal,t.body,t.ledger_rel_path,-bm25(` + table + `) FROM ` + table + ` JOIN turns t ON t.id=` + table + `.rowid WHERE ` + table + ` MATCH ? AND ` + strings.Join(where, " AND ") + ` ORDER BY bm25(` + table + `) LIMIT ?`
		all := []any{match}
		all = append(all, args...)
		all = append(all, capN)
		rows, err = e.store.DB.QueryContext(ctx, sqlText, all...)
	}
	if err != nil {
		return Result{}, err
	}
	defer rows.Close()
	cs := []candidate{}
	rank := 0
	for rows.Next() {
		var c candidate
		var scopeJSON string
		if err := rows.Scan(&c.id, &c.EvidenceID, &c.SessionID, &c.SourceSystem, &c.ObservedAt, &c.Role, &scopeJSON, &c.Ordinal, &c.Body, &c.LedgerPath, &c.lexical); err != nil {
			return Result{}, err
		}
		_ = json.Unmarshal([]byte(scopeJSON), &c.ScopeHints)
		rank++
		c.turnRank = rank
		cs = append(cs, c)
	}
	if err := rows.Err(); err != nil {
		return Result{}, err
	}

	sessionScore := map[string]float64{}
	for _, c := range cs {
		sessionScore[c.SessionID] += c.lexical
	}
	type pair struct {
		sid   string
		score float64
	}
	ss := make([]pair, 0, len(sessionScore))
	for sid, s := range sessionScore {
		ss = append(ss, pair{sid, s})
	}
	sort.SliceStable(ss, func(i, j int) bool {
		if ss[i].score == ss[j].score {
			return ss[i].sid < ss[j].sid
		}
		return ss[i].score > ss[j].score
	})
	sr := map[string]int{}
	for i, p := range ss {
		sr[p.sid] = i + 1
	}
	fusion := q.SessionFusion
	for i := range cs {
		cs[i].sessionRank = sr[cs[i].SessionID]
		if fusion {
			cs[i].score = 1.0/(60+float64(cs[i].turnRank)) + 1.0/(60+float64(cs[i].sessionRank))
		} else {
			cs[i].score = cs[i].lexical
		}
	}
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].score == cs[j].score {
			return cs[i].turnRank < cs[j].turnRank
		}
		return cs[i].score > cs[j].score
	})
	if len(cs) > q.Limit {
		cs = cs[:q.Limit]
	}

	hits := make([]Hit, 0, len(cs))
	for _, c := range cs {
		h := Hit{Turn: c.Turn, LexicalScore: c.lexical, TurnRank: c.turnRank, SessionRank: c.sessionRank, Score: c.score}
		if q.NeighborTurns > 0 {
			contextTurns, err := e.neighbors(ctx, c.SessionID, c.Ordinal, q.NeighborTurns)
			if err != nil {
				return Result{}, err
			}
			h.Context = contextTurns
		}
		hits = append(hits, h)
	}
	return Result{Query: q.Text, Backend: backend, Hits: hits, CandidateCount: rank, TookMS: time.Since(started).Milliseconds()}, nil
}

func (e *Engine) neighbors(ctx context.Context, sessionID string, ordinal, n int) ([]Turn, error) {
	rows, err := e.store.DB.QueryContext(ctx, `SELECT evidence_id,session_id,source_system,observed_at,coalesce(role,''),scope_json,ordinal,body,ledger_rel_path FROM turns WHERE session_id=? AND ordinal BETWEEN ? AND ? ORDER BY ordinal`, sessionID, ordinal-n, ordinal+n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Turn{}
	for rows.Next() {
		var t Turn
		var scopeJSON string
		if err := rows.Scan(&t.EvidenceID, &t.SessionID, &t.SourceSystem, &t.ObservedAt, &t.Role, &scopeJSON, &t.Ordinal, &t.Body, &t.LedgerPath); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(scopeJSON), &t.ScopeHints)
		out = append(out, t)
	}
	return out, rows.Err()
}
