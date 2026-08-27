import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { ReadableMarkdown, ReadableObject } from "./ReadableContent";

afterEach(cleanup);

describe("readable content", () => {
  it("renders headings, lists, tables and code as readable content", () => {
    render(<ReadableMarkdown content={"# 项目简报\n\n- 第一项\n- 第二项\n\n| 状态 | 数量 |\n| --- | --- |\n| 完成 | 3 |\n\n```json\n{\"ok\":true}\n```"}/>);
    expect(screen.getByRole("heading", { name: "项目简报" })).toBeInTheDocument();
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
    expect(screen.getByRole("table")).toHaveTextContent("完成");
    expect(screen.getByText('{"ok":true}')).toBeInTheDocument();
  });

  it("shows human fields first and keeps raw JSON behind an advanced disclosure", () => {
    render(<ReadableObject value={{ title: "可信记忆", body: "## 结论\n\n保留原始来源。", confidence: .95, object_id: "obj-1", content_hash: "secret-hash" }}/>);
    expect(screen.getByRole("heading", { name: "可信记忆" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "结论" })).toBeInTheDocument();
    expect(screen.getByText("95%")).toBeInTheDocument();
    const disclosure = screen.getByText("查看原始数据（高级）").closest("details");
    expect(disclosure).not.toHaveAttribute("open");
    expect(disclosure).toHaveTextContent("secret-hash");
  });
});
