import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { HealthPage, HomePage, MemoryPage } from "./App";
import { APIClient, Project } from "./api";
import { ContentPreview } from "./ContentPreview";
import { HelpCenter } from "./HelpCenter";
import { ProjectWorkspace } from "./ProjectWorkspace";

afterEach(cleanup);

describe("beginner experience", () => {
  it("shows a short preview and opens the full content in a drawer", () => {
    const close = vi.fn();
    const long = "这是很长的项目核心上下文。".repeat(30);
    const { rerender } = render(<ContentPreview title="核心上下文" content={long} open={false} onOpen={vi.fn()} onClose={close}/>);
    expect(screen.getByRole("button", { name: /查看全文/ })).toBeInTheDocument();
    rerender(<ContentPreview title="核心上下文" content={long} open onOpen={vi.fn()} onClose={close}/>);
    expect(screen.getByRole("dialog", { name: "核心上下文" })).toHaveTextContent(long);
    fireEvent.click(screen.getByRole("button", { name: "关闭全文" }));
    expect(close).toHaveBeenCalled();
  });

  it("processes only pending, changed or failed whole sources in basic mode", async () => {
    const get = vi.fn(async (path: string) => {
      if (path.startsWith("/v1/layers")) return { layers: [{ count: 4 }, { count: 8 }, { count: 2 }, { count: 3 }, { count: 0 }, { count: 0 }], needs_review: 0 };
      if (path.startsWith("/v1/memories")) return { memories: [], total: 3 };
      if (path.startsWith("/v1/episodes")) return { episodes: [], total: 2 };
      if (path.startsWith("/v1/process/sources")) return { sources: [
        { session_id: "new", project_id: "p1", title: "新文件", source_system: "markdown", evidence_count: 1, imported_at: "2026-08-24T00:00:00Z", status: "pending", status_detail: "尚未整理" },
        { session_id: "done", project_id: "p1", title: "旧文件", source_system: "markdown", evidence_count: 1, imported_at: "2026-08-20T00:00:00Z", last_processed_at: "2026-08-20T01:00:00Z", status: "completed", status_detail: "已经完成" },
      ] };
      throw new Error(`unexpected GET ${path}`);
    });
    const post = vi.fn(async () => ({ results: [{ knowledge_units: 2 }], total: 1, succeeded: 1, failed: 0, skipped: 0 }));
    render(<MemoryPage api={{ get, post } as unknown as APIClient} projectID="p1" openRun={vi.fn()} mode="basic"/>);
    await screen.findByText("默认只整理新导入的原材料");
    expect(screen.queryByRole("button", { name: "强制重新处理全部" })).not.toBeInTheDocument();
    expect(screen.getAllByText("项目记忆").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: "处理新增或失败的原材料" }));
    await waitFor(() => expect(post).toHaveBeenCalledWith("/v1/process", { project_id: "p1", session_ids: ["new"], mode: "incremental" }));
  });

  it("keeps AI tasks suggested until the user accepts and hides finance in basic mode", async () => {
    const project: Project = { project_id: "p1", slug: "p1", name: "项目一", description: "说明", status: "active", color: "#52715f", default_currency: "CNY", budget_minor: 0 };
    const summary = { project, metrics: { evidence: 2, knowledge_units: 3, available_memories: 1, pending_review: 1 }, finance: { currencies: [{ currency: "CNY", net_minor: 100 }] } };
    const detail = { summary, goals: [], milestones: [], decisions: [], risks: [], context_blocks: [], finance_accounts: [{}], automation: { project_id: "p1", import_mode: "auto_new" }, tasks: [{ task_id: "task-ai", project_id: "p1", title: "AI 建议行动", description: "", status: "suggested", priority: 2, source_kind: "ai_suggestion", source_record_id: "goal-1", source_evidence_ids: ["ev-1"], created_at: "", updated_at: "" }] };
    const get = vi.fn(async () => detail);
    const patch = vi.fn(async () => ({ status: "todo" }));
    render(<ProjectWorkspace api={{ get, patch, post: vi.fn() } as unknown as APIClient} projectID="p1" projects={[summary]} select={vi.fn()} onNavigate={vi.fn()} mode="basic"/>);
    await screen.findByText("AI 建议行动");
    expect(screen.queryByText("项目收支（可选）")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "加入待办" }));
    await waitFor(() => expect(patch).toHaveBeenCalledWith("/v1/project-tasks/task-ai", { status: "todo" }));
  });

  it("provides a plain-language operation manual for basic and advanced pages", () => {
    render(<HelpCenter/>);
    expect(screen.getByRole("heading", { name: "操作手册：六步完成从原材料到可用记忆" })).toBeInTheDocument();
    expect(screen.getByText(/不会替你启动或调度外部 AI/)).toBeInTheDocument();
    expect(screen.getByText(/Context Budget/)).toBeInTheDocument();
    expect(screen.getAllByText("怎么操作").length).toBeGreaterThan(10);
    expect(screen.getAllByText("系统不会做什么").length).toBeGreaterThan(10);
    expect(screen.getByRole("heading", { name: "记忆总览" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "插件中心" })).toBeInTheDocument();
  });

  it("puts new memory space at the start of memory overview", async () => {
    const project: Project = { project_id: "p1", slug: "p1", name: "项目一", description: "说明", status: "active", color: "#52715f", default_currency: "CNY", budget_minor: 0 };
    const get = vi.fn(async (path: string) => {
      if (path.startsWith("/v1/operations")) return { operations: [] };
      if (path.startsWith("/v1/episodes")) return { episodes: [] };
      if (path.startsWith("/v1/project-tasks")) return { tasks: [{ task_id: "task-home", title: "核对压力测试结果", status: "todo", priority: 1, source_kind: "manual" }] };
      if (path.startsWith("/v1/process/sources")) return { sources: [] };
      if (path.startsWith("/v1/memory-pins")) return { memories: [], total: 0 };
      throw new Error(`unexpected GET ${path}`);
    });
    const create = vi.fn();
    render(<HomePage api={{ get } as unknown as APIClient} project={project} summary={{ project, metrics: { evidence: 0 } }} onNavigate={vi.fn()} onCreateSpace={create}/>);
    fireEvent.click(await screen.findByRole("button", { name: "新建记忆空间" }));
    expect(create).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("heading", { name: "先把真正需要你处理的事情放在这里" })).toBeInTheDocument();
    expect(screen.getByText("核对压力测试结果")).toBeInTheDocument();
    expect(screen.getByText("已固定的记忆")).toBeInTheDocument();
    expect(screen.queryByText("重要记忆")).not.toBeInTheDocument();
    expect(screen.queryByText("CURRENT PROJECT")).not.toBeInTheDocument();
  });

  it("opens real review actions and only shows owner-pinned memories", async () => {
    const project: Project = { project_id: "p1", slug: "p1", name: "项目一", description: "说明", status: "active", color: "#52715f", default_currency: "CNY", budget_minor: 0 };
    const get = vi.fn(async (path: string) => {
      if (path.startsWith("/v1/operations")) return { operations: [{ operation_id: "op-1", operation_type: "memory_update", risk_tier: "protected" }] };
      if (path.startsWith("/v1/episodes")) return { episodes: [] };
      if (path.startsWith("/v1/project-tasks")) return { tasks: [] };
      if (path.startsWith("/v1/process/sources")) return { sources: [] };
      if (path.startsWith("/v1/memory-pins")) return { memories: [{ memory_id: "m1", summary: "Owner 固定的发布边界", body: "未经确认不得正式发布。", importance: .1, source_evidence_ids: ["ev-1"], pinned_at: "2026-08-26T02:00:00Z" }], total: 1 };
      throw new Error(`unexpected GET ${path}`);
    });
    const put = vi.fn(async () => ({ pinned: false }));
    const navigate = vi.fn();
    render(<HomePage api={{ get, put } as unknown as APIClient} project={project} summary={{ project, metrics: { evidence: 2 } }} onNavigate={navigate} onCreateSpace={vi.fn()}/>);
    fireEvent.click(await screen.findByRole("button", { name: /1 项变化需要你确认/ }));
    expect(navigate).toHaveBeenCalledWith("review");
    expect(screen.getByText("Owner 固定的发布边界")).toBeInTheDocument();
    expect(screen.queryByText(/10% 重要度/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "取消固定 Owner 固定的发布边界" }));
    await waitFor(() => expect(put).toHaveBeenCalledWith("/v1/memories/m1/pin", { project_id: "p1", pinned: false }));
  });

  it("lets the owner pin a project memory without changing its content", async () => {
    const get = vi.fn(async (path: string) => {
      if (path.startsWith("/v1/layers")) return { layers: [{ count: 1 }, { count: 2 }, { count: 1 }, { count: 1 }, { count: 0 }, { count: 0 }], needs_review: 0 };
      if (path.startsWith("/v1/process/sources")) return { sources: [] };
      if (path.startsWith("/v1/memories") && path.includes("limit=8")) return { memories: [], total: 1 };
      if (path.startsWith("/v1/episodes") && path.includes("limit=8")) return { episodes: [], total: 0 };
      if (path.startsWith("/v1/memories")) return { memories: [{ memory_id: "m1", summary: "发布边界", body: "必须经过 Owner 确认。", status: "active", tier: "procedural", source_evidence_ids: ["ev-1"], confidence: .9 }], total: 1 };
      if (path.startsWith("/v1/memory-pins")) return { memories: [], total: 0 };
      throw new Error(`unexpected GET ${path}`);
    });
    const put = vi.fn(async () => ({ pinned: true }));
    const { container } = render(<MemoryPage api={{ get, put } as unknown as APIClient} projectID="p1" openRun={vi.fn()} mode="basic"/>);
    await screen.findByText("默认只整理新导入的原材料");
    const memoryTab = Array.from(container.querySelectorAll<HTMLButtonElement>(".memory-view-tabs button")).find((button) => button.textContent?.includes("项目记忆"));
    expect(memoryTab).toBeTruthy();
    fireEvent.click(memoryTab!);
    fireEvent.click(await screen.findByRole("button", { name: "固定到首页" }));
    await waitFor(() => expect(put).toHaveBeenCalledWith("/v1/memories/m1/pin", { project_id: "p1", pinned: true }));
    expect(screen.getByText(/这是你的选择，不使用 AI 重要度排序/)).toBeInTheDocument();
  });

  it("links the operation manual directly from health and recovery", async () => {
    const open = vi.fn();
    const get = vi.fn(async () => ({ status: "ok", version: "2.0.0", doctor: { status: "pass", checks: [] }, search: { consistent: true, indexed: 0 } }));
    render(<HealthPage api={{ get } as unknown as APIClient} onOpenManual={open}/>);
    fireEvent.click(await screen.findByRole("button", { name: "打开操作手册" }));
    expect(open).toHaveBeenCalledTimes(1);
  });
});
