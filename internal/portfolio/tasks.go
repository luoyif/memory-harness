package portfolio

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

func optionalTime(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return "", errors.New("due_at must be RFC3339")
	}
	return parsed.UTC().Format(time.RFC3339Nano), nil
}

func (s *Service) validateTaskEvidence(ctx context.Context, projectID string, ids []string) error {
	for _, evidenceID := range ids {
		evidenceID = strings.TrimSpace(evidenceID)
		if evidenceID == "" {
			continue
		}
		var found int
		if err := s.control.DB.QueryRowContext(ctx, `SELECT 1 FROM record_projects WHERE record_type='evidence' AND record_id=? AND project_id=?`, evidenceID, projectID).Scan(&found); err != nil {
			if err == sql.ErrNoRows {
				return errors.New("task source Evidence must belong to the same project")
			}
			return err
		}
	}
	return nil
}

func (s *Service) CreateProjectTask(ctx context.Context, input ProjectTaskInput) (ProjectTask, error) {
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.SourceKind = strings.TrimSpace(input.SourceKind)
	input.SourceRecordID = strings.TrimSpace(input.SourceRecordID)
	if input.SourceKind == "" {
		input.SourceKind = "manual"
	}
	if input.Title == "" || utf8.RuneCountInString(input.Title) > 160 {
		return ProjectTask{}, errors.New("task title must contain 1 to 160 characters")
	}
	if input.Priority == 0 {
		input.Priority = 3
	}
	if input.Priority < 1 || input.Priority > 5 {
		return ProjectTask{}, errors.New("task priority must be between 1 and 5")
	}
	if input.SourceKind != "manual" && input.SourceKind != "ai_suggestion" {
		return ProjectTask{}, errors.New("source_kind must be manual or ai_suggestion")
	}
	if input.SourceKind == "ai_suggestion" && input.SourceRecordID == "" {
		return ProjectTask{}, errors.New("AI suggestions require source_record_id")
	}
	if err := s.projectExists(ctx, input.ProjectID); err != nil {
		return ProjectTask{}, err
	}
	if err := s.validateTaskEvidence(ctx, input.ProjectID, input.SourceEvidenceIDs); err != nil {
		return ProjectTask{}, err
	}
	dueAt, err := optionalTime(input.DueAt)
	if err != nil {
		return ProjectTask{}, err
	}
	now := nowString()
	status := "todo"
	taskID := stableID("task-", input.ProjectID, input.Title, now)
	if input.SourceKind == "ai_suggestion" {
		status = "suggested"
		taskID = stableID("suggested-task-", input.ProjectID, input.SourceRecordID)
	}
	_, err = s.control.DB.ExecContext(ctx, `INSERT INTO project_tasks(task_id,project_id,title,description,status,priority,due_at,source_kind,source_record_id,source_evidence_ids_json,created_at,updated_at)
		VALUES(?,?,?,?,?,?,nullif(?,''),?,nullif(?,''),?,?,?)
		ON CONFLICT(project_id,source_kind,source_record_id) DO UPDATE SET title=excluded.title,description=excluded.description,priority=excluded.priority,due_at=excluded.due_at,source_evidence_ids_json=excluded.source_evidence_ids_json,updated_at=excluded.updated_at`,
		taskID, input.ProjectID, input.Title, input.Description, status, input.Priority, dueAt, input.SourceKind, input.SourceRecordID, encodeStrings(input.SourceEvidenceIDs), now, now)
	if err != nil {
		return ProjectTask{}, err
	}
	if input.SourceKind == "ai_suggestion" {
		err = s.control.DB.QueryRowContext(ctx, `SELECT task_id FROM project_tasks WHERE project_id=? AND source_kind=? AND source_record_id=?`, input.ProjectID, input.SourceKind, input.SourceRecordID).Scan(&taskID)
		if err != nil {
			return ProjectTask{}, err
		}
	}
	return s.ProjectTask(ctx, taskID)
}

func (s *Service) ProjectTask(ctx context.Context, taskID string) (ProjectTask, error) {
	var item ProjectTask
	var dueAt, sourceRecordID, completedAt sql.NullString
	var evidenceRaw string
	err := s.control.DB.QueryRowContext(ctx, `SELECT task_id,project_id,title,description,status,priority,due_at,source_kind,source_record_id,source_evidence_ids_json,completed_at,created_at,updated_at FROM project_tasks WHERE task_id=?`, strings.TrimSpace(taskID)).
		Scan(&item.TaskID, &item.ProjectID, &item.Title, &item.Description, &item.Status, &item.Priority, &dueAt, &item.SourceKind, &sourceRecordID, &evidenceRaw, &completedAt, &item.CreatedAt, &item.UpdatedAt)
	item.DueAt = dueAt.String
	item.SourceRecordID = sourceRecordID.String
	item.SourceEvidenceIDs = decodeStrings(evidenceRaw)
	item.CompletedAt = completedAt.String
	return item, err
}

func (s *Service) ListProjectTasks(ctx context.Context, projectID, status string) ([]ProjectTask, error) {
	projectID = strings.TrimSpace(projectID)
	status = strings.TrimSpace(status)
	if err := s.projectExists(ctx, projectID); err != nil {
		return nil, err
	}
	query := `SELECT task_id FROM project_tasks WHERE project_id=?`
	args := []any{projectID}
	if status != "" {
		query += ` AND status=?`
		args = append(args, status)
	}
	query += ` ORDER BY CASE status WHEN 'in_progress' THEN 0 WHEN 'todo' THEN 1 WHEN 'suggested' THEN 2 WHEN 'done' THEN 3 ELSE 4 END,CASE WHEN due_at IS NULL THEN 1 ELSE 0 END,due_at,priority,updated_at DESC`
	rows, err := s.control.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	items := make([]ProjectTask, 0, len(ids))
	for _, id := range ids {
		item, err := s.ProjectTask(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func validTaskTransition(from, to string) bool {
	allowed := map[string]map[string]bool{
		"suggested":   {"todo": true, "dismissed": true},
		"todo":        {"in_progress": true, "done": true, "dismissed": true},
		"in_progress": {"todo": true, "done": true},
		"done":        {"todo": true},
		"dismissed":   {"todo": true},
	}
	return allowed[from][to]
}

func (s *Service) UpdateProjectTaskStatus(ctx context.Context, taskID, status string) (ProjectTask, error) {
	item, err := s.ProjectTask(ctx, taskID)
	if err != nil {
		return ProjectTask{}, err
	}
	status = strings.TrimSpace(status)
	if status == item.Status {
		return item, nil
	}
	if !validTaskTransition(item.Status, status) {
		return ProjectTask{}, errors.New("invalid project task status transition")
	}
	completedAt := any(nil)
	if status == "done" {
		completedAt = nowString()
	}
	_, err = s.control.DB.ExecContext(ctx, `UPDATE project_tasks SET status=?,completed_at=?,updated_at=? WHERE task_id=?`, status, completedAt, nowString(), item.TaskID)
	if err != nil {
		return ProjectTask{}, err
	}
	return s.ProjectTask(ctx, item.TaskID)
}

func (s *Service) ProjectAutomation(ctx context.Context, projectID string) (ProjectAutomation, error) {
	projectID = strings.TrimSpace(projectID)
	if err := s.projectExists(ctx, projectID); err != nil {
		return ProjectAutomation{}, err
	}
	item := ProjectAutomation{ProjectID: projectID, ImportMode: "auto_new"}
	err := s.control.DB.QueryRowContext(ctx, `SELECT import_mode,updated_at FROM project_automation WHERE project_id=?`, projectID).Scan(&item.ImportMode, &item.UpdatedAt)
	if err == sql.ErrNoRows {
		return item, nil
	}
	return item, err
}

func (s *Service) SetProjectAutomation(ctx context.Context, projectID, importMode string) (ProjectAutomation, error) {
	projectID = strings.TrimSpace(projectID)
	importMode = strings.TrimSpace(importMode)
	if importMode != "auto_new" && importMode != "manual" {
		return ProjectAutomation{}, errors.New("import_mode must be auto_new or manual")
	}
	if err := s.projectExists(ctx, projectID); err != nil {
		return ProjectAutomation{}, err
	}
	now := nowString()
	_, err := s.control.DB.ExecContext(ctx, `INSERT INTO project_automation(project_id,import_mode,updated_at) VALUES(?,?,?) ON CONFLICT(project_id) DO UPDATE SET import_mode=excluded.import_mode,updated_at=excluded.updated_at`, projectID, importMode, now)
	if err != nil {
		return ProjectAutomation{}, err
	}
	return ProjectAutomation{ProjectID: projectID, ImportMode: importMode, UpdatedAt: now}, nil
}
