package portfolio

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/luoyif/memory-harness/internal/store"
)

const (
	InboxProjectID    = "project-inbox"
	PersonalProjectID = "project-personal"
)

var (
	slugPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
	colorPattern    = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

type Service struct {
	control *store.ControlStore
	search  *store.SearchStore
}

func New(control *store.ControlStore, search *store.SearchStore) *Service {
	return &Service{control: control, search: search}
}

func nowString() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func stableID(prefix string, values ...string) string {
	h := sha256.New()
	for _, value := range values {
		_, _ = h.Write([]byte(strings.TrimSpace(value)))
		_, _ = h.Write([]byte{0})
	}
	return prefix + hex.EncodeToString(h.Sum(nil)[:12])
}

func encodeStrings(values []string) string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	b, _ := json.Marshal(out)
	return string(b)
}

func decodeStrings(raw string) []string {
	var out []string
	_ = json.Unmarshal([]byte(raw), &out)
	if out == nil {
		out = []string{}
	}
	return out
}

func normalizeTime(value string, fallback time.Time) (string, error) {
	if strings.TrimSpace(value) == "" {
		return fallback.UTC().Format(time.RFC3339Nano), nil
	}
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("invalid date-time %q", value)
	}
	return parsed.UTC().Format(time.RFC3339Nano), nil
}

func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

func (s *Service) projectExists(ctx context.Context, projectID string) error {
	var exists int
	err := s.control.DB.QueryRowContext(ctx, `SELECT 1 FROM projects WHERE project_id=?`, projectID).Scan(&exists)
	if err == sql.ErrNoRows {
		return fmt.Errorf("project %q: %w", projectID, sql.ErrNoRows)
	}
	return err
}

func (s *Service) CreateProject(ctx context.Context, input ProjectInput) (Project, error) {
	input.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.DefaultCurrency = strings.ToUpper(strings.TrimSpace(input.DefaultCurrency))
	if input.Slug == "" {
		input.Slug = stableID("space-", input.Name, nowString())
	}
	if !slugPattern.MatchString(input.Slug) {
		return Project{}, errors.New("slug must contain lowercase letters, digits and hyphens")
	}
	if input.Name == "" || utf8.RuneCountInString(input.Name) > 120 {
		return Project{}, errors.New("name must contain 1 to 120 characters")
	}
	if input.DefaultCurrency == "" {
		input.DefaultCurrency = "CNY"
	}
	if !currencyPattern.MatchString(input.DefaultCurrency) {
		return Project{}, errors.New("default_currency must be a three-letter uppercase code")
	}
	if input.Color == "" {
		input.Color = "#52715F"
	}
	if !colorPattern.MatchString(input.Color) {
		return Project{}, errors.New("color must be a six-digit hex color")
	}
	projectID := stableID("project-", input.Slug)
	now := nowString()
	tx, err := s.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return Project{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO projects(project_id,slug,name,description,status,color,default_currency,budget_minor,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		projectID, input.Slug, input.Name, input.Description, "active", input.Color, input.DefaultCurrency, input.BudgetMinor, now, now)
	if err != nil {
		return Project{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO project_automation(project_id,import_mode,updated_at) VALUES(?,'auto_new',?)`, projectID, now); err != nil {
		return Project{}, err
	}
	aliases := append([]string{input.Slug, input.Name}, input.Aliases...)
	seenAliases := map[string]bool{}
	for _, alias := range aliases {
		alias = strings.TrimSpace(alias)
		key := strings.ToLower(alias)
		if alias == "" || seenAliases[key] {
			continue
		}
		seenAliases[key] = true
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_aliases(alias,project_id,created_at) VALUES(?,?,?)`, alias, projectID, now); err != nil {
			return Project{}, fmt.Errorf("project alias %q: %w", alias, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Project{}, err
	}
	project, err := s.Project(ctx, projectID)
	if err == nil {
		err = s.indexProject(ctx, project)
	}
	return project, err
}

func (s *Service) ListProjects(ctx context.Context, includeArchived bool) ([]Project, error) {
	query := `SELECT project_id,slug,name,description,status,color,default_currency,budget_minor,created_at,updated_at FROM projects`
	if !includeArchived {
		query += ` WHERE status<>'archived'`
	}
	query += ` ORDER BY CASE project_id WHEN 'project-inbox' THEN 0 WHEN 'project-personal' THEN 1 ELSE 2 END,name`
	rows, err := s.control.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	projects := []Project{}
	for rows.Next() {
		var project Project
		if err := rows.Scan(&project.ProjectID, &project.Slug, &project.Name, &project.Description, &project.Status, &project.Color, &project.DefaultCurrency, &project.BudgetMinor, &project.CreatedAt, &project.UpdatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	rowErr := rows.Err()
	if err := rows.Close(); err != nil && rowErr == nil {
		rowErr = err
	}
	if rowErr != nil {
		return nil, rowErr
	}
	for i := range projects {
		projects[i].Aliases, err = s.aliases(ctx, projects[i].ProjectID)
		if err != nil {
			return nil, err
		}
	}
	return projects, nil
}

func (s *Service) Project(ctx context.Context, projectID string) (Project, error) {
	var project Project
	err := s.control.DB.QueryRowContext(ctx, `SELECT project_id,slug,name,description,status,color,default_currency,budget_minor,created_at,updated_at FROM projects WHERE project_id=?`, strings.TrimSpace(projectID)).
		Scan(&project.ProjectID, &project.Slug, &project.Name, &project.Description, &project.Status, &project.Color, &project.DefaultCurrency, &project.BudgetMinor, &project.CreatedAt, &project.UpdatedAt)
	if err != nil {
		return Project{}, err
	}
	project.Aliases, err = s.aliases(ctx, project.ProjectID)
	return project, err
}

func (s *Service) aliases(ctx context.Context, projectID string) ([]string, error) {
	rows, err := s.control.DB.QueryContext(ctx, `SELECT alias FROM project_aliases WHERE project_id=? ORDER BY alias COLLATE NOCASE`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return nil, err
		}
		out = append(out, alias)
	}
	return out, rows.Err()
}

// Resolve uses only registry IDs and aliases. Empty/unknown hints fail closed
// to Inbox; no caller-provided value is ever interpreted as a path.
func (s *Service) Resolve(ctx context.Context, value string) (Project, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		project, err := s.Project(ctx, InboxProjectID)
		return project, false, err
	}
	var projectID string
	err := s.control.DB.QueryRowContext(ctx, `SELECT project_id FROM projects WHERE project_id=? OR slug=? COLLATE NOCASE UNION SELECT project_id FROM project_aliases WHERE alias=? COLLATE NOCASE LIMIT 1`, value, value, value).Scan(&projectID)
	if err == sql.ErrNoRows {
		project, fallbackErr := s.Project(ctx, InboxProjectID)
		return project, false, fallbackErr
	}
	if err != nil {
		return Project{}, false, err
	}
	project, err := s.Project(ctx, projectID)
	return project, true, err
}

func (s *Service) LinkRecord(ctx context.Context, recordType, recordID, projectID string, primary bool) error {
	allowed := map[string]bool{"evidence": true, "episode": true, "knowledge_unit": true, "memory": true, "living": true, "asset": true, "decision": true, "goal": true, "risk": true}
	if !allowed[recordType] || strings.TrimSpace(recordID) == "" {
		return errors.New("invalid record link")
	}
	if err := s.projectExists(ctx, projectID); err != nil {
		return err
	}
	tx, err := s.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if primary {
		if _, err := tx.ExecContext(ctx, `UPDATE record_projects SET is_primary=0 WHERE record_type=? AND record_id=?`, recordType, recordID); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO record_projects(record_type,record_id,project_id,relation,is_primary,created_at) VALUES(?,?,?,?,?,?) ON CONFLICT(record_type,record_id,project_id) DO UPDATE SET relation=excluded.relation,is_primary=excluded.is_primary`,
		recordType, recordID, projectID, "member", boolInt(primary), nowString())
	if err != nil {
		return err
	}
	if projectID != InboxProjectID {
		_, _ = tx.ExecContext(ctx, `DELETE FROM record_projects WHERE record_type=? AND record_id=? AND project_id=? AND relation='fallback'`, recordType, recordID, InboxProjectID)
	}
	return tx.Commit()
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (s *Service) ProjectForRecord(ctx context.Context, recordType, recordID string) (string, error) {
	var projectID string
	err := s.control.DB.QueryRowContext(ctx, `SELECT project_id FROM record_projects WHERE record_type=? AND record_id=? ORDER BY is_primary DESC,created_at LIMIT 1`, recordType, recordID).Scan(&projectID)
	if err == sql.ErrNoRows {
		return InboxProjectID, nil
	}
	return projectID, err
}

func (s *Service) ProjectSummary(ctx context.Context, projectID string) (ProjectSummary, error) {
	project, err := s.Project(ctx, projectID)
	if err != nil {
		return ProjectSummary{}, err
	}
	metrics := ProjectMetrics{}
	counts := []struct {
		recordType string
		target     *int
	}{{"evidence", &metrics.Evidence}, {"knowledge_unit", &metrics.KnowledgeUnits}, {"episode", &metrics.Episodes}, {"memory", &metrics.Memories}}
	for _, count := range counts {
		if err := s.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM record_projects WHERE project_id=? AND record_type=?`, projectID, count.recordType).Scan(count.target); err != nil {
			return ProjectSummary{}, err
		}
	}
	if err := s.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM temporal_facts WHERE project_id=? AND status='active'`, projectID).Scan(&metrics.Facts); err != nil {
		return ProjectSummary{}, err
	}
	if err := s.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM memory_records m JOIN record_projects rp ON rp.record_type='memory' AND rp.record_id=m.memory_id WHERE rp.project_id=? AND m.status IN ('active','corroborated')`, projectID).Scan(&metrics.AvailableMemories); err != nil {
		return ProjectSummary{}, err
	}
	if err := s.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM project_goals WHERE project_id=? AND status NOT IN ('completed','cancelled')`, projectID).Scan(&metrics.OpenGoals); err != nil {
		return ProjectSummary{}, err
	}
	if err := s.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM project_risks WHERE project_id=? AND status='open'`, projectID).Scan(&metrics.OpenRisks); err != nil {
		return ProjectSummary{}, err
	}
	if err := s.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM memory_operations o JOIN record_projects rp ON rp.record_type='memory' AND rp.record_id=o.target_memory_id WHERE rp.project_id=? AND o.status='review_required'`, projectID).Scan(&metrics.PendingReview); err != nil {
		return ProjectSummary{}, err
	}
	if err := s.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM project_tasks WHERE project_id=? AND status IN ('todo','in_progress')`, projectID).Scan(&metrics.OpenTasks); err != nil {
		return ProjectSummary{}, err
	}
	if err := s.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM project_tasks WHERE project_id=? AND status='suggested'`, projectID).Scan(&metrics.SuggestedTasks); err != nil {
		return ProjectSummary{}, err
	}
	finance, err := s.FinanceSummary(ctx, projectID)
	return ProjectSummary{Project: project, Metrics: metrics, Finance: finance}, err
}

func (s *Service) CreateFact(ctx context.Context, input FactInput) (TemporalFact, error) {
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.Subject = strings.TrimSpace(input.Subject)
	input.Predicate = strings.TrimSpace(input.Predicate)
	input.Object = strings.TrimSpace(input.Object)
	if input.Subject == "" || input.Predicate == "" || input.Object == "" {
		return TemporalFact{}, errors.New("subject, predicate and object are required")
	}
	if err := s.projectExists(ctx, input.ProjectID); err != nil {
		return TemporalFact{}, err
	}
	now := time.Now().UTC()
	validFrom, err := normalizeTime(input.ValidFrom, now)
	if err != nil {
		return TemporalFact{}, err
	}
	observedAt, err := normalizeTime(input.ObservedAt, now)
	if err != nil {
		return TemporalFact{}, err
	}
	validUntil := ""
	if input.ValidUntil != "" {
		validUntil, err = normalizeTime(input.ValidUntil, now)
		if err != nil {
			return TemporalFact{}, err
		}
		if validUntil <= validFrom {
			return TemporalFact{}, errors.New("valid_until must be after valid_from")
		}
	}
	if input.Confidence == 0 {
		input.Confidence = 1
	}
	if input.Confidence < 0 || input.Confidence > 1 {
		return TemporalFact{}, errors.New("confidence must be between 0 and 1")
	}
	factID := stableID("fact-", input.ProjectID, input.Subject, input.Predicate, input.Object, validFrom)
	recordedAt := nowString()
	tx, err := s.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return TemporalFact{}, err
	}
	defer tx.Rollback()
	if input.SupersedesFactID != "" {
		var priorProject, priorValidFrom, priorStatus string
		err := tx.QueryRowContext(ctx, `SELECT project_id,valid_from,status FROM temporal_facts WHERE fact_id=?`, input.SupersedesFactID).Scan(&priorProject, &priorValidFrom, &priorStatus)
		if err != nil {
			return TemporalFact{}, fmt.Errorf("superseded fact: %w", err)
		}
		if priorProject != input.ProjectID || priorValidFrom >= validFrom || priorStatus != "active" {
			return TemporalFact{}, errors.New("superseded fact must be active, in the same project and older")
		}
		if _, err := tx.ExecContext(ctx, `UPDATE temporal_facts SET status='superseded',valid_until=? WHERE fact_id=?`, validFrom, input.SupersedesFactID); err != nil {
			return TemporalFact{}, err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO temporal_facts(fact_id,project_id,subject,predicate,object,status,observed_at,recorded_at,valid_from,valid_until,supersedes_fact_id,source_memory_id,source_evidence_ids_json,confidence) VALUES(?,?,?,?,?,'active',?,?,?,?,?,?,?,?)`,
		factID, input.ProjectID, input.Subject, input.Predicate, input.Object, observedAt, recordedAt, validFrom, nullable(validUntil), nullable(input.SupersedesFactID), nullable(input.SourceMemoryID), encodeStrings(input.SourceEvidenceIDs), input.Confidence)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return s.Fact(ctx, factID)
		}
		return TemporalFact{}, err
	}
	if err := tx.Commit(); err != nil {
		return TemporalFact{}, err
	}
	if input.SupersedesFactID != "" {
		prior, priorErr := s.Fact(ctx, input.SupersedesFactID)
		if priorErr != nil {
			return TemporalFact{}, priorErr
		}
		if err := s.indexFact(ctx, prior); err != nil {
			return TemporalFact{}, err
		}
	}
	fact, err := s.Fact(ctx, factID)
	if err == nil {
		err = s.indexFact(ctx, fact)
	}
	return fact, err
}

func (s *Service) Fact(ctx context.Context, factID string) (TemporalFact, error) {
	var fact TemporalFact
	var observedAt, validUntil, supersedes, memoryID sql.NullString
	var evidenceJSON string
	err := s.control.DB.QueryRowContext(ctx, `SELECT fact_id,project_id,subject,predicate,object,status,observed_at,recorded_at,valid_from,valid_until,supersedes_fact_id,source_memory_id,source_evidence_ids_json,confidence FROM temporal_facts WHERE fact_id=?`, factID).
		Scan(&fact.FactID, &fact.ProjectID, &fact.Subject, &fact.Predicate, &fact.Object, &fact.Status, &observedAt, &fact.RecordedAt, &fact.ValidFrom, &validUntil, &supersedes, &memoryID, &evidenceJSON, &fact.Confidence)
	if err != nil {
		return TemporalFact{}, err
	}
	fact.ObservedAt = observedAt.String
	fact.ValidUntil = validUntil.String
	fact.SupersedesFactID = supersedes.String
	fact.SourceMemoryID = memoryID.String
	fact.SourceEvidenceIDs = decodeStrings(evidenceJSON)
	return fact, nil
}

func (s *Service) ListFacts(ctx context.Context, projectID, asOf string, includeHistory bool, limit int) ([]TemporalFact, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	where := []string{"project_id=?"}
	args := []any{projectID}
	if asOf != "" {
		bound, err := normalizeTime(asOf, time.Now())
		if err != nil {
			return nil, err
		}
		where = append(where, "valid_from<=?", "(valid_until IS NULL OR valid_until>?)")
		args = append(args, bound, bound)
	} else if !includeHistory {
		where = append(where, "status='active'", "valid_from<=?", "(valid_until IS NULL OR valid_until>?)")
		now := nowString()
		args = append(args, now, now)
	}
	args = append(args, limit)
	rows, err := s.control.DB.QueryContext(ctx, `SELECT fact_id FROM temporal_facts WHERE `+strings.Join(where, " AND ")+` ORDER BY valid_from DESC,recorded_at DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	rowErr := rows.Err()
	if err := rows.Close(); err != nil && rowErr == nil {
		rowErr = err
	}
	if rowErr != nil {
		return nil, rowErr
	}
	items := make([]TemporalFact, 0, len(ids))
	for _, id := range ids {
		item, err := s.Fact(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) UpsertContextBlock(ctx context.Context, input ContextBlockInput) (ContextBlock, error) {
	input.Label = strings.TrimSpace(input.Label)
	input.Content = strings.TrimSpace(input.Content)
	if err := s.projectExists(ctx, input.ProjectID); err != nil {
		return ContextBlock{}, err
	}
	if input.Label == "" || input.Content == "" {
		return ContextBlock{}, errors.New("label and content are required")
	}
	if input.BudgetChars <= 0 {
		input.BudgetChars = 4000
	}
	if input.BudgetChars > 50000 || utf8.RuneCountInString(input.Content) > input.BudgetChars {
		return ContextBlock{}, errors.New("content exceeds context block budget")
	}
	blockID := stableID("block-", input.ProjectID, input.Label)
	now := nowString()
	_, err := s.control.DB.ExecContext(ctx, `INSERT INTO context_blocks(block_id,project_id,label,description,content,budget_chars,read_only,status,source_refs_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,'active',?,?,?) ON CONFLICT(project_id,label) DO UPDATE SET description=excluded.description,content=excluded.content,budget_chars=excluded.budget_chars,read_only=excluded.read_only,source_refs_json=excluded.source_refs_json,updated_at=excluded.updated_at`,
		blockID, input.ProjectID, input.Label, strings.TrimSpace(input.Description), input.Content, input.BudgetChars, boolInt(input.ReadOnly), encodeStrings(input.SourceRefs), now, now)
	if err != nil {
		return ContextBlock{}, err
	}
	return s.contextBlock(ctx, blockID)
}

func (s *Service) contextBlock(ctx context.Context, blockID string) (ContextBlock, error) {
	var block ContextBlock
	var readOnly int
	var sources string
	err := s.control.DB.QueryRowContext(ctx, `SELECT block_id,project_id,label,description,content,budget_chars,read_only,status,source_refs_json,created_at,updated_at FROM context_blocks WHERE block_id=?`, blockID).
		Scan(&block.BlockID, &block.ProjectID, &block.Label, &block.Description, &block.Content, &block.BudgetChars, &readOnly, &block.Status, &sources, &block.CreatedAt, &block.UpdatedAt)
	block.ReadOnly = readOnly == 1
	block.SourceRefs = decodeStrings(sources)
	return block, err
}

func (s *Service) ListContextBlocks(ctx context.Context, projectID string) ([]ContextBlock, error) {
	rows, err := s.control.DB.QueryContext(ctx, `SELECT block_id FROM context_blocks WHERE project_id=? AND status='active' ORDER BY label`, projectID)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	rowErr := rows.Err()
	rows.Close()
	if rowErr != nil {
		return nil, rowErr
	}
	items := make([]ContextBlock, 0, len(ids))
	for _, id := range ids {
		item, err := s.contextBlock(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func sortedCurrencies(values map[string]CurrencySummary) []CurrencySummary {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]CurrencySummary, 0, len(keys))
	for _, key := range keys {
		out = append(out, values[key])
	}
	return out
}
