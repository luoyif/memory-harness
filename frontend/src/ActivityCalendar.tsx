import { useEffect, useMemo, useRef, useState } from "react";
import { CalendarDays, CheckCircle2, FileInput, Sparkles } from "lucide-react";

export type ActivityCalendarTask = {
  task_id: string;
  title: string;
  status: string;
  priority: number;
  due_at?: string;
};

export type ActivityCalendarDay = {
  date: string;
  evidence: number;
  knowledge_units: number;
  episodes: number;
  memories: number;
  output: number;
  tasks_due: number;
  tasks_done: number;
  tasks: ActivityCalendarTask[];
};

export type ActivityCalendarData = {
  project_id: string;
  timezone: string;
  utc_offset: string;
  from: string;
  until: string;
  definition: string;
  days: ActivityCalendarDay[];
};

function dateLabel(value: string) {
  const parsed = new Date(`${value}T12:00:00`);
  return Number.isNaN(parsed.getTime())
    ? value
    : new Intl.DateTimeFormat("zh-CN", { month: "long", day: "numeric", weekday: "short" }).format(parsed);
}

function level(value: number, max: number) {
  if (value <= 0) return 0;
  return Math.max(1, Math.min(4, Math.ceil((value / Math.max(1, max)) * 4)));
}

export function ActivityCalendar({ data }: { data?: ActivityCalendarData }) {
  const days = useMemo(
    () => [...(data?.days || [])].sort((left, right) => left.date.localeCompare(right.date)),
    [data?.days],
  );
  const [selectedDate, setSelectedDate] = useState("");
  const heatmapViewport = useRef<HTMLDivElement>(null);
  const [visibleColumns, setVisibleColumns] = useState(0);
  useEffect(() => {
    if (days.length && !days.some((item) => item.date === selectedDate)) {
      setSelectedDate(days[days.length - 1].date);
    }
  }, [days, selectedDate]);
  useEffect(() => {
    const viewport = heatmapViewport.current;
    if (!viewport) return;
    const update = (width: number) => {
      // A cell is 20px wide with a 4px gap. Keep complete week columns and
      // discard only the oldest columns so the current week is always visible.
      setVisibleColumns(Math.max(1, Math.floor((Math.max(0, width) + 4) / 24)));
    };
    update(viewport.clientWidth);
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver((entries) => update(entries[0]?.contentRect.width || viewport.clientWidth));
    observer.observe(viewport);
    return () => observer.disconnect();
  }, []);
  const maxOutput = useMemo(() => Math.max(0, ...days.map((item) => item.output)), [days]);
  const selected = days.find((item) => item.date === selectedDate) || days[days.length - 1];
  const first = days[0]?.date ? new Date(`${days[0].date}T12:00:00`) : undefined;
  const leading = first && !Number.isNaN(first.getTime()) ? (first.getDay() + 6) % 7 : 0;
  const trailing = (7 - ((leading + days.length) % 7)) % 7;
  const cells = useMemo(
    () => [
      ...Array.from({ length: leading }, (_, index) => ({ day: undefined, index })),
      ...days.map((day, index) => ({ day, index: leading + index })),
      ...Array.from({ length: trailing }, (_, index) => ({
        day: undefined,
        index: leading + days.length + index,
      })),
    ],
    [days, leading, trailing],
  );
  const visibleCells = visibleColumns > 0 ? cells.slice(-visibleColumns * 7) : cells;
  const visibleFrom = visibleCells.find((cell) => cell.day)?.day?.date || days[0]?.date;
  const totalOutput = days.reduce((sum, item) => sum + item.output, 0);
  const activeDays = days.filter((item) => item.output > 0).length;
  const tasksDue = days.reduce((sum, item) => sum + item.tasks_due, 0);

  if (!days.length) return null;
  return (
    <section className="activity-calendar" aria-labelledby="activity-calendar-title">
      <header>
        <div>
          <p>近四周记忆活动</p>
          <h2 id="activity-calendar-title">每天留下了多少可用记忆</h2>
          <span>最右侧是今天；颜色越深，生成越多。有圆点表示当天有待办，点击日期可查看。</span>
        </div>
        <div className="activity-calendar-summary" aria-label="记忆活动汇总">
          <article><strong>{totalOutput}</strong><span>近四周记忆产出</span></article>
          <article><strong>{activeDays}</strong><span>有产出的天数</span></article>
          <article><strong>{tasksDue}</strong><span>日历中的待办</span></article>
        </div>
      </header>
      <div className="activity-calendar-body">
        <div className="activity-heatmap-wrap">
          <div className="activity-heatmap-row">
            <div className="activity-weekdays" aria-hidden="true"><span>一</span><span>三</span><span>五</span><span>日</span></div>
            <div className="activity-heatmap-viewport" ref={heatmapViewport}>
              <div className="activity-heatmap" role="grid" aria-label={`${visibleFrom} 至 ${data?.until} 的每日记忆产出，最右侧为当前周`}>
              {visibleCells.map((cell) => {
                const item = cell.day;
                if (!item) return <span className="activity-cell empty-cell" key={`empty-${cell.index}`} aria-hidden="true" />;
              const intensity = level(item.output, maxOutput);
              const label = `${dateLabel(item.date)}：记忆产出 ${item.output}，导入原材料 ${item.evidence}，到期待办 ${item.tasks_due}，完成待办 ${item.tasks_done}`;
              return <button
                type="button"
                role="gridcell"
                aria-label={label}
                aria-selected={selected?.date === item.date}
                title={label}
                className={`activity-cell intensity-${intensity}${item.tasks_due ? " has-task" : ""}${selected?.date === item.date ? " selected" : ""}`}
                key={item.date}
                onClick={() => setSelectedDate(item.date)}
              ><span>{item.output}</span>{item.tasks_due > 0 && <i aria-hidden="true" />}</button>;
            })}
              </div>
            </div>
          </div>
          <div className="activity-legend" aria-label="颜色图例"><span>少</span>{[0, 1, 2, 3, 4].map((item) => <i key={item} className={`intensity-${item}`} />)}<span>多（最高 {maxOutput}）</span><b>● 有待办</b></div>
        </div>
        {selected && <aside className="activity-day-detail" aria-live="polite">
          <header><CalendarDays size={18}/><div><strong>{dateLabel(selected.date)}</strong><span>{selected.output ? `生成 ${selected.output} 条记忆结果` : "当天没有新的记忆产出"}</span></div></header>
          <div className="activity-day-metrics">
            <span><FileInput size={14}/><b>{selected.evidence}</b> 份原材料</span>
            <span><Sparkles size={14}/><b>{selected.knowledge_units}</b> 条关键信息</span>
            <span><CheckCircle2 size={14}/><b>{selected.tasks_done}</b> 项完成</span>
          </div>
          <div className="activity-day-tasks">
            <strong>当天待办</strong>
            {selected.tasks.length ? selected.tasks.map((task) => <article key={task.task_id}><span>P{task.priority}</span><div><b>{task.title}</b><small>{task.status === "done" ? "已完成" : task.status === "suggested" ? "AI 建议待确认" : task.status === "in_progress" ? "正在处理" : "待开始"}</small></div></article>) : <p>当天没有设置到期待办。</p>}
          </div>
        </aside>}
      </div>
    </section>
  );
}
