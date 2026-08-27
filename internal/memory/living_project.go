package memory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ClearLivingViewsForProject removes only disposable project-scoped Living
// projections. Canonical Evidence, Memory records and Object revisions are not
// changed. It is used when the active Blueprint disables growth.living.
func (e *Engine) ClearLivingViewsForProject(ctx context.Context, projectID string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return errors.New("project_id is required")
	}
	rows, err := e.control.DB.QueryContext(ctx, `SELECT canonical_path FROM living_views WHERE project_id=?`, projectID)
	if err != nil {
		return err
	}
	paths := []string{}
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			rows.Close()
			return err
		}
		paths = append(paths, path)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	tx, err := e.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM record_projects WHERE record_type='living' AND project_id=?`, projectID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM living_views WHERE project_id=?`, projectID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		err := os.Remove(filepath.Join(e.memoryDir, filepath.Base(path)))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
