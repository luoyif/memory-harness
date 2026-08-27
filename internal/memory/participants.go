package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

func normalizeParticipant(input ParticipantIdentity) (ParticipantIdentity, error) {
	input.ParticipantID = strings.TrimSpace(input.ParticipantID)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Role = strings.ToLower(strings.TrimSpace(input.Role))
	if input.DisplayName == "" || utf8.RuneCountInString(input.DisplayName) > 100 {
		return input, errors.New("participant display_name is required and must be at most 100 characters")
	}
	if input.Role == "" {
		input.Role = "participant"
	}
	if utf8.RuneCountInString(input.Role) > 64 {
		return input, errors.New("participant role must be at most 64 characters")
	}
	if input.ParticipantID != "" && utf8.RuneCountInString(input.ParticipantID) > 160 {
		return input, errors.New("participant_id must be at most 160 characters")
	}
	aliases := make([]string, 0, len(input.Aliases))
	seen := map[string]bool{strings.ToLower(input.DisplayName): true}
	for _, alias := range input.Aliases {
		alias = strings.TrimSpace(alias)
		key := strings.ToLower(alias)
		if alias == "" || seen[key] {
			continue
		}
		if utf8.RuneCountInString(alias) > 100 {
			return input, errors.New("participant alias must be at most 100 characters")
		}
		seen[key] = true
		aliases = append(aliases, alias)
		if len(aliases) > 20 {
			return input, errors.New("a participant may have at most 20 aliases")
		}
	}
	input.Aliases = aliases
	return input, nil
}

func (e *Engine) sessionParticipants(ctx context.Context, sessionID string) ([]ParticipantIdentity, error) {
	rows, err := e.control.DB.QueryContext(ctx, `SELECT participant_id,display_name,role,aliases_json FROM session_participants WHERE session_id=? ORDER BY CASE role WHEN 'first_person_speaker' THEN 0 ELSE 1 END,display_name`, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	participants := []ParticipantIdentity{}
	for rows.Next() {
		var item ParticipantIdentity
		var aliasesRaw string
		if err := rows.Scan(&item.ParticipantID, &item.DisplayName, &item.Role, &aliasesRaw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(aliasesRaw), &item.Aliases)
		participants = append(participants, item)
	}
	return participants, rows.Err()
}

func (e *Engine) SessionParticipants(ctx context.Context, sessionID string) ([]ParticipantIdentity, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session_id is required")
	}
	if err := e.search.DB.QueryRowContext(ctx, `SELECT 1 FROM turns WHERE session_id=? LIMIT 1`, sessionID).Scan(new(int)); err != nil {
		return nil, err
	}
	return e.sessionParticipants(ctx, sessionID)
}

func (e *Engine) ReplaceSessionParticipants(ctx context.Context, sessionID string, inputs []ParticipantIdentity) ([]ParticipantIdentity, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session_id is required")
	}
	if len(inputs) > 50 {
		return nil, errors.New("a session may have at most 50 participants")
	}
	if err := e.search.DB.QueryRowContext(ctx, `SELECT 1 FROM turns WHERE session_id=? LIMIT 1`, sessionID).Scan(new(int)); err != nil {
		return nil, err
	}
	participants := make([]ParticipantIdentity, 0, len(inputs))
	seenIDs := map[string]bool{}
	firstPerson := 0
	for _, input := range inputs {
		item, err := normalizeParticipant(input)
		if err != nil {
			return nil, err
		}
		if item.ParticipantID == "" {
			item.ParticipantID = stableID("participant_", sessionID, strings.ToLower(item.DisplayName), item.Role)
		}
		if seenIDs[item.ParticipantID] {
			return nil, fmt.Errorf("duplicate participant_id %q", item.ParticipantID)
		}
		seenIDs[item.ParticipantID] = true
		if item.Role == "first_person_speaker" {
			firstPerson++
		}
		participants = append(participants, item)
	}
	if firstPerson > 1 {
		return nil, errors.New("only one first_person_speaker may be declared for a session")
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	tx, err := e.control.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM session_participants WHERE session_id=?`, sessionID); err != nil {
		return nil, err
	}
	now := nowString()
	for _, item := range participants {
		aliasesRaw, _ := json.Marshal(item.Aliases)
		if _, err := tx.ExecContext(ctx, `INSERT INTO session_participants(session_id,participant_id,display_name,role,aliases_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, sessionID, item.ParticipantID, item.DisplayName, item.Role, string(aliasesRaw), now, now); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return participants, nil
}
