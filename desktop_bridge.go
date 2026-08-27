package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	coreapp "github.com/luoyif/memory-harness/internal/app"
	"github.com/luoyif/memory-harness/internal/buildinfo"
	"github.com/luoyif/memory-harness/internal/config"
	"github.com/luoyif/memory-harness/internal/dshbridge"
	"github.com/luoyif/memory-harness/internal/exporter"
	"github.com/luoyif/memory-harness/internal/ownerauth"
	"github.com/luoyif/memory-harness/internal/plugins"
	"github.com/luoyif/memory-harness/internal/portablebundle"
	"github.com/luoyif/memory-harness/internal/server"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type DesktopBootstrap struct {
	Endpoint  string `json:"endpoint"`
	SessionID string `json:"session_id"`
	Token     string `json:"token"`
	CSRFToken string `json:"csrf_token"`
	ExpiresAt string `json:"expires_at"`
	Version   string `json:"version"`
}

type DesktopPortablePreflight struct {
	Path     string                             `json:"path"`
	Manifest portablebundle.Manifest            `json:"manifest"`
	Report   portablebundle.CompatibilityReport `json:"report"`
}

type DesktopPortableExport struct {
	Path     string                  `json:"path"`
	Manifest portablebundle.Manifest `json:"manifest"`
}

type DesktopBridge struct {
	mu            sync.RWMutex
	ctx           context.Context
	home          string
	core          *coreapp.App
	server        *server.Server
	listener      net.Listener
	credential    ownerauth.Credential
	endpoint      string
	startErr      error
	serveErr      chan error
	showOnStartup bool
}

func NewDesktopBridge() *DesktopBridge { return &DesktopBridge{showOnStartup: true} }

func newDesktopBridgeForTest(home string) *DesktopBridge {
	return &DesktopBridge{home: home}
}

func (b *DesktopBridge) Startup(ctx context.Context) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ctx = ctx
	cfg, err := config.Resolve(b.home, "127.0.0.1:0")
	if err != nil {
		b.startErr = err
		return
	}
	core, err := coreapp.Open(cfg)
	if err != nil {
		b.startErr = err
		return
	}
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		core.Close()
		b.startErr = err
		return
	}
	localServer := server.New(core)
	credential, err := localServer.IssueOwnerSession("wails-desktop")
	if err != nil {
		listener.Close()
		core.Close()
		b.startErr = err
		return
	}
	b.core = core
	b.server = localServer
	b.listener = listener
	b.credential = credential
	b.endpoint = "http://" + listener.Addr().String()
	b.serveErr = make(chan error, 1)
	go func() {
		err := localServer.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			b.serveErr <- err
		}
		close(b.serveErr)
	}()
	// macOS may restore a previously closed Wails window as hidden while the
	// application process keeps running. Re-show the primary window after the
	// startup lock is released so reopening the app always returns to the UI.
	if b.showOnStartup {
		time.AfterFunc(150*time.Millisecond, b.Show)
	}
}

func (b *DesktopBridge) Bootstrap() (DesktopBootstrap, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.startErr != nil {
		return DesktopBootstrap{}, b.startErr
	}
	if b.core == nil || b.server == nil || b.endpoint == "" || b.credential.Token == "" {
		return DesktopBootstrap{}, errors.New("desktop core is not ready")
	}
	if time.Until(b.credential.ExpiresAt) < 30*time.Minute {
		oldToken := b.credential.Token
		credential, err := b.server.IssueOwnerSession("wails-desktop-renewed")
		if err != nil {
			return DesktopBootstrap{}, err
		}
		b.credential = credential
		b.server.RevokeOwnerSession(context.Background(), oldToken)
	}
	select {
	case err, ok := <-b.serveErr:
		if ok && err != nil {
			return DesktopBootstrap{}, fmt.Errorf("desktop core stopped: %w", err)
		}
	default:
	}
	return DesktopBootstrap{
		Endpoint: b.endpoint, SessionID: b.credential.SessionID, Token: b.credential.Token,
		CSRFToken: b.credential.CSRFToken, ExpiresAt: b.credential.ExpiresAt.UTC().Format(time.RFC3339Nano),
		Version: buildinfo.Version,
	}, nil
}

func (b *DesktopBridge) InstallPluginPackage(projectID string, capabilities []string, developerMode bool) (plugins.PluginVersion, error) {
	b.mu.RLock()
	ctx, core := b.ctx, b.core
	b.mu.RUnlock()
	if core == nil {
		return plugins.PluginVersion{}, errors.New("desktop core is not ready")
	}
	path, err := wailsruntime.OpenFileDialog(ctx, wailsruntime.OpenDialogOptions{
		Title:   "安装 Memory Harness 插件",
		Filters: []wailsruntime.FileFilter{{DisplayName: "Memory Harness Plugin (*.mhplugin)", Pattern: "*.mhplugin"}},
	})
	if err != nil || path == "" {
		return plugins.PluginVersion{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return plugins.PluginVersion{}, err
	}
	item, err := core.Plugins.Install(context.Background(), raw, plugins.InstallOptions{DeveloperMode: developerMode, EnableProject: strings.TrimSpace(projectID), Capabilities: capabilities})
	if err == nil {
		_ = core.Owner.Audit(context.Background(), ownerauth.Principal{SessionID: b.credential.SessionID}, "plugin.install", "plugin", item.PluginID, "allowed", map[string]any{"version": item.Version, "source": filepath.Base(path)})
	}
	return item, err
}

func (b *DesktopBridge) ExportBackup() (string, error) {
	b.mu.RLock()
	ctx, core := b.ctx, b.core
	b.mu.RUnlock()
	if core == nil {
		return "", errors.New("desktop core is not ready")
	}
	defaultName := "Memory-Harness-Backup-" + time.Now().Format("20060102-150405") + ".tar.gz"
	path, err := wailsruntime.SaveFileDialog(ctx, wailsruntime.SaveDialogOptions{Title: "导出 Memory Harness 备份", DefaultFilename: defaultName, Filters: []wailsruntime.FileFilter{{DisplayName: "Gzip Archive (*.tar.gz)", Pattern: "*.tar.gz"}}})
	if err != nil || path == "" {
		return "", err
	}
	if err := exporter.Create(context.Background(), core, path); err != nil {
		return "", err
	}
	return path, nil
}

func (b *DesktopBridge) ExportPortableBundle(projectID string, objectIDs []string, includeDependencies bool) (DesktopPortableExport, error) {
	b.mu.RLock()
	ctx, core := b.ctx, b.core
	b.mu.RUnlock()
	if core == nil || ctx == nil {
		return DesktopPortableExport{}, errors.New("desktop core is not ready")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return DesktopPortableExport{}, errors.New("project_id is required")
	}
	defaultName := "Memory-Harness-Portable-" + time.Now().Format("20060102-150405") + ".mhbundle.tar.gz"
	path, err := wailsruntime.SaveFileDialog(ctx, wailsruntime.SaveDialogOptions{Title: "导出 Portable Memory Bundle", DefaultFilename: defaultName, Filters: []wailsruntime.FileFilter{{DisplayName: "Memory Harness Bundle (*.mhbundle.tar.gz)", Pattern: "*.mhbundle.tar.gz"}}})
	if err != nil || path == "" {
		return DesktopPortableExport{}, err
	}
	manifest, err := core.Portable.Export(context.Background(), portablebundle.ExportOptions{ProjectID: projectID, ObjectIDs: objectIDs, IncludeDependencies: includeDependencies}, path)
	if err != nil {
		return DesktopPortableExport{}, err
	}
	_ = core.Owner.Audit(context.Background(), ownerauth.Principal{SessionID: b.credential.SessionID}, "portable.export", "bundle", manifest.BundleID, "allowed", map[string]any{"project_id": projectID, "object_count": manifest.ObjectCount, "evidence_count": manifest.EvidenceCount})
	return DesktopPortableExport{Path: path, Manifest: manifest}, nil
}

func portableTargetCapabilities(ctx context.Context, core *coreapp.App) ([]string, []string, error) {
	types, err := core.Harness.ListTypes(ctx)
	if err != nil {
		return nil, nil, err
	}
	caps := map[string]bool{"evidence:v1": true}
	known := make([]string, 0, len(types))
	for _, item := range types {
		known = append(known, item.TypeID)
		caps["object-type:"+item.TypeID] = true
		caps["plugin:"+item.PluginID] = true
	}
	capabilities := make([]string, 0, len(caps))
	for value := range caps {
		capabilities = append(capabilities, value)
	}
	sort.Strings(capabilities)
	sort.Strings(known)
	return capabilities, known, nil
}

func (b *DesktopBridge) PreflightPortableBundle() (DesktopPortablePreflight, error) {
	b.mu.RLock()
	ctx, core := b.ctx, b.core
	b.mu.RUnlock()
	if core == nil || ctx == nil {
		return DesktopPortablePreflight{}, errors.New("desktop core is not ready")
	}
	path, err := wailsruntime.OpenFileDialog(ctx, wailsruntime.OpenDialogOptions{Title: "检查 Portable Memory Bundle", Filters: []wailsruntime.FileFilter{{DisplayName: "Memory Harness Bundle (*.mhbundle.tar.gz)", Pattern: "*.mhbundle.tar.gz"}}})
	if err != nil || path == "" {
		return DesktopPortablePreflight{}, err
	}
	capabilities, knownTypes, err := portableTargetCapabilities(context.Background(), core)
	if err != nil {
		return DesktopPortablePreflight{}, err
	}
	manifest, report, err := core.Portable.Preflight(context.Background(), path, portablebundle.PreflightOptions{TargetID: "memory-harness-desktop", Capabilities: capabilities, KnownObjectTypes: knownTypes, SupportsPresentations: true})
	if err != nil {
		return DesktopPortablePreflight{}, err
	}
	outcome := "allowed"
	if report.Blocked {
		outcome = "denied"
	}
	_ = core.Owner.Audit(context.Background(), ownerauth.Principal{SessionID: b.credential.SessionID}, "portable.preflight", "bundle", manifest.BundleID, outcome, map[string]any{"compatible": report.Compatible, "blocked": report.Blocked, "missing_capabilities": report.MissingCapabilities, "unmapped_types": report.UnmappedObjectTypes})
	return DesktopPortablePreflight{Path: path, Manifest: manifest, Report: report}, nil
}

func (b *DesktopBridge) ImportPortableBundle(projectID, path, expectedBundleID, expectedBundleHash, idempotencyKey string) (portablebundle.ImportResult, error) {
	b.mu.RLock()
	core := b.core
	b.mu.RUnlock()
	if core == nil {
		return portablebundle.ImportResult{}, errors.New("desktop core is not ready")
	}
	capabilities, knownTypes, err := portableTargetCapabilities(context.Background(), core)
	if err != nil {
		return portablebundle.ImportResult{}, err
	}
	manifest, report, err := core.Portable.Preflight(context.Background(), path, portablebundle.PreflightOptions{TargetID: "memory-harness-desktop", Capabilities: capabilities, KnownObjectTypes: knownTypes, SupportsPresentations: true})
	if err != nil {
		return portablebundle.ImportResult{}, err
	}
	if report.Blocked || manifest.BundleID != strings.TrimSpace(expectedBundleID) || manifest.BundleHash != strings.TrimSpace(expectedBundleHash) {
		return portablebundle.ImportResult{}, errors.New("bundle changed or became blocked after preflight; choose and inspect it again")
	}
	result, err := core.Portable.Import(context.Background(), path, portablebundle.ImportOptions{TargetProjectID: projectID, TargetID: "memory-harness-desktop", Capabilities: capabilities, KnownObjectTypes: knownTypes, SupportsPresentations: true, IdempotencyKey: idempotencyKey})
	if err == nil {
		_ = core.Owner.Audit(context.Background(), ownerauth.Principal{SessionID: b.credential.SessionID}, "portable.import", "bundle", result.BundleID, "allowed", map[string]any{"target_project_id": projectID, "candidate_count": len(result.CandidateObjectIDs), "evidence_imported": result.EvidenceImported, "no_direct_activation": result.NoDirectActivation})
	}
	return result, err
}

func (b *DesktopBridge) ProbeDeepSeekHarness() (dshbridge.Probe, error) {
	b.mu.RLock()
	ctx := b.ctx
	b.mu.RUnlock()
	if ctx == nil {
		return dshbridge.Probe{}, errors.New("desktop core is not ready")
	}
	path, err := wailsruntime.OpenDirectoryDialog(ctx, wailsruntime.OpenDialogOptions{
		Title: "选择 DeepSeek Harness 目录",
	})
	if err != nil || path == "" {
		return dshbridge.Probe{}, err
	}
	return dshbridge.Inspect(context.Background(), path)
}

func (b *DesktopBridge) Shutdown(ctx context.Context) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.server != nil {
		b.server.RevokeOwnerSession(ctx, b.credential.Token)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = b.server.Shutdown(shutdownCtx)
		cancel()
	}
	if b.listener != nil {
		_ = b.listener.Close()
	}
	if b.core != nil {
		_ = b.core.Close()
	}
	b.core = nil
	b.server = nil
	b.listener = nil
	b.credential = ownerauth.Credential{}
}

func (b *DesktopBridge) Show() {
	b.mu.RLock()
	ctx := b.ctx
	b.mu.RUnlock()
	if ctx != nil {
		wailsruntime.WindowShow(ctx)
		wailsruntime.WindowUnminimise(ctx)
	}
}
