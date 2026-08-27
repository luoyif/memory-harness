package ownerauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/luoyif/memory-harness/internal/store"
)

const (
	TokenHeader = "X-Memory-Harness-Owner"
	CSRFHeader  = "X-Memory-Harness-CSRF"
)

var (
	ErrUnauthorized = errors.New("valid desktop owner session required")
	ErrCSRF         = errors.New("valid owner CSRF token required")
	ErrOrigin       = errors.New("owner request origin is not allowed")
)

type Credential struct {
	SessionID string    `json:"session_id"`
	Token     string    `json:"token"`
	CSRFToken string    `json:"csrf_token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Principal struct {
	SessionID string    `json:"session_id"`
	Label     string    `json:"label"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type session struct {
	principal Principal
	csrfHash  [32]byte
}

type Service struct {
	control  *store.ControlStore
	mu       sync.RWMutex
	sessions map[[32]byte]session
	ttl      time.Duration
	now      func() time.Time
}

func New(control *store.ControlStore) *Service {
	return &Service{control: control, sessions: map[[32]byte]session{}, ttl: 8 * time.Hour, now: time.Now}
}

func randomSecret(prefix string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

func digest(value string) [32]byte { return sha256.Sum256([]byte(strings.TrimSpace(value))) }

func (s *Service) Issue(label string) (Credential, error) {
	token, err := randomSecret("mho_")
	if err != nil {
		return Credential{}, err
	}
	csrf, err := randomSecret("mhc_")
	if err != nil {
		return Credential{}, err
	}
	now := s.now().UTC()
	tokenHash := digest(token)
	sessionID := "owner-" + hex.EncodeToString(tokenHash[:12])
	principal := Principal{SessionID: sessionID, Label: strings.TrimSpace(label), IssuedAt: now, ExpiresAt: now.Add(s.ttl)}
	s.mu.Lock()
	s.sessions[tokenHash] = session{principal: principal, csrfHash: digest(csrf)}
	s.mu.Unlock()
	_ = s.Audit(context.Background(), principal, "owner.session.issue", "owner_session", sessionID, "allowed", map[string]any{"label": principal.Label, "expires_at": principal.ExpiresAt})
	return Credential{SessionID: sessionID, Token: token, CSRFToken: csrf, ExpiresAt: principal.ExpiresAt}, nil
}

func (s *Service) Revoke(ctx context.Context, token string) {
	hash := digest(token)
	s.mu.Lock()
	item, ok := s.sessions[hash]
	delete(s.sessions, hash)
	s.mu.Unlock()
	if ok {
		_ = s.Audit(ctx, item.principal, "owner.session.revoke", "owner_session", item.principal.SessionID, "allowed", nil)
	}
}

func (s *Service) Authenticate(r *http.Request, requireCSRF bool) (Principal, error) {
	token := strings.TrimSpace(r.Header.Get(TokenHeader))
	if !strings.HasPrefix(token, "mho_") {
		return Principal{}, ErrUnauthorized
	}
	hash := digest(token)
	s.mu.RLock()
	item, ok := s.sessions[hash]
	s.mu.RUnlock()
	if !ok || !s.now().UTC().Before(item.principal.ExpiresAt) {
		if ok {
			s.mu.Lock()
			delete(s.sessions, hash)
			s.mu.Unlock()
		}
		return Principal{}, ErrUnauthorized
	}
	if !requireCSRF {
		return item.principal, nil
	}
	csrfHash := digest(r.Header.Get(CSRFHeader))
	if subtle.ConstantTimeCompare(csrfHash[:], item.csrfHash[:]) != 1 {
		return Principal{}, ErrCSRF
	}
	if !allowedOrigin(r) {
		return Principal{}, ErrOrigin
	}
	return item.principal, nil
}

func allowedOrigin(r *http.Request) bool {
	origin := strings.TrimRight(strings.TrimSpace(r.Header.Get("Origin")), "/")
	if origin == "" {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(r.Host))
	allowed := map[string]bool{
		"http://" + host:  true,
		"https://" + host: true,
	}
	return allowed[strings.ToLower(origin)] || IsTrustedDesktopOrigin(origin)
}

// IsTrustedDesktopOrigin accepts Wails production origins and loopback HTTP
// origins used by Wails' development server. Owner and CSRF secrets are still
// required; arbitrary remote web origins remain forbidden.
func IsTrustedDesktopOrigin(origin string) bool {
	origin = strings.ToLower(strings.TrimRight(strings.TrimSpace(origin), "/"))
	if origin == "wails://wails" || origin == "http://wails.localhost" || origin == "https://wails.localhost" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Hostname() == "" {
		return false
	}
	host := parsed.Hostname()
	if parsed.Scheme == "wails" {
		return host == "wails" || host == "wails.localhost"
	}
	if parsed.Scheme != "http" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Service) Audit(ctx context.Context, principal Principal, action, resourceType, resourceID, status string, detail any) error {
	if s.control == nil {
		return nil
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	seed := principal.SessionID + "\x00" + action + "\x00" + resourceType + "\x00" + resourceID + "\x00" + status + "\x00" + now
	id := sha256.Sum256([]byte(seed))
	_, err = s.control.DB.ExecContext(ctx, `INSERT INTO owner_audit_log(event_id,owner_session_id,action,resource_type,resource_id,status,detail_json,created_at) VALUES(?,?,?,?,?,?,?,?)`, "own-"+hex.EncodeToString(id[:12]), principal.SessionID, action, resourceType, resourceID, status, string(raw), now)
	return err
}

func (s *Service) AuditDenied(ctx context.Context, r *http.Request, reason string) error {
	detail := map[string]any{"method": r.Method, "path": r.URL.Path, "origin": r.Header.Get("Origin"), "reason": reason}
	return s.Audit(ctx, Principal{}, "owner.request", "http_route", r.URL.Path, "denied", detail)
}
