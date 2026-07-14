package scheduler

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/assistantbrief"
	"github.com/meshclaw/meshclaw/internal/assistantwatch"
	"github.com/meshclaw/meshclaw/internal/datadoctor"
	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/mailadapter"
	"github.com/meshclaw/meshclaw/internal/monitor"
	"github.com/meshclaw/meshclaw/internal/osauto"
)

func evidenceRecordForTest(path string) evidence.Record {
	return evidence.Record{StoredAt: path}
}

func TestDueInterval(t *testing.T) {
	now := time.Date(2026, 5, 24, 7, 0, 0, 0, time.UTC)
	job := Job{ID: "serverops-quickcheck", Interval: "6h", Enabled: true}
	state := State{LastRun: map[string]time.Time{job.ID: now.Add(-7 * time.Hour)}}
	if !Due(job, state, now) {
		t.Fatalf("expected interval job to be due after interval")
	}
	state.LastRun[job.ID] = now.Add(-2 * time.Hour)
	if Due(job, state, now) {
		t.Fatalf("expected interval job not to be due before interval")
	}
}

func TestDueDaily(t *testing.T) {
	now := time.Date(2026, 5, 24, 10, 0, 0, 0, time.Local)
	job := Job{ID: "daily-expert-audit", DailyAt: "09:00", Enabled: true}
	state := State{LastRun: map[string]time.Time{job.ID: now.AddDate(0, 0, -1)}}
	if !Due(job, state, now) {
		t.Fatalf("expected daily job to be due after daily time on a new day")
	}
	state.LastRun[job.ID] = now.Add(-1 * time.Hour)
	if Due(job, state, now) {
		t.Fatalf("expected daily job not to be due on same day")
	}
}

func TestDueDailyWaitsUntilDailyAt(t *testing.T) {
	now := time.Date(2026, 5, 24, 7, 0, 0, 0, time.Local)
	job := Job{ID: "morning-briefing", DailyAt: "08:00", Enabled: true}
	state := State{LastRun: map[string]time.Time{job.ID: now.AddDate(0, 0, -1)}}
	if Due(job, state, now) {
		t.Fatalf("daily job should not be due before its daily_at time")
	}
	state.LastRun = map[string]time.Time{}
	if Due(job, state, now) {
		t.Fatalf("first run should also wait until daily_at time")
	}
}

func TestNextDueInterval(t *testing.T) {
	now := time.Date(2026, 5, 24, 7, 0, 0, 0, time.UTC)
	job := Job{ID: "serverops-quickcheck", Interval: "6h", Enabled: true}
	state := State{LastRun: map[string]time.Time{job.ID: now.Add(-2 * time.Hour)}}
	want := now.Add(4 * time.Hour)
	if got := NextDue(job, state, now); !got.Equal(want) {
		t.Fatalf("next due = %s, want %s", got, want)
	}
	state.LastRun[job.ID] = now.Add(-7 * time.Hour)
	if got := NextDue(job, state, now); !got.Equal(now) {
		t.Fatalf("overdue next due = %s, want now %s", got, now)
	}
}

func TestNextDueDaily(t *testing.T) {
	now := time.Date(2026, 5, 24, 7, 0, 0, 0, time.Local)
	job := Job{ID: "morning-briefing", DailyAt: "08:00", Enabled: true}
	state := State{LastRun: map[string]time.Time{job.ID: now.Add(-1 * time.Hour)}}
	got := NextDue(job, state, now)
	if got.Hour() != 8 || got.Minute() != 0 {
		t.Fatalf("next due hour/minute = %s", got)
	}
	after := time.Date(2026, 5, 24, 9, 0, 0, 0, time.Local)
	state.LastRun[job.ID] = after.AddDate(0, 0, -1)
	got = NextDue(job, state, after)
	if !got.Equal(after) {
		t.Fatalf("missed daily job should be due now: got=%s now=%s", got, after)
	}
	state.LastRun[job.ID] = after
	got = NextDue(job, state, after)
	if !got.After(after) || got.Day() != 25 {
		t.Fatalf("already-run daily job should be due tomorrow: got=%s now=%s", got, after)
	}
}

func TestDueHourlyWaitsUntilMinute(t *testing.T) {
	now := time.Date(2026, 5, 24, 7, 6, 0, 0, time.Local)
	job := Job{ID: "local-ai-briefing", HourlyAt: "07", Enabled: true}
	state := State{LastRun: map[string]time.Time{}}
	if Due(job, state, now) {
		t.Fatalf("hourly job should wait until minute 07")
	}
	now = time.Date(2026, 5, 24, 7, 7, 0, 0, time.Local)
	if !Due(job, state, now) {
		t.Fatalf("hourly job should be due at minute 07")
	}
	state.LastRun[job.ID] = now
	if Due(job, state, now.Add(10*time.Minute)) {
		t.Fatalf("hourly job should not run twice in the same hourly slot")
	}
	if !Due(job, state, now.Add(time.Hour)) {
		t.Fatalf("hourly job should run in the next hourly slot")
	}
}

func TestNextDueHourly(t *testing.T) {
	job := Job{ID: "local-ai-briefing", HourlyAt: "07", Enabled: true}
	now := time.Date(2026, 5, 24, 7, 1, 0, 0, time.Local)
	state := State{LastRun: map[string]time.Time{}}
	got := NextDue(job, state, now)
	if got.Hour() != 7 || got.Minute() != 7 {
		t.Fatalf("next hourly due = %s, want 07:07", got)
	}
	now = time.Date(2026, 5, 24, 7, 30, 0, 0, time.Local)
	got = NextDue(job, state, now)
	if !got.Equal(now) {
		t.Fatalf("missed hourly job should be due now: got=%s now=%s", got, now)
	}
	state.LastRun[job.ID] = now
	got = NextDue(job, state, now)
	if got.Hour() != 8 || got.Minute() != 7 {
		t.Fatalf("already-run hourly job should be due next hour: got=%s", got)
	}
}

func TestAssistantWatchOnlySendsOnMatch(t *testing.T) {
	result := RunResult{
		Job:     Job{Kind: "assistant_watch"},
		Payload: AssistantWatchPayload{Result: assistantwatch.CheckResult{}},
	}
	if shouldSendScheduleResult(result) {
		t.Fatalf("assistant watch should not send when there are no matched observations")
	}
	result.Payload = AssistantWatchPayload{Result: assistantwatch.CheckResult{
		Matched: []assistantwatch.PriceObservation{{Query: "러닝 벨트", Matched: true}},
	}}
	if !shouldSendScheduleResult(result) {
		t.Fatalf("assistant watch should send when a price matches")
	}
}

func TestMailWatchOnlySendsOnMessagesOrErrors(t *testing.T) {
	result := RunResult{
		Job:     Job{Kind: "mail_watch"},
		Success: true,
		Payload: MailWatchPayload{Results: []mailadapter.WatchResult{{Messages: nil}}},
	}
	if shouldSendScheduleResult(result) {
		t.Fatalf("mail watch should stay quiet with no new messages")
	}
	result.Payload = MailWatchPayload{Errors: []string{"account: login failed"}}
	if !shouldSendScheduleResult(result) {
		t.Fatalf("mail watch should send when there are account errors")
	}
	result.Payload = MailWatchPayload{Results: []mailadapter.WatchResult{{Messages: []mailadapter.MessageSummary{{ID: "1", Subject: "hello"}}}}}
	if !shouldSendScheduleResult(result) {
		t.Fatalf("mail watch should send when there are new messages")
	}
	result.Success = false
	result.Payload = MailWatchPayload{}
	if !shouldSendScheduleResult(result) {
		t.Fatalf("mail watch should send when the job failed")
	}
}

func TestFilterNewMailMessagesSkipsSeenIDs(t *testing.T) {
	state := MailWatchState{Seen: map[string][]string{"acct": []string{"1"}}}
	filtered, skipped := filterNewMailMessages(state, "acct", []mailadapter.MessageSummary{
		{ID: "1", Subject: "old"},
		{ID: "2", Subject: "new"},
	})
	if skipped != 1 || len(filtered) != 1 || filtered[0].ID != "2" {
		t.Fatalf("filtered=%#v skipped=%d", filtered, skipped)
	}
	filtered, skipped = filterNewMailMessages(state, "acct", []mailadapter.MessageSummary{{ID: "2", Subject: "new again"}})
	if skipped != 1 || len(filtered) != 0 {
		t.Fatalf("second run filtered=%#v skipped=%d", filtered, skipped)
	}
}

func TestMailWatchStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_MAIL_WATCH_STATE", filepath.Join(dir, "mail-watch-state.json"))
	state := MailWatchState{
		Seen:   map[string][]string{"acct": []string{"1", "2"}},
		Errors: map[string]string{"acct": "acct: login failed"},
	}
	if err := SaveMailWatchState(state); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadMailWatchState()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(loaded.Seen["acct"], ","); got != "1,2" {
		t.Fatalf("loaded seen = %q", got)
	}
	if loaded.Errors["acct"] != "acct: login failed" {
		t.Fatalf("loaded errors = %#v", loaded.Errors)
	}
}

func TestArgosHealthOnlySendsOnProblem(t *testing.T) {
	result := RunResult{
		Job:     Job{Kind: "argos_health"},
		Success: true,
		Payload: ArgosHealthPayload{Doctor: osauto.ArgosMacDoctorReport{OK: true}},
	}
	if shouldSendScheduleResult(result) {
		t.Fatalf("argos health should stay quiet when the doctor report is ok")
	}
	result.Payload = ArgosHealthPayload{Doctor: osauto.ArgosMacDoctorReport{
		OK:       false,
		Problems: []string{"Accessibility permission is missing"},
	}}
	if !shouldSendScheduleResult(result) {
		t.Fatalf("argos health should send when the doctor report has a problem")
	}
	result.Success = false
	result.Payload = ArgosHealthPayload{Doctor: osauto.ArgosMacDoctorReport{OK: true}}
	if !shouldSendScheduleResult(result) {
		t.Fatalf("argos health should send when the job failed")
	}
}

func TestLocalHygieneOnlySendsOnAttentionNeeded(t *testing.T) {
	result := RunResult{
		Job:     Job{Kind: "local_hygiene"},
		Success: true,
		Payload: LocalHygienePayload{RemovedBytes: 1024},
	}
	if shouldSendScheduleResult(result) {
		t.Fatalf("local hygiene should stay quiet for small routine cleanup")
	}
	result.Payload = LocalHygienePayload{Warnings: []string{"log directory is large"}}
	if !shouldSendScheduleResult(result) {
		t.Fatalf("local hygiene should send when there are warnings")
	}
	result.Payload = LocalHygienePayload{Skipped: []string{"/tmp/locked"}}
	if !shouldSendScheduleResult(result) {
		t.Fatalf("local hygiene should send when cleanup skipped files")
	}
	result.Payload = LocalHygienePayload{RemovedBytes: 60 * 1024 * 1024}
	if !shouldSendScheduleResult(result) {
		t.Fatalf("local hygiene should send when a large cleanup happened")
	}
	result.Success = false
	result.Payload = LocalHygienePayload{}
	if !shouldSendScheduleResult(result) {
		t.Fatalf("local hygiene should send when the job failed")
	}
}

func TestDataDoctorNeverPushesRawCountersToSignal(t *testing.T) {
	result := RunResult{
		Job:     Job{Kind: "data_doctor"},
		Success: true,
		Payload: datadoctor.Report{OK: true},
	}
	if shouldSendScheduleResult(result) {
		t.Fatalf("data doctor should stay quiet when healthy")
	}
	result.Payload = datadoctor.Report{OK: false, Warnings: []string{"evidence files are growing"}}
	if shouldSendScheduleResult(result) {
		t.Fatalf("data doctor should keep warnings in evidence instead of Signal")
	}
	result.Success = false
	result.Payload = datadoctor.Report{OK: true}
	if shouldSendScheduleResult(result) {
		t.Fatalf("data doctor failures should be inspected from evidence, not raw Signal counters")
	}
}

func TestLocalAIBriefingOwnsSignalDelivery(t *testing.T) {
	result := RunResult{
		Job:     Job{Kind: "local_ai_briefing", TargetID: "argos-briefing"},
		Success: true,
		Payload: LocalAIBriefingPayload{Execute: true, TargetID: "argos-briefing"},
	}
	if shouldSendScheduleResult(result) {
		t.Fatalf("scheduler should not send a second Signal message for local-ai briefing")
	}
}

func TestFormatScheduleMessageLocalAIBriefingUsesResultSummaryNoLogs(t *testing.T) {
	stdout := strings.Join([]string{
		"[briefing 14:08:39] === LOCAL-AI daily briefing start ===",
		"[briefing 14:08:42] news: pool=78 balanced={'한국': 3}",
		"[briefing 14:08:59] calling local model gpt-oss:20b ...",
		`{"markdown":"/Users/argos/.meshclaw/briefings/briefing.md","html":"/Users/argos/.meshclaw/briefings/briefing.html","obsidian_note":"/Users/argos/Documents/Obsidian Vault/Daily Briefings/Argos.md","target":"argos-briefing","model":"gpt-oss:20b","signal_parts":1,"signal_chars":1878,"counts":{"news":12,"news_by_region":{"한국":3,"미국":3,"글로벌":5,"아시아":1},"mail":5,"servers_online":15,"servers_total":17}}`,
	}, "\n")
	msg := formatScheduleMessage(RunResult{
		Job:      Job{Kind: "local_ai_briefing", TargetID: "argos-briefing"},
		Status:   "ok",
		Success:  true,
		Summary:  "local-ai briefing sent",
		Payload:  LocalAIBriefingPayload{Execute: true, TargetID: "argos-briefing", Stdout: stdout},
		Evidence: evidenceRecordForTest("/tmp/local-ai.json"),
	})
	for _, want := range []string{
		"로컬 AI 브리핑을 만들고 보고방 전송까지 마쳤습니다.",
		"실행 결과:",
		"뉴스 12건, 메일 5건, 서버 15/17대 온라인",
		"뉴스 구성: 한국 3건, 미국 3건, 글로벌 5건, 아시아 1건",
		"Signal 보고방 본문 1개로 전송했습니다.",
		"Markdown 보고서를 첨부했습니다.",
		"Obsidian 문서도 저장했습니다.",
		"최근 보고와 상태는 대시보드",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("local-ai message missing %q:\n%s", want, msg)
		}
	}
	for _, blocked := range []string{"실행 로그", "news: pool", "calling local model", "gpt-oss", "/Users/argos", "Report:", "/tmp/local-ai.json"} {
		if strings.Contains(msg, blocked) {
			t.Fatalf("local-ai message leaked internal text %q:\n%s", blocked, msg)
		}
	}
}

func TestAssistantAutoCheckStaysQuietWhenHealthy(t *testing.T) {
	result := RunResult{
		Job:     Job{Kind: "assistant_auto_check", TargetID: "argos-assistant"},
		Status:  "ok",
		Success: true,
		Summary: "assistant auto-check: profile=ok memory=ok skills=8 layers=2",
		Payload: AssistantAutoCheckPayload{
			ProfileStatus: "ok",
			SkillCount:    8,
			MemoryStatus:  "ok",
			MemoryLayers:  []string{"procedural", "ops"},
			SelfTestLine:  "요약: OK 10개, 주의 0개",
		},
		Evidence: evidenceRecordForTest("/tmp/assistant-auto-check.json"),
	}
	if shouldSendScheduleResult(result) {
		t.Fatalf("healthy assistant auto-check should stay quiet in Signal and remain evidence/dashboard-only")
	}
	msg := formatScheduleMessage(result)
	for _, want := range []string{"Argos 비서 자동 점검입니다.", "현재 상태는 정상", "비서 설정", "정체성, 말투, 안전 규칙", "뉴스, 날씨, 일정, 메일, 지도, Mac 작업", "장기/단기 기억", "대표 기능을 시험", "OK 10개, 주의 0개", "파일 삭제, 결제, 메일/Signal 발송, 서버 재시작이나 배포는 하지 않았습니다", "최근 보고와 상태는 대시보드"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("assistant auto-check message missing %q:\n%s", want, msg)
		}
	}
	for _, blocked := range []string{"assistant auto-check:", "profile=ok", "layers=2", "프로필은", "메모리 계층", "MeshClaw 자동 작업 결과", "/tmp/assistant-auto-check.json"} {
		if strings.Contains(msg, blocked) {
			t.Fatalf("assistant auto-check message leaked internal text %q:\n%s", blocked, msg)
		}
	}
}

func TestAssistantAutoCheckSendsWhenWarning(t *testing.T) {
	result := RunResult{
		Job:     Job{Kind: "assistant_auto_check", TargetID: "argos-assistant"},
		Status:  "ok",
		Success: true,
		Payload: AssistantAutoCheckPayload{
			ProfileStatus: "ok",
			SkillCount:    8,
			MemoryStatus:  "ok",
			SelfTestLine:  "요약: OK 9개, 주의 1개",
		},
	}
	if !shouldSendScheduleResult(result) {
		t.Fatalf("assistant auto-check should send when warnings appear")
	}
	result.Payload = AssistantAutoCheckPayload{
		ProfileStatus: "warn",
		SkillCount:    8,
		MemoryStatus:  "ok",
		SelfTestLine:  "요약: OK 10개, 주의 0개",
	}
	if !shouldSendScheduleResult(result) {
		t.Fatalf("assistant auto-check should send when profile is not ok")
	}
}

func TestAssistantAutoCheckDefaultPlanRunsEightTimesDaily(t *testing.T) {
	plan := DefaultPlan()
	for _, job := range plan.Jobs {
		if job.ID == "assistant-auto-check" {
			if job.Interval != "3h" || job.TargetID != "argos-assistant" {
				t.Fatalf("assistant-auto-check interval=%q target=%q", job.Interval, job.TargetID)
			}
			return
		}
	}
	t.Fatal("assistant-auto-check job missing")
}

func TestFormatScheduleMessageAssistantWatchNoEnglishSummary(t *testing.T) {
	msg := formatScheduleMessage(RunResult{
		Job:      Job{ID: "assistant-watch", Kind: "assistant_watch"},
		Status:   "ok",
		Success:  true,
		Summary:  "assistant watch: total=3 due=0 matched=0 links=0",
		Payload:  AssistantWatchPayload{Result: assistantwatch.CheckResult{Total: 3}},
		Evidence: evidenceRecordForTest("/tmp/assistant-watch.json"),
	})
	for _, want := range []string{"등록해 둔 사용자 알림 3개를 확인했습니다.", "조건에 맞은 새 항목은 없어서 따로 보낼 알림은 없습니다.", "다음에 할 일:", "비서방에서 알림 조건을 수정", "최근 보고와 상태는 대시보드"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("assistant watch message missing %q:\n%s", want, msg)
		}
	}
	for _, blocked := range []string{"사용자 알림 감시 결과", "등록된 알림:", "assistant watch:", "total=", "matched=", "watch_link_", "/tmp/assistant-watch.json"} {
		if strings.Contains(msg, blocked) {
			t.Fatalf("assistant watch message leaked English/internal text %q:\n%s", blocked, msg)
		}
	}
}

func TestFormatScheduleMessageExpertAuditNoEnglishSummary(t *testing.T) {
	msg := formatScheduleMessage(RunResult{
		Job:      Job{ID: "daily-expert-audit", Kind: "expert_audit_handoff"},
		Status:   "ok",
		Success:  true,
		Summary:  "daily expert audit handoff created for Codex/Claude",
		Payload:  ExpertAuditPayload{ServerOnline: 2, ServerTotal: 3, Alerts: 1, OpenWebUIFailures: 0, RecentEvidence: 8},
		Evidence: evidenceRecordForTest("/tmp/expert.json"),
	})
	for _, want := range []string{
		"오늘 심층 점검 결과입니다.",
		"서버 상태: 2/3대가 온라인이고, 경고는 1개입니다.",
		"Open WebUI 경로: 실패 0개",
		"최근 작업 기록: 최근 8개 기록",
		"자동 실행 결과: 삭제, 결제, 메일/Signal 발송, 서버 재시작, 배포는 0건입니다.",
		"최근 보고와 상태는 대시보드",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expert audit message missing %q:\n%s", want, msg)
		}
	}
	for _, blocked := range []string{"daily expert audit handoff", "MeshClaw 자동 작업 결과", "상태: 정상", "요약:", "/tmp/expert.json"} {
		if strings.Contains(msg, blocked) {
			t.Fatalf("expert audit message leaked generic/internal text %q:\n%s", blocked, msg)
		}
	}
}

func TestFormatScheduleMessageDataDoctorKorean(t *testing.T) {
	msg := formatScheduleMessage(RunResult{
		Job:     Job{ID: "data-doctor", Kind: "data_doctor"},
		Status:  "failed",
		Success: false,
		Payload: datadoctor.Report{
			OK:       false,
			Warnings: []string{"evidence files exceed 5000; keep for audit, but add archival policy"},
			Locations: []datadoctor.Location{
				{ID: "public_argos", Files: 2, Bytes: 1024},
				{ID: "logs", Files: 3, Bytes: 2048},
			},
			Evidence:   datadoctor.EvidenceState{Files: 6000, Bytes: 4096},
			StateFiles: []datadoctor.StateFile{{ID: "state", Exists: true, OK: true}},
		},
		Evidence: evidenceRecordForTest("/tmp/data-doctor.json"),
	})
	for _, want := range []string{"MeshClaw 데이터 보관 상태를 확인했습니다.", "확인할 점이 1개 있습니다.", "공개 산출물에는 2개 파일", "로그에는 3개 파일", "자동 작업 보관 파일은 6000개", "자동 작업 보관 파일이 기준치 5000개를 넘었습니다", "비서방에서 보관 계획 후보를 먼저 확인", "상태 JSON은 1개를 검사했고"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("data doctor message missing %q:\n%s", want, msg)
		}
	}
	for _, blocked := range []string{"상태: 확인 필요", "ok=false", "warnings=", "public_argos", "evidence files exceed", "기록:", "증거 기록", "증거 파일", "archive plan"} {
		if strings.Contains(msg, blocked) {
			t.Fatalf("data doctor message leaked English/internal text %q:\n%s", blocked, msg)
		}
	}
}

func TestScheduleWatchMessageHidesLoopbackLink(t *testing.T) {
	message := formatScheduleMessage(RunResult{
		Job: Job{ID: "assistant-watch", Kind: "assistant_watch"},
		Payload: AssistantWatchPayload{Result: assistantwatch.CheckResult{
			Matched: []assistantwatch.PriceObservation{{
				Query:       "테스트 상품",
				URL:         "http://127.0.0.1:18088/",
				FoundAmount: 29900,
				Matched:     true,
			}},
		}},
	})
	if strings.Contains(message, "http://127.0.0.1") {
		t.Fatalf("loopback URL should not be sent to Signal:\n%s", message)
	}
	if !strings.Contains(message, "iPhone에서 열 수 없습니다") {
		t.Fatalf("expected iPhone-safe explanation:\n%s", message)
	}
}

func TestFormatServerOpsMessageNaturalKorean(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	previous := RunResult{
		Job:     Job{ID: "serverops-quickcheck", Kind: "serverops_quickcheck"},
		Success: true,
		Payload: ServerOpsPayload{
			States: map[string]*monitor.NodeState{
				"d1": {Name: "d1", Online: true, CPU: 0, Memory: 4, Disk: 86},
				"s1": {Name: "s1", Online: false},
				"s2": {Name: "s2", Online: false},
			},
			Alerts: []monitor.Alert{
				{Level: "critical", Node: "s1", Type: "offline", Message: "Node s1 is offline"},
				{Level: "critical", Node: "s2", Type: "offline", Message: "Node s2 is offline"},
				{Level: "warning", Node: "d1", Type: "disk", Message: "Disk usage high: 86.0%"},
			},
		},
	}
	if _, err := evidence.Store("schedule-serverops_quickcheck", "ops", "previous", previous); err != nil {
		t.Fatal(err)
	}
	msg := formatScheduleMessage(RunResult{
		Job:     Job{ID: "serverops-quickcheck", Kind: "serverops_quickcheck"},
		Success: true,
		Payload: ServerOpsPayload{
			States: map[string]*monitor.NodeState{
				"d1": {Name: "d1", Online: true, CPU: 0, Memory: 4, Disk: 87},
				"s1": {Name: "s1", Online: false},
				"s2": {Name: "s2", Online: false},
			},
			Alerts: []monitor.Alert{
				{Level: "critical", Node: "s1", Type: "offline", Message: "Node s1 is offline"},
				{Level: "critical", Node: "s2", Type: "offline", Message: "Node s2 is offline"},
				{Level: "warning", Node: "d1", Type: "disk", Message: "Disk usage high: 87.0%"},
			},
		},
	})
	for _, want := range []string{
		"전체적으로 3대 중 1대가 정상 가동 중입니다.",
		"이전 보고와 비교하면",
		"오프라인 노드는 이전과 동일하게 s1, s2입니다",
		"d1 디스크는 87.0%로 이전보다 1.0%p 늘었습니다",
		"s1은 노드가 오프라인입니다.",
		"d1은 디스크 사용량이 높습니다: 87.0%.",
		"다음에 할 일",
		"s1, s2는 계속 오프라인이면 전원, 네트워크, Tailscale, SSH 접속 상태를 확인하세요.",
		"조치가 필요하면 보고방이 아니라 비서방에서 요청하고 승인하세요.",
		"d1은 CPU 0%, 메모리 4%, 디스크 87%를 사용 중입니다.",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("serverops message missing %q:\n%s", want, msg)
		}
	}
	for _, blocked := range []string{"Node s1 is offline", "Disk usage high", "status=ok", "serverops quickcheck"} {
		if strings.Contains(msg, blocked) {
			t.Fatalf("serverops message leaked internal/English text %q:\n%s", blocked, msg)
		}
	}
}

func TestDefaultPlanIncludesArgosHealthAndHygiene(t *testing.T) {
	plan := DefaultPlan()
	ids := map[string]bool{}
	targets := map[string]string{}
	enabled := map[string]bool{}
	for _, job := range plan.Jobs {
		ids[job.ID] = true
		targets[job.ID] = job.TargetID
		enabled[job.ID] = job.Enabled
	}
	for _, want := range []string{"local-ai-briefing", "argos-health", "assistant-auto-check", "local-hygiene", "data-doctor"} {
		if !ids[want] {
			t.Fatalf("missing scheduled job %q in %#v", want, plan.Jobs)
		}
	}
	if targets["local-ai-briefing"] != "argos-briefing" || targets["mail-watch"] != "argos-briefing" || targets["assistant-watch"] != "argos-briefing" || targets["argos-health"] != "argos-briefing" || targets["assistant-auto-check"] != "argos-assistant" || targets["serverops-quickcheck"] != "argos-ops" {
		t.Fatalf("schedule targets should use current Argos rooms: %#v", targets)
	}
	if !enabled["local-ai-briefing"] || enabled["morning-briefing"] {
		t.Fatalf("local-ai briefing should be the enabled report path and morning briefing should be fallback only: %#v", enabled)
	}
	if targets["data-doctor"] != "" {
		t.Fatalf("data-doctor should not have a Signal target by default: %#v", targets)
	}
}

func TestRunLocalHygieneRemovesOnlyEphemeralOldFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	now := time.Date(2026, 5, 26, 4, 0, 0, 0, time.UTC)
	publicDir := filepath.Join(home, ".meshclaw", "public", "argos")
	evidenceDir := filepath.Join(home, ".meshclaw", "evidence", "2026-05-25")
	if err := os.MkdirAll(publicDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(evidenceDir, 0700); err != nil {
		t.Fatal(err)
	}
	oldPublic := filepath.Join(publicDir, "news-old.html")
	index := filepath.Join(publicDir, "index.html")
	evidenceFile := filepath.Join(evidenceDir, "record.json")
	for _, path := range []string{oldPublic, index, evidenceFile} {
		if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, now.Add(-72*time.Hour), now.Add(-72*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	payloadAny, summary, err := runLocalHygiene(now)
	if err != nil {
		t.Fatal(err)
	}
	payload := payloadAny.(LocalHygienePayload)
	if !strings.Contains(summary, "removed=1") || len(payload.RemovedFiles) != 1 {
		t.Fatalf("summary=%q payload=%#v", summary, payload)
	}
	if _, err := os.Stat(oldPublic); !os.IsNotExist(err) {
		t.Fatalf("old public file should be removed, err=%v", err)
	}
	for _, path := range []string{index, evidenceFile} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to remain: %v", path, err)
		}
	}
}

func TestRunLocalHygieneCapsRecentEphemeralFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	now := time.Date(2026, 5, 26, 4, 0, 0, 0, time.UTC)
	publicDir := filepath.Join(home, ".meshclaw", "public", "argos")
	doctorDir := filepath.Join(home, ".meshclaw", "doctor")
	for _, dir := range []string{publicDir, doctorDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	index := filepath.Join(publicDir, "index.html")
	if err := os.WriteFile(index, []byte("index"), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 105; i++ {
		path := filepath.Join(publicDir, "news-"+strconv.Itoa(i)+".html")
		if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
		mod := now.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(path, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 25; i++ {
		path := filepath.Join(doctorDir, "recording-"+strconv.Itoa(i)+".mp4")
		if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
		mod := now.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(path, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	payloadAny, _, err := runLocalHygiene(now)
	if err != nil {
		t.Fatal(err)
	}
	payload := payloadAny.(LocalHygienePayload)
	if payload.PublicFiles != 101 {
		t.Fatalf("public files = %d, want 101 including index", payload.PublicFiles)
	}
	if files, _ := countFiles(doctorDir); files != 20 {
		t.Fatalf("doctor files = %d, want 20", files)
	}
	if _, err := os.Stat(index); err != nil {
		t.Fatalf("index should remain: %v", err)
	}
	if len(payload.RemovedFiles) != 10 {
		t.Fatalf("removed files = %d, want 10", len(payload.RemovedFiles))
	}
}

func TestFormatScheduleMessageLocalHygiene(t *testing.T) {
	msg := formatScheduleMessage(RunResult{
		Job:     Job{ID: "local-hygiene", Kind: "local_hygiene"},
		Summary: "local hygiene ok",
		Payload: LocalHygienePayload{
			RemovedFiles:  []LocalHygieneRemoved{{Path: "/tmp/a", Bytes: 10}},
			RemovedBytes:  10,
			PublicFiles:   2,
			EvidenceFiles: 3,
			LogBytes:      4096,
			Warnings:      []string{"evidence files are growing: 10771 files; keep for audit, but consider archival policy"},
		},
	})
	for _, want := range []string{"MeshClaw 로컬 정리를 실행해서 임시 파일 1개", "현재 공개 산출물은 2개", "자동 작업 보관 파일은 3개", "확인할 점: 자동 작업 보관 파일이 늘어나고 있습니다: 10771개", "비서방에서 보관 계획을 먼저 요청"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("hygiene message missing %q:\n%s", want, msg)
		}
	}
	for _, blocked := range []string{"MeshClaw 로컬 정리 점검 완료", "정리:", "상태:", "주의:", "MeshClaw local hygiene", "evidence files are growing", "keep for audit", "증거 기록", "증거 파일"} {
		if strings.Contains(msg, blocked) {
			t.Fatalf("hygiene message should not expose English operational text %q:\n%s", blocked, msg)
		}
	}
}

func TestFormatScheduleMessageMorningBriefingKorean(t *testing.T) {
	msg := formatScheduleMessage(RunResult{
		Job:     Job{ID: "morning-briefing", Kind: "assistant_morning"},
		Status:  "ok",
		Summary: "morning briefing generated: news=64 errors=0",
		Payload: assistantbrief.Brief{Text: "오늘 브리핑 본문"},
	})
	for _, want := range []string{"아침 브리핑을 준비했습니다.", "오늘 브리핑 본문"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("morning briefing message missing %q:\n%s", want, msg)
		}
	}
	for _, blocked := range []string{"MeshClaw scheduled job", "status=ok", "morning briefing generated"} {
		if strings.Contains(msg, blocked) {
			t.Fatalf("morning briefing should not expose English schedule text %q:\n%s", blocked, msg)
		}
	}
}

func TestFormatScheduleMessageMailWatch(t *testing.T) {
	msg := formatScheduleMessage(RunResult{
		Job:     Job{ID: "mail-watch", Kind: "mail_watch"},
		Summary: "mail watch ok",
		Payload: MailWatchPayload{
			Results: []mailadapter.WatchResult{{
				Account: mailadapter.AccountPublic{ID: "operator", Email: "operator@example.com"},
				Messages: []mailadapter.MessageSummary{{
					From:      "sender@example.com",
					Subject:   "hello",
					Snippet:   "짧은 안내 본문입니다.",
					HasAttach: true,
				}},
			}},
			Errors: []string{"fools: secret not found"},
		},
		Evidence: evidenceRecordForTest("/tmp/mail-watch.json"),
	})
	for _, want := range []string{
		"새 메일이 1개 들어왔습니다.",
		"일부 메일 계정은 확인이 필요합니다.",
		"1. hello",
		"보낸 사람: sender@example.com",
		"계정: operator@example.com",
		"미리보기: 짧은 안내 본문입니다.",
		"첨부 파일이 있습니다.",
		"fools 계정의 저장된 암호 또는 Keychain 접근 권한을 확인해야 합니다.",
		"비서방에서 '이 메일 요약해줘'",
		"최근 보고와 상태는 대시보드",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("mail watch message missing %q:\n%s", want, msg)
		}
	}
	for _, blocked := range []string{"메일 감시 결과", "새 메일:", "[operator@example.com]", "sender@example.com | hello", "오류: fools", "secret not found", "/tmp/mail-watch.json", "상세 기록", "자세한 근거"} {
		if strings.Contains(msg, blocked) {
			t.Fatalf("mail watch message leaked generic/internal text %q:\n%s", blocked, msg)
		}
	}
}

func TestFormatScheduleMessageMailWatchHidesHTMLPreview(t *testing.T) {
	msg := formatScheduleMessage(RunResult{
		Job:     Job{ID: "mail-watch", Kind: "mail_watch"},
		Summary: "mail watch ok",
		Payload: MailWatchPayload{
			Results: []mailadapter.WatchResult{{
				Account: mailadapter.AccountPublic{ID: "operator", Email: "operator@example.com"},
				Messages: []mailadapter.MessageSummary{{
					From:    "Listing <no_reply@listing.co>",
					Subject: "[리스팅] 신규 M&A 매물 제안",
					Snippet: `<!DOCTYPE html> <html lang="ko"> <head><meta charset="utf-8"/></head><body>이번주 신규 매물 안내입니다.</body></html>`,
				}},
			}},
		},
		Evidence: evidenceRecordForTest("/tmp/mail-watch.json"),
	})
	for _, want := range []string{"새 메일이 1개 들어왔습니다.", "[리스팅] 신규 M&A 매물 제안", "보낸 사람: Listing <no_reply@listing.co>", "최근 보고와 상태는 대시보드"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("mail watch HTML message missing %q:\n%s", want, msg)
		}
	}
	for _, blocked := range []string{"<!DOCTYPE", "<html", "<meta", "<body>", "/tmp/mail-watch.json", "상세 기록"} {
		if strings.Contains(msg, blocked) {
			t.Fatalf("mail watch HTML preview leaked %q:\n%s", blocked, msg)
		}
	}
}

func TestFormatScheduleMessageDataDoctor(t *testing.T) {
	msg := formatScheduleMessage(RunResult{
		Job:     Job{ID: "data-doctor", Kind: "data_doctor"},
		Summary: "data doctor ok",
		Payload: datadoctor.Report{
			OK: true,
			Locations: []datadoctor.Location{
				{ID: "public_argos", Files: 2, Bytes: 2048},
				{ID: "logs", Files: 1, Bytes: 4096},
			},
			Evidence: datadoctor.EvidenceState{Files: 3, Bytes: 8192},
			StateFiles: []datadoctor.StateFile{
				{ID: "messenger-targets", Exists: true, OK: true},
				{ID: "schedule-state", Exists: true, OK: true},
			},
		},
	})
	for _, want := range []string{"MeshClaw 데이터 보관 상태를 확인했습니다.", "공개 산출물에는 2개 파일", "자동 작업 보관 파일은 3개", "상태 JSON은 2개를 검사했고, 깨진 파일은 0개입니다", "보관 계획 후보를 먼저 확인"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("data doctor message missing %q:\n%s", want, msg)
		}
	}
	for _, blocked := range []string{"증거 기록", "archive plan", "상세 기록"} {
		if strings.Contains(msg, blocked) {
			t.Fatalf("data doctor message should use user-facing wording, found %q:\n%s", blocked, msg)
		}
	}
}

func TestFormatScheduleMessageDataDoctorShowsInvalidStateJSON(t *testing.T) {
	msg := formatScheduleMessage(RunResult{
		Job:     Job{ID: "data-doctor", Kind: "data_doctor"},
		Summary: "data doctor warning",
		Payload: datadoctor.Report{
			OK:       false,
			Evidence: datadoctor.EvidenceState{Files: 1, Bytes: 128},
			StateFiles: []datadoctor.StateFile{
				{ID: "messenger-targets", Exists: true, OK: true},
				{ID: "schedule-state", Exists: true, OK: false, Error: "invalid json"},
			},
			Warnings: []string{"state file is not valid JSON: schedule-state"},
		},
	})
	for _, want := range []string{"상태 JSON은 2개를 검사했고, 깨진 파일은 1개입니다", "깨진 상태 파일은 schedule-state입니다", "확인할 점: 상태 파일 JSON 형식을 확인해야 합니다: schedule-state", "비서방에서 보관 계획 후보를 먼저 확인"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("data doctor invalid-state message missing %q:\n%s", want, msg)
		}
	}
	for _, blocked := range []string{"증거 기록", "archive plan", "상세 기록"} {
		if strings.Contains(msg, blocked) {
			t.Fatalf("data doctor invalid-state message should use user-facing wording, found %q:\n%s", blocked, msg)
		}
	}
}

func TestRunOnceIncludesHumanMessageInJSONResult(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".meshclaw")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "messenger-targets.json"), []byte(`{"targets":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := RunOnce(context.Background(), RunOptions{
		JobID: "data-doctor",
		Now:   time.Date(2026, 5, 26, 14, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"MeshClaw 데이터 보관 상태를 확인했습니다.", "상태 JSON"} {
		if !strings.Contains(result.Message, want) {
			t.Fatalf("message missing %q:\n%s", want, result.Message)
		}
	}
	if result.Evidence.StoredAt == "" {
		t.Fatalf("expected evidence to be stored: %#v", result.Evidence)
	}
}

func TestRunOnceDataDoctorSummaryIncludesStateCounts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".meshclaw")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "messenger-targets.json"), []byte(`{"targets":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "schedule-state.json"), []byte(`{"last_run":`), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := RunOnce(context.Background(), RunOptions{
		JobID: "data-doctor",
		Now:   time.Date(2026, 5, 26, 14, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected data-doctor warning error for invalid state JSON")
	}
	if !result.Success || result.Status != "warning" {
		t.Fatalf("data-doctor warnings should be reported without failing automation: success=%t status=%s", result.Success, result.Status)
	}
	for _, want := range []string{"state=2", "invalid=1"} {
		if !strings.Contains(result.Summary, want) {
			t.Fatalf("summary missing %q: %s", want, result.Summary)
		}
	}
}

func TestRunDueDoesNotFailForDataDoctorWarnings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".meshclaw")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "messenger-targets.json"), []byte(`{"targets":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "custom-state.json"), []byte(`{"bad":`), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 26, 14, 0, 0, 0, time.UTC)
	state := State{LastRun: map[string]time.Time{}}
	for _, job := range DefaultPlan().Jobs {
		if job.ID != "data-doctor" {
			state.LastRun[job.ID] = now
		}
	}
	if err := SaveState(state); err != nil {
		t.Fatal(err)
	}
	results, err := RunDue(context.Background(), RunOptions{Now: now})
	if err != nil {
		t.Fatalf("run-due should keep data-doctor warnings non-fatal: %v", err)
	}
	if len(results) != 1 || results[0].Job.ID != "data-doctor" {
		t.Fatalf("expected only data-doctor to run: %#v", results)
	}
	if !results[0].Success || results[0].Status != "warning" {
		t.Fatalf("unexpected data-doctor result: success=%t status=%s", results[0].Success, results[0].Status)
	}
}

func TestRefreshArgosDashboardRunsConfiguredScript(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "argos-dashboard.sh")
	out := filepath.Join(dir, "dashboard.html")
	body := "#!/usr/bin/env bash\nset -euo pipefail\nprintf '<html>ok</html>' > '" + out + "'\nprintf '%s\\n' '" + out + "'\n"
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MESHCLAW_ARGOS_DASHBOARD_SCRIPT", script)
	result := refreshArgosDashboard(context.Background())
	if !result.Attempted || !result.OK || result.Path != out {
		t.Fatalf("dashboard refresh = %#v, want ok path %s", result, out)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("dashboard file not written: %v", err)
	}
}

func TestArgosDashboardHidesRawOpenURLInEvidenceList(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	evidenceDir := filepath.Join(home, ".meshclaw", "evidence", "2026-06-13")
	if err := os.MkdirAll(evidenceDir, 0700); err != nil {
		t.Fatal(err)
	}
	rawURL := "https://www.google.com/search?tbm=shop&q=%EA%B2%80%EC%83%89+M+%EC%82%AC%EC%9D%B4%EC%A6%88"
	record := `{"id":"20260613T083800Z-demo","time":"2026-06-13T08:38:00+09:00","kind":"open_url","summary":"` + rawURL + ` 열어줘"}`
	if err := os.WriteFile(filepath.Join(evidenceDir, "20260613T083800Z-demo.json"), []byte(record), 0600); err != nil {
		t.Fatal(err)
	}
	rawURL2 := "https://example.com/private/path?token=secret-value"
	record2 := `{"id":"20260613T083801Z-demo2","time":"2026-06-13T08:38:01+09:00","kind":"signal-open_url","summary":"사용자 URL 열기 요청","payload":{"url":"` + rawURL2 + `","group_id":"SECRET-GROUP-ID","password_handle":"vault://meshclaw/mail/secret","command":["signal-cli","send","-g","SECRET-GROUP-ID"],"local_path":"/Users/argos/.meshclaw/evidence/private.json"}}`
	if err := os.WriteFile(filepath.Join(evidenceDir, "20260613T083801Z-demo2.json"), []byte(record2), 0600); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(dir, "meshclaw")
	fakeBody := `#!/usr/bin/env bash
case "$*" in
  "messenger dispatch-health --runtime-required --json")
    printf '%s\n' '{"result":{"status":"healthy","dispatcher":{"running":true,"pid":123},"schedule_status":{"due_count":0,"next_due_job":"mail-watch"},"binary_sha256":"abc123"}}'
    ;;
  "schedule plan --json")
    printf '%s\n' '{"jobs":[{"id":"mail-watch","kind":"mail_watch","interval":"15m","target_id":"argos-briefing","enabled":true}]}'
    ;;
  "schedule status --json")
    printf '%s\n' '{"jobs":[{"id":"mail-watch","enabled":true,"due":true,"last_run":"2026-06-13T08:31:06+09:00","next_due":"2026-06-13T08:46:06+09:00"}]}'
    ;;
  "workflows plan-execute latest --json")
    printf '%s\n' '{"result":{"counts":{"approval_pending":0,"vault_missing":0}}}'
    ;;
  "messenger targets --json")
    printf '%s\n' '{"targets":[]}'
    ;;
  *)
    printf '%s\n' '{}'
    ;;
esac
`
	if err := os.WriteFile(fake, []byte(fakeBody), 0700); err != nil {
		t.Fatal(err)
	}
	publicRoot := filepath.Join(dir, "public")
	out := filepath.Join(publicRoot, "argos", "dashboard.html")
	staleDetail := filepath.Join(publicRoot, "argos", "evidence", "stale.html")
	if err := os.MkdirAll(filepath.Dir(staleDetail), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staleDetail, []byte("password_handle SECRET-GROUP-ID https://www.google.com/search?tbm=shop"), 0600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join("..", "..", "scripts", "argos-dashboard.sh")
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"MESHCLAW_BIN="+fake,
		"MESHCLAW_ARGOS_PUBLIC_ROOT="+publicRoot,
		"MESHCLAW_ARGOS_DASHBOARD="+out,
		"MESHCLAW_LANG=ko",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("dashboard script failed: %v\n%s", err, output)
	}
	if _, err := os.Stat(staleDetail); !os.IsNotExist(err) {
		t.Fatalf("stale detail page should be removed before regeneration: %v", err)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, forbidden := range []string{rawURL, rawURL2, "tbm=shop", "%EA%B2%80", "token=secret", ">open_url<", "· open_url", "signal-open_url"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("dashboard leaked raw URL/internal kind %q:\n%s", forbidden, text)
		}
	}
	for _, want := range []string{"Today Activity", "Breakdown", "URL 열기 요청", "www.google.com", "example.com", "mail-watch", "due", "06-13 08:31", "06-13 08:46", "Next"} {
		if !strings.Contains(text, want) {
			t.Fatalf("dashboard missing friendly evidence text %q:\n%s", want, text)
		}
	}
	detail, err := os.ReadFile(filepath.Join(publicRoot, "argos", "evidence", "20260613T083801Z-demo2.html"))
	if err != nil {
		t.Fatal(err)
	}
	detailText := string(detail)
	for _, forbidden := range []string{rawURL2, "token=secret", "secret-value", "SECRET-GROUP-ID", "vault://meshclaw/mail/secret", "signal-cli", "group_id", "password_handle", "command", "/Users/argos/.meshclaw"} {
		if strings.Contains(detailText, forbidden) {
			t.Fatalf("dashboard detail leaked sensitive text %q:\n%s", forbidden, detailText)
		}
	}
	for _, want := range []string{"결과:", "URL 열기 요청", "example.com", "_redacted_fields", "[local path hidden]"} {
		if !strings.Contains(detailText, want) {
			t.Fatalf("dashboard detail missing redacted/friendly text %q:\n%s", want, detailText)
		}
	}
}
