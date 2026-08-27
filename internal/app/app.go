package app

import (
	"context"

	"github.com/luoyif/memory-harness/internal/adaptation"
	"github.com/luoyif/memory-harness/internal/agentauth"
	"github.com/luoyif/memory-harness/internal/blueprint"
	"github.com/luoyif/memory-harness/internal/builtin"
	"github.com/luoyif/memory-harness/internal/config"
	"github.com/luoyif/memory-harness/internal/contextbridge"
	"github.com/luoyif/memory-harness/internal/experience"
	"github.com/luoyif/memory-harness/internal/growth"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/ledger"
	"github.com/luoyif/memory-harness/internal/memory"
	"github.com/luoyif/memory-harness/internal/modelconfig"
	"github.com/luoyif/memory-harness/internal/ownerauth"
	"github.com/luoyif/memory-harness/internal/pipeline"
	"github.com/luoyif/memory-harness/internal/plugins"
	"github.com/luoyif/memory-harness/internal/portablebundle"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/profile"
	"github.com/luoyif/memory-harness/internal/reprojection"
	"github.com/luoyif/memory-harness/internal/search"
	"github.com/luoyif/memory-harness/internal/store"
	"github.com/luoyif/memory-harness/internal/teammemory"
	"github.com/luoyif/memory-harness/internal/unifiedsearch"
)

type App struct {
	Config       config.Config
	Control      *store.ControlStore
	SearchStore  *store.SearchStore
	Ledger       *ledger.Ledger
	Search       *search.Engine
	Memory       *memory.Engine
	Portfolio    *portfolio.Service
	Portable     *portablebundle.Service
	Profiles     *profile.Service
	Experience   *experience.Service
	Adaptation   *adaptation.Service
	TeamMemory   *teammemory.Service
	Unified      *unifiedsearch.Engine
	Agents       *agentauth.Service
	Models       *modelconfig.Service
	Owner        *ownerauth.Service
	Harness      *harness.Service
	Plugins      *plugins.Service
	Pipelines    *pipeline.Service
	Blueprints   *blueprint.Service
	Growth       *growth.Service
	Reprojection *reprojection.Service
	Context      *contextbridge.Service
}

func Open(cfg config.Config) (*App, error) {
	if err := cfg.EnsureDirs(); err != nil {
		return nil, err
	}
	control, err := store.OpenControl(cfg.ControlDB())
	if err != nil {
		return nil, err
	}
	searchStore, err := store.OpenSearch(cfg.SearchDB())
	if err != nil {
		control.Close()
		return nil, err
	}
	memoryEngine := memory.New(control, searchStore, cfg.MemoryDir())
	portfolioService := portfolio.New(control, searchStore)
	modelService := modelconfig.New(control, modelconfig.NewDefaultSecretStore(cfg.Home), nil)
	harnessService := harness.New(control)
	evidenceLedger := ledger.New(cfg, control, searchStore)
	blueprintService := blueprint.New(control)
	pipelineService := pipeline.New(control, harnessService, modelService)
	growthService := growth.New(control, memoryEngine, portfolioService, harnessService, pipelineService, blueprintService, modelService)
	profileService := profile.New(control, memoryEngine, portfolioService, harnessService)
	experienceService := experience.New(control, harnessService, searchStore)
	adaptationService := adaptation.New(control, harnessService, blueprintService, experienceService)
	portableService := portablebundle.New(control, harnessService, evidenceLedger)
	agentService := agentauth.New(control)
	teamMemoryService := teammemory.New(harnessService, agentService, portfolioService)
	reprojectionService := reprojection.New(memoryEngine, portfolioService)
	if err := growthService.RegisterStages(); err != nil {
		searchStore.Close()
		control.Close()
		return nil, err
	}
	pluginService := plugins.New(control, harnessService, pipelineService)
	pluginService.SetBlueprintPublisher(blueprintService)
	pipelineService.SetBlueprintResolver(blueprintService.Snapshot)
	a := &App{Config: cfg, Control: control, SearchStore: searchStore, Ledger: evidenceLedger, Search: search.New(searchStore), Memory: memoryEngine, Portfolio: portfolioService, Portable: portableService, Profiles: profileService, Experience: experienceService, Adaptation: adaptationService, TeamMemory: teamMemoryService, Unified: unifiedsearch.New(searchStore), Agents: agentService, Models: modelService, Owner: ownerauth.New(control), Harness: harnessService, Pipelines: pipelineService, Plugins: pluginService, Blueprints: blueprintService, Growth: growthService, Reprojection: reprojectionService}
	a.Context = contextbridge.New(harnessService, a.Unified, a.Ledger, blueprintService, profileService)
	if err := builtin.Bootstrap(context.Background(), builtin.Services{Harness: harnessService, Pipelines: pipelineService, Plugins: a.Plugins, Blueprints: blueprintService}); err != nil {
		a.Close()
		return nil, err
	}
	if err := pipelineService.RecoverInterrupted(context.Background()); err != nil {
		a.Close()
		return nil, err
	}
	memoryEngine.SetCandidateExtractor(modelService)
	if err := memoryEngine.Recover(context.Background()); err != nil {
		a.Close()
		return nil, err
	}
	if _, err := memoryEngine.ReconcileAgentAssets(context.Background()); err != nil {
		a.Close()
		return nil, err
	}
	projects, err := portfolioService.ListProjects(context.Background(), false)
	if err != nil {
		a.Close()
		return nil, err
	}
	for _, project := range projects {
		if err := growthService.ReconcileProjectKnowledgeProducts(context.Background(), project.ProjectID); err != nil {
			a.Close()
			return nil, err
		}
		if _, err := profileService.ReconcileProject(context.Background(), project.ProjectID); err != nil {
			a.Close()
			return nil, err
		}
	}
	if _, err := portfolioService.RebuildIndex(context.Background()); err != nil {
		a.Close()
		return nil, err
	}
	return a, nil
}

func (a *App) Close() error {
	e1 := a.SearchStore.Close()
	e2 := a.Control.Close()
	if e1 != nil {
		return e1
	}
	return e2
}
