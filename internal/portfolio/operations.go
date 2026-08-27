package portfolio

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

func inSet(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func (s *Service) CreateGoal(ctx context.Context, input GoalInput) (Goal, error) {
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return Goal{}, errors.New("goal title is required")
	}
	if err := s.projectExists(ctx, input.ProjectID); err != nil {
		return Goal{}, err
	}
	if input.Status == "" {
		input.Status = "active"
	}
	if !inSet(input.Status, "active", "paused", "completed", "cancelled") {
		return Goal{}, errors.New("invalid goal status")
	}
	targetAt := ""
	var err error
	if input.TargetAt != "" {
		targetAt, err = normalizeTime(input.TargetAt, time.Now())
		if err != nil {
			return Goal{}, err
		}
	}
	now := nowString()
	goalID := stableID("goal-", input.ProjectID, input.Title, now)
	_, err = s.control.DB.ExecContext(ctx, `INSERT INTO project_goals(goal_id,project_id,title,description,status,priority,target_at,source_evidence_id,created_at,updated_at) VALUES(?,?,?,?,?,?,nullif(?,''),nullif(?,''),?,?)`,
		goalID, input.ProjectID, input.Title, strings.TrimSpace(input.Description), input.Status, input.Priority, targetAt, input.SourceEvidenceID, now, now)
	if err != nil {
		return Goal{}, err
	}
	goal, err := s.Goal(ctx, goalID)
	if err == nil {
		err = s.indexGoal(ctx, goal)
	}
	return goal, err
}

func (s *Service) Goal(ctx context.Context, goalID string) (Goal, error) {
	var goal Goal
	var targetAt, evidenceID sql.NullString
	err := s.control.DB.QueryRowContext(ctx, `SELECT goal_id,project_id,title,description,status,priority,target_at,source_evidence_id,created_at,updated_at FROM project_goals WHERE goal_id=?`, goalID).
		Scan(&goal.GoalID, &goal.ProjectID, &goal.Title, &goal.Description, &goal.Status, &goal.Priority, &targetAt, &evidenceID, &goal.CreatedAt, &goal.UpdatedAt)
	goal.TargetAt = targetAt.String
	goal.SourceEvidenceID = evidenceID.String
	return goal, err
}

func (s *Service) ListGoals(ctx context.Context, projectID string) ([]Goal, error) {
	rows, err := s.control.DB.QueryContext(ctx, `SELECT goal_id FROM project_goals WHERE project_id=? ORDER BY CASE status WHEN 'active' THEN 0 WHEN 'paused' THEN 1 ELSE 2 END,priority DESC,updated_at DESC`, projectID)
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
	items := make([]Goal, 0, len(ids))
	for _, id := range ids {
		item, err := s.Goal(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) UpdateGoalStatus(ctx context.Context, goalID, status string) (Goal, error) {
	status = strings.TrimSpace(status)
	if !inSet(status, "active", "paused", "completed", "cancelled") {
		return Goal{}, errors.New("invalid goal status")
	}
	result, err := s.control.DB.ExecContext(ctx, `UPDATE project_goals SET status=?,updated_at=? WHERE goal_id=?`, status, nowString(), strings.TrimSpace(goalID))
	if err != nil {
		return Goal{}, err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return Goal{}, sql.ErrNoRows
	}
	goal, err := s.Goal(ctx, goalID)
	if err == nil {
		err = s.indexGoal(ctx, goal)
	}
	return goal, err
}

func (s *Service) CreateMilestone(ctx context.Context, input MilestoneInput) (Milestone, error) {
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return Milestone{}, errors.New("milestone title is required")
	}
	if err := s.projectExists(ctx, input.ProjectID); err != nil {
		return Milestone{}, err
	}
	if input.GoalID != "" {
		goal, err := s.Goal(ctx, input.GoalID)
		if err != nil || goal.ProjectID != input.ProjectID {
			return Milestone{}, errors.New("goal must belong to the same project")
		}
	}
	if input.Status == "" {
		input.Status = "planned"
	}
	if !inSet(input.Status, "planned", "in_progress", "completed", "cancelled") {
		return Milestone{}, errors.New("invalid milestone status")
	}
	dueAt := ""
	var err error
	if input.DueAt != "" {
		dueAt, err = normalizeTime(input.DueAt, time.Now())
		if err != nil {
			return Milestone{}, err
		}
	}
	now := nowString()
	id := stableID("milestone-", input.ProjectID, input.Title, now)
	_, err = s.control.DB.ExecContext(ctx, `INSERT INTO project_milestones(milestone_id,project_id,goal_id,title,status,due_at,created_at,updated_at) VALUES(?,?,nullif(?,''),?,?,nullif(?,''),?,?)`, id, input.ProjectID, input.GoalID, input.Title, input.Status, dueAt, now, now)
	if err != nil {
		return Milestone{}, err
	}
	return s.milestone(ctx, id)
}

func (s *Service) milestone(ctx context.Context, id string) (Milestone, error) {
	var item Milestone
	var goalID, dueAt, completedAt sql.NullString
	err := s.control.DB.QueryRowContext(ctx, `SELECT milestone_id,project_id,goal_id,title,status,due_at,completed_at,created_at,updated_at FROM project_milestones WHERE milestone_id=?`, id).
		Scan(&item.MilestoneID, &item.ProjectID, &goalID, &item.Title, &item.Status, &dueAt, &completedAt, &item.CreatedAt, &item.UpdatedAt)
	item.GoalID = goalID.String
	item.DueAt = dueAt.String
	item.CompletedAt = completedAt.String
	return item, err
}

func (s *Service) ListMilestones(ctx context.Context, projectID string) ([]Milestone, error) {
	rows, err := s.control.DB.QueryContext(ctx, `SELECT milestone_id FROM project_milestones WHERE project_id=? ORDER BY CASE status WHEN 'in_progress' THEN 0 WHEN 'planned' THEN 1 ELSE 2 END,due_at`, projectID)
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
	items := make([]Milestone, 0, len(ids))
	for _, id := range ids {
		item, err := s.milestone(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) UpdateMilestoneStatus(ctx context.Context, milestoneID, status string) (Milestone, error) {
	status = strings.TrimSpace(status)
	if !inSet(status, "planned", "in_progress", "completed", "cancelled") {
		return Milestone{}, errors.New("invalid milestone status")
	}
	now := nowString()
	completedAt := ""
	if status == "completed" {
		completedAt = now
	}
	result, err := s.control.DB.ExecContext(ctx, `UPDATE project_milestones SET status=?,completed_at=nullif(?,''),updated_at=? WHERE milestone_id=?`, status, completedAt, now, strings.TrimSpace(milestoneID))
	if err != nil {
		return Milestone{}, err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return Milestone{}, sql.ErrNoRows
	}
	return s.milestone(ctx, milestoneID)
}

func (s *Service) CreateDecision(ctx context.Context, input DecisionInput) (Decision, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Decision = strings.TrimSpace(input.Decision)
	if input.Title == "" || input.Decision == "" {
		return Decision{}, errors.New("decision title and content are required")
	}
	if err := s.projectExists(ctx, input.ProjectID); err != nil {
		return Decision{}, err
	}
	decidedAt, err := normalizeTime(input.DecidedAt, time.Now())
	if err != nil {
		return Decision{}, err
	}
	id := stableID("decision-", input.ProjectID, input.Title, decidedAt)
	now := nowString()
	tx, err := s.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return Decision{}, err
	}
	defer tx.Rollback()
	if input.SupersedesDecisionID != "" {
		var projectID, status string
		if err := tx.QueryRowContext(ctx, `SELECT project_id,status FROM project_decisions WHERE decision_id=?`, input.SupersedesDecisionID).Scan(&projectID, &status); err != nil {
			return Decision{}, err
		}
		if projectID != input.ProjectID || status != "active" {
			return Decision{}, errors.New("superseded decision must be active in the same project")
		}
		if _, err := tx.ExecContext(ctx, `UPDATE project_decisions SET status='superseded' WHERE decision_id=?`, input.SupersedesDecisionID); err != nil {
			return Decision{}, err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO project_decisions(decision_id,project_id,title,decision,rationale,status,decided_at,supersedes_decision_id,source_evidence_ids_json,created_at) VALUES(?,?,?,?,?,'active',?,nullif(?,''),?,?)`,
		id, input.ProjectID, input.Title, input.Decision, strings.TrimSpace(input.Rationale), decidedAt, input.SupersedesDecisionID, encodeStrings(input.SourceEvidenceIDs), now)
	if err != nil {
		return Decision{}, err
	}
	if err := tx.Commit(); err != nil {
		return Decision{}, err
	}
	if input.SupersedesDecisionID != "" {
		prior, priorErr := s.decision(ctx, input.SupersedesDecisionID)
		if priorErr != nil {
			return Decision{}, priorErr
		}
		if err := s.indexDecision(ctx, prior); err != nil {
			return Decision{}, err
		}
	}
	decision, err := s.decision(ctx, id)
	if err == nil {
		err = s.indexDecision(ctx, decision)
	}
	return decision, err
}

func (s *Service) decision(ctx context.Context, id string) (Decision, error) {
	var item Decision
	var supersedes sql.NullString
	var sources string
	err := s.control.DB.QueryRowContext(ctx, `SELECT decision_id,project_id,title,decision,rationale,status,decided_at,supersedes_decision_id,source_evidence_ids_json,created_at FROM project_decisions WHERE decision_id=?`, id).
		Scan(&item.DecisionID, &item.ProjectID, &item.Title, &item.Decision, &item.Rationale, &item.Status, &item.DecidedAt, &supersedes, &sources, &item.CreatedAt)
	item.SupersedesDecisionID = supersedes.String
	item.SourceEvidenceIDs = decodeStrings(sources)
	return item, err
}

func (s *Service) ListDecisions(ctx context.Context, projectID string) ([]Decision, error) {
	rows, err := s.control.DB.QueryContext(ctx, `SELECT decision_id FROM project_decisions WHERE project_id=? ORDER BY decided_at DESC`, projectID)
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
	items := make([]Decision, 0, len(ids))
	for _, id := range ids {
		item, err := s.decision(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) CreateRisk(ctx context.Context, input RiskInput) (Risk, error) {
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" || input.Probability < 1 || input.Probability > 5 || input.Impact < 1 || input.Impact > 5 {
		return Risk{}, errors.New("risk title and probability/impact from 1 to 5 are required")
	}
	if err := s.projectExists(ctx, input.ProjectID); err != nil {
		return Risk{}, err
	}
	now := nowString()
	id := stableID("risk-", input.ProjectID, input.Title, now)
	_, err := s.control.DB.ExecContext(ctx, `INSERT INTO project_risks(risk_id,project_id,title,description,probability,impact,status,mitigation,owner,source_evidence_id,created_at,updated_at) VALUES(?,?,?,?,?,?,'open',?,?,nullif(?,''),?,?)`,
		id, input.ProjectID, input.Title, strings.TrimSpace(input.Description), input.Probability, input.Impact, strings.TrimSpace(input.Mitigation), strings.TrimSpace(input.Owner), input.SourceEvidenceID, now, now)
	if err != nil {
		return Risk{}, err
	}
	risk, err := s.risk(ctx, id)
	if err == nil {
		err = s.indexRisk(ctx, risk)
	}
	return risk, err
}

func (s *Service) risk(ctx context.Context, id string) (Risk, error) {
	var item Risk
	var evidence sql.NullString
	err := s.control.DB.QueryRowContext(ctx, `SELECT risk_id,project_id,title,description,probability,impact,status,mitigation,owner,source_evidence_id,created_at,updated_at FROM project_risks WHERE risk_id=?`, id).
		Scan(&item.RiskID, &item.ProjectID, &item.Title, &item.Description, &item.Probability, &item.Impact, &item.Status, &item.Mitigation, &item.Owner, &evidence, &item.CreatedAt, &item.UpdatedAt)
	item.SourceEvidenceID = evidence.String
	item.Score = item.Probability * item.Impact
	return item, err
}

func (s *Service) ListRisks(ctx context.Context, projectID string) ([]Risk, error) {
	rows, err := s.control.DB.QueryContext(ctx, `SELECT risk_id FROM project_risks WHERE project_id=? ORDER BY CASE status WHEN 'open' THEN 0 ELSE 1 END,impact*probability DESC,updated_at DESC`, projectID)
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
	items := make([]Risk, 0, len(ids))
	for _, id := range ids {
		item, err := s.risk(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) UpdateRiskStatus(ctx context.Context, riskID, status string) (Risk, error) {
	status = strings.TrimSpace(status)
	if !inSet(status, "open", "monitoring", "mitigated", "accepted", "closed") {
		return Risk{}, errors.New("invalid risk status")
	}
	result, err := s.control.DB.ExecContext(ctx, `UPDATE project_risks SET status=?,updated_at=? WHERE risk_id=?`, status, nowString(), strings.TrimSpace(riskID))
	if err != nil {
		return Risk{}, err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return Risk{}, sql.ErrNoRows
	}
	risk, err := s.risk(ctx, riskID)
	if err == nil {
		err = s.indexRisk(ctx, risk)
	}
	return risk, err
}

func (s *Service) CreateFinanceAccount(ctx context.Context, account FinanceAccount) (FinanceAccount, error) {
	account.Name = strings.TrimSpace(account.Name)
	account.Currency = strings.ToUpper(strings.TrimSpace(account.Currency))
	if account.Name == "" || !currencyPattern.MatchString(account.Currency) {
		return FinanceAccount{}, errors.New("account name and three-letter currency are required")
	}
	if err := s.projectExists(ctx, account.ProjectID); err != nil {
		return FinanceAccount{}, err
	}
	if account.AccountType == "" {
		account.AccountType = "cash"
	}
	if !inSet(account.AccountType, "cash", "bank", "receivable", "payable", "virtual") {
		return FinanceAccount{}, errors.New("invalid account type")
	}
	now := nowString()
	account.AccountID = stableID("account-", account.ProjectID, account.Name, account.Currency)
	account.Status = "active"
	account.CreatedAt, account.UpdatedAt = now, now
	_, err := s.control.DB.ExecContext(ctx, `INSERT INTO finance_accounts(account_id,project_id,name,account_type,currency,opening_balance_minor,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		account.AccountID, account.ProjectID, account.Name, account.AccountType, account.Currency, account.OpeningBalanceMinor, account.Status, now, now)
	return account, err
}

func (s *Service) ListFinanceAccounts(ctx context.Context, projectID string) ([]FinanceAccount, error) {
	rows, err := s.control.DB.QueryContext(ctx, `SELECT account_id,project_id,name,account_type,currency,opening_balance_minor,status,created_at,updated_at FROM finance_accounts WHERE project_id=? ORDER BY name`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []FinanceAccount{}
	for rows.Next() {
		var item FinanceAccount
		if err := rows.Scan(&item.AccountID, &item.ProjectID, &item.Name, &item.AccountType, &item.Currency, &item.OpeningBalanceMinor, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) CreateFinanceEntry(ctx context.Context, input FinanceEntryInput) (FinanceEntry, bool, error) {
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	input.Description = strings.TrimSpace(input.Description)
	if input.AmountMinor == 0 || input.Description == "" || !currencyPattern.MatchString(input.Currency) {
		return FinanceEntry{}, false, errors.New("non-zero amount, description and three-letter currency are required")
	}
	if !inSet(input.EntryType, "income", "expense", "transfer", "adjustment") {
		return FinanceEntry{}, false, errors.New("invalid entry type")
	}
	if input.Status == "" {
		input.Status = "posted"
	}
	if !inSet(input.Status, "planned", "posted", "void") {
		return FinanceEntry{}, false, errors.New("invalid finance status")
	}
	if err := s.projectExists(ctx, input.ProjectID); err != nil {
		return FinanceEntry{}, false, err
	}
	if input.AccountID != "" {
		var projectID, currency string
		err := s.control.DB.QueryRowContext(ctx, `SELECT project_id,currency FROM finance_accounts WHERE account_id=?`, input.AccountID).Scan(&projectID, &currency)
		if err != nil || projectID != input.ProjectID || currency != input.Currency {
			return FinanceEntry{}, false, errors.New("account must belong to the same project and currency")
		}
	}
	if input.IdempotencyKey != "" {
		var id string
		err := s.control.DB.QueryRowContext(ctx, `SELECT entry_id FROM finance_entries WHERE idempotency_key=?`, input.IdempotencyKey).Scan(&id)
		if err == nil {
			entry, getErr := s.financeEntry(ctx, id)
			return entry, true, getErr
		}
		if err != sql.ErrNoRows {
			return FinanceEntry{}, false, err
		}
	}
	occurredAt, err := normalizeTime(input.OccurredAt, time.Now())
	if err != nil {
		return FinanceEntry{}, false, err
	}
	now := nowString()
	id := stableID("entry-", input.ProjectID, input.IdempotencyKey, input.Description, fmt.Sprint(input.AmountMinor), occurredAt, now)
	_, err = s.control.DB.ExecContext(ctx, `INSERT INTO finance_entries(entry_id,project_id,account_id,entry_type,category,description,amount_minor,currency,occurred_at,status,source_evidence_id,idempotency_key,created_at) VALUES(?,?,nullif(?,''),?,?,?,?,?,?,?,nullif(?,''),nullif(?,''),?)`,
		id, input.ProjectID, input.AccountID, input.EntryType, strings.TrimSpace(input.Category), input.Description, input.AmountMinor, input.Currency, occurredAt, input.Status, input.SourceEvidenceID, input.IdempotencyKey, now)
	if err != nil {
		return FinanceEntry{}, false, err
	}
	entry, err := s.financeEntry(ctx, id)
	if err == nil {
		err = s.indexFinanceEntry(ctx, entry)
	}
	return entry, false, err
}

func (s *Service) financeEntry(ctx context.Context, id string) (FinanceEntry, error) {
	var item FinanceEntry
	var accountID, evidenceID, idempotencyKey sql.NullString
	err := s.control.DB.QueryRowContext(ctx, `SELECT entry_id,project_id,account_id,entry_type,category,description,amount_minor,currency,occurred_at,status,source_evidence_id,idempotency_key,created_at FROM finance_entries WHERE entry_id=?`, id).
		Scan(&item.EntryID, &item.ProjectID, &accountID, &item.EntryType, &item.Category, &item.Description, &item.AmountMinor, &item.Currency, &item.OccurredAt, &item.Status, &evidenceID, &idempotencyKey, &item.CreatedAt)
	item.AccountID = accountID.String
	item.SourceEvidenceID = evidenceID.String
	item.IdempotencyKey = idempotencyKey.String
	return item, err
}

func (s *Service) ListFinanceEntries(ctx context.Context, projectID string, limit int) ([]FinanceEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.control.DB.QueryContext(ctx, `SELECT entry_id FROM finance_entries WHERE project_id=? ORDER BY occurred_at DESC,created_at DESC LIMIT ?`, projectID, limit)
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
	items := make([]FinanceEntry, 0, len(ids))
	for _, id := range ids {
		item, err := s.financeEntry(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) VoidFinanceEntry(ctx context.Context, entryID string) (FinanceEntry, error) {
	entryID = strings.TrimSpace(entryID)
	result, err := s.control.DB.ExecContext(ctx, `UPDATE finance_entries SET status='void' WHERE entry_id=? AND status!='void'`, entryID)
	if err != nil {
		return FinanceEntry{}, err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		entry, getErr := s.financeEntry(ctx, entryID)
		if getErr != nil {
			return FinanceEntry{}, getErr
		}
		if entry.Status == "void" {
			return entry, nil
		}
		return FinanceEntry{}, sql.ErrNoRows
	}
	entry, err := s.financeEntry(ctx, entryID)
	if err == nil {
		err = s.indexFinanceEntry(ctx, entry)
	}
	return entry, err
}

func (s *Service) FinanceSummary(ctx context.Context, projectID string) (FinanceSummary, error) {
	project, err := s.Project(ctx, projectID)
	if err != nil {
		return FinanceSummary{}, err
	}
	values := map[string]CurrencySummary{}
	values[project.DefaultCurrency] = CurrencySummary{Currency: project.DefaultCurrency, BudgetMinor: project.BudgetMinor, RemainingMinor: project.BudgetMinor}
	rows, err := s.control.DB.QueryContext(ctx, `SELECT currency,entry_type,coalesce(sum(amount_minor),0) FROM finance_entries WHERE project_id=? AND status='posted' GROUP BY currency,entry_type`, projectID)
	if err != nil {
		return FinanceSummary{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var currency, entryType string
		var amount int64
		if err := rows.Scan(&currency, &entryType, &amount); err != nil {
			return FinanceSummary{}, err
		}
		item := values[currency]
		item.Currency = currency
		if entryType == "income" {
			item.IncomeMinor += amount
		} else if entryType == "expense" {
			if amount < 0 {
				item.ExpenseMinor += -amount
			} else {
				item.ExpenseMinor += amount
			}
		} else {
			item.NetMinor += amount
		}
		values[currency] = item
	}
	for key, item := range values {
		item.NetMinor += item.IncomeMinor - item.ExpenseMinor
		item.RemainingMinor = item.BudgetMinor - item.ExpenseMinor
		values[key] = item
	}
	return FinanceSummary{Currencies: sortedCurrencies(values)}, rows.Err()
}
