import { Fragment, ReactNode } from "react";
import { Braces, ExternalLink } from "lucide-react";

const fieldLabels: Record<string, string> = {
  title: "标题",
  name: "名称",
  summary: "摘要",
  body: "正文",
  statement: "内容",
  description: "说明",
  purpose: "用途",
  intent: "目标",
  rationale: "理由",
  decision: "决定",
  status: "状态",
  domain: "主题",
  tier: "记忆类型",
  asset_form: "内容形式",
  visibility: "可见范围",
  confidence: "置信度",
  importance: "系统参考分",
  strength: "稳定程度",
  steps: "步骤",
  rules: "规则",
  constraints: "约束",
  inputs: "输入",
  outputs: "输出",
  examples: "示例",
};

const technicalKey = /(^|_)(id|ids|hash|version|plugin|run|stage|schema|created_at|updated_at|observed_at|recorded_at|source_refs?|scopes?)$/i;

function safeLink(value: string) {
  try {
    const url = new URL(value, window.location.origin);
    return ["http:", "https:", "mailto:"].includes(url.protocol) ? value : "";
  } catch {
    return "";
  }
}

function inline(text: string, keyPrefix: string): ReactNode[] {
  const pattern = /(\*\*[^*]+\*\*|`[^`]+`|\[[^\]]+\]\([^)]+\))/g;
  const nodes: ReactNode[] = [];
  let cursor = 0;
  for (const match of text.matchAll(pattern)) {
    const index = match.index || 0;
    if (index > cursor) nodes.push(text.slice(cursor, index));
    const token = match[0];
    const key = `${keyPrefix}-${index}`;
    if (token.startsWith("**")) {
      nodes.push(<strong key={key}>{token.slice(2, -2)}</strong>);
    } else if (token.startsWith("`")) {
      nodes.push(<code key={key}>{token.slice(1, -1)}</code>);
    } else {
      const parts = token.match(/^\[([^\]]+)\]\(([^)]+)\)$/);
      const href = safeLink(parts?.[2] || "");
      nodes.push(href ? <a key={key} href={href} target="_blank" rel="noreferrer">{parts?.[1]}<ExternalLink size={11}/></a> : token);
    }
    cursor = index + token.length;
  }
  if (cursor < text.length) nodes.push(text.slice(cursor));
  return nodes;
}

function isBlockStart(lines: string[], index: number) {
  const line = lines[index] || "";
  const next = lines[index + 1] || "";
  return /^(```|#{1,4}\s|>\s?|[-*+]\s+|\d+[.)]\s+|---+$)/.test(line.trim()) || (line.includes("|") && /^\s*\|?\s*:?-{3,}/.test(next));
}

export function ReadableMarkdown({ content, className = "" }: { content: string; className?: string }) {
  const lines = String(content || "").replace(/\r\n?/g, "\n").split("\n");
  const blocks: ReactNode[] = [];
  let index = 0;
  while (index < lines.length) {
    const raw = lines[index];
    const line = raw.trim();
    if (!line) { index += 1; continue; }
    if (line.startsWith("```")) {
      const language = line.slice(3).trim();
      const code: string[] = [];
      index += 1;
      while (index < lines.length && !lines[index].trim().startsWith("```")) code.push(lines[index++]);
      if (index < lines.length) index += 1;
      blocks.push(<div className="readable-code" key={`code-${index}`}><span>{language || "代码"}</span><pre><code>{code.join("\n")}</code></pre></div>);
      continue;
    }
    const heading = line.match(/^(#{1,4})\s+(.+)$/);
    if (heading) {
      const level = heading[1].length;
      const text = inline(heading[2], `heading-${index}`);
      blocks.push(level === 1 ? <h2 key={`h-${index}`}>{text}</h2> : level === 2 ? <h3 key={`h-${index}`}>{text}</h3> : <h4 key={`h-${index}`}>{text}</h4>);
      index += 1;
      continue;
    }
    if (/^---+$/.test(line)) { blocks.push(<hr key={`hr-${index}`}/>); index += 1; continue; }
    if (line.startsWith(">")) {
      const quote: string[] = [];
      while (index < lines.length && lines[index].trim().startsWith(">")) quote.push(lines[index++].trim().replace(/^>\s?/, ""));
      blocks.push(<blockquote key={`quote-${index}`}>{inline(quote.join(" "), `quote-${index}`)}</blockquote>);
      continue;
    }
    if (/^[-*+]\s+/.test(line)) {
      const items: string[] = [];
      while (index < lines.length && /^[-*+]\s+/.test(lines[index].trim())) items.push(lines[index++].trim().replace(/^[-*+]\s+/, ""));
      blocks.push(<ul key={`ul-${index}`}>{items.map((item, itemIndex) => <li key={itemIndex}>{inline(item, `ul-${index}-${itemIndex}`)}</li>)}</ul>);
      continue;
    }
    if (/^\d+[.)]\s+/.test(line)) {
      const items: string[] = [];
      while (index < lines.length && /^\d+[.)]\s+/.test(lines[index].trim())) items.push(lines[index++].trim().replace(/^\d+[.)]\s+/, ""));
      blocks.push(<ol key={`ol-${index}`}>{items.map((item, itemIndex) => <li key={itemIndex}>{inline(item, `ol-${index}-${itemIndex}`)}</li>)}</ol>);
      continue;
    }
    if (line.includes("|") && /^\s*\|?\s*:?-{3,}/.test((lines[index + 1] || "").trim())) {
      const split = (value: string) => value.trim().replace(/^\||\|$/g, "").split("|").map((cell) => cell.trim());
      const headers = split(line);
      index += 2;
      const rows: string[][] = [];
      while (index < lines.length && lines[index].includes("|") && lines[index].trim()) rows.push(split(lines[index++]));
      blocks.push(<div className="readable-table-wrap" key={`table-${index}`}><table><thead><tr>{headers.map((cell, cellIndex) => <th key={cellIndex}>{inline(cell, `th-${index}-${cellIndex}`)}</th>)}</tr></thead><tbody>{rows.map((row, rowIndex) => <tr key={rowIndex}>{headers.map((_, cellIndex) => <td key={cellIndex}>{inline(row[cellIndex] || "", `td-${index}-${rowIndex}-${cellIndex}`)}</td>)}</tr>)}</tbody></table></div>);
      continue;
    }
    const paragraph = [line];
    index += 1;
    while (index < lines.length && lines[index].trim() && !isBlockStart(lines, index)) paragraph.push(lines[index++].trim());
    blocks.push(<p key={`p-${index}`}>{inline(paragraph.join(" "), `p-${index}`)}</p>);
  }
  return <div className={`readable-markdown ${className}`.trim()}>{blocks.length ? blocks : <p>尚未填写内容</p>}</div>;
}

function readableValue(value: unknown, key: string): ReactNode {
  if (value === null || value === undefined || value === "") return <span className="readable-empty">未填写</span>;
  if (typeof value === "boolean") return value ? "是" : "否";
  if (typeof value === "number") return value >= 0 && value <= 1 ? `${Math.round(value * 100)}%` : String(value);
  if (typeof value === "string") return value.includes("\n") || value.length > 180 ? <ReadableMarkdown content={value}/> : <p>{inline(value, key)}</p>;
  if (Array.isArray(value)) return value.length ? <ul>{value.map((item, index) => <li key={index}>{typeof item === "object" ? readableValue(item, `${key}-${index}`) : String(item)}</li>)}</ul> : <span className="readable-empty">暂无</span>;
  return <div className="readable-nested">{Object.entries(value as Record<string, unknown>).map(([childKey, childValue]) => <div key={childKey}><strong>{fieldLabels[childKey] || childKey.replaceAll("_", " ")}</strong>{readableValue(childValue, `${key}-${childKey}`)}</div>)}</div>;
}

export function ReadableObject({ value }: { value: Record<string, unknown> }) {
  const leadKeys = new Set(["title", "name", "summary", "body", "statement"]);
  const entries = Object.entries(value).filter(([key, item]) => !leadKeys.has(key) && !technicalKey.test(key) && item !== null && item !== "");
  const title = String(value.title || value.name || "");
  const summary = String(value.summary || value.statement || "");
  const body = String(value.body || "");
  return <div className="readable-object">
    {title && <h3>{title}</h3>}
    {summary && summary !== title && <ReadableMarkdown content={summary}/>}
    {body && body !== summary && <ReadableMarkdown content={body}/>}
    {entries.length > 0 && <div className="readable-fields">{entries.map(([key, item]) => <section key={key}><span>{fieldLabels[key] || key.replaceAll("_", " ")}</span>{readableValue(item, key)}</section>)}</div>}
    <details className="raw-data-details"><summary><Braces size={14}/>查看原始数据（高级）</summary><pre>{JSON.stringify(value, null, 2)}</pre></details>
  </div>;
}

export function InlineFragments({ children }: { children: string }) {
  return <Fragment>{inline(children, "inline")}</Fragment>;
}
