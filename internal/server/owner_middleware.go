package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/luoyif/memory-harness/internal/ownerauth"
)

type ownerContextKey struct{}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

// Unwrap lets http.ResponseController reach the real connection. Long-running
// authenticated operations can then adjust their write deadline without
// weakening the default timeout for every other route.
func (w *statusRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecorder) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func ownerFromContext(ctx context.Context) (ownerauth.Principal, bool) {
	principal, ok := ctx.Value(ownerContextKey{}).(ownerauth.Principal)
	return principal, ok
}

func isPublicOrAgentRoute(r *http.Request) bool {
	if r.URL.Path == "/health" || r.URL.Path == "/v1/version" || !strings.HasPrefix(r.URL.Path, "/v1/") {
		return true
	}
	return strings.HasPrefix(r.URL.Path, "/v1/agent/")
}

func requiresCSRF(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

func (s *Server) ownerBoundary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.ownerAuthBypass || isPublicOrAgentRoute(r) {
			next.ServeHTTP(w, r)
			return
		}
		principal, err := s.app.Owner.Authenticate(r, requiresCSRF(r.Method))
		if err != nil {
			_ = s.app.Owner.AuditDenied(r.Context(), r, err.Error())
			status := http.StatusUnauthorized
			code := "owner_unauthorized"
			if errors.Is(err, ownerauth.ErrCSRF) || errors.Is(err, ownerauth.ErrOrigin) {
				status = http.StatusForbidden
				code = "owner_request_forbidden"
			}
			writeErr(w, status, code, err.Error())
			return
		}
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r.WithContext(context.WithValue(r.Context(), ownerContextKey{}, principal)))
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		_ = s.app.Owner.Audit(r.Context(), principal, "owner.request", "http_route", r.URL.Path, "allowed", map[string]any{"method": r.Method, "status": status})
	})
}

func (s *Server) ownerSession(w http.ResponseWriter, r *http.Request) {
	principal, ok := ownerFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "owner_unauthorized", "valid desktop owner session required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"owner": principal, "csrf_required_for_mutation": true})
}
