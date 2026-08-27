package config_test

import (
	"github.com/luoyif/memory-harness/internal/config"
	"testing"
)

func TestRejectNonLoopback(t *testing.T) {
	if _, err := config.Resolve(t.TempDir(), "0.0.0.0:19777"); err == nil {
		t.Fatal("expected non-loopback rejection")
	}
}
func TestAllowLoopback(t *testing.T) {
	if _, err := config.Resolve(t.TempDir(), "127.0.0.1:19777"); err != nil {
		t.Fatal(err)
	}
}
