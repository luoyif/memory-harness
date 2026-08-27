package dashboard

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/app"
)

const (
	defaultActivityDays = 371
	maxActivityDays     = 371
)

type ActivityTask struct {
	TaskID   string `json:"task_id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Priority int    `json:"priority"`
	DueAt    string `json:"due_at,omitempty"`
}

type ActivityDay struct {
	Date           string         `json:"date"`
	Evidence       int            `json:"evidence"`
	KnowledgeUnits int            `json:"knowledge_units"`
	Episodes       int            `json:"episodes"`
	Memories       int            `json:"memories"`
	Output         int            `json:"output"`
	TasksDue       int            `json:"tasks_due"`
	TasksDone      int            `json:"tasks_done"`
	Tasks          []ActivityTask `json:"tasks"`
}

type ActivityCalendar struct {
	ProjectID  string        `json:"project_id"`
	Timezone   string        `json:"timezone"`
	UTCOffset  string        `json:"utc_offset"`
	From       string        `json:"from"`
	Until      string        `json:"until"`
	Definition string        `json:"definition"`
	Days       []ActivityDay `json:"days"`
}

func boundedActivityDays(days int) int {
	if days <= 0 {
		return defaultActivityDays
	}
	if days < 28 {
		return 28
	}
	if days > maxActivityDays {
		return maxActivityDays
	}
	return days
}

func parseStoredTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, errors.New("empty timestamp")
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp %q", value)
}

func ReadProjectActivity(ctx context.Context, a *app.App, projectID string, now time.Time, days int) (ActivityCalendar, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return ActivityCalendar{}, errors.New("project_id is required")
	}
	if _, err := a.Portfolio.Project(ctx, projectID); err != nil {
		return ActivityCalendar{}, err
	}
	days = boundedActivityDays(days)
	localNow := now.In(time.Local)
	startLocal := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, localNow.Location()).AddDate(0, 0, -(days - 1))
	endLocal := startLocal.AddDate(0, 0, days)
	zone, offset := localNow.Zone()
	if strings.TrimSpace(zone) == "" {
		zone = "Local"
	}
	out := ActivityCalendar{
		ProjectID: projectID, Timezone: zone, UTCOffset: formatOffset(offset), From: startLocal.Format("2006-01-02"),
		Until: localNow.Format("2006-01-02"), Definition: "输出量 = 当天新生成的关键信息 + 会话复盘 + 长期记忆",
		Days: make([]ActivityDay, days),
	}
	byDate := map[string]*ActivityDay{}
	for index := range out.Days {
		date := startLocal.AddDate(0, 0, index).Format("2006-01-02")
		out.Days[index] = ActivityDay{Date: date, Tasks: []ActivityTask{}}
		byDate[date] = &out.Days[index]
	}
	startUTC := startLocal.UTC().Format(time.RFC3339Nano)
	endUTC := endLocal.UTC().Format(time.RFC3339Nano)
	type series struct {
		query string
		apply func(*ActivityDay)
	}
	seriesList := []series{
		{`SELECT er.captured_at FROM evidence_receipts er JOIN record_projects rp ON rp.record_type='evidence' AND rp.record_id=er.evidence_id WHERE rp.project_id=? AND er.captured_at>=? AND er.captured_at<?`, func(day *ActivityDay) { day.Evidence++ }},
		{`SELECT coalesce(u.processed_at,u.created_at) FROM knowledge_units u JOIN record_projects rp ON rp.record_type='knowledge_unit' AND rp.record_id=u.unit_id WHERE rp.project_id=? AND coalesce(u.processed_at,u.created_at)>=? AND coalesce(u.processed_at,u.created_at)<?`, func(day *ActivityDay) { day.KnowledgeUnits++ }},
		{`SELECT e.created_at FROM episodes e JOIN record_projects rp ON rp.record_type='episode' AND rp.record_id=e.episode_id WHERE rp.project_id=? AND e.created_at>=? AND e.created_at<?`, func(day *ActivityDay) { day.Episodes++ }},
		{`SELECT m.created_at FROM memory_records m JOIN record_projects rp ON rp.record_type='memory' AND rp.record_id=m.memory_id WHERE rp.project_id=? AND m.created_at>=? AND m.created_at<?`, func(day *ActivityDay) { day.Memories++ }},
	}
	for _, item := range seriesList {
		rows, err := a.Control.DB.QueryContext(ctx, item.query, projectID, startUTC, endUTC)
		if err != nil {
			return ActivityCalendar{}, err
		}
		for rows.Next() {
			var raw string
			if err := rows.Scan(&raw); err != nil {
				rows.Close()
				return ActivityCalendar{}, err
			}
			parsed, err := parseStoredTime(raw)
			if err != nil {
				rows.Close()
				return ActivityCalendar{}, err
			}
			if day := byDate[parsed.In(time.Local).Format("2006-01-02")]; day != nil {
				item.apply(day)
			}
		}
		if err := rows.Close(); err != nil {
			return ActivityCalendar{}, err
		}
	}
	for index := range out.Days {
		out.Days[index].Output = out.Days[index].KnowledgeUnits + out.Days[index].Episodes + out.Days[index].Memories
	}
	taskRows, err := a.Control.DB.QueryContext(ctx, `SELECT task_id,title,status,priority,coalesce(due_at,''),coalesce(completed_at,'') FROM project_tasks WHERE project_id=? AND ((due_at>=? AND due_at<?) OR (completed_at>=? AND completed_at<?)) ORDER BY due_at,priority,title`, projectID, startUTC, endUTC, startUTC, endUTC)
	if err != nil {
		return ActivityCalendar{}, err
	}
	defer taskRows.Close()
	for taskRows.Next() {
		var task ActivityTask
		var dueAt, completedAt string
		if err := taskRows.Scan(&task.TaskID, &task.Title, &task.Status, &task.Priority, &dueAt, &completedAt); err != nil {
			return ActivityCalendar{}, err
		}
		if dueAt != "" {
			parsed, err := parseStoredTime(dueAt)
			if err != nil {
				return ActivityCalendar{}, err
			}
			task.DueAt = dueAt
			if day := byDate[parsed.In(time.Local).Format("2006-01-02")]; day != nil {
				day.TasksDue++
				day.Tasks = append(day.Tasks, task)
			}
		}
		if completedAt != "" {
			parsed, err := parseStoredTime(completedAt)
			if err != nil {
				return ActivityCalendar{}, err
			}
			if day := byDate[parsed.In(time.Local).Format("2006-01-02")]; day != nil {
				day.TasksDone++
			}
		}
	}
	return out, taskRows.Err()
}
