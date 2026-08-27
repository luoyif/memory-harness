package portfolio

type Project struct {
	ProjectID       string   `json:"project_id"`
	Slug            string   `json:"slug"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Status          string   `json:"status"`
	Color           string   `json:"color"`
	DefaultCurrency string   `json:"default_currency"`
	BudgetMinor     int64    `json:"budget_minor"`
	Aliases         []string `json:"aliases"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

type ProjectInput struct {
	Slug            string   `json:"slug"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Color           string   `json:"color"`
	DefaultCurrency string   `json:"default_currency"`
	BudgetMinor     int64    `json:"budget_minor"`
	Aliases         []string `json:"aliases"`
}

type ProjectMetrics struct {
	Evidence          int `json:"evidence"`
	KnowledgeUnits    int `json:"knowledge_units"`
	Episodes          int `json:"episodes"`
	Memories          int `json:"memories"`
	AvailableMemories int `json:"available_memories"`
	Facts             int `json:"facts"`
	OpenGoals         int `json:"open_goals"`
	OpenRisks         int `json:"open_risks"`
	PendingReview     int `json:"pending_review"`
	OpenTasks         int `json:"open_tasks"`
	SuggestedTasks    int `json:"suggested_tasks"`
}

type ProjectSummary struct {
	Project Project        `json:"project"`
	Metrics ProjectMetrics `json:"metrics"`
	Finance FinanceSummary `json:"finance"`
}

type TemporalFact struct {
	FactID            string   `json:"fact_id"`
	ProjectID         string   `json:"project_id"`
	Subject           string   `json:"subject"`
	Predicate         string   `json:"predicate"`
	Object            string   `json:"object"`
	Status            string   `json:"status"`
	ObservedAt        string   `json:"observed_at,omitempty"`
	RecordedAt        string   `json:"recorded_at"`
	ValidFrom         string   `json:"valid_from"`
	ValidUntil        string   `json:"valid_until,omitempty"`
	SupersedesFactID  string   `json:"supersedes_fact_id,omitempty"`
	SourceMemoryID    string   `json:"source_memory_id,omitempty"`
	SourceEvidenceIDs []string `json:"source_evidence_ids"`
	Confidence        float64  `json:"confidence"`
}

type FactInput struct {
	ProjectID         string   `json:"project_id"`
	Subject           string   `json:"subject"`
	Predicate         string   `json:"predicate"`
	Object            string   `json:"object"`
	ObservedAt        string   `json:"observed_at"`
	ValidFrom         string   `json:"valid_from"`
	ValidUntil        string   `json:"valid_until"`
	SupersedesFactID  string   `json:"supersedes_fact_id"`
	SourceMemoryID    string   `json:"source_memory_id"`
	SourceEvidenceIDs []string `json:"source_evidence_ids"`
	Confidence        float64  `json:"confidence"`
}

type ContextBlock struct {
	BlockID     string   `json:"block_id"`
	ProjectID   string   `json:"project_id"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Content     string   `json:"content"`
	BudgetChars int      `json:"budget_chars"`
	ReadOnly    bool     `json:"read_only"`
	Status      string   `json:"status"`
	SourceRefs  []string `json:"source_refs"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

type ContextBlockInput struct {
	ProjectID   string   `json:"project_id"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Content     string   `json:"content"`
	BudgetChars int      `json:"budget_chars"`
	ReadOnly    bool     `json:"read_only"`
	SourceRefs  []string `json:"source_refs"`
}

type Goal struct {
	GoalID           string `json:"goal_id"`
	ProjectID        string `json:"project_id"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	Status           string `json:"status"`
	Priority         int    `json:"priority"`
	TargetAt         string `json:"target_at,omitempty"`
	SourceEvidenceID string `json:"source_evidence_id,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

type GoalInput struct {
	ProjectID        string `json:"project_id"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	Status           string `json:"status"`
	Priority         int    `json:"priority"`
	TargetAt         string `json:"target_at"`
	SourceEvidenceID string `json:"source_evidence_id"`
}

type Milestone struct {
	MilestoneID string `json:"milestone_id"`
	ProjectID   string `json:"project_id"`
	GoalID      string `json:"goal_id,omitempty"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	DueAt       string `json:"due_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type MilestoneInput struct {
	ProjectID string `json:"project_id"`
	GoalID    string `json:"goal_id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	DueAt     string `json:"due_at"`
}

type ProjectTask struct {
	TaskID            string   `json:"task_id"`
	ProjectID         string   `json:"project_id"`
	Title             string   `json:"title"`
	Description       string   `json:"description"`
	Status            string   `json:"status"`
	Priority          int      `json:"priority"`
	DueAt             string   `json:"due_at,omitempty"`
	SourceKind        string   `json:"source_kind"`
	SourceRecordID    string   `json:"source_record_id,omitempty"`
	SourceEvidenceIDs []string `json:"source_evidence_ids"`
	CompletedAt       string   `json:"completed_at,omitempty"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
}

type ProjectTaskInput struct {
	ProjectID         string   `json:"project_id"`
	Title             string   `json:"title"`
	Description       string   `json:"description"`
	Priority          int      `json:"priority"`
	DueAt             string   `json:"due_at"`
	SourceKind        string   `json:"source_kind"`
	SourceRecordID    string   `json:"source_record_id"`
	SourceEvidenceIDs []string `json:"source_evidence_ids"`
}

type ProjectAutomation struct {
	ProjectID  string `json:"project_id"`
	ImportMode string `json:"import_mode"`
	UpdatedAt  string `json:"updated_at"`
}

type Decision struct {
	DecisionID           string   `json:"decision_id"`
	ProjectID            string   `json:"project_id"`
	Title                string   `json:"title"`
	Decision             string   `json:"decision"`
	Rationale            string   `json:"rationale"`
	Status               string   `json:"status"`
	DecidedAt            string   `json:"decided_at"`
	SupersedesDecisionID string   `json:"supersedes_decision_id,omitempty"`
	SourceEvidenceIDs    []string `json:"source_evidence_ids"`
	CreatedAt            string   `json:"created_at"`
}

type DecisionInput struct {
	ProjectID            string   `json:"project_id"`
	Title                string   `json:"title"`
	Decision             string   `json:"decision"`
	Rationale            string   `json:"rationale"`
	DecidedAt            string   `json:"decided_at"`
	SupersedesDecisionID string   `json:"supersedes_decision_id"`
	SourceEvidenceIDs    []string `json:"source_evidence_ids"`
}

type Risk struct {
	RiskID           string `json:"risk_id"`
	ProjectID        string `json:"project_id"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	Probability      int    `json:"probability"`
	Impact           int    `json:"impact"`
	Score            int    `json:"score"`
	Status           string `json:"status"`
	Mitigation       string `json:"mitigation"`
	Owner            string `json:"owner"`
	SourceEvidenceID string `json:"source_evidence_id,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

type RiskInput struct {
	ProjectID        string `json:"project_id"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	Probability      int    `json:"probability"`
	Impact           int    `json:"impact"`
	Mitigation       string `json:"mitigation"`
	Owner            string `json:"owner"`
	SourceEvidenceID string `json:"source_evidence_id"`
}

type FinanceAccount struct {
	AccountID           string `json:"account_id"`
	ProjectID           string `json:"project_id"`
	Name                string `json:"name"`
	AccountType         string `json:"account_type"`
	Currency            string `json:"currency"`
	OpeningBalanceMinor int64  `json:"opening_balance_minor"`
	Status              string `json:"status"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

type FinanceEntry struct {
	EntryID          string `json:"entry_id"`
	ProjectID        string `json:"project_id"`
	AccountID        string `json:"account_id,omitempty"`
	EntryType        string `json:"entry_type"`
	Category         string `json:"category"`
	Description      string `json:"description"`
	AmountMinor      int64  `json:"amount_minor"`
	Currency         string `json:"currency"`
	OccurredAt       string `json:"occurred_at"`
	Status           string `json:"status"`
	SourceEvidenceID string `json:"source_evidence_id,omitempty"`
	IdempotencyKey   string `json:"idempotency_key,omitempty"`
	CreatedAt        string `json:"created_at"`
}

type FinanceEntryInput struct {
	ProjectID        string `json:"project_id"`
	AccountID        string `json:"account_id"`
	EntryType        string `json:"entry_type"`
	Category         string `json:"category"`
	Description      string `json:"description"`
	AmountMinor      int64  `json:"amount_minor"`
	Currency         string `json:"currency"`
	OccurredAt       string `json:"occurred_at"`
	Status           string `json:"status"`
	SourceEvidenceID string `json:"source_evidence_id"`
	IdempotencyKey   string `json:"idempotency_key"`
}

type CurrencySummary struct {
	Currency       string `json:"currency"`
	IncomeMinor    int64  `json:"income_minor"`
	ExpenseMinor   int64  `json:"expense_minor"`
	NetMinor       int64  `json:"net_minor"`
	BudgetMinor    int64  `json:"budget_minor"`
	RemainingMinor int64  `json:"remaining_minor"`
}

type FinanceSummary struct {
	Currencies []CurrencySummary `json:"currencies"`
}

type Connector struct {
	ConnectorID string `json:"connector_id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	ProjectID   string `json:"project_id"`
	Cursor      string `json:"cursor,omitempty"`
	LastSyncAt  string `json:"last_sync_at,omitempty"`
	LastError   string `json:"last_error,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type ConnectorInput struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	ProjectID string `json:"project_id"`
}

type ImportBatch struct {
	BatchID        string   `json:"batch_id"`
	ConnectorID    string   `json:"connector_id,omitempty"`
	ProjectID      string   `json:"project_id"`
	IdempotencyKey string   `json:"idempotency_key"`
	Status         string   `json:"status"`
	ItemCount      int      `json:"item_count"`
	EvidenceIDs    []string `json:"evidence_ids"`
	Error          string   `json:"error,omitempty"`
	CreatedAt      string   `json:"created_at"`
	CompletedAt    string   `json:"completed_at,omitempty"`
}
