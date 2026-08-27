package ledger

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/luoyif/memory-harness/internal/config"
	"github.com/luoyif/memory-harness/internal/contracts"
	"github.com/luoyif/memory-harness/internal/store"
)

var safeNameRx = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

var ErrEvidenceConflict = errors.New("evidence_id already exists with different content")

type Ledger struct {
	cfg     config.Config
	control *store.ControlStore
	search  *store.SearchStore
	mu      sync.Mutex
}

type AppendResult struct {
	EvidenceID string `json:"evidence_id"`
	SessionID  string `json:"session_id"`
	LedgerPath string `json:"ledger_path"`
	LineHash   string `json:"line_hash"`
	Ordinal    int    `json:"ordinal"`
	Duplicate  bool   `json:"duplicate"`
}

func New(cfg config.Config, control *store.ControlStore, search *store.SearchStore) *Ledger {
	return &Ledger{cfg: cfg, control: control, search: search}
}

func safeSource(s string) string {
	s = safeNameRx.ReplaceAllString(strings.TrimSpace(s), "-")
	s = strings.Trim(s, "-._")
	if s == "" {
		return "unknown"
	}
	if len(s) > 48 {
		s = s[:48]
	}
	return s
}

func sessionToken(id string) string {
	h := sha256.Sum256([]byte(id))
	return hex.EncodeToString(h[:8])
}

func (l *Ledger) sessionRelPath(e contracts.EvidenceEnvelope) string {
	return filepath.ToSlash(filepath.Join("sessions", safeSource(e.SourceSystem), sessionToken(e.LogicalSessionID()), "events.jsonl"))
}

func roleValue(e contracts.EvidenceEnvelope) string {
	if e.Role == nil {
		return ""
	}
	return *e.Role
}

func receiptFor(e contracts.EvidenceEnvelope, rel, hash string, ordinal int) store.Receipt {
	return store.Receipt{
		EvidenceID:    e.EvidenceID,
		LineHash:      hash,
		SourceSystem:  e.SourceSystem,
		SessionID:     e.LogicalSessionID(),
		ObservedAt:    e.EffectiveObservedAt().Format("2006-01-02T15:04:05.999999999Z07:00"),
		CapturedAt:    e.CapturedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		LedgerRelPath: rel,
		Ordinal:       ordinal,
	}
}

func indexedTurn(e contracts.EvidenceEnvelope, r store.Receipt) store.IndexedTurn {
	scopeJSON, _ := json.Marshal(e.ScopeHints)
	return store.IndexedTurn{
		EvidenceID:    e.EvidenceID,
		LineHash:      r.LineHash,
		SessionID:     r.SessionID,
		SourceSystem:  r.SourceSystem,
		ObservedAt:    r.ObservedAt,
		CapturedAt:    r.CapturedAt,
		Role:          roleValue(e),
		ScopeJSON:     string(scopeJSON),
		Ordinal:       r.Ordinal,
		Body:          e.SearchText(),
		LedgerRelPath: r.LedgerRelPath,
	}
}

type scanHit struct {
	Found   bool
	Hash    string
	Ordinal int
	Raw     []byte
}

func scanFileFor(path, evidenceID string) (scanHit, int, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return scanHit{}, 0, nil
	}
	if err != nil {
		return scanHit{}, 0, err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	ordinal := 0
	for {
		line, readErr := r.ReadBytes('\n')
		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			ordinal++
			var head struct {
				EvidenceID string `json:"evidence_id"`
			}
			if err := json.Unmarshal(line, &head); err != nil {
				return scanHit{}, ordinal, fmt.Errorf("invalid ledger line %d: %w", ordinal, err)
			}
			if head.EvidenceID == evidenceID {
				raw := append([]byte(nil), line...)
				return scanHit{Found: true, Hash: contracts.HashBytes(line), Ordinal: ordinal, Raw: raw}, ordinal, nil
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return scanHit{}, ordinal, readErr
		}
	}
	return scanHit{}, ordinal, nil
}

func (l *Ledger) Append(ctx context.Context, raw []byte) (AppendResult, error) {
	e, compact, err := contracts.ParseEvidence(raw)
	if err != nil {
		return AppendResult{}, err
	}
	lineHash := contracts.HashBytes(compact)
	rel := l.sessionRelPath(e)
	abs := filepath.Join(l.cfg.LedgerDir(), filepath.FromSlash(rel))

	l.mu.Lock()
	defer l.mu.Unlock()

	if prior, ok, err := l.control.Receipt(ctx, e.EvidenceID); err != nil {
		return AppendResult{}, err
	} else if ok {
		if prior.LineHash != lineHash {
			return AppendResult{}, fmt.Errorf("%w: %s", ErrEvidenceConflict, e.EvidenceID)
		}
		priorAbs := filepath.Join(l.cfg.LedgerDir(), filepath.FromSlash(prior.LedgerRelPath))
		hit, _, err := scanFileFor(priorAbs, e.EvidenceID)
		if err != nil {
			return AppendResult{}, err
		}
		if !hit.Found || hit.Hash != prior.LineHash {
			return AppendResult{}, fmt.Errorf("ledger/control integrity mismatch for %s", e.EvidenceID)
		}
		// Ensure a deleted/rebuilt search cache is repaired even on an idempotent re-send.
		if err := l.search.UpsertTurn(ctx, indexedTurn(e, prior)); err != nil {
			return AppendResult{}, err
		}
		return AppendResult{EvidenceID: e.EvidenceID, SessionID: prior.SessionID, LedgerPath: prior.LedgerRelPath, LineHash: prior.LineHash, Ordinal: prior.Ordinal, Duplicate: true}, nil
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return AppendResult{}, err
	}
	hit, count, err := scanFileFor(abs, e.EvidenceID)
	if err != nil {
		return AppendResult{}, err
	}
	duplicate := false
	ordinal := count + 1
	if hit.Found {
		if hit.Hash != lineHash {
			return AppendResult{}, fmt.Errorf("%w: %s", ErrEvidenceConflict, e.EvidenceID)
		}
		duplicate = true
		ordinal = hit.Ordinal
		compact = hit.Raw
	} else {
		f, err := os.OpenFile(abs, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return AppendResult{}, err
		}
		if _, err = f.Write(append(compact, '\n')); err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err != nil {
			return AppendResult{}, err
		}
		if closeErr != nil {
			return AppendResult{}, closeErr
		}
	}

	receipt := receiptFor(e, rel, lineHash, ordinal)
	if err := l.control.UpsertReceipt(ctx, receipt); err != nil {
		return AppendResult{}, err
	}
	if err := l.search.UpsertTurn(ctx, indexedTurn(e, receipt)); err != nil {
		return AppendResult{}, err
	}
	return AppendResult{EvidenceID: e.EvidenceID, SessionID: receipt.SessionID, LedgerPath: rel, LineHash: lineHash, Ordinal: ordinal, Duplicate: duplicate}, nil
}

func (l *Ledger) ReadEvidence(ctx context.Context, evidenceID string) ([]byte, error) {
	receipt, ok, err := l.control.Receipt(ctx, evidenceID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, os.ErrNotExist
	}
	abs := filepath.Join(l.cfg.LedgerDir(), filepath.FromSlash(receipt.LedgerRelPath))
	hit, _, err := scanFileFor(abs, evidenceID)
	if err != nil {
		return nil, err
	}
	if !hit.Found {
		return nil, os.ErrNotExist
	}
	if hit.Hash != receipt.LineHash {
		return nil, fmt.Errorf("ledger integrity mismatch for %s", evidenceID)
	}
	return hit.Raw, nil
}

func (l *Ledger) Walk(ctx context.Context, fn func(rel string, ordinal int, e contracts.EvidenceEnvelope, compact []byte) error) error {
	root := filepath.Join(l.cfg.LedgerDir(), "sessions")
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		rel, err := filepath.Rel(l.cfg.LedgerDir(), path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		r := bufio.NewReader(f)
		ordinal := 0
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			line, readErr := r.ReadBytes('\n')
			line = bytes.TrimSpace(line)
			if len(line) > 0 {
				ordinal++
				e, compact, err := contracts.ParseEvidence(line)
				if err != nil {
					return fmt.Errorf("%s line %d: %w", rel, ordinal, err)
				}
				if err := fn(rel, ordinal, e, compact); err != nil {
					return err
				}
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				return readErr
			}
		}
		return nil
	})
}

func (l *Ledger) RebuildSearch(ctx context.Context) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	count := 0
	err := l.search.Rebuild(ctx, func(add func(store.IndexedTurn) error) error {
		return l.Walk(ctx, func(rel string, ordinal int, e contracts.EvidenceEnvelope, compact []byte) error {
			r := receiptFor(e, rel, contracts.HashBytes(compact), ordinal)
			if err := add(indexedTurn(e, r)); err != nil {
				return err
			}
			count++
			return nil
		})
	})
	return count, err
}
