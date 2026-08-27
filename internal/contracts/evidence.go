package contracts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ContentBlock struct {
	Type           string `json:"type"`
	Text           string `json:"text,omitempty"`
	AttachmentHash string `json:"attachment_hash,omitempty"`
}

type Provenance struct {
	CaptureMethod string  `json:"capture_method"`
	RawBatchID    *string `json:"raw_batch_id,omitempty"`
	RawHash       *string `json:"raw_hash,omitempty"`
	RawPath       *string `json:"raw_path,omitempty"`
	Parser        *string `json:"parser,omitempty"`
	ParserVersion *string `json:"parser_version,omitempty"`
}

type EvidenceEnvelope struct {
	SchemaVersion          string         `json:"schema_version"`
	EvidenceID             string         `json:"evidence_id"`
	SourceSystem           string         `json:"source_system"`
	SourceAccount          *string        `json:"source_account,omitempty"`
	ExternalConversationID *string        `json:"external_conversation_id,omitempty"`
	ExternalMessageID      *string        `json:"external_message_id,omitempty"`
	ParentEvidenceID       *string        `json:"parent_evidence_id,omitempty"`
	Role                   *string        `json:"role,omitempty"`
	ObservedAt             *time.Time     `json:"observed_at,omitempty"`
	CapturedAt             time.Time      `json:"captured_at"`
	Content                []ContentBlock `json:"content"`
	Provenance             Provenance     `json:"provenance"`
	ScopeHints             []string       `json:"scope_hints,omitempty"`
	Visibility             string         `json:"visibility,omitempty"`
	ParseWarnings          []string       `json:"parse_warnings,omitempty"`
}

func ParseEvidence(raw []byte) (EvidenceEnvelope, []byte, error) {
	var env EvidenceEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return env, nil, err
	}
	if env.SchemaVersion != "0.1" {
		return env, nil, errors.New("unsupported evidence schema_version")
	}
	if strings.TrimSpace(env.EvidenceID) == "" {
		return env, nil, errors.New("evidence_id is required")
	}
	if strings.TrimSpace(env.SourceSystem) == "" {
		return env, nil, errors.New("source_system is required")
	}
	if env.CapturedAt.IsZero() {
		return env, nil, errors.New("captured_at is required")
	}
	if strings.TrimSpace(env.Provenance.CaptureMethod) == "" {
		return env, nil, errors.New("provenance.capture_method is required")
	}
	if len(env.Content) == 0 {
		return env, nil, errors.New("content must not be empty")
	}
	for i, block := range env.Content {
		if strings.TrimSpace(block.Type) == "" {
			return env, nil, fmt.Errorf("content[%d].type is required", i)
		}
	}
	if env.Visibility != "" {
		switch env.Visibility {
		case "private", "shared", "global", "restricted":
		default:
			return env, nil, errors.New("invalid visibility")
		}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var generic any
	if err := dec.Decode(&generic); err != nil {
		return env, nil, err
	}
	canonical, err := json.Marshal(generic)
	if err != nil {
		return env, nil, err
	}
	return env, canonical, nil
}

func (e EvidenceEnvelope) LogicalSessionID() string {
	if e.ExternalConversationID != nil && strings.TrimSpace(*e.ExternalConversationID) != "" {
		return strings.TrimSpace(*e.ExternalConversationID)
	}
	if e.Provenance.RawBatchID != nil && strings.TrimSpace(*e.Provenance.RawBatchID) != "" {
		return strings.TrimSpace(*e.Provenance.RawBatchID)
	}
	return e.EvidenceID
}

func (e EvidenceEnvelope) EffectiveObservedAt() time.Time {
	if e.ObservedAt != nil && !e.ObservedAt.IsZero() {
		return e.ObservedAt.UTC()
	}
	return e.CapturedAt.UTC()
}

func (e EvidenceEnvelope) SearchText() string {
	parts := make([]string, 0, len(e.Content))
	for _, b := range e.Content {
		if t := strings.TrimSpace(b.Text); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n")
}

func HashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
