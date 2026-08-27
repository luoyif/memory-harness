package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/luoyif/memory-harness/internal/store"
)

const (
	CompilerVersion = "rules-v2"
	PolicyVersion   = "growth-policy-v1"
)

type Engine struct {
	control   *store.ControlStore
	search    *store.SearchStore
	memoryDir string
	mu        sync.Mutex
	extractor CandidateExtractor
}

type turn struct {
	EvidenceID   string
	SessionID    string
	SourceSystem string
	ObservedAt   string
	CapturedAt   string
	Role         string
	ScopeJSON    string
	Body         string
}

type candidate struct {
	EvidenceID string
	Statement  string
	Key        string
	UnitType   string
	TierHint   string
	RiskTier   string
	Confidence float64
	Scopes     []string
	ObservedAt string
	Structure  KnowledgeStructure
}

type viewSpec struct {
	id      string
	kind    string
	title   string
	summary string
	path    string
	ids     []string
}

func New(control *store.ControlStore, search *store.SearchStore, memoryDir string) *Engine {
	return &Engine{control: control, search: search, memoryDir: memoryDir}
}

func (e *Engine) SetCandidateExtractor(extractor CandidateExtractor) { e.extractor = extractor }

func stableID(prefix string, parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return prefix + hex.EncodeToString(h.Sum(nil)[:12])
}

func nowString() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func jsonStrings(values []string) string {
	values = unique(values)
	b, _ := json.Marshal(values)
	return string(b)
}

func decodeStrings(raw string) []string {
	var values []string
	_ = json.Unmarshal([]byte(raw), &values)
	if values == nil {
		values = []string{}
	}
	return values
}

func unique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func appendUnique(values []string, additions ...string) []string {
	return unique(append(values, additions...))
}

func without(values []string, removals ...string) []string {
	blocked := map[string]bool{}
	for _, value := range removals {
		blocked[value] = true
	}
	out := []string{}
	for _, value := range values {
		if !blocked[value] {
			out = append(out, value)
		}
	}
	return unique(out)
}

func normalizeStatement(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func compact(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit-1]) + "…"
}

func splitStatements(value string) []string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	var out []string
	var b strings.Builder
	flush := func() {
		statement := strings.TrimSpace(b.String())
		b.Reset()
		if statement != "" {
			out = append(out, compact(statement, 360))
		}
	}
	for _, r := range value {
		b.WriteRune(r)
		switch r {
		case '。', '！', '？', '.', '!', '?', '\n':
			flush()
		}
	}
	flush()
	return out
}

func lowInformationStatement(statement string) bool {
	trimmed := strings.Trim(strings.TrimSpace(statement), "。.!！?？,，;；:： ")
	if utf8.RuneCountInString(trimmed) < 8 {
		return true
	}
	lower := strings.ToLower(trimmed)
	fillers := []string{"嗯", "呃", "啊", "对对", "就是", "然后", "这个", "那个", "东西", "我觉得", "其实", "反正", "可能", "大概", "okay", "you know"}
	meaningful := lower
	for _, filler := range fillers {
		meaningful = strings.ReplaceAll(meaningful, filler, "")
	}
	meaningful = strings.TrimSpace(meaningful)
	if utf8.RuneCountInString(meaningful) < 7 {
		return true
	}
	vague := []string{"我懂你意思", "至少我自己", "就这样", "就是这样", "大概是这样", "有这个东西", "有这些东西", "对明白"}
	for _, value := range vague {
		if strings.Trim(lower, "。.!！?？,， ") == value {
			return true
		}
	}
	return false
}

func modelMetaStatement(statement string) bool {
	lower := strings.ToLower(statement)
	markers := []string{"this transcript chunk", "to the distiller", "downstream notes must", "the extraction prompt", "本段转录", "提炼器", "抽取提示词"}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func hanRunes(value string) int {
	count := 0
	for _, r := range value {
		if unicode.Is(unicode.Han, r) {
			count++
		}
	}
	return count
}

func primarilyChineseEvidence(value string) bool {
	han, latin := 0, 0
	for _, r := range value {
		switch {
		case unicode.Is(unicode.Han, r):
			han++
		case unicode.In(r, unicode.Latin):
			latin++
		}
	}
	return han >= 80 && han*2 >= latin
}

func durableMarker(statement string) bool {
	lower := strings.ToLower(statement)
	markers := []string{
		"我决定", "我们决定", "决定采用", "决定使用", "最终选择", "确定使用", "确定采用",
		"项目目标是", "我的目标是", "我们的目标是", "目标为", "计划在", "下一步计划", "必须先", "要求必须", "系统必须", "不允许", "不得",
		"步骤是", "流程是", "操作时必须", "每次都", "长期偏好", "我偏好", "我喜欢", "我的身份", "我的职业", "我是一名", "我是一个",
		"风险是", "风险在于", "主要风险", "当前风险", "担心的是", "隐患是", "难点是",
		"验证通过", "测试通过", "已经完成", "已经上线", "失败原因是", "当前项目", "目前项目", "当前状态", "正在开发", "正在完成",
		"we decided", "i decided", "decision is", "project goal", "my goal is", "plan to", "must first", "must not", "i prefer", "i am a", "workflow is", "procedure is", "risk is", "verified", "tests passed", "completed", "current project", "currently building",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func keepStatement(statement, role string) bool {
	trimmed := strings.TrimSpace(statement)
	if lowInformationStatement(trimmed) {
		return false
	}
	lower := strings.ToLower(trimmed)
	if strings.ContainsAny(trimmed, "🎼🎵♫") || strings.Count(lower, "就是") >= 3 || strings.Count(lower, "对") >= 5 || strings.Count(lower, "嗯") >= 3 || strings.Count(lower, "呃") >= 2 {
		return false
	}
	ack := []string{"好的", "谢谢", "明白了", "知道了", "ok", "okay", "thanks", "thank you"}
	for _, value := range ack {
		if strings.Trim(lower, "。.!！ ") == value {
			return false
		}
	}
	if strings.HasSuffix(trimmed, "?") || strings.HasSuffix(trimmed, "？") || strings.HasSuffix(trimmed, "吗") || strings.HasSuffix(trimmed, "吗？") {
		return false
	}
	// rules-v2 is deliberately conservative. It is a safe outage fallback, not
	// a replacement for semantic distillation. A sentence without an explicit
	// durable signal stays only in canonical Evidence.
	return durableMarker(lower)
}

func classify(statement string) (unitType, tier, risk string, confidence float64) {
	lower := strings.ToLower(statement)
	has := func(values ...string) bool {
		for _, value := range values {
			if strings.Contains(lower, value) {
				return true
			}
		}
		return false
	}
	switch {
	case has("更正", "纠正", "改为", "并不是", "correction", "correct that"):
		return "correction", "semantic", "C", 0.78
	case has("我的职业", "我的身份", "我叫", "我是一个", "我是一名", "长期偏好", "我偏好", "我喜欢", "i am a", "i prefer"):
		return "identity", "identity_core", "D", 0.82
	case has("步骤", "流程", "每次都", "操作时", "必须先", "procedure", "workflow", "runbook"):
		return "procedure", "procedural", "C", 0.80
	case has("决定", "采用", "选择了", "确定使用", "decision", "decided"):
		return "decision", "semantic", "B", 0.82
	case has("目标", "计划", "我希望", "要完成", "goal", "plan to"):
		return "goal", "semantic", "B", 0.76
	case has("风险", "担心", "隐患", "问题", "难点", "risk", "concern", "problem", "blocker"):
		return "risk", "semantic", "B", 0.74
	case has("验证通过", "测试通过", "已经完成", "失败", "成功", "verified", "passed", "failed"):
		return "outcome", "episodic", "A", 0.86
	case has("当前", "正在", "目前", "状态", "currently", "status"):
		return "state", "episodic", "A", 0.72
	default:
		return "fact", "semantic", "A", 0.68
	}
}

func (e *Engine) readTurns(ctx context.Context, sessionID string) ([]turn, error) {
	rows, err := e.search.DB.QueryContext(ctx, `SELECT evidence_id,session_id,source_system,observed_at,captured_at,coalesce(role,''),scope_json,body FROM turns WHERE session_id=? ORDER BY observed_at,id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []turn
	for rows.Next() {
		var item turn
		if err := rows.Scan(&item.EvidenceID, &item.SessionID, &item.SourceSystem, &item.ObservedAt, &item.CapturedAt, &item.Role, &item.ScopeJSON, &item.Body); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func extractCandidates(turns []turn) []candidate {
	var out []candidate
	for _, item := range turns {
		for _, statement := range splitStatements(item.Body) {
			if !keepStatement(statement, strings.ToLower(item.Role)) {
				continue
			}
			key := normalizeStatement(statement)
			if key == "" {
				continue
			}
			unitType, tier, risk, confidence := classify(statement)
			structure := normalizeKnowledgeStructure(KnowledgeStructure{}, item, statement, unitType, CompilerVersion, confidence, map[string]bool{})
			out = append(out, candidate{
				EvidenceID: item.EvidenceID,
				Statement:  statement,
				Key:        key,
				UnitType:   unitType,
				TierHint:   tier,
				RiskTier:   risk,
				Confidence: confidence,
				Scopes:     decodeStrings(item.ScopeJSON),
				ObservedAt: item.ObservedAt,
				Structure:  structure,
			})
		}
	}
	return out
}

// extractCandidates always returns a safe local projection, but it also
// returns the model-quality failure separately. First-time capture may retain
// a clearly labelled local preview; an explicit high-quality rebuild can then
// refuse to replace a better existing projection when the provider is down.
func (e *Engine) extractCandidates(ctx context.Context, turns []turn) ([]candidate, string, error) {
	fallback := extractCandidates(turns)
	if e.extractor == nil {
		return fallback, CompilerVersion, nil
	}
	request := ExtractionRequest{SessionID: turns[0].SessionID, Turns: make([]ExtractionTurn, 0, len(turns))}
	storedParticipants, participantErr := e.sessionParticipants(ctx, turns[0].SessionID)
	if participantErr != nil {
		return fallback, CompilerVersion + "/fallback", fmt.Errorf("load session participant context: %w", participantErr)
	}
	request.Participants = storedParticipants
	participants := make(map[string]bool, len(request.Participants))
	for _, participant := range request.Participants {
		if id := strings.TrimSpace(participant.ParticipantID); id != "" {
			participants[id] = true
		}
	}
	turnByEvidence := make(map[string]turn, len(turns))
	for _, item := range turns {
		turnByEvidence[item.EvidenceID] = item
		request.Turns = append(request.Turns, ExtractionTurn{EvidenceID: item.EvidenceID, SourceSystem: item.SourceSystem, Role: item.Role, Text: item.Body, Scopes: decodeStrings(item.ScopeJSON), ObservedAt: item.ObservedAt})
	}
	result, err := e.extractor.Extract(ctx, request)
	if err != nil {
		return fallback, CompilerVersion + "/fallback", fmt.Errorf("model extraction failed: %w", err)
	}
	if strings.TrimSpace(result.Compiler) == "" {
		compiler := strings.TrimSpace(e.extractor.Compiler(ctx))
		if strings.HasPrefix(compiler, "agent/") {
			return fallback, CompilerVersion + "/fallback", errors.New("configured model returned no extraction result")
		}
		return fallback, CompilerVersion, nil
	}
	totalRunes := 0
	for _, item := range turns {
		totalRunes += utf8.RuneCountInString(item.Body)
	}
	maxCandidates := len(turns) * 12
	if chunks := (totalRunes + 3999) / 4000; chunks*6 > maxCandidates {
		maxCandidates = chunks * 6
	}
	if maxCandidates > 200 {
		maxCandidates = 200
	}
	if len(result.Candidates) > maxCandidates {
		return fallback, CompilerVersion + "/fallback", fmt.Errorf("model extraction exceeded the bounded candidate limit: got %d, maximum %d", len(result.Candidates), maxCandidates)
	}
	allowedTypes := map[string]bool{"fact": true, "state": true, "decision": true, "goal": true, "risk": true, "outcome": true, "procedure": true, "identity": true, "correction": true}
	allowedTiers := map[string]bool{"episodic": true, "semantic": true, "procedural": true, "identity_core": true}
	allowedRisks := map[string]bool{"A": true, "B": true, "C": true, "D": true}
	out := make([]candidate, 0, len(result.Candidates))
	filtered := 0
	for _, extracted := range result.Candidates {
		item, ok := turnByEvidence[strings.TrimSpace(extracted.EvidenceID)]
		statement := compact(strings.TrimSpace(extracted.Statement), 500)
		unitType := strings.ToLower(strings.TrimSpace(extracted.UnitType))
		tier := strings.ToLower(strings.TrimSpace(extracted.TierHint))
		risk := strings.ToUpper(strings.TrimSpace(extracted.RiskTier))
		if !ok && len(turns) == 1 {
			// Each provider request contains one bounded slice of one Evidence
			// turn. Binding a mistyped ID back to that sole source is exact and
			// prevents a harmless model copy error from losing good knowledge.
			item, ok = turns[0], true
		}
		switch unitType {
		case "constraint", "requirement", "rule":
			unitType = "procedure"
		case "preference":
			unitType = "identity"
		case "insight":
			unitType = "fact"
		}
		// MemoryOS, not the provider, owns tier and risk policy. Normalize a
		// semantically valid candidate and elevate protected writes to review
		// rather than throwing the candidate away for a low model risk label.
		switch unitType {
		case "procedure":
			tier, risk = "procedural", "C"
		case "identity":
			tier, risk = "identity_core", "D"
		case "correction":
			tier, risk = "semantic", "C"
		case "state", "outcome":
			tier, risk = "episodic", "A"
		case "decision", "goal", "risk":
			tier, risk = "semantic", "B"
		case "fact":
			tier, risk = "semantic", "A"
		}
		if !ok || utf8.RuneCountInString(statement) < 8 || !allowedTypes[unitType] || !allowedTiers[tier] || !allowedRisks[risk] || extracted.Confidence < 0 || extracted.Confidence > 1 {
			filtered++
			continue
		}
		if (unitType == "procedure" || unitType == "identity" || unitType == "correction" || tier == "procedural" || tier == "identity_core") && risk != "C" && risk != "D" {
			filtered++
			continue
		}
		key := normalizeStatement(statement)
		if key == "" {
			filtered++
			continue
		}
		if lowInformationStatement(statement) || modelMetaStatement(statement) {
			filtered++
			continue
		}
		if primarilyChineseEvidence(item.Body) && hanRunes(statement) < 2 {
			filtered++
			continue
		}
		structure := normalizeKnowledgeStructure(extracted.Structure, item, statement, unitType, result.Compiler, extracted.Confidence, participants)
		out = append(out, candidate{EvidenceID: item.EvidenceID, Statement: statement, Key: key, UnitType: unitType, TierHint: tier, RiskTier: risk, Confidence: extracted.Confidence, Scopes: decodeStrings(item.ScopeJSON), ObservedAt: item.ObservedAt, Structure: structure})
	}
	if len(out) == 0 {
		return fallback, CompilerVersion + "/fallback", errors.New("model extraction produced no valid durable knowledge candidates")
	}
	compiler := strings.TrimSpace(result.Compiler)
	if filtered > 0 {
		compiler += "+filtered"
	}
	if result.Degraded || result.FailedChunks > 0 {
		reason := strings.TrimSpace(result.FailureReason)
		if reason == "" {
			reason = "provider returned an incomplete batch"
		}
		return out, compiler, fmt.Errorf("model extraction completed only %d of %d chunks: %s", result.SucceededChunks, result.TotalChunks, reason)
	}
	return out, compiler, nil
}

func (e *Engine) EnqueueAndProcess(ctx context.Context, sessionID string) (RunResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return RunResult{}, errors.New("session_id is required")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	turns, err := e.readTurns(ctx, sessionID)
	if err != nil {
		return RunResult{}, err
	}
	if len(turns) == 0 {
		return RunResult{}, sql.ErrNoRows
	}
	signatureParts := []string{sessionID, strconv.Itoa(len(turns))}
	for _, item := range turns {
		signatureParts = append(signatureParts, item.EvidenceID)
	}
	jobID := stableID("job_", signatureParts...)
	now := nowString()
	payload, _ := json.Marshal(map[string]string{"session_id": sessionID})
	_, err = e.control.DB.ExecContext(ctx, `INSERT INTO jobs(job_id,kind,idempotency_key,status,payload_json,attempts,created_at,updated_at)
VALUES(?,?,?,?,?,0,?,?) ON CONFLICT(idempotency_key) DO NOTHING`, jobID, "compile_session", "compile:"+strings.Join(signatureParts, ":"), "queued", string(payload), now, now)
	if err != nil {
		return RunResult{}, err
	}
	var status string
	if err := e.control.DB.QueryRowContext(ctx, `SELECT status FROM jobs WHERE job_id=?`, jobID).Scan(&status); err != nil {
		return RunResult{}, err
	}
	if status == "completed" {
		return e.resultForSession(ctx, jobID, sessionID, "completed")
	}
	if _, err := e.control.DB.ExecContext(ctx, `UPDATE jobs SET status='running',attempts=attempts+1,updated_at=? WHERE job_id=?`, nowString(), jobID); err != nil {
		return RunResult{}, err
	}
	result, processErr := e.processSession(ctx, jobID, turns)
	if processErr != nil {
		_, _ = e.control.DB.ExecContext(ctx, `UPDATE jobs SET status='failed',updated_at=? WHERE job_id=?`, nowString(), jobID)
		return RunResult{}, processErr
	}
	_, err = e.control.DB.ExecContext(ctx, `UPDATE jobs SET status='completed',updated_at=? WHERE job_id=?`, nowString(), jobID)
	if err != nil {
		return RunResult{}, err
	}
	result.Status = "completed"
	return result, nil
}

// ReprocessSession replaces only the rebuildable derivatives of one session.
// Canonical Evidence remains untouched. Model extraction is completed before
// the old projection is removed. An explicit rebuild never replaces existing
// memory with a rules fallback or partial model response.
func (e *Engine) ReprocessSession(ctx context.Context, sessionID string) (RunResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return RunResult{}, errors.New("session_id is required")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	turns, err := e.readTurns(ctx, sessionID)
	if err != nil {
		return RunResult{}, err
	}
	if len(turns) == 0 {
		return RunResult{}, sql.ErrNoRows
	}
	candidates, compiler, extractionErr := e.extractCandidates(ctx, turns)
	if extractionErr != nil {
		return RunResult{}, fmt.Errorf("当前模型没有完成高质量沉淀；已有记忆已原样保留，请检查模型连接后重试: %w", extractionErr)
	}
	now := nowString()
	jobID := stableID("job_", "reprocess", sessionID, compiler, now)
	payload, _ := json.Marshal(map[string]string{"session_id": sessionID, "mode": "reprocess", "compiler": compiler})
	if _, err := e.control.DB.ExecContext(ctx, `INSERT INTO jobs(job_id,kind,idempotency_key,status,payload_json,attempts,created_at,updated_at) VALUES(?, 'recompile_session', ?, 'running', ?, 1, ?, ?)`, jobID, "reprocess:"+jobID, string(payload), now, now); err != nil {
		return RunResult{}, err
	}
	if err := e.purgeSessionDerived(ctx, turns); err != nil {
		_, _ = e.control.DB.ExecContext(ctx, `UPDATE jobs SET status='failed',updated_at=? WHERE job_id=?`, nowString(), jobID)
		return RunResult{}, err
	}
	result, err := e.persistSession(ctx, jobID, turns, candidates, compiler, true)
	if err != nil {
		_, _ = e.control.DB.ExecContext(ctx, `UPDATE jobs SET status='failed',updated_at=? WHERE job_id=?`, nowString(), jobID)
		return RunResult{}, err
	}
	if _, err := e.control.DB.ExecContext(ctx, `UPDATE jobs SET status='completed',updated_at=? WHERE job_id=?`, nowString(), jobID); err != nil {
		return RunResult{}, err
	}
	result.Status = "completed"
	return result, nil
}

func (e *Engine) purgeSessionDerived(ctx context.Context, turns []turn) error {
	episodeID := stableID("ep_", turns[0].SessionID)
	evidenceIDs := make([]string, 0, len(turns))
	for _, item := range turns {
		evidenceIDs = append(evidenceIDs, item.EvidenceID)
	}
	type linkedMemory struct {
		id, tier, status, evidenceRaw, episodeRaw string
	}
	memories := []linkedMemory{}
	rows, err := e.control.DB.QueryContext(ctx, `SELECT memory_id,tier,status,evidence_ids_json,episode_ids_json FROM memory_records WHERE EXISTS(SELECT 1 FROM json_each(memory_records.episode_ids_json) WHERE value=?)`, episodeID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var item linkedMemory
		if err := rows.Scan(&item.id, &item.tier, &item.status, &item.evidenceRaw, &item.episodeRaw); err != nil {
			rows.Close()
			return err
		}
		memories = append(memories, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	tx, err := e.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM record_projects WHERE record_type='knowledge_unit' AND record_id IN (SELECT unit_id FROM knowledge_units WHERE episode_id=?)`, episodeID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM memory_operations WHERE episode_id=?`, episodeID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM knowledge_units WHERE episode_id=?`, episodeID); err != nil {
		return err
	}
	for _, item := range memories {
		episodes := without(decodeStrings(item.episodeRaw), episodeID)
		evidence := without(decodeStrings(item.evidenceRaw), evidenceIDs...)
		if len(episodes) == 0 {
			assetRows, queryErr := tx.QueryContext(ctx, `SELECT asset_id,source_memory_ids_json FROM agent_assets WHERE EXISTS(SELECT 1 FROM json_each(agent_assets.source_memory_ids_json) WHERE value=?)`, item.id)
			if queryErr != nil {
				return queryErr
			}
			type linkedAsset struct{ id, sources string }
			assets := []linkedAsset{}
			for assetRows.Next() {
				var asset linkedAsset
				if err := assetRows.Scan(&asset.id, &asset.sources); err != nil {
					assetRows.Close()
					return err
				}
				assets = append(assets, asset)
			}
			assetRows.Close()
			for _, asset := range assets {
				sources := without(decodeStrings(asset.sources), item.id)
				if len(sources) == 0 {
					if _, err := tx.ExecContext(ctx, `DELETE FROM record_projects WHERE record_type='asset' AND record_id=?`, asset.id); err != nil {
						return err
					}
					if _, err := tx.ExecContext(ctx, `DELETE FROM agent_assets WHERE asset_id=?`, asset.id); err != nil {
						return err
					}
				} else if _, err := tx.ExecContext(ctx, `UPDATE agent_assets SET source_memory_ids_json=?,updated_at=? WHERE asset_id=?`, jsonStrings(sources), nowString(), asset.id); err != nil {
					return err
				}
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM record_projects WHERE record_type='memory' AND record_id=?`, item.id); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM memory_records WHERE memory_id=?`, item.id); err != nil {
				return err
			}
			continue
		}
		status := item.status
		if status == "corroborated" && len(episodes) == 1 {
			status = "active"
		}
		if _, err := tx.ExecContext(ctx, `UPDATE memory_records SET status=?,strength=?,evidence_ids_json=?,episode_ids_json=?,updated_at=? WHERE memory_id=?`, status, len(episodes), jsonStrings(evidence), jsonStrings(episodes), nowString(), item.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (e *Engine) ProcessAll(ctx context.Context) ([]RunResult, error) {
	rows, err := e.search.DB.QueryContext(ctx, `SELECT DISTINCT session_id FROM turns ORDER BY session_id`)
	if err != nil {
		return nil, err
	}
	var sessions []string
	for rows.Next() {
		var sessionID string
		if err := rows.Scan(&sessionID); err != nil {
			rows.Close()
			return nil, err
		}
		sessions = append(sessions, sessionID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	results := make([]RunResult, 0, len(sessions))
	for _, sessionID := range sessions {
		result, err := e.EnqueueAndProcess(ctx, sessionID)
		if err != nil {
			return results, fmt.Errorf("process session %s: %w", sessionID, err)
		}
		results = append(results, result)
	}
	return results, nil
}

func (e *Engine) Recover(ctx context.Context) error {
	if _, err := e.control.DB.ExecContext(ctx, `UPDATE jobs SET status='queued',updated_at=? WHERE kind='compile_session' AND status='running'`, nowString()); err != nil {
		return err
	}
	rows, err := e.control.DB.QueryContext(ctx, `SELECT payload_json FROM jobs WHERE kind='compile_session' AND status='queued' ORDER BY created_at`)
	if err != nil {
		return err
	}
	var sessions []string
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		var payload struct {
			SessionID string `json:"session_id"`
		}
		if json.Unmarshal([]byte(raw), &payload) == nil && payload.SessionID != "" {
			sessions = append(sessions, payload.SessionID)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, sessionID := range unique(sessions) {
		if _, err := e.EnqueueAndProcess(ctx, sessionID); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) processSession(ctx context.Context, jobID string, turns []turn) (RunResult, error) {
	candidates, compiler, _ := e.extractCandidates(ctx, turns)
	return e.persistSession(ctx, jobID, turns, candidates, compiler, false)
}

func (e *Engine) persistSession(ctx context.Context, jobID string, turns []turn, candidates []candidate, compiler string, forceRevision bool) (RunResult, error) {
	sessionID := turns[0].SessionID
	episodeID := stableID("ep_", sessionID)
	evidenceIDs := make([]string, 0, len(turns))
	sources := make([]string, 0, len(turns))
	title := "未命名会话"
	for _, item := range turns {
		evidenceIDs = append(evidenceIDs, item.EvidenceID)
		sources = append(sources, item.SourceSystem)
		if title == "未命名会话" && strings.TrimSpace(item.Body) != "" {
			title = compact(item.Body, 72)
		}
	}
	summary := fmt.Sprintf("%d 条原始 Evidence，抽取 %d 个可追溯知识点；由 %s 编译。", len(turns), len(candidates), compiler)
	now := nowString()
	var oldEvidence, oldCompiler string
	var revision int
	err := e.control.DB.QueryRowContext(ctx, `SELECT evidence_ids_json,compiler,revision FROM episodes WHERE episode_id=?`, episodeID).Scan(&oldEvidence, &oldCompiler, &revision)
	if err != nil && err != sql.ErrNoRows {
		return RunResult{}, err
	}
	newEvidence := jsonStrings(evidenceIDs)
	if err == sql.ErrNoRows {
		revision = 1
	} else if forceRevision || oldEvidence != newEvidence || oldCompiler != compiler {
		revision++
	}
	_, err = e.control.DB.ExecContext(ctx, `INSERT INTO episodes(episode_id,session_id,source_system,title,summary,status,evidence_ids_json,started_at,ended_at,compiler,revision,created_at,updated_at)
VALUES(?,?,?,?,?,'compiled',?,?,?,?,?,?,?)
ON CONFLICT(episode_id) DO UPDATE SET source_system=excluded.source_system,title=excluded.title,summary=excluded.summary,status=excluded.status,evidence_ids_json=excluded.evidence_ids_json,started_at=excluded.started_at,ended_at=excluded.ended_at,compiler=excluded.compiler,revision=excluded.revision,updated_at=excluded.updated_at`,
		episodeID, sessionID, strings.Join(unique(sources), ", "), title, summary, newEvidence, turns[0].ObservedAt, turns[len(turns)-1].ObservedAt, compiler, revision, now, now)
	if err != nil {
		return RunResult{}, err
	}
	for _, item := range candidates {
		unitID := stableID("ku_", item.EvidenceID, item.Key)
		_, err := e.control.DB.ExecContext(ctx, `INSERT INTO knowledge_units(unit_id,episode_id,evidence_id,unit_type,tier_hint,statement,normalized_key,confidence,risk_tier,status,scope_json,observed_at,created_at)
VALUES(?,?,?,?,?,?,?,?,?,'candidate',?,?,?) ON CONFLICT(evidence_id,normalized_key) DO NOTHING`,
			unitID, episodeID, item.EvidenceID, item.UnitType, item.TierHint, item.Statement, item.Key, item.Confidence, item.RiskTier, jsonStrings(item.Scopes), item.ObservedAt, now)
		if err != nil {
			return RunResult{}, err
		}
		item.Structure.Temporal.RecordedAt = now
		item.Structure.Provenance.EpisodeID = episodeID
		if err := e.persistKnowledgeStructure(ctx, unitID, item.Structure); err != nil {
			return RunResult{}, err
		}
	}
	operations, err := e.consolidate(ctx, episodeID, sessionID, title, summary, evidenceIDs, turns[0].ObservedAt)
	if err != nil {
		return RunResult{}, err
	}
	if err := e.backfillProjectLinks(ctx); err != nil {
		return RunResult{}, err
	}
	qualityStatus := "high_quality"
	if strings.Contains(compiler, "fallback") || strings.Contains(compiler, "partial-") {
		qualityStatus = "degraded"
	} else if !strings.HasPrefix(compiler, "agent/") {
		qualityStatus = "local_rules"
	}
	return RunResult{JobID: jobID, SessionID: sessionID, EpisodeID: episodeID, Compiler: compiler, QualityStatus: qualityStatus, Evidence: len(turns), KnowledgeUnits: len(candidates), Operations: operations}, nil
}

func (e *Engine) backfillProjectLinks(ctx context.Context) error {
	now := nowString()
	statements := []string{
		`INSERT OR IGNORE INTO record_projects(record_type,record_id,project_id,relation,is_primary,created_at) SELECT 'episode',episode_id,'project-inbox','fallback',1,? FROM episodes e WHERE NOT EXISTS(SELECT 1 FROM record_projects rp WHERE rp.record_type='episode' AND rp.record_id=e.episode_id)`,
		`INSERT OR IGNORE INTO record_projects(record_type,record_id,project_id,relation,is_primary,created_at) SELECT 'knowledge_unit',unit_id,'project-inbox','fallback',1,? FROM knowledge_units u WHERE NOT EXISTS(SELECT 1 FROM record_projects rp WHERE rp.record_type='knowledge_unit' AND rp.record_id=u.unit_id)`,
		`INSERT OR IGNORE INTO record_projects(record_type,record_id,project_id,relation,is_primary,created_at) SELECT 'memory',memory_id,'project-inbox','fallback',1,? FROM memory_records m WHERE NOT EXISTS(SELECT 1 FROM record_projects rp WHERE rp.record_type='memory' AND rp.record_id=m.memory_id)`,
		`INSERT OR IGNORE INTO record_projects(record_type,record_id,project_id,relation,is_primary,created_at) SELECT 'asset',asset_id,'project-inbox','fallback',1,? FROM agent_assets a WHERE NOT EXISTS(SELECT 1 FROM record_projects rp WHERE rp.record_type='asset' AND rp.record_id=a.asset_id)`,
	}
	for _, statement := range statements {
		if _, err := e.control.DB.ExecContext(ctx, statement, now); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) resultForSession(ctx context.Context, jobID, sessionID, status string) (RunResult, error) {
	var result RunResult
	result.JobID, result.SessionID, result.Status = jobID, sessionID, status
	err := e.control.DB.QueryRowContext(ctx, `SELECT episode_id,compiler,json_array_length(evidence_ids_json),(SELECT count(*) FROM knowledge_units WHERE episode_id=episodes.episode_id) FROM episodes WHERE session_id=?`, sessionID).
		Scan(&result.EpisodeID, &result.Compiler, &result.Evidence, &result.KnowledgeUnits)
	if strings.Contains(result.Compiler, "fallback") || strings.Contains(result.Compiler, "partial-") {
		result.QualityStatus = "degraded"
	} else if strings.HasPrefix(result.Compiler, "agent/") {
		result.QualityStatus = "high_quality"
	} else {
		result.QualityStatus = "local_rules"
	}
	return result, err
}

func (e *Engine) consolidate(ctx context.Context, episodeID, sessionID, title, summary string, evidenceIDs []string, observedAt string) (int, error) {
	operations := 0
	// Every compiled session is itself a durable episodic memory. Its stable key
	// lets later turns revise the same episode without duplicating it.
	episodicKey := "episode:" + sessionID
	episodicID := stableID("mem_", "episodic", episodicKey)
	now := nowString()
	_, err := e.control.DB.ExecContext(ctx, `INSERT INTO memory_records(memory_id,tier,asset_form,domain,status,summary,body,canonical_key,confidence,importance,strength,evidence_ids_json,episode_ids_json,scopes_json,visibility,observed_at,created_at,updated_at)
VALUES(?,'episodic','episode','personal','active',?,?,?,?,0.65,1,?,?,?,'private',?,?,?)
ON CONFLICT(tier,canonical_key) DO UPDATE SET summary=excluded.summary,body=excluded.body,evidence_ids_json=excluded.evidence_ids_json,episode_ids_json=excluded.episode_ids_json,updated_at=excluded.updated_at`,
		episodicID, title, summary, episodicKey, 0.88, jsonStrings(evidenceIDs), jsonStrings([]string{episodeID}), `[]`, observedAt, now, now)
	if err != nil {
		return 0, err
	}
	opID := stableID("op_", "CREATE", episodicID, episodeID)
	result, err := e.control.DB.ExecContext(ctx, `INSERT INTO memory_operations(operation_id,operation_type,status,target_memory_id,episode_id,evidence_ids_json,reason_codes_json,risk_tier,confidence,patch_json,created_at,applied_at)
VALUES(?,'CREATE','applied',?,?,?,'["episode_compiled"]','A',0.88,'{}',?,?) ON CONFLICT(operation_id) DO NOTHING`, opID, episodicID, episodeID, jsonStrings(evidenceIDs), now, now)
	if err != nil {
		return 0, err
	}
	if n, _ := result.RowsAffected(); n > 0 {
		operations++
	}

	rows, err := e.control.DB.QueryContext(ctx, `SELECT unit_id,evidence_id,unit_type,tier_hint,statement,normalized_key,confidence,risk_tier,scope_json,observed_at FROM knowledge_units WHERE episode_id=? AND processed_at IS NULL ORDER BY observed_at,unit_id`, episodeID)
	if err != nil {
		return operations, err
	}
	var units []KnowledgeUnit
	for rows.Next() {
		var unit KnowledgeUnit
		var scopes string
		if err := rows.Scan(&unit.UnitID, &unit.EvidenceID, &unit.UnitType, &unit.TierHint, &unit.Statement, &unit.NormalizedKey, &unit.Confidence, &unit.RiskTier, &scopes, &unit.ObservedAt); err != nil {
			rows.Close()
			return operations, err
		}
		unit.Scopes = decodeStrings(scopes)
		units = append(units, unit)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return operations, err
	}
	rows.Close()
	if err := e.hydrateKnowledgeUnits(ctx, units); err != nil {
		return operations, err
	}
	for _, unit := range units {
		created, err := e.consolidateUnit(ctx, unit, episodeID, episodicID)
		if err != nil {
			return operations, err
		}
		if created {
			operations++
		}
	}
	return operations, nil
}

func (e *Engine) consolidateUnit(ctx context.Context, unit KnowledgeUnit, episodeID, episodicID string) (bool, error) {
	now := nowString()
	if unit.TierHint == "episodic" {
		opID := stableID("op_", "EXTRACT", unit.UnitID, episodicID)
		result, err := e.control.DB.ExecContext(ctx, `INSERT INTO memory_operations(operation_id,operation_type,status,target_memory_id,unit_id,episode_id,evidence_ids_json,reason_codes_json,risk_tier,confidence,patch_json,created_at,applied_at)
VALUES(?,'EXTRACT','applied',?,?,?,?, '["episode_context"]','A',?,'{}',?,?) ON CONFLICT(operation_id) DO NOTHING`, opID, episodicID, unit.UnitID, episodeID, jsonStrings([]string{unit.EvidenceID}), unit.Confidence, now, now)
		if err != nil {
			return false, err
		}
		if _, err := e.control.DB.ExecContext(ctx, `UPDATE knowledge_units SET status='attached',processed_at=? WHERE unit_id=?`, now, unit.UnitID); err != nil {
			return false, err
		}
		n, _ := result.RowsAffected()
		return n > 0, nil
	}

	tier := unit.TierHint
	canonicalKey := unit.NormalizedKey
	memoryID := stableID("mem_", tier, canonicalKey)
	assetForm := unit.UnitType
	status := "active"
	opType := "CREATE"
	opStatus := "applied"
	reasons := []string{"new_atomic_knowledge"}
	if unit.UnitType == "correction" {
		canonicalKey = "correction:" + canonicalKey
		memoryID = stableID("mem_", tier, canonicalKey)
		status = "conflict"
		opType = "CORRECT"
		opStatus = "review_required"
		reasons = []string{"explicit_correction", "human_review_required"}
	} else if tier == "procedural" || tier == "identity_core" {
		status = "candidate"
		opStatus = "review_required"
		reasons = []string{"protected_tier", "human_review_required"}
	}
	// A durable statement without a resolved subject is still useful as a
	// review candidate, but must never look like an active fact or silently
	// influence an Agent. A participant binding can resolve it later and the
	// normal review path can then publish it.
	resolution := normalizeResolution(unit.Structure.Attribution.Resolution)
	if resolution == "unresolved" || resolution == "ambiguous" {
		status = "candidate"
		opStatus = "review_required"
		reasons = appendUnique(reasons, "subject_"+resolution, "human_review_required")
	}

	var existing MemoryRecord
	var evidenceRaw, episodeRaw, scopesRaw sql.NullString
	err := e.control.DB.QueryRowContext(ctx, `SELECT memory_id,status,strength,evidence_ids_json,episode_ids_json,scopes_json FROM memory_records WHERE tier=? AND canonical_key=?`, tier, canonicalKey).
		Scan(&existing.MemoryID, &existing.Status, &existing.Strength, &evidenceRaw, &episodeRaw, &scopesRaw)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	if err == sql.ErrNoRows {
		_, err = e.control.DB.ExecContext(ctx, `INSERT INTO memory_records(memory_id,tier,asset_form,domain,status,summary,body,canonical_key,confidence,importance,strength,evidence_ids_json,episode_ids_json,scopes_json,visibility,observed_at,created_at,updated_at)
VALUES(?,?,?,?,?,?,?,?,?,0.55,1,?,?,?,'private',?,?,?)`, memoryID, tier, assetForm, domainFromScopes(unit.Scopes), status, compact(unit.Statement, 160), unit.Statement, canonicalKey, unit.Confidence, jsonStrings([]string{unit.EvidenceID}), jsonStrings([]string{episodeID}), jsonStrings(unit.Scopes), unit.ObservedAt, now, now)
		if err != nil {
			return false, err
		}
	} else {
		memoryID = existing.MemoryID
		if opStatus == "applied" {
			opType = "CORROBORATE"
			reasons = []string{"exact_knowledge_match", "independent_evidence"}
			evidenceIDs := appendUnique(decodeStrings(evidenceRaw.String), unit.EvidenceID)
			episodeIDs := appendUnique(decodeStrings(episodeRaw.String), episodeID)
			scopes := appendUnique(decodeStrings(scopesRaw.String), unit.Scopes...)
			_, err = e.control.DB.ExecContext(ctx, `UPDATE memory_records SET status='corroborated',strength=strength+1,evidence_ids_json=?,episode_ids_json=?,scopes_json=?,updated_at=?,last_reinforced_at=? WHERE memory_id=?`, jsonStrings(evidenceIDs), jsonStrings(episodeIDs), jsonStrings(scopes), now, now, memoryID)
			if err != nil {
				return false, err
			}
		} else {
			// Additional evidence is still valuable while a protected or
			// unresolved statement waits for review. Accumulate its provenance
			// without promoting the candidate to an active fact.
			evidenceIDs := appendUnique(decodeStrings(evidenceRaw.String), unit.EvidenceID)
			episodeIDs := appendUnique(decodeStrings(episodeRaw.String), episodeID)
			scopes := appendUnique(decodeStrings(scopesRaw.String), unit.Scopes...)
			_, err = e.control.DB.ExecContext(ctx, `UPDATE memory_records SET strength=?,evidence_ids_json=?,episode_ids_json=?,scopes_json=?,updated_at=?,last_reinforced_at=? WHERE memory_id=?`, len(episodeIDs), jsonStrings(evidenceIDs), jsonStrings(episodeIDs), jsonStrings(scopes), now, now, memoryID)
			if err != nil {
				return false, err
			}
		}
	}

	opID := stableID("op_", opType, unit.UnitID, memoryID)
	appliedAt := any(nil)
	if opStatus == "applied" {
		appliedAt = now
	}
	result, err := e.control.DB.ExecContext(ctx, `INSERT INTO memory_operations(operation_id,operation_type,status,target_memory_id,unit_id,episode_id,evidence_ids_json,reason_codes_json,risk_tier,confidence,patch_json,created_at,applied_at)
VALUES(?,?,?,?,?,?,?,?,?,?,'{}',?,?) ON CONFLICT(operation_id) DO NOTHING`, opID, opType, opStatus, memoryID, unit.UnitID, episodeID, jsonStrings([]string{unit.EvidenceID}), jsonStrings(reasons), unit.RiskTier, unit.Confidence, now, appliedAt)
	if err != nil {
		return false, err
	}
	unitStatus := "consolidated"
	if opStatus == "review_required" {
		unitStatus = "review_required"
	}
	if _, err := e.control.DB.ExecContext(ctx, `UPDATE knowledge_units SET status=?,processed_at=? WHERE unit_id=?`, unitStatus, now, unit.UnitID); err != nil {
		return false, err
	}
	if tier == "procedural" {
		classification := classifyAgentAsset(unit)
		// Keep the legacy identity salt so existing assets retain stable IDs while
		// their type becomes an independently governed attribute.
		assetID := stableID("asset_", "procedure", memoryID)
		assetStatus := "candidate"
		if classification.Ambiguous {
			assetStatus = "review_required"
		}
		_, err = e.control.DB.ExecContext(ctx, `INSERT INTO agent_assets(asset_id,asset_type,title,summary,status,version,risk_level,source_memory_ids_json,validation_status,created_at,updated_at)
VALUES(?,?,?,?,?,'0.3','high',?,'not_run',?,?) ON CONFLICT(asset_id) DO UPDATE SET asset_type=excluded.asset_type,title=excluded.title,summary=excluded.summary,version=excluded.version,status=CASE WHEN agent_assets.status='candidate' AND excluded.asset_type='unclassified' THEN 'review_required' ELSE agent_assets.status END,updated_at=excluded.updated_at`, assetID, classification.Type, compact(unit.Statement, 80), unit.Statement, assetStatus, jsonStrings([]string{memoryID}), now, now)
		if err != nil {
			return false, err
		}
		if err := e.recordAssetClassification(ctx, assetID, classification, now); err != nil {
			return false, err
		}
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

func domainFromScopes(scopes []string) string {
	for _, scope := range scopes {
		if strings.HasPrefix(scope, "project:") || strings.HasPrefix(scope, "workspace:") || strings.HasPrefix(scope, "repo:") {
			return scope
		}
	}
	return "personal"
}

func (e *Engine) recordAssetClassification(ctx context.Context, assetID string, classification assetClassification, now string) error {
	scoresRaw, _ := json.Marshal(classification.Scores)
	reasonsRaw, _ := json.Marshal(classification.Reasons)
	status := "classified"
	if classification.Ambiguous {
		status = "ambiguous"
	}
	_, err := e.control.DB.ExecContext(ctx, `INSERT INTO agent_asset_classifications(asset_id,classifier_version,classification_status,scores_json,reasons_json,updated_at)
VALUES(?,'deterministic-v3',?,?,?,?) ON CONFLICT(asset_id) DO UPDATE SET classifier_version=excluded.classifier_version,classification_status=excluded.classification_status,scores_json=excluded.scores_json,reasons_json=excluded.reasons_json,updated_at=excluded.updated_at`, assetID, status, string(scoresRaw), string(reasonsRaw), now)
	return err
}

// ReconcileAgentAssets upgrades derived asset classifications in place. It is
// intentionally idempotent: identity, review status, validation status and source
// lineage are preserved; only classifier-owned presentation fields are updated.
func (e *Engine) ReconcileAgentAssets(ctx context.Context) (int, error) {
	rows, err := e.control.DB.QueryContext(ctx, `SELECT a.asset_id,m.body FROM agent_assets a JOIN json_each(a.source_memory_ids_json) src JOIN memory_records m ON m.memory_id=src.value LEFT JOIN agent_asset_classifications c ON c.asset_id=a.asset_id WHERE a.status IN ('candidate','review_required') AND coalesce(c.classification_status,'')<>'manual_classified' ORDER BY a.asset_id`)
	if err != nil {
		return 0, err
	}
	type item struct{ id, body string }
	items := []item{}
	for rows.Next() {
		var value item
		if err := rows.Scan(&value.id, &value.body); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, value)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	changed := 0
	for _, value := range items {
		classification := classifyAgentAssetText(value.body)
		now := nowString()
		result, err := e.control.DB.ExecContext(ctx, `UPDATE agent_assets SET asset_type=?,title=?,summary=?,version='0.3',status=CASE WHEN status='candidate' AND ? THEN 'review_required' ELSE status END,updated_at=? WHERE asset_id=? AND (asset_type<>? OR title<>? OR summary<>? OR version<>'0.3' OR (status='candidate' AND ?))`, classification.Type, compact(value.body, 80), value.body, classification.Ambiguous, now, value.id, classification.Type, compact(value.body, 80), value.body, classification.Ambiguous)
		if err != nil {
			return changed, err
		}
		if err := e.recordAssetClassification(ctx, value.id, classification, now); err != nil {
			return changed, err
		}
		if n, _ := result.RowsAffected(); n > 0 {
			changed += int(n)
		}
	}
	return changed, nil
}

func (e *Engine) ReviewOperation(ctx context.Context, operationID, decision, reviewer string) (MemoryOperation, error) {
	decision = strings.ToLower(strings.TrimSpace(decision))
	if decision != "approve" && decision != "reject" {
		return MemoryOperation{}, errors.New("decision must be approve or reject")
	}
	if strings.TrimSpace(reviewer) == "" {
		reviewer = "local-user"
	}
	tx, err := e.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return MemoryOperation{}, err
	}
	defer tx.Rollback()
	var status, target string
	if err := tx.QueryRowContext(ctx, `SELECT status,coalesce(target_memory_id,'') FROM memory_operations WHERE operation_id=?`, operationID).Scan(&status, &target); err != nil {
		return MemoryOperation{}, err
	}
	if status != "review_required" {
		return MemoryOperation{}, fmt.Errorf("operation is not awaiting review: %s", status)
	}
	now := nowString()
	if decision == "approve" {
		if _, err := tx.ExecContext(ctx, `UPDATE memory_operations SET status='applied',decided_at=?,applied_at=?,reviewed_by=? WHERE operation_id=?`, now, now, reviewer, operationID); err != nil {
			return MemoryOperation{}, err
		}
		if target != "" {
			if _, err := tx.ExecContext(ctx, `UPDATE memory_records SET status='active',updated_at=? WHERE memory_id=?`, now, target); err != nil {
				return MemoryOperation{}, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE agent_assets SET status='approved',updated_at=? WHERE EXISTS (SELECT 1 FROM json_each(agent_assets.source_memory_ids_json) WHERE value=?)`, now, target); err != nil {
				return MemoryOperation{}, err
			}
		}
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE memory_operations SET status='rejected',decided_at=?,reviewed_by=? WHERE operation_id=?`, now, reviewer, operationID); err != nil {
			return MemoryOperation{}, err
		}
		if target != "" {
			if _, err := tx.ExecContext(ctx, `UPDATE memory_records SET status='deprecated',updated_at=? WHERE memory_id=?`, now, target); err != nil {
				return MemoryOperation{}, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE agent_assets SET status='rejected',updated_at=? WHERE EXISTS (SELECT 1 FROM json_each(agent_assets.source_memory_ids_json) WHERE value=?)`, now, target); err != nil {
				return MemoryOperation{}, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return MemoryOperation{}, err
	}
	return e.operation(ctx, operationID)
}

func (e *Engine) RebuildLivingViewsForProject(ctx context.Context, projectID string) ([]LivingView, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, errors.New("project_id is required")
	}
	memories, _, err := e.ListMemoriesForProject(ctx, projectID, "", "", 500, 0)
	if err != nil {
		return nil, err
	}
	active := make([]MemoryRecord, 0, len(memories))
	for _, item := range memories {
		if item.Status == "active" || item.Status == "corroborated" || item.Status == "candidate" || item.Status == "conflict" {
			active = append(active, item)
		}
	}
	sort.Slice(active, func(i, j int) bool { return active[i].UpdatedAt > active[j].UpdatedAt })
	ids := make([]string, 0, len(active))
	for _, item := range active {
		ids = append(ids, item.MemoryID)
	}
	hot := append([]string(nil), ids...)
	if len(hot) > 8 {
		hot = hot[:8]
	}
	lastEpisode := []string{}
	for _, item := range active {
		if item.Tier == "episodic" {
			lastEpisode = []string{item.MemoryID}
			break
		}
	}
	now := nowString()
	if len(active) == 0 {
		tx, err := e.control.DB.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `DELETE FROM record_projects WHERE record_type='living' AND project_id=?`, projectID); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM living_views WHERE project_id=?`, projectID); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return []LivingView{}, nil
	}
	filePrefix := stableID("project-", projectID)
	views := []viewSpec{
		{stableID("living-", projectID, "memory-index"), "index", "Memory Index", fmt.Sprintf("%d 条可用记忆，按层级与更新时间组织。", len(active)), "memory/" + filePrefix + "-Memory-Index.md", ids},
		{stableID("living-", projectID, "hot-index"), "hot", "Hot Index", fmt.Sprintf("最近活跃的 %d 条记忆。", len(hot)), "memory/" + filePrefix + "-Hot-Index.md", hot},
		{stableID("living-", projectID, "active-context"), "context", "Active Context", "最近一次工作 Episode，可用于恢复上下文。", "memory/" + filePrefix + "-Active-Context.md", lastEpisode},
	}
	tx, err := e.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// living_views is a disposable projection. Replace only this project's
	// membership links; canonical Evidence, Memory and Object revisions remain untouched.
	if _, err := tx.ExecContext(ctx, `DELETE FROM record_projects WHERE record_type='living' AND project_id=?`, projectID); err != nil {
		return nil, err
	}
	out := make([]LivingView, 0, len(views))
	for _, view := range views {
		_, err := tx.ExecContext(ctx, `INSERT INTO living_views(view_id,project_id,view_type,title,summary,status,source_memory_ids_json,canonical_path,updated_at)
VALUES(?,?,?,?,?,'active',?,?,?) ON CONFLICT(view_id) DO UPDATE SET project_id=excluded.project_id,view_type=excluded.view_type,title=excluded.title,summary=excluded.summary,status=excluded.status,source_memory_ids_json=excluded.source_memory_ids_json,canonical_path=excluded.canonical_path,updated_at=excluded.updated_at`, view.id, projectID, view.kind, view.title, view.summary, jsonStrings(view.ids), view.path, now)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO record_projects(record_type,record_id,project_id,relation,is_primary,created_at) VALUES('living',?,?,'projection',1,?) ON CONFLICT(record_type,record_id,project_id) DO UPDATE SET relation='projection',is_primary=1`, view.id, projectID, now); err != nil {
			return nil, err
		}
		out = append(out, LivingView{ViewID: view.id, ProjectID: projectID, ViewType: view.kind, Title: view.title, Summary: view.summary, Status: "active", SourceMemoryIDs: append([]string(nil), view.ids...), CanonicalPath: view.path, UpdatedAt: now})
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if err := e.writeLivingMarkdown(active, views, now); err != nil {
		return nil, err
	}
	return out, nil
}

func (e *Engine) writeLivingMarkdown(memories []MemoryRecord, views []viewSpec, now string) error {
	byID := make(map[string]MemoryRecord, len(memories))
	for _, item := range memories {
		byID[item.MemoryID] = item
	}
	for _, view := range views {
		var b strings.Builder
		fmt.Fprintf(&b, "# %s\n\n%s\n\n", view.title, view.summary)
		fmt.Fprintf(&b, "> Generated by MemoryOS %s at %s. Rebuildable from canonical Evidence.\n\n", CompilerVersion, now)
		if len(view.ids) == 0 {
			b.WriteString("_No memory records yet._\n")
		}
		for _, id := range view.ids {
			item, ok := byID[id]
			if !ok {
				continue
			}
			fmt.Fprintf(&b, "- **[%s]** %s  \n  `%s` · strength %.0f · %s\n", item.Tier, item.Summary, item.MemoryID, item.Strength, item.Status)
		}
		path := filepath.Join(e.memoryDir, filepath.Base(view.path))
		if err := atomicWrite(path, []byte(b.String())); err != nil {
			return err
		}
	}
	return nil
}

func atomicWrite(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".memoryos-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// ResolveAssetClassification lets the local Owner resolve an ambiguous derived
// asset without bypassing governance. It changes only the classifier-owned type
// while keeping the derived content, stable identity and source lineage intact.
func (e *Engine) ResolveAssetClassification(ctx context.Context, assetID, assetType, reviewer string) (AgentAsset, error) {
	assetID = strings.TrimSpace(assetID)
	assetType = strings.ToLower(strings.TrimSpace(assetType))
	if !IsGovernedAgentAssetType(assetType) {
		return AgentAsset{}, errors.New("asset_type must be prompt, skill, rule, constraint, procedure, tool_recipe or mcp")
	}
	if strings.TrimSpace(reviewer) == "" {
		reviewer = "local-owner"
	}
	tx, err := e.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return AgentAsset{}, err
	}
	defer tx.Rollback()
	var currentStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM agent_assets WHERE asset_id=?`, assetID).Scan(&currentStatus); err != nil {
		return AgentAsset{}, err
	}
	if currentStatus != "candidate" && currentStatus != "review_required" {
		return AgentAsset{}, fmt.Errorf("asset classification can only be changed before activation: %s", currentStatus)
	}
	now := nowString()
	if _, err := tx.ExecContext(ctx, `UPDATE agent_assets SET asset_type=?,status='candidate',updated_at=? WHERE asset_id=?`, assetType, now, assetID); err != nil {
		return AgentAsset{}, err
	}
	reasons, _ := json.Marshal([]string{"owner-override:" + reviewer})
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_asset_classifications(asset_id,classifier_version,classification_status,scores_json,reasons_json,updated_at)
VALUES(?,'manual-v2','manual_classified','{}',?,?) ON CONFLICT(asset_id) DO UPDATE SET classifier_version=excluded.classifier_version,classification_status=excluded.classification_status,scores_json=excluded.scores_json,reasons_json=excluded.reasons_json,updated_at=excluded.updated_at`, assetID, string(reasons), now); err != nil {
		return AgentAsset{}, err
	}
	if err := tx.Commit(); err != nil {
		return AgentAsset{}, err
	}
	return e.asset(ctx, assetID)
}
