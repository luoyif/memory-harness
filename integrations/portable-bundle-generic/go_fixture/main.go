package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/luoyif/memory-harness/internal/app"
	"github.com/luoyif/memory-harness/internal/config"
	"github.com/luoyif/memory-harness/internal/contracts"
	"github.com/luoyif/memory-harness/internal/growth"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/memory"
	"github.com/luoyif/memory-harness/internal/portablebundle"
	"github.com/luoyif/memory-harness/internal/portfolio"
)

type inspection struct {
	Manifest portablebundle.Manifest         `json:"manifest"`
	Objects  []portablebundle.ObjectRecord   `json:"objects"`
	Evidence []portablebundle.EvidenceRecord `json:"evidence"`
}

func ptr(value string) *string { return &value }
func openTemp() (*app.App, string, error) {
	home, err := os.MkdirTemp("", "mh-portable-go-")
	if err != nil {
		return nil, "", err
	}
	cfg, err := config.Resolve(home, "")
	if err != nil {
		return nil, "", err
	}
	a, err := app.Open(cfg)
	if err != nil {
		return nil, "", err
	}
	return a, home, nil
}

func one(ctx context.Context, a *app.App, projectID, typeID string) (harness.Object, error) {
	items, err := a.Harness.ListObjects(ctx, projectID, typeID, "", 20)
	if err != nil {
		return harness.Object{}, err
	}
	if len(items) == 0 {
		return harness.Object{}, fmt.Errorf("missing object type %s", typeID)
	}
	return items[0], nil
}

func generate(output string) error {
	ctx := context.Background()
	a, home, err := openTemp()
	if err != nil {
		return err
	}
	defer os.RemoveAll(home)
	defer a.Close()
	project, err := a.Portfolio.CreateProject(ctx, portfolio.ProjectInput{Slug: "generic-cross", Name: "Generic Cross", DefaultCurrency: "CNY"})
	if err != nil {
		return err
	}
	observed := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	envelope := contracts.EvidenceEnvelope{
		SchemaVersion: "0.1", EvidenceID: "ev-generic-cross", SourceSystem: "meeting",
		ExternalConversationID: ptr("generic-cross-session"), Role: ptr("user"), ObservedAt: &observed, CapturedAt: observed,
		Content:    []contracts.ContentBlock{{Type: "text", Text: "每次发布前必须先运行完整测试，检查回滚条件，再执行部署；当前项目目标是完成跨 Harness Portable Bundle 验收。"}},
		Provenance: contracts.Provenance{CaptureMethod: "integration_fixture"}, Visibility: "private",
	}
	raw, _ := json.Marshal(envelope)
	captured, err := a.Ledger.Append(ctx, raw)
	if err != nil {
		return err
	}
	if _, err := a.Growth.Process(ctx, growth.ProcessInput{ProjectID: project.ProjectID, SessionID: captured.SessionID, EvidenceIDs: []string{captured.EvidenceID}, Primary: true}); err != nil {
		return err
	}

	ku, err := one(ctx, a, project.ProjectID, memory.StructuredKnowledgeUnitTypeV2)
	if err != nil {
		return err
	}
	mem, err := one(ctx, a, project.ProjectID, memory.StructuredMemoryRecordTypeV1)
	if err != nil {
		return err
	}
	product, err := one(ctx, a, project.ProjectID, harness.KnowledgeProductTypeV1)
	if err != nil {
		return err
	}
	skill, err := one(ctx, a, project.ProjectID, harness.GovernedAgentAssetTypeV3)
	if err != nil {
		return err
	}
	roots := []string{ku.ObjectID, mem.ObjectID, product.ObjectID, skill.ObjectID}
	output, err = filepath.Abs(output)
	if err != nil {
		return err
	}
	manifest, err := a.Portable.Export(ctx, portablebundle.ExportOptions{ProjectID: project.ProjectID, ObjectIDs: roots, IncludeDependencies: true}, output)
	if err != nil {
		return err
	}
	encoded, _ := json.Marshal(manifest)
	fmt.Println(string(encoded))
	return nil
}
func inspect(path string) error {
	manifest, objects, evidence, err := portablebundle.Inspect(path)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(inspection{Manifest: manifest, Objects: objects, Evidence: evidence})
	fmt.Println(string(raw))
	return nil
}

func main() {
	if len(os.Args) != 3 {
		panic("usage: go_fixture <generate|inspect> <bundle-path>")
	}
	var err error
	switch os.Args[1] {
	case "generate":
		err = generate(os.Args[2])
	case "inspect":
		err = inspect(os.Args[2])
	default:
		err = fmt.Errorf("unknown command %s", os.Args[1])
	}
	if err != nil {
		panic(err)
	}
}
