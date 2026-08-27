package ownerauth_test

import (
	"net/http"
	"testing"

	"github.com/luoyif/memory-harness/internal/ownerauth"
	"github.com/luoyif/memory-harness/internal/testutil"
)

func TestOwnerSessionRequiresTokenCSRFAndOrigin(t *testing.T) {
	a, _ := testutil.Open(t)
	service := ownerauth.New(a.Control)
	credential, err := service.Issue("desktop-test")
	if err != nil {
		t.Fatal(err)
	}

	request, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:19777/v1/agents", nil)
	if _, err := service.Authenticate(request, true); err != ownerauth.ErrUnauthorized {
		t.Fatalf("missing token err=%v", err)
	}
	request.Header.Set(ownerauth.TokenHeader, credential.Token)
	if _, err := service.Authenticate(request, true); err != ownerauth.ErrCSRF {
		t.Fatalf("missing csrf err=%v", err)
	}
	request.Header.Set(ownerauth.CSRFHeader, credential.CSRFToken)
	request.Header.Set("Origin", "https://attacker.example")
	if _, err := service.Authenticate(request, true); err != ownerauth.ErrOrigin {
		t.Fatalf("forged origin err=%v", err)
	}
	request.Header.Set("Origin", "http://127.0.0.1:19777")
	principal, err := service.Authenticate(request, true)
	if err != nil || principal.SessionID != credential.SessionID {
		t.Fatalf("principal=%#v err=%v", principal, err)
	}
	request.Header.Set("Origin", "http://localhost:5173")
	if _, err := service.Authenticate(request, true); err != nil {
		t.Fatalf("Wails loopback development origin was rejected: %v", err)
	}
	request.Header.Set("Origin", "wails://wails.localhost:34115")
	if _, err := service.Authenticate(request, true); err != nil {
		t.Fatalf("Wails custom development origin was rejected: %v", err)
	}

	service.Revoke(t.Context(), credential.Token)
	if _, err := service.Authenticate(request, true); err != ownerauth.ErrUnauthorized {
		t.Fatalf("revoked err=%v", err)
	}
}
