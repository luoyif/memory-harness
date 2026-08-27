package dshbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	AdapterVersion        = "0.1.0"
	VerifiedPluginName    = "dsh-knowledge-hub"
	VerifiedPluginVersion = "0.4.1"
	maxMetadataBytes      = 1 << 20
	maxSourceBytes        = 4 << 20
)

type Probe struct {
	Status             string   `json:"status"`
	AdapterVersion     string   `json:"adapter_version"`
	RootPath           string   `json:"root_path"`
	PluginPath         string   `json:"plugin_path"`
	PluginName         string   `json:"plugin_name"`
	PluginVersion      string   `json:"plugin_version"`
	ContractVerified   bool     `json:"contract_verified"`
	BundlePatchPresent bool     `json:"bundle_patch_present"`
	RuntimeReachable   bool     `json:"runtime_reachable"`
	RuntimeEndpoint    string   `json:"runtime_endpoint"`
	Checks             []Check  `json:"checks"`
	Limits             []string `json:"limits"`
}

type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type packageMetadata struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	DSH     struct {
		Bundle struct {
			Patch string `json:"patch"`
		} `json:"bundle"`
	} `json:"dsh"`
}

func boundedRead(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds the inspection limit", filepath.Base(path))
	}
	return data, nil
}

func pluginDirectory(root string) (string, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if !filepath.IsAbs(root) {
		return "", errors.New("DeepSeek Harness path must be absolute")
	}
	for _, candidate := range []string{root, filepath.Join(root, "plugins", VerifiedPluginName)} {
		raw, err := boundedRead(filepath.Join(candidate, "package.json"), maxMetadataBytes)
		if err != nil {
			continue
		}
		var metadata packageMetadata
		if json.Unmarshal(raw, &metadata) == nil && metadata.Name == VerifiedPluginName {
			return candidate, nil
		}
	}
	return "", errors.New("dsh-knowledge-hub package was not found in the selected directory")
}

func runtimeReachable(ctx context.Context, endpoint string) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false
	}
	request.Header.Set("Accept", "application/json")
	response, err := (&http.Client{Timeout: 1500 * time.Millisecond}).Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	return response.StatusCode >= 200 && response.StatusCode < 300
}

func inspect(ctx context.Context, root string, reachability func(context.Context, string) bool) (Probe, error) {
	pluginPath, err := pluginDirectory(root)
	if err != nil {
		return Probe{}, err
	}
	metadataRaw, err := boundedRead(filepath.Join(pluginPath, "package.json"), maxMetadataBytes)
	if err != nil {
		return Probe{}, err
	}
	var metadata packageMetadata
	if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
		return Probe{}, fmt.Errorf("parse DSH package metadata: %w", err)
	}
	sourceRaw, err := boundedRead(filepath.Join(pluginPath, "lib", "index.js"), maxSourceBytes)
	if err != nil {
		return Probe{}, err
	}
	source := string(sourceRaw)
	checks := []Check{
		{Name: "package", Status: status(metadata.Name == VerifiedPluginName), Detail: metadata.Name + "@" + metadata.Version},
		{Name: "pre-step recall hook", Status: status(strings.Contains(source, "agent/pre-step")), Detail: "agent/pre-step"},
		{Name: "completed-turn capture hook", Status: status(strings.Contains(source, "session/event") && strings.Contains(source, "turn/end")), Detail: "session/event + turn/end"},
		{Name: "surface reader", Status: status(strings.Contains(source, "sessionQuery.readSurface")), Detail: "bounded visible session surface"},
		{Name: "review guard", Status: status(strings.Contains(source, "ctx.tools.guard") && strings.Contains(source, "reviewGateReasonFor")), Detail: "canonical writes remain review-gated"},
	}
	patch := strings.TrimPrefix(metadata.DSH.Bundle.Patch, "./")
	patchPresent := false
	if patch != "" && !strings.Contains(patch, "..") {
		if info, statErr := os.Stat(filepath.Join(pluginPath, patch)); statErr == nil && info.Mode().IsRegular() {
			patchPresent = true
		}
	}
	checks = append(checks, Check{Name: "Cordis bundle patch", Status: status(patchPresent), Detail: metadata.DSH.Bundle.Patch})
	verified := metadata.Version == VerifiedPluginVersion && patchPresent
	for _, check := range checks {
		verified = verified && check.Status == "pass"
	}
	endpoint := "http://127.0.0.1:3080/knowledge-hub/api/state"
	live := reachability(ctx, endpoint)
	checks = append(checks, Check{Name: "live runtime", Status: map[bool]string{true: "pass", false: "offline"}[live], Detail: endpoint})
	probeStatus := "contract-mismatch"
	if verified {
		probeStatus = "contract-verified"
	}
	if verified && live {
		probeStatus = "live"
	}
	return Probe{
		Status: probeStatus, AdapterVersion: AdapterVersion, RootPath: filepath.Clean(root), PluginPath: pluginPath,
		PluginName: metadata.Name, PluginVersion: metadata.Version, ContractVerified: verified, BundlePatchPresent: patchPresent,
		RuntimeReachable: live, RuntimeEndpoint: endpoint, Checks: checks,
		Limits: []string{"Static contract verification does not prove live event delivery.", "Live status only confirms the local Knowledge Hub state endpoint responds."},
	}, nil
}

func Inspect(ctx context.Context, root string) (Probe, error) {
	return inspect(ctx, root, runtimeReachable)
}

func status(ok bool) string {
	if ok {
		return "pass"
	}
	return "fail"
}
