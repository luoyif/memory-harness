package unifiedsearch

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/luoyif/memory-harness/internal/localembedding"
	"github.com/luoyif/memory-harness/internal/store"
)

type Query struct {
	Text           string   `json:"query"`
	ProjectID      string   `json:"project_id,omitempty"`
	AllProjects    bool     `json:"all_projects,omitempty"`
	Kinds          []string `json:"kinds,omitempty"`
	AsOf           string   `json:"as_of,omitempty"`
	IncludeHistory bool     `json:"include_history,omitempty"`
	Limit          int      `json:"limit,omitempty"`
}

type Hit struct {
	ResultID          string         `json:"result_id"`
	Kind              string         `json:"kind"`
	SourceID          string         `json:"source_id"`
	ProjectID         string         `json:"project_id"`
	Title             string         `json:"title"`
	Snippet           string         `json:"snippet"`
	Status            string         `json:"status"`
	ObservedAt        string         `json:"observed_at"`
	ValidFrom         string         `json:"valid_from,omitempty"`
	ValidUntil        string         `json:"valid_until,omitempty"`
	Score             float64        `json:"score"`
	LexicalRank       int            `json:"lexical_rank"`
	VectorRank        int            `json:"vector_rank"`
	VectorSimilarity  float64        `json:"vector_similarity"`
	RecencyRank       int            `json:"recency_rank"`
	TemporalRank      int            `json:"temporal_rank"`
	TemporalRelevance float64        `json:"temporal_relevance"`
	TemporalRelation  string         `json:"temporal_relation"`
	Metadata          map[string]any `json:"metadata"`
}

type Result struct {
	ContextID      string `json:"context_id"`
	Query          string `json:"query"`
	ProjectID      string `json:"project_id,omitempty"`
	AllProjects    bool   `json:"all_projects"`
	Backend        string `json:"backend"`
	Embedding      string `json:"embedding"`
	Dimensions     int    `json:"embedding_dimensions"`
	CandidateCount int    `json:"candidate_count"`
	TookMS         int64  `json:"took_ms"`
	Hits           []Hit  `json:"hits"`
}

type Engine struct{ store *store.SearchStore }

func New(search *store.SearchStore) *Engine { return &Engine{store: search} }

func containsHan(value string) bool {
	for _, r := range value {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func quote(value string) string { return `"` + strings.ReplaceAll(value, `"`, `""`) + `"` }

func unicodeQuery(value string) string {
	terms := []string{}
	for _, field := range strings.Fields(value) {
		field = strings.TrimFunc(field, func(r rune) bool { return unicode.IsPunct(r) || unicode.IsSymbol(r) })
		if field != "" {
			terms = append(terms, quote(field))
		}
	}
	if len(terms) == 0 {
		return quote(value)
	}
	return strings.Join(terms, " OR ")
}

func timeBound(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return time.Now().UTC().Format(time.RFC3339Nano), nil
	}
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("invalid as_of %q", value)
	}
	return parsed.UTC().Format(time.RFC3339Nano), nil
}

type candidate struct {
	id                int64
	docKey            string
	kind              string
	sourceID          string
	projectID         string
	title             string
	body              string
	status            string
	observedAt        string
	validFrom         sql.NullString
	validUntil        sql.NullString
	metadataJSON      string
	lexical           float64
	lexicalRank       int
	vectorRank        int
	vectorSimilarity  float64
	recencyRank       int
	temporalRank      int
	temporalRelevance float64
	temporalRelation  string
	temporalDistance  float64
	score             float64
}

func candidateTemporalPosition(item candidate, anchor time.Time) (distanceHours float64, relation string, relevance float64) {
	parse := func(value string) (time.Time, bool) {
		if strings.TrimSpace(value) == "" {
			return time.Time{}, false
		}
		value = strings.TrimSpace(value)
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, value)
		}
		return parsed, err == nil
	}
	if item.validFrom.Valid {
		start, ok := parse(item.validFrom.String)
		if ok {
			end, hasEnd := time.Time{}, false
			if item.validUntil.Valid {
				end, hasEnd = parse(item.validUntil.String)
			}
			if !start.After(anchor) && (!hasEnd || end.After(anchor)) {
				return 0, "active", 1
			}
			if start.After(anchor) {
				distanceHours = start.Sub(anchor).Hours()
				relation = "future"
			} else if hasEnd {
				distanceHours = anchor.Sub(end).Hours()
				relation = "past"
			}
		}
	}
	if relation == "" {
		observed, ok := parse(item.observedAt)
		if !ok {
			return math.MaxFloat64, "unknown", 0
		}
		if observed.After(anchor) {
			distanceHours = observed.Sub(anchor).Hours()
			relation = "future"
		} else {
			distanceHours = anchor.Sub(observed).Hours()
			relation = "past"
		}
	}
	if distanceHours < 0 {
		distanceHours = -distanceHours
	}
	relevance = math.Exp(-math.Ln2 * distanceHours / (24 * 45))
	return distanceHours, relation, math.Round(relevance*10000) / 10000
}

func (e *Engine) Search(ctx context.Context, query Query) (Result, error) {
	started := time.Now()
	query.Text = strings.TrimSpace(query.Text)
	if query.Text == "" {
		return Result{}, errors.New("query is required")
	}
	if !query.AllProjects && strings.TrimSpace(query.ProjectID) == "" {
		return Result{}, errors.New("project_id is required unless all_projects is true")
	}
	if query.Limit <= 0 {
		query.Limit = 10
	}
	if query.Limit > 100 {
		query.Limit = 100
	}
	asOf, err := timeBound(query.AsOf)
	if err != nil {
		return Result{}, err
	}
	table := "documents_fts"
	backend := "unicode61-bm25+local-feature-embedding+time-relevance-rrf"
	match := unicodeQuery(query.Text)
	runeCount := len([]rune(strings.ReplaceAll(query.Text, " ", "")))
	useLike := runeCount < 3
	if useLike {
		backend = "substring+local-feature-embedding+time-relevance-rrf"
	} else if containsHan(query.Text) {
		table = "documents_tri"
		backend = "trigram-bm25+local-feature-embedding+time-relevance-rrf"
		match = quote(query.Text)
	}
	filters := []string{"1=1"}
	filterArgs := []any{}
	if !query.AllProjects {
		filters = append(filters, "d.project_id=?")
		filterArgs = append(filterArgs, query.ProjectID)
	}
	if len(query.Kinds) > 0 {
		placeholders := make([]string, 0, len(query.Kinds))
		allowed := map[string]bool{"evidence": true, "episode": true, "memory": true, "asset": true, "object": true, "project": true, "fact": true, "goal": true, "decision": true, "risk": true, "finance": true, "experience": true}
		for _, kind := range query.Kinds {
			kind = strings.TrimSpace(kind)
			if !allowed[kind] {
				return Result{}, fmt.Errorf("unsupported kind %q", kind)
			}
			placeholders = append(placeholders, "?")
			filterArgs = append(filterArgs, kind)
		}
		filters = append(filters, "d.kind IN ("+strings.Join(placeholders, ",")+")")
	}
	if !query.IncludeHistory {
		filters = append(filters, "d.status NOT IN ('superseded','expired','deprecated','void','archived')")
	}
	if strings.TrimSpace(query.AsOf) != "" || !query.IncludeHistory {
		filters = append(filters, "(d.valid_from IS NULL OR d.valid_from<=?)", "(d.valid_until IS NULL OR d.valid_until>?)")
		filterArgs = append(filterArgs, asOf, asOf)
	}
	candidateLimit := min(max(query.Limit*15, 100), 600)
	var statement string
	args := []any{}
	if useLike {
		where := append(append([]string{}, filters...), "(d.title LIKE ? OR d.body LIKE ?)")
		args = append(args, filterArgs...)
		args = append(args, "%"+query.Text+"%", "%"+query.Text+"%", candidateLimit)
		statement = `SELECT d.id,d.doc_key,d.kind,d.source_id,d.project_id,d.title,d.body,d.status,d.observed_at,d.valid_from,d.valid_until,d.metadata_json,1.0 FROM documents d WHERE ` + strings.Join(where, " AND ") + ` ORDER BY d.observed_at DESC LIMIT ?`
	} else {
		args = append(args, match)
		args = append(args, filterArgs...)
		args = append(args, candidateLimit)
		statement = `SELECT d.id,d.doc_key,d.kind,d.source_id,d.project_id,d.title,d.body,d.status,d.observed_at,d.valid_from,d.valid_until,d.metadata_json,-bm25(` + table + `) FROM ` + table + ` JOIN documents d ON d.id=` + table + `.rowid WHERE ` + table + ` MATCH ? AND ` + strings.Join(filters, " AND ") + ` ORDER BY bm25(` + table + `),d.observed_at DESC LIMIT ?`
	}
	rows, err := e.store.DB.QueryContext(ctx, statement, args...)
	if err != nil {
		return Result{}, err
	}
	defer rows.Close()
	candidates := []candidate{}
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.docKey, &item.kind, &item.sourceID, &item.projectID, &item.title, &item.body, &item.status, &item.observedAt, &item.validFrom, &item.validUntil, &item.metadataJSON, &item.lexical); err != nil {
			return Result{}, err
		}
		item.lexicalRank = len(candidates) + 1
		candidates = append(candidates, item)
	}
	if err := rows.Err(); err != nil {
		return Result{}, err
	}
	byID := make(map[int64]int, len(candidates))
	for index := range candidates {
		byID[candidates[index].id] = index
	}
	queryVector := localembedding.Encode(query.Text)
	vectorRows, err := e.store.DB.QueryContext(ctx, `SELECT d.id,d.doc_key,d.kind,d.source_id,d.project_id,d.title,d.body,d.status,d.observed_at,d.valid_from,d.valid_until,d.metadata_json,e.vector,e.dimensions,e.algorithm FROM documents d JOIN document_embeddings e ON e.document_id=d.id WHERE `+strings.Join(filters, " AND "), filterArgs...)
	if err != nil {
		return Result{}, err
	}
	type vectorCandidate struct {
		item       candidate
		similarity float64
	}
	vectorCandidates := []vectorCandidate{}
	for vectorRows.Next() {
		var item candidate
		var raw []byte
		var dimensions int
		var algorithm string
		if err := vectorRows.Scan(&item.id, &item.docKey, &item.kind, &item.sourceID, &item.projectID, &item.title, &item.body, &item.status, &item.observedAt, &item.validFrom, &item.validUntil, &item.metadataJSON, &raw, &dimensions, &algorithm); err != nil {
			vectorRows.Close()
			return Result{}, err
		}
		if algorithm != localembedding.Algorithm || dimensions != localembedding.Dimensions {
			continue
		}
		vector, decodeErr := localembedding.Unmarshal(raw, dimensions)
		if decodeErr != nil {
			continue
		}
		similarity := localembedding.Similarity(queryVector, vector)
		if similarity > localembedding.MinimumSimilarity {
			vectorCandidates = append(vectorCandidates, vectorCandidate{item: item, similarity: similarity})
		}
	}
	if err := vectorRows.Err(); err != nil {
		vectorRows.Close()
		return Result{}, err
	}
	if err := vectorRows.Close(); err != nil {
		return Result{}, err
	}
	sort.SliceStable(vectorCandidates, func(i, j int) bool {
		if vectorCandidates[i].similarity == vectorCandidates[j].similarity {
			return vectorCandidates[i].item.observedAt > vectorCandidates[j].item.observedAt
		}
		return vectorCandidates[i].similarity > vectorCandidates[j].similarity
	})
	vectorLimit := candidateLimit
	if len(vectorCandidates) > vectorLimit {
		vectorCandidates = vectorCandidates[:vectorLimit]
	}
	for rank, ranked := range vectorCandidates {
		if index, exists := byID[ranked.item.id]; exists {
			candidates[index].vectorRank = rank + 1
			candidates[index].vectorSimilarity = ranked.similarity
			continue
		}
		if len(candidates) >= candidateLimit {
			continue
		}
		ranked.item.vectorRank = rank + 1
		ranked.item.vectorSimilarity = ranked.similarity
		byID[ranked.item.id] = len(candidates)
		candidates = append(candidates, ranked.item)
	}
	anchorTime, _ := time.Parse(time.RFC3339Nano, asOf)
	byRecency := make([]int, len(candidates))
	byTemporal := make([]int, len(candidates))
	for i := range candidates {
		byRecency[i], byTemporal[i] = i, i
		distance, relation, relevance := candidateTemporalPosition(candidates[i], anchorTime)
		candidates[i].temporalDistance, candidates[i].temporalRelation, candidates[i].temporalRelevance = distance, relation, relevance
	}
	sort.SliceStable(byRecency, func(i, j int) bool { return candidates[byRecency[i]].observedAt > candidates[byRecency[j]].observedAt })
	for rank, index := range byRecency {
		candidates[index].recencyRank = rank + 1
	}
	sort.SliceStable(byTemporal, func(i, j int) bool {
		left, right := candidates[byTemporal[i]], candidates[byTemporal[j]]
		if left.temporalDistance == right.temporalDistance {
			return left.observedAt > right.observedAt
		}
		return left.temporalDistance < right.temporalDistance
	})
	explicitAnchor := strings.TrimSpace(query.AsOf) != ""
	for rank, index := range byTemporal {
		candidates[index].temporalRank = rank + 1
		lexicalWeight, vectorWeight, recencyWeight, temporalWeight, relevanceWeight := 1.0, 0.85, 0.20, 0.55, 0.0
		if explicitAnchor {
			lexicalWeight, vectorWeight, recencyWeight, temporalWeight, relevanceWeight = 0.55, 0.45, 0.05, 1.80, 0.02
		}
		lexicalScore, vectorScore := 0.0, 0.0
		if candidates[index].lexicalRank > 0 {
			lexicalScore = lexicalWeight / (60 + float64(candidates[index].lexicalRank))
		}
		if candidates[index].vectorRank > 0 {
			vectorScore = vectorWeight/(60+float64(candidates[index].vectorRank)) + 0.01*candidates[index].vectorSimilarity
		}
		candidates[index].score = lexicalScore + vectorScore + recencyWeight/(60+float64(candidates[index].recencyRank)) + temporalWeight/(60+float64(rank+1)) + relevanceWeight*candidates[index].temporalRelevance
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			leftLexical, rightLexical := candidates[i].lexicalRank, candidates[j].lexicalRank
			if leftLexical == 0 {
				leftLexical = math.MaxInt
			}
			if rightLexical == 0 {
				rightLexical = math.MaxInt
			}
			if leftLexical != rightLexical {
				return leftLexical < rightLexical
			}
			leftVector, rightVector := candidates[i].vectorRank, candidates[j].vectorRank
			if leftVector == 0 {
				leftVector = math.MaxInt
			}
			if rightVector == 0 {
				rightVector = math.MaxInt
			}
			return leftVector < rightVector
		}
		return candidates[i].score > candidates[j].score
	})
	candidateCount := len(candidates)
	if len(candidates) > query.Limit {
		candidates = candidates[:query.Limit]
	}
	hits := make([]Hit, 0, len(candidates))
	for _, item := range candidates {
		meta := map[string]any{}
		_ = json.Unmarshal([]byte(item.metadataJSON), &meta)
		hits = append(hits, Hit{ResultID: item.docKey, Kind: item.kind, SourceID: item.sourceID, ProjectID: item.projectID, Title: item.title, Snippet: compact(item.body, 320), Status: item.status, ObservedAt: item.observedAt, ValidFrom: item.validFrom.String, ValidUntil: item.validUntil.String, Score: item.score, LexicalRank: item.lexicalRank, VectorRank: item.vectorRank, VectorSimilarity: math.Round(item.vectorSimilarity*10000) / 10000, RecencyRank: item.recencyRank, TemporalRank: item.temporalRank, TemporalRelevance: item.temporalRelevance, TemporalRelation: item.temporalRelation, Metadata: meta})
	}
	contextID := fmt.Sprintf("ctx-%x", time.Now().UTC().UnixNano())
	return Result{ContextID: contextID, Query: query.Text, ProjectID: query.ProjectID, AllProjects: query.AllProjects, Backend: backend, Embedding: localembedding.Algorithm, Dimensions: localembedding.Dimensions, CandidateCount: candidateCount, TookMS: time.Since(started).Milliseconds(), Hits: hits}, nil
}

func compact(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len([]rune(value)) <= limit {
		return value
	}
	return string([]rune(value)[:limit-1]) + "…"
}
