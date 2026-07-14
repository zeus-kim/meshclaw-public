package main

import (
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/scheduler"
)

func TestFormatScheduleLastRun(t *testing.T) {
	if got := formatScheduleLastRun(time.Time{}); got != "never" {
		t.Fatalf("zero last run = %q, want never", got)
	}
	last := time.Date(2026, 5, 26, 6, 10, 0, 0, time.Local)
	if got := formatScheduleLastRun(last); got != "2026-05-26 06:10:00" {
		t.Fatalf("last run = %q", got)
	}
}

func TestFormatScheduleNextDue(t *testing.T) {
	if got := formatScheduleNextDue(time.Time{}, true); got != "now" {
		t.Fatalf("due next due = %q, want now", got)
	}
	if got := formatScheduleNextDue(time.Time{}, false); got != "unknown" {
		t.Fatalf("zero next due = %q, want unknown", got)
	}
	next := time.Date(2026, 5, 26, 8, 0, 0, 0, time.Local)
	if got := formatScheduleNextDue(next, false); got != "2026-05-26 08:00:00" {
		t.Fatalf("next due = %q", got)
	}
}

func TestFormatScheduleWhen(t *testing.T) {
	if got := formatScheduleWhen(scheduler.Job{Interval: "6h"}); got != "6h" {
		t.Fatalf("interval when = %q", got)
	}
	if got := formatScheduleWhen(scheduler.Job{DailyAt: "08:00"}); got != "daily@08:00" {
		t.Fatalf("daily when = %q", got)
	}
	if got := formatScheduleWhen(scheduler.Job{HourlyAt: "07"}); got != "hourly@07" {
		t.Fatalf("hourly when = %q", got)
	}
	if got := formatScheduleWhen(scheduler.Job{}); got != "manual" {
		t.Fatalf("manual when = %q", got)
	}
}

func TestFormatScheduleStatusSummary(t *testing.T) {
	line := formatScheduleStatusSummary("/tmp/schedule-state.json", 8, map[string]interface{}{
		"status":              "healthy",
		"due_count":           0,
		"next_due_job":        "mail-watch",
		"next_due_in_seconds": int64(300),
	})
	want := "schedule_state=/tmp/schedule-state.json status=healthy jobs=8 due=0 next=mail-watch in=300s"
	if line != want {
		t.Fatalf("summary line = %q, want %q", line, want)
	}
}

func TestFormatScheduleStatusSummaryShowsDueJobs(t *testing.T) {
	line := formatScheduleStatusSummary("/tmp/schedule-state.json", 8, map[string]interface{}{
		"status":              "backlog",
		"due_count":           2,
		"due_jobs":            []string{"assistant-watch", "argos-health"},
		"next_due_job":        "assistant-watch",
		"next_due_in_seconds": int64(0),
	})
	want := "schedule_state=/tmp/schedule-state.json status=backlog jobs=8 due=2 due_jobs=assistant-watch,argos-health next=assistant-watch in=0s"
	if line != want {
		t.Fatalf("summary line = %q, want %q", line, want)
	}
}

func TestScheduleStatusPayloadIncludesNextDue(t *testing.T) {
	now := time.Date(2026, 5, 26, 6, 0, 0, 0, time.UTC)
	plan := scheduler.Plan{Jobs: []scheduler.Job{{ID: "mail-watch", Kind: "mail_watch", Mode: "assistant", Interval: "15m", TargetID: "meshclaw-briefing", Enabled: true}}}
	state := scheduler.State{LastRun: map[string]time.Time{"mail-watch": now.Add(-10 * time.Minute)}}
	payload := scheduleStatusPayload(plan, state, nil, now)
	jobs, ok := payload["jobs"].([]scheduleJobStatus)
	if !ok || len(jobs) != 1 {
		t.Fatalf("jobs payload has unexpected type/value: %#v", payload["jobs"])
	}
	if jobs[0].Due || jobs[0].When != "15m" || !jobs[0].NextDue.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("unexpected job status: %#v", jobs[0])
	}
	if jobs[0].NextDueInSeconds != 300 || jobs[0].OverdueSeconds != 0 {
		t.Fatalf("unexpected timing seconds: %#v", jobs[0])
	}
	if dueCount, _ := payload["due_count"].(int); dueCount != 0 {
		t.Fatalf("due_count = %d, want 0", dueCount)
	}
	if dueJobs, ok := payload["due_jobs"].([]string); !ok || len(dueJobs) != 0 {
		t.Fatalf("due_jobs = %#v, want empty []string", payload["due_jobs"])
	}
	if payload["status"] != "healthy" || payload["next_due_job"] != "mail-watch" {
		t.Fatalf("unexpected schedule summary: %#v", payload)
	}
	if got, _ := payload["next_due_in_seconds"].(int64); got != 300 {
		t.Fatalf("next_due_in_seconds = %d, want 300", got)
	}
}

func TestScheduleStatusPayloadDueMeansNextDueNow(t *testing.T) {
	now := time.Date(2026, 5, 26, 6, 0, 0, 0, time.UTC)
	plan := scheduler.Plan{Jobs: []scheduler.Job{{ID: "mail-watch", Kind: "mail_watch", Mode: "assistant", Interval: "15m", Enabled: true}}}
	state := scheduler.State{LastRun: map[string]time.Time{"mail-watch": now.Add(-20 * time.Minute)}}
	payload := scheduleStatusPayload(plan, state, nil, now)
	jobs, ok := payload["jobs"].([]scheduleJobStatus)
	if !ok || len(jobs) != 1 {
		t.Fatalf("jobs payload has unexpected type/value: %#v", payload["jobs"])
	}
	if !jobs[0].Due || !jobs[0].NextDue.Equal(now) {
		t.Fatalf("due job should report next_due=now: %#v", jobs[0])
	}
	if jobs[0].NextDueInSeconds != 0 || jobs[0].OverdueSeconds != 300 {
		t.Fatalf("unexpected due timing seconds: %#v", jobs[0])
	}
	if dueCount, _ := payload["due_count"].(int); dueCount != 1 {
		t.Fatalf("due_count = %d, want 1", dueCount)
	}
	if dueJobs, ok := payload["due_jobs"].([]string); !ok || len(dueJobs) != 1 || dueJobs[0] != "mail-watch" {
		t.Fatalf("due_jobs = %#v, want mail-watch", payload["due_jobs"])
	}
	if payload["status"] != "backlog" || payload["next_due_job"] != "mail-watch" {
		t.Fatalf("unexpected due summary: %#v", payload)
	}
	if got, _ := payload["next_due_in_seconds"].(int64); got != 0 {
		t.Fatalf("next_due_in_seconds = %d, want 0", got)
	}
}

func TestScheduleStatusPayloadDailyWaitsUntilDailyAt(t *testing.T) {
	now := time.Date(2026, 5, 27, 0, 10, 0, 0, time.Local)
	plan := scheduler.Plan{Jobs: []scheduler.Job{
		{ID: "morning-briefing", Kind: "assistant_morning", Mode: "briefing", DailyAt: "08:00", Enabled: true},
		{ID: "daily-expert-audit", Kind: "expert_audit_handoff", Mode: "ops", DailyAt: "09:00", Enabled: true},
	}}
	state := scheduler.State{LastRun: map[string]time.Time{
		"morning-briefing":   now.AddDate(0, 0, -1),
		"daily-expert-audit": now.AddDate(0, 0, -1),
	}}
	payload := scheduleStatusPayload(plan, state, nil, now)
	if payload["status"] != "healthy" {
		t.Fatalf("daily jobs before DailyAt should not create backlog: %#v", payload)
	}
	if dueCount, _ := payload["due_count"].(int); dueCount != 0 {
		t.Fatalf("due_count = %d, want 0", dueCount)
	}
	if payload["next_due_job"] != "morning-briefing" {
		t.Fatalf("next_due_job = %#v, want morning-briefing", payload["next_due_job"])
	}
	jobs, ok := payload["jobs"].([]scheduleJobStatus)
	if !ok || len(jobs) != 2 {
		t.Fatalf("jobs payload has unexpected type/value: %#v", payload["jobs"])
	}
	for _, job := range jobs {
		if job.Due {
			t.Fatalf("daily job should not be due before DailyAt: %#v", job)
		}
	}
}

func TestFormatScheduleRunMessagePreview(t *testing.T) {
	got := formatScheduleRunMessagePreview(scheduler.RunResult{
		Message: "MeshClaw 데이터 상태 확인\n- state JSON: 16개 검사 / 깨진 파일 0개",
	})
	want := "message:\n  MeshClaw 데이터 상태 확인\n  - state JSON: 16개 검사 / 깨진 파일 0개\n"
	if got != want {
		t.Fatalf("preview = %q, want %q", got, want)
	}
	if got := formatScheduleRunMessagePreview(scheduler.RunResult{}); got != "" {
		t.Fatalf("empty message preview = %q, want empty", got)
	}
}
