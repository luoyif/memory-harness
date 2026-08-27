package agentauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/store"
)

type Service struct{ control *store.ControlStore }

func New(control *store.ControlStore) *Service { return &Service{control: control} }

func nowString() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func newToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := "mos_" + base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(digest[:]), nil
}

func tokenHash(token string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(digest[:])
}

func stableID(prefix string, values ...string) string {
	h := sha256.New()
	for _, value := range values {
		_, _ = h.Write([]byte(value))
		_, _ = h.Write([]byte{0})
	}
	return prefix + hex.EncodeToString(h.Sum(nil)[:12])
}

func normalizePermissions(values []string) ([]string, error) {
	allowed := map[string]bool{}
	for _, value := range AllowedPermissions {
		allowed[value] = true
	}
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		if !allowed[value] {
			return nil, fmt.Errorf("unsupported agent permission %q", value)
		}
		seen[value] = true
		out = append(out, value)
	}
	if len(out) == 0 {
		out = []string{PermissionMemoryRead, PermissionProjectRead}
	}
	sort.Strings(out)
	return out, nil
}

func normalizeProjects(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func (s *Service) validateProjects(ctx context.Context, ids []string) error {
	for _, id := range ids {
		var n int
		if err := s.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM projects WHERE project_id=?`, id).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			return fmt.Errorf("unknown project_id %q", id)
		}
	}
	return nil
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Credential, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Kind = strings.ToLower(strings.TrimSpace(input.Kind))
	if input.Name == "" || input.Kind == "" {
		return Credential{}, errors.New("agent name and kind are required")
	}
	permissions, err := normalizePermissions(input.Permissions)
	if err != nil {
		return Credential{}, err
	}
	projects := normalizeProjects(input.ProjectIDs)
	if !input.AllProjects && len(projects) == 0 {
		return Credential{}, errors.New("at least one project grant is required unless all_projects is true")
	}
	if err := s.validateProjects(ctx, projects); err != nil {
		return Credential{}, err
	}
	token, hash, err := newToken()
	if err != nil {
		return Credential{}, err
	}
	now := nowString()
	agentID := stableID("agent-", input.Name, input.Kind, hash)
	permissionsRaw, _ := json.Marshal(permissions)
	tx, err := s.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return Credential{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_principals(agent_id,name,kind,status,token_hash,permissions_json,all_projects,created_at,updated_at) VALUES(?,?,?,'active',?,?,?,?,?)`, agentID, input.Name, input.Kind, hash, string(permissionsRaw), input.AllProjects, now, now); err != nil {
		return Credential{}, err
	}
	for _, projectID := range projects {
		if _, err := tx.ExecContext(ctx, `INSERT INTO agent_project_grants(agent_id,project_id,created_at) VALUES(?,?,?)`, agentID, projectID, now); err != nil {
			return Credential{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Credential{}, err
	}
	principal, err := s.Get(ctx, agentID)
	return Credential{Agent: principal, Token: token}, err
}

func (s *Service) Get(ctx context.Context, agentID string) (Principal, error) {
	var item Principal
	var permissionsRaw string
	var allProjects int
	var lastUsed sql.NullString
	err := s.control.DB.QueryRowContext(ctx, `SELECT agent_id,name,kind,status,permissions_json,all_projects,created_at,updated_at,last_used_at FROM agent_principals WHERE agent_id=?`, strings.TrimSpace(agentID)).
		Scan(&item.AgentID, &item.Name, &item.Kind, &item.Status, &permissionsRaw, &allProjects, &item.CreatedAt, &item.UpdatedAt, &lastUsed)
	if err != nil {
		return Principal{}, err
	}
	_ = json.Unmarshal([]byte(permissionsRaw), &item.Permissions)
	item.AllProjects = allProjects == 1
	item.LastUsedAt = lastUsed.String
	rows, err := s.control.DB.QueryContext(ctx, `SELECT project_id FROM agent_project_grants WHERE agent_id=? ORDER BY project_id`, item.AgentID)
	if err != nil {
		return Principal{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var projectID string
		if err := rows.Scan(&projectID); err != nil {
			return Principal{}, err
		}
		item.ProjectIDs = append(item.ProjectIDs, projectID)
	}
	return item, rows.Err()
}

func (s *Service) List(ctx context.Context) ([]Principal, error) {
	rows, err := s.control.DB.QueryContext(ctx, `SELECT agent_id FROM agent_principals ORDER BY name`)
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
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	out := make([]Principal, 0, len(ids))
	for _, id := range ids {
		item, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Service) Update(ctx context.Context, agentID string, input UpdateInput) (Principal, error) {
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status != "active" && status != "disabled" {
		return Principal{}, errors.New("status must be active or disabled")
	}
	permissions, err := normalizePermissions(input.Permissions)
	if err != nil {
		return Principal{}, err
	}
	projects := normalizeProjects(input.ProjectIDs)
	if !input.AllProjects && len(projects) == 0 {
		return Principal{}, errors.New("at least one project grant is required unless all_projects is true")
	}
	if err := s.validateProjects(ctx, projects); err != nil {
		return Principal{}, err
	}
	permissionsRaw, _ := json.Marshal(permissions)
	tx, err := s.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return Principal{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE agent_principals SET status=?,permissions_json=?,all_projects=?,updated_at=? WHERE agent_id=?`, status, string(permissionsRaw), input.AllProjects, nowString(), agentID)
	if err != nil {
		return Principal{}, err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return Principal{}, sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM agent_project_grants WHERE agent_id=?`, agentID); err != nil {
		return Principal{}, err
	}
	for _, projectID := range projects {
		if _, err := tx.ExecContext(ctx, `INSERT INTO agent_project_grants(agent_id,project_id,created_at) VALUES(?,?,?)`, agentID, projectID, nowString()); err != nil {
			return Principal{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Principal{}, err
	}
	return s.Get(ctx, agentID)
}

func (s *Service) RotateToken(ctx context.Context, agentID string) (Credential, error) {
	token, hash, err := newToken()
	if err != nil {
		return Credential{}, err
	}
	result, err := s.control.DB.ExecContext(ctx, `UPDATE agent_principals SET token_hash=?,updated_at=? WHERE agent_id=?`, hash, nowString(), strings.TrimSpace(agentID))
	if err != nil {
		return Credential{}, err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return Credential{}, sql.ErrNoRows
	}
	principal, err := s.Get(ctx, agentID)
	return Credential{Agent: principal, Token: token}, err
}

func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	if !strings.HasPrefix(strings.TrimSpace(token), "mos_") {
		return Principal{}, errors.New("invalid agent token")
	}
	var agentID, status string
	if err := s.control.DB.QueryRowContext(ctx, `SELECT agent_id,status FROM agent_principals WHERE token_hash=?`, tokenHash(token)).Scan(&agentID, &status); err != nil {
		return Principal{}, errors.New("invalid agent token")
	}
	if status != "active" {
		return Principal{}, errors.New("agent is disabled")
	}
	_, _ = s.control.DB.ExecContext(ctx, `UPDATE agent_principals SET last_used_at=? WHERE agent_id=?`, nowString(), agentID)
	return s.Get(ctx, agentID)
}

func HasPermission(principal Principal, permission string) bool {
	for _, value := range principal.Permissions {
		if value == permission {
			return true
		}
	}
	return false
}

func CanAccessProject(principal Principal, projectID string) bool {
	if principal.AllProjects {
		return true
	}
	for _, id := range principal.ProjectIDs {
		if id == projectID {
			return true
		}
	}
	return false
}

func (s *Service) CanAccessRecord(ctx context.Context, principal Principal, recordType, recordID string) (bool, error) {
	if principal.AllProjects {
		var n int
		err := s.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM record_projects WHERE record_type=? AND record_id=?`, recordType, recordID).Scan(&n)
		return n > 0, err
	}
	if len(principal.ProjectIDs) == 0 {
		return false, nil
	}
	query := `SELECT count(*) FROM record_projects WHERE record_type=? AND record_id=? AND project_id IN (` + strings.TrimRight(strings.Repeat("?,", len(principal.ProjectIDs)), ",") + `)`
	args := []any{recordType, recordID}
	for _, projectID := range principal.ProjectIDs {
		args = append(args, projectID)
	}
	var n int
	err := s.control.DB.QueryRowContext(ctx, query, args...).Scan(&n)
	return n > 0, err
}

func (s *Service) Audit(ctx context.Context, principal Principal, action, resourceType, resourceID, projectID, status string, detail any) error {
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	now := nowString()
	eventID := stableID("audit-", principal.AgentID, action, resourceType, resourceID, projectID, status, now)
	_, err = s.control.DB.ExecContext(ctx, `INSERT INTO agent_audit_log(event_id,agent_id,action,resource_type,resource_id,project_id,status,detail_json,created_at) VALUES(?,?,?,?,nullif(?,''),nullif(?,''),?,?,?)`, eventID, principal.AgentID, action, resourceType, resourceID, projectID, status, string(raw), now)
	return err
}

func (s *Service) ListAudit(ctx context.Context, agentID string, limit int) ([]AuditEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.control.DB.QueryContext(ctx, `SELECT event_id,agent_id,action,resource_type,coalesce(resource_id,''),coalesce(project_id,''),status,detail_json,created_at FROM agent_audit_log WHERE (?='' OR agent_id=?) ORDER BY created_at DESC LIMIT ?`, agentID, agentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEvent{}
	for rows.Next() {
		var item AuditEvent
		if err := rows.Scan(&item.EventID, &item.AgentID, &item.Action, &item.ResourceType, &item.ResourceID, &item.ProjectID, &item.Status, &item.DetailJSON, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
