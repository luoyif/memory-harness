package dshbridge

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectVerifiesPinnedContractWithoutClaimingRuntime(t *testing.T) {
	root := t.TempDir()
	plugin := filepath.Join(root, "plugins", VerifiedPluginName)
	if err := os.MkdirAll(filepath.Join(plugin, "lib"), 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := `{"name":"dsh-knowledge-hub","version":"0.4.1","dsh":{"bundle":{"patch":"./cordis.patch.yml"}}}`
	source := `ctx.on('agent/pre-step'); ctx.on('session/event'); 'turn/end'; ctx.sessionQuery.readSurface(); ctx.tools.guard(); reviewGateReasonFor()`
	for path, content := range map[string]string{
		filepath.Join(plugin, "package.json"):     metadata,
		filepath.Join(plugin, "lib", "index.js"):  source,
		filepath.Join(plugin, "cordis.patch.yml"): "- insert: []\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	probe, err := inspect(context.Background(), root, func(context.Context, string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if !probe.ContractVerified || probe.PluginVersion != VerifiedPluginVersion || probe.RuntimeReachable || probe.Status != "contract-verified" {
		t.Fatalf("probe=%#v", probe)
	}
}

func TestInspectRejectsUnknownPackage(t *testing.T) {
	if _, err := Inspect(context.Background(), t.TempDir()); err == nil {
		t.Fatal("unknown directory was accepted")
	}
}
