package messenger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/osauto"
	"github.com/meshclaw/meshclaw/internal/tts"
)

func TestAssistantDailyAgendaPackageTargetsBriefingWithArtifacts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", home+"/targets.json")
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}
	loc := time.FixedZone("KST", 9*60*60)
	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, loc)
	end := start.Add(time.Hour)
	due := start.Add(2 * time.Hour)
	t.Setenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_CALENDAR_STDOUT", fmt.Sprintf(`{"kind":"argos_calendar_helper","ok":true,"count":1,"events":[{"title":"마케팅 주간회의","start":%q,"end":%q,"calendar":"Work","location":"Zoom"}]}`, start.Format(time.RFC3339), end.Format(time.RFC3339)))
	t.Setenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_REMINDER_STDOUT", fmt.Sprintf(`{"kind":"argos_reminder_helper","ok":true,"count":1,"reminders":[{"title":"캠페인 성과표 확인","due":%q,"calendar":"Reminders","notes":"ROAS 확인"}]}`, due.Format(time.RFC3339)))

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-daily-agenda", Mode: "assistant"}, "오늘 일정과 할 일을 하루 계획 패키지로 보고방에 보내줘. 음성으로도 준비해줘.")
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"하루 실행계획 패키지를 Signal로 보낼 준비를 했습니다.",
		"대상: 보고방",
		"보고방은 one-way/no-reply",
		"오늘 하루 실행계획 패키지를 만들었습니다.",
		"일정 1개, 할 일 1개",
		"마케팅 주간회의",
		"캠페인 성과표 확인",
		"하루 타임라인 SVG",
		"edge-tts MP3 하루 계획 브리핑",
		"캘린더/리마인더 실행 초안:",
		"`캘린더 메모 초안 승인`: 첫 일정 준비물과 이동시간 메모 초안만 작성",
		"`리마인더 초안 승인`: 첫 번째 할 일 리마인더 초안만 작성",
		"실제 캘린더 수정, 리마인더 추가, 완료 처리는 별도 승인 전 중지",
		"첫 일정 준비물과 이동시간을 캘린더 메모 초안으로 정리해줘. 아직 수정하지 마",
		"첫 번째 할 일 리마인더 초안만 만들어줘. 아직 생성하지 마",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("daily agenda package reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"meshclaw-attachment:", "/Documents/Argos Vault/", "/.meshclaw/evidence/"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("visible reply should not expose raw path %q:\n%s", unwanted, visible)
		}
	}
	attachments := signalReplyAttachments(reply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("daily agenda package missing %s attachment: %#v", ext, attachments)
		}
	}
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), ".html") {
			t.Fatalf("HTML should not be attached for mobile Signal package: %#v", attachments)
		}
	}
	markdown := firstAttachmentExt(attachments, ".md")
	data, err := os.ReadFile(markdown)
	if err != nil {
		t.Fatalf("read markdown attachment: %v", err)
	}
	for _, want := range []string{"## 일정", "## 할 일", "## 캘린더/리마인더 실행 초안", "## 실행 순서", "마케팅 주간회의", "캠페인 성과표 확인", "`캘린더 메모 초안 승인`", "`리마인더 초안 승인`", "실제 캘린더 수정, 리마인더 추가, 완료 처리는 별도 승인 전 중지", "캘린더 메모나 리마인더는 초안으로만", "별도 승인 전에는 하지 않습니다"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("daily agenda markdown missing %q:\n%s", want, string(data))
		}
	}
	timeline := firstAttachmentExt(attachments, ".svg")
	timelineData, err := os.ReadFile(timeline)
	if err != nil {
		t.Fatalf("read timeline attachment: %v", err)
	}
	for _, want := range []string{"<svg", "하루 타임라인", "마케팅 주간회의", "캠페인 성과표 확인"} {
		if !strings.Contains(string(timelineData), want) {
			t.Fatalf("daily agenda timeline missing %q:\n%s", want, string(timelineData))
		}
	}
}

func TestAssistantDailyAgendaPackageCanIncludeMailPriority(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, "targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}
	loc := time.FixedZone("KST", 9*60*60)
	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, loc)
	end := start.Add(time.Hour)
	due := start.Add(3 * time.Hour)
	t.Setenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_CALENDAR_STDOUT", fmt.Sprintf(`{"events":[{"title":"제품 전략 회의","start":%q,"end":%q,"calendar":"Work","location":"회의실 A"}]}`, start.Format(time.RFC3339), end.Format(time.RFC3339)))
	t.Setenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_REMINDER_STDOUT", fmt.Sprintf(`{"reminders":[{"title":"회의자료 최종 확인","due":%q,"calendar":"Reminders","notes":"PPT 확인"}]}`, due.Format(time.RFC3339)))
	t.Setenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_MAIL_STDOUT", fmt.Sprintf(`{"results":[{"account":{"id":"work","backend":"imap","tls":true},"limit":2,"messages":[{"id":"m1","from":"partner@example.com","subject":"내일 계약서 검토 후 회신 부탁드립니다","date":%q,"snippet":"내일 오전 미팅 전에 계약서 조건을 검토하고 회신 부탁드립니다."},{"id":"m2","from":"newsletter@example.com","subject":"주간 뉴스레터","date":%q,"snippet":"이번 주 할인 소식과 프로모션입니다. unsubscribe"}]}]}`, start.Format(time.RFC3339), start.Add(time.Hour).Format(time.RFC3339)))

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-daily-agenda-mail", Mode: "assistant"}, "오늘 일정과 할 일과 메일을 하루 계획 패키지로 보고방에 보내줘. PPT와 표와 음성으로도 준비해줘.")
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"하루 실행계획 패키지를 Signal로 보낼 준비를 했습니다.",
		"오늘 하루 실행계획 패키지를 만들었습니다.",
		"메일 확인 결과: 최근 메일 2개, 오늘 처리 후보 1개",
		"먼저 볼 메일:",
		"내일 계약서 검토 후 회신 부탁드립니다",
		"제품 전략 회의",
		"회의자료 최종 확인",
		"캘린더/리마인더 실행 초안:",
		"`캘린더 메모 초안 승인`: 첫 일정 준비물과 이동시간 메모 초안만 작성",
		"`리마인더 초안 승인`: 첫 번째 할 일 리마인더 초안만 작성",
		"첫 일정 준비물과 이동시간을 캘린더 메모 초안으로 정리해줘. 아직 수정하지 마",
		"첫 번째 할 일 리마인더 초안만 만들어줘. 아직 생성하지 마",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("daily agenda with mail missing %q:\n%s", want, visible)
		}
	}
	if strings.Contains(visible, "메일 우선순위 보고를 Signal로 보낼 준비") {
		t.Fatalf("daily agenda mail request was hijacked by mail-priority package:\n%s", visible)
	}
	attachments := signalReplyAttachments(reply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("daily agenda with mail missing %s attachment: %#v", ext, attachments)
		}
	}
	markdown := firstAttachmentExt(attachments, ".md")
	data, err := os.ReadFile(markdown)
	if err != nil {
		t.Fatalf("read markdown attachment: %v", err)
	}
	for _, want := range []string{"## 메일 처리", "내일 계약서 검토 후 회신 부탁드립니다", "## 캘린더/리마인더 실행 초안", "## 실행 순서", "`캘린더 메모 초안 승인`", "`리마인더 초안 승인`", "캘린더 메모나 리마인더는 초안으로만"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("daily agenda with mail markdown missing %q:\n%s", want, string(data))
		}
	}
	timeline := firstAttachmentExt(attachments, ".svg")
	timelineData, err := os.ReadFile(timeline)
	if err != nil {
		t.Fatalf("read timeline attachment: %v", err)
	}
	for _, want := range []string{"메일 처리 블록", "내일 계약서 검토 후 회신 부탁드립니다"} {
		if !strings.Contains(string(timelineData), want) {
			t.Fatalf("daily agenda timeline should include mail item %q:\n%s", want, string(timelineData))
		}
	}
}

func TestAssistantDailyAgendaPackageRoutingRequiresArtifactSignal(t *testing.T) {
	if !isAssistantDailyAgendaPackageRequest("오늘 일정과 할 일을 하루 계획 패키지로 보고방에 보내줘") {
		t.Fatal("expected targeted daily agenda package request")
	}
	if !isAssistantDailyAgendaPackageRequest("오늘 일정과 할 일과 메일을 하루 계획 패키지로 보고방에 보내줘") {
		t.Fatal("expected targeted daily agenda package request with mail")
	}
	if !isAssistantDailyAgendaPackageRequest("send today's calendar and reminders as a daily agenda package with PPT, spreadsheet, and voice to the briefing room") {
		t.Fatal("expected English targeted daily agenda package request")
	}
	if isAssistantDailyAgendaPackageRequest("오늘 일정, 할 일, 메일, 날씨를 하루 계획으로 묶어줘") {
		t.Fatal("plain daily plan starter should stay on readable text briefing")
	}
}

func TestAssistantDailyAgendaPackageEnglishCanSendToBriefing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, "targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}
	loc := time.FixedZone("KST", 9*60*60)
	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, loc)
	end := start.Add(time.Hour)
	due := start.Add(2 * time.Hour)
	t.Setenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_CALENDAR_STDOUT", fmt.Sprintf(`{"kind":"argos_calendar_helper","ok":true,"count":1,"events":[{"title":"Marketing weekly meeting","start":%q,"end":%q,"calendar":"Work","location":"Zoom"}]}`, start.Format(time.RFC3339), end.Format(time.RFC3339)))
	t.Setenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_REMINDER_STDOUT", fmt.Sprintf(`{"kind":"argos_reminder_helper","ok":true,"count":1,"reminders":[{"title":"Check campaign performance sheet","due":%q,"calendar":"Reminders","notes":"Review ROAS"}]}`, due.Format(time.RFC3339)))
	t.Setenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_MAIL_STDOUT", fmt.Sprintf(`{"results":[{"account":{"id":"work","backend":"imap","tls":true},"limit":1,"messages":[{"id":"m1","from":"ops@example.com","subject":"Please review the revised supplier contract","date":%q,"snippet":"Please review and reply by tomorrow morning."}]}]}`, start.Format(time.RFC3339)))

	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-daily-agenda-en", Mode: "assistant"},
		"send today's calendar, reminders, and email as a daily agenda package with PPT, spreadsheet, and voice to the briefing room",
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Prepared the daily agenda package for Signal delivery.",
		"Target: Briefing room",
		"The briefing room is one-way/no-reply",
		"Created Today's daily agenda package.",
		"Result: 1 events, 1 tasks",
		"Mail result: 1 recent mail items, 1 handling candidates today",
		"Marketing weekly meeting",
		"Check campaign performance sheet",
		"First mail to check:",
		"Please review the revised supplier contract",
		"Mobile-openable daily timeline SVG",
		"edge-tts MP3 daily agenda brief",
		"Calendar/reminder execution drafts:",
		"`calendar note draft approved`: draft only the first event's materials and travel-time note",
		"`reminder draft approved`: draft only the first task reminder",
		"actual calendar edits, reminder adds, and completion changes stay stopped until separate approval",
		"turn the first event's materials and travel time into a draft calendar note only; do not edit yet",
		"draft a reminder for the first task only; do not create it yet",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("English daily agenda reply missing %q:\n%s", want, visible)
		}
	}
	if containsHangul(visible) {
		t.Fatalf("English daily agenda visible text should not expose Korean:\n%s", visible)
	}
	for _, unwanted := range []string{"meshclaw-attachment:", "/Documents/Argos Vault/", "/.meshclaw/evidence/"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("visible reply should not expose raw path %q:\n%s", unwanted, visible)
		}
	}
	attachments := signalReplyAttachments(reply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("English daily agenda package missing %s attachment: %#v", ext, attachments)
		}
	}
	for _, attachment := range attachments {
		lower := strings.ToLower(attachment)
		if strings.HasSuffix(lower, ".html") {
			t.Fatalf("HTML should not be attached for mobile Signal package: %#v", attachments)
		}
		if !(strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".csv") || strings.HasSuffix(lower, ".svg")) {
			continue
		}
		data, err := os.ReadFile(attachment)
		if err != nil {
			t.Fatalf("read English daily agenda artifact %s: %v", attachment, err)
		}
		if containsHangul(string(data)) {
			t.Fatalf("English daily agenda artifact should not expose Korean in %s:\n%s", attachment, string(data))
		}
	}
	reportMarkdown := ""
	for _, attachment := range attachments {
		if !strings.HasSuffix(strings.ToLower(attachment), ".md") {
			continue
		}
		data, err := os.ReadFile(attachment)
		if err != nil {
			t.Fatalf("read English daily agenda markdown candidate: %v", err)
		}
		if strings.Contains(string(data), "## Execution Order") {
			reportMarkdown = string(data)
			break
		}
	}
	if reportMarkdown == "" {
		t.Fatalf("English daily agenda should include report markdown: %#v", attachments)
	}
	for _, want := range []string{"## Calendar/Reminder Execution Drafts", "`calendar note draft approved`", "`reminder draft approved`", "actual calendar edits, reminder adds, and completion changes stay stopped until separate approval", "Calendar notes and reminders can be drafted only", "actual creation, edits, or completion changes wait for separate approval"} {
		if !strings.Contains(reportMarkdown, want) {
			t.Fatalf("English daily agenda markdown missing %q:\n%s", want, reportMarkdown)
		}
	}
	for _, unwanted := range []string{"create a reminder for the first task tomorrow at 9 AM", "reminder creation"} {
		if strings.Contains(visible, unwanted) || strings.Contains(reportMarkdown, unwanted) {
			t.Fatalf("daily agenda should use draft wording, found %q:\nvisible=%s\nmarkdown=%s", unwanted, visible, reportMarkdown)
		}
	}
}

func TestAssistantDailyAgendaPackageShowsReadIssues(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	data := assistantDailyAgendaData{
		Title: "Today's daily agenda package",
		Start: time.Date(2026, 6, 14, 0, 0, 0, 0, time.FixedZone("KST", 9*60*60)),
		End:   time.Date(2026, 6, 15, 0, 0, 0, 0, time.FixedZone("KST", 9*60*60)),
		CalendarResult: osauto.Result{
			Error: "Calendar permission prompt timed out",
		},
		ReminderResult: osauto.Result{
			Error: "Reminders permission prompt timed out",
		},
	}
	summary := strings.Join(assistantDailyAgendaPackageSummary(
		data,
		osauto.Result{OK: true},
		osauto.Result{OK: true},
		osauto.Result{OK: true},
		"/tmp/timeline.svg",
		nil,
		false,
		tts.Result{},
		nil,
	), "\n")
	for _, want := range []string{
		"Read issues:",
		"Calendar: needs attention - Calendar permission prompt timed out",
		"Reminders: needs attention - Reminders permission prompt timed out",
		"Result: 0 events, 0 tasks",
		"draft a reminder for the first task only; do not create it yet",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("daily agenda read issue summary missing %q:\n%s", want, summary)
		}
	}
}

func TestAssistantDailyAgendaPackageExecuteSendsToBriefingWithArtifacts(t *testing.T) {
	home := t.TempDir()
	signalArgs := filepath.Join(home, "signal-args.txt")
	fakeSignal := filepath.Join(home, "signal-cli")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %q\nprintf 'agenda-signal-321\\n'\n", signalArgs)
	if err := os.WriteFile(fakeSignal, []byte(script), 0700); err != nil {
		t.Fatalf("write fake signal-cli: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_SIGNAL_CLI", fakeSignal)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, "targets.json"))
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}
	loc := time.FixedZone("KST", 9*60*60)
	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 9, 30, 0, 0, loc)
	end := start.Add(45 * time.Minute)
	due := start.Add(3 * time.Hour)
	t.Setenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_CALENDAR_STDOUT", fmt.Sprintf(`{"kind":"argos_calendar_helper","ok":true,"count":1,"events":[{"title":"Founder ops review","start":%q,"end":%q,"calendar":"Work","location":"Zoom"}]}`, start.Format(time.RFC3339), end.Format(time.RFC3339)))
	t.Setenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_REMINDER_STDOUT", fmt.Sprintf(`{"kind":"argos_reminder_helper","ok":true,"count":1,"reminders":[{"title":"Send launch checklist","due":%q,"calendar":"Reminders","notes":"Review blockers"}]}`, due.Format(time.RFC3339)))

	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-daily-agenda-execute-en", Mode: "assistant", Execute: true},
		"send today's calendar and reminders as a daily agenda package with PPT, spreadsheet, timeline SVG, and voice to the briefing room",
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Sent the daily agenda package to Signal.",
		"Target: Briefing room",
		"Signal ID: agenda-signal-321",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("execute daily agenda reply missing %q:\n%s", want, visible)
		}
	}
	if containsHangul(visible) {
		t.Fatalf("English execute daily agenda reply should not expose Korean:\n%s", visible)
	}
	for _, unwanted := range []string{"meshclaw-attachment:", "/Documents/Argos Vault/", "/.meshclaw/evidence/"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("execute daily agenda reply should not expose %q:\n%s", unwanted, visible)
		}
	}
	argsData, err := os.ReadFile(signalArgs)
	if err != nil {
		t.Fatalf("read fake signal args: %v", err)
	}
	args := string(argsData)
	for _, want := range []string{"send", "-g", "group-briefing", "--attachment", ".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3", "Open on mobile:", "DOCX report: https://argos.example.test/argos/", "PPTX deck: https://argos.example.test/argos/", "XLSX table: https://argos.example.test/argos/", "SVG chart: https://argos.example.test/argos/", "MP3 voice brief: https://argos.example.test/argos/"} {
		if !strings.Contains(args, want) {
			t.Fatalf("fake signal args missing %q:\n%s", want, args)
		}
	}
	if strings.Contains(args, ".html") {
		t.Fatalf("HTML should not be sent for mobile daily agenda package:\n%s", args)
	}
}
