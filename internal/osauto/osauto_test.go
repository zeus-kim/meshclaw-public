package osauto

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"
)

func TestFailedResult(t *testing.T) {
	result := failed("meshclaw_automation_test", "nope")
	if result.OK || result.Error != "nope" || result.Action != "test" {
		t.Fatalf("result=%#v", result)
	}
}

func TestRunWithInputBlocksGUICommandsDuringUnitTests(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "open", args: []string{"-a", "Safari"}},
		{name: "osascript", args: []string{"-e", `tell application "Safari" to activate`}},
		{name: "xdg-open", args: []string{"https://example.com"}},
		{name: "shortcuts", args: []string{"run", "Open Safari"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := runWithInput(context.Background(), "meshclaw_automation_open_app", "", tc.name, tc.args...)
			if result.OK {
				t.Fatalf("GUI command should be blocked during unit tests: %#v", result)
			}
			if !strings.Contains(result.Error, "blocked during go test") {
				t.Fatalf("error should explain test GUI block, got %q", result.Error)
			}
			if len(result.Command) == 0 || result.Command[0] != tc.name {
				t.Fatalf("command should be preserved for evidence: %#v", result.Command)
			}
		})
	}
}

func TestRunWithInputAllowsNonGUICommandsDuringUnitTests(t *testing.T) {
	result := runWithInput(context.Background(), "meshclaw_automation_test", "", "true")
	if !result.OK {
		t.Fatalf("non-GUI command should still run in unit tests: %#v", result)
	}
}

func TestUIRunnerScreenCaptureThumbnailFallsBackToRunnerRecording(t *testing.T) {
	fakeBin := t.TempDir()
	qlmanage := filepath.Join(fakeBin, "qlmanage")
	script := `#!/bin/sh
set -eu
outdir=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    shift
    outdir="$1"
  fi
  shift || true
done
mkdir -p "$outdir"
printf 'fake png' > "$outdir/fallback.png"
`
	if err := os.WriteFile(qlmanage, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	var sawRecord bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/screen-record" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		sawRecord = true
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		output, _ := req["output"].(string)
		if output == "" {
			t.Fatal("screen-record output missing")
		}
		if err := os.WriteFile(output, []byte("fake mp4"), 0600); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"argos_screen_record","ok":true}`))
	}))
	defer server.Close()
	t.Setenv("MESHCLAW_ARGOS_UI_RUNNER_URL", server.URL)

	output := filepath.Join(t.TempDir(), "capture.png")
	result := uiRunnerScreenCaptureThumbnail(context.Background(), output)
	if !result.OK {
		t.Fatalf("fallback capture failed: %#v", result)
	}
	if !sawRecord {
		t.Fatal("fallback did not call UI Runner screen-record")
	}
	if result.URL != output {
		t.Fatalf("url=%q want %q", result.URL, output)
	}
	if !strings.Contains(result.Stdout, "ui_runner_screen_record_thumbnail") {
		t.Fatalf("stdout should identify fallback: %#v", result)
	}
	if data, err := os.ReadFile(output); err != nil || string(data) != "fake png" {
		t.Fatalf("output = %q err=%v", string(data), err)
	}
}

func TestNormalizeBrowserTabCleanupHosts(t *testing.T) {
	got := normalizeBrowserTabCleanupHosts([]string{
		"https://www.coupang.com/np/search?q=x",
		"*.coupang.com",
		"COUPANG.com",
		"bad host",
		"",
	})
	want := []string{"coupang.com", "www.coupang.com"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("hosts=%#v want %#v", got, want)
	}
}

func TestBrowserTabCleanupAppleScriptTargetsSafariAndChrome(t *testing.T) {
	script := browserTabCleanupAppleScript([]string{"coupang.com"})
	for _, want := range []string{
		`set meshclawTargetHosts to {"coupang.com"}`,
		`application "Safari" is running`,
		`application "Google Chrome" is running`,
		`on meshclawHostFromURL(theURL)`,
		`if currentHost ends with "." & h then return true`,
		`close t`,
		`return "closed_tabs=" & meshclawClosedTabs`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q:\n%s", want, script)
		}
	}
	if strings.Contains(script, "delete") || strings.Contains(script, "remove") {
		t.Fatalf("script should close tabs only, not delete browser data:\n%s", script)
	}
}

func TestClassifyArgosActionOpenApp(t *testing.T) {
	action := ClassifyArgosAction("Safari 열어줘")
	if action.Action != "open_app" || action.App != "Safari" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionOpenCalculator(t *testing.T) {
	action := ClassifyArgosAction("계산기 앱 열어줘")
	if action.Action != "open_app" || action.App != "Calculator" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionNote(t *testing.T) {
	action := ClassifyArgosAction("Notes에 메모해줘. 제목은 장보기. 내용은 우유와 커피 사기.")
	if action.Action != "note_create" || action.NoteTitle != "장보기" || action.NoteBody != "우유와 커피 사기" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionNoteInfersTitleAndKeepsQuotes(t *testing.T) {
	action := ClassifyArgosAction(`Argos 메모 작성해줘. 내용은 "따옴표 테스트"`)
	if action.Action != "note_create" || action.NoteTitle != "Argos" || action.NoteBody != `"따옴표 테스트"` {
		t.Fatalf("action=%#v", action)
	}
	if got := notesHTMLBody(action.NoteBody); got != `"따옴표 테스트"` {
		t.Fatalf("notesHTMLBody escaped quotes: %q", got)
	}
}

func TestClassifyArgosActionReminder(t *testing.T) {
	action := ClassifyArgosAction("내일 오전 9시에 우유 사기 리마인더 추가해줘")
	if action.Action != "reminder_create" || !strings.Contains(action.ReminderTitle, "우유 사기") || action.ReminderDue == "" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionReminderCreateBeatsListWordsInTitle(t *testing.T) {
	action := ClassifyArgosAction("10분 뒤에 Argos 언어팩 결과 확인 리마인더 추가해줘")
	if action.Action != "reminder_create" || !strings.Contains(action.ReminderTitle, "Argos 언어팩 결과 확인") || action.ReminderDue == "" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionReminderNaturalTitleEnding(t *testing.T) {
	action := ClassifyArgosAction("내일 아침 8시에 약 챙기라고 리마인더 하나 만들어줘. 제목은 약 챙기기면 돼.")
	if action.Action != "reminder_create" || action.ReminderTitle != "약 챙기기" || action.ReminderDue == "" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionReminderList(t *testing.T) {
	action := ClassifyArgosAction("오늘 할 일 뭐 있어?")
	if action.Action != "reminders_list" || action.ReminderStart == "" || action.ReminderEnd == "" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionHelpDoesNotCreateReminder(t *testing.T) {
	for _, input := range []string{
		"지금 할 수 있는 일을 짧게 알려줘",
		"뭘 할 수 있어?",
		"what can you do?",
	} {
		action := ClassifyArgosAction(input)
		if action.Action != "help" || action.ReminderTitle != "" || action.ReminderDue != "" {
			t.Fatalf("%q action=%#v", input, action)
		}
		decision := CheckArgosPermission(action)
		if !decision.Allowed || decision.Grantable {
			t.Fatalf("%q decision=%#v", input, decision)
		}
	}
}

func TestClassifyArgosActionReminderComplete(t *testing.T) {
	action := ClassifyArgosAction("우유 사기 리마인더 완료해줘")
	if action.Action != "reminder_complete" || action.ReminderQuery != "우유 사기" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionReminderDelete(t *testing.T) {
	action := ClassifyArgosAction("우유 사기 리마인더 삭제해줘")
	if action.Action != "reminder_delete" || action.ReminderQuery != "우유 사기" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionCalendarEvent(t *testing.T) {
	action := ClassifyArgosAction("내일 오후 3시에 Argos 회의 일정 추가해줘")
	if action.Action != "calendar_event_create" || !strings.Contains(action.CalendarTitle, "Argos") || action.CalendarStart == "" || action.CalendarEnd == "" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionCalendarTitleKeepsMeetingWord(t *testing.T) {
	action := ClassifyArgosAction("내일 오후 3시에 Argos 테스트 회의 일정 추가해줘")
	if action.Action != "calendar_event_create" || action.CalendarTitle != "Argos 테스트 회의" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionCalendarList(t *testing.T) {
	action := ClassifyArgosAction("내일 일정 뭐 있어?")
	if action.Action != "calendar_events_list" || action.CalendarStart == "" || action.CalendarEnd == "" {
		t.Fatalf("action=%#v", action)
	}
	if action.CalendarTitle != "" {
		t.Fatalf("calendar list should not become create action: %#v", action)
	}
}

func TestClassifyArgosActionCalendarListBeatsIncidentalNoteWords(t *testing.T) {
	action := ClassifyArgosAction("내일 내가 잡아둔 일정 뭐 있어? 방금 넣은 병원 전화도 보이는지 봐줘.")
	if action.Action != "calendar_events_list" || action.CalendarStart == "" || action.CalendarEnd == "" {
		t.Fatalf("calendar list should win over incidental note/create words: %#v", action)
	}
	if action.NoteBody != "" || action.NoteTitle != "" {
		t.Fatalf("calendar list should not become note create: %#v", action)
	}
}

func TestClassifyArgosActionCalendarDelete(t *testing.T) {
	action := ClassifyArgosAction("내일 Argos 회의 일정 삭제해줘")
	if action.Action != "calendar_event_delete" || action.CalendarQuery != "argos" || action.CalendarStart == "" || action.CalendarEnd == "" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionContactsSearch(t *testing.T) {
	action := ClassifyArgosAction("연락처에서 홍길동 전화번호 찾아줘")
	if action.Action != "contacts_search" || action.ContactQuery != "홍길동" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionContactsSearchRejectsVagueAnyone(t *testing.T) {
	action := ClassifyArgosAction("연락처에서 아무나 한 명 전화번호 찾아줘")
	if action.Action == "contacts_search" || action.Action == "browser_search" || action.Error == "" {
		t.Fatalf("vague contact request should ask for a specific query: %#v", action)
	}
}

func TestParseReminderDue(t *testing.T) {
	now := time.Date(2026, 5, 24, 22, 30, 0, 0, time.Local)
	due, matched := parseReminderDue(now, "내일 오후 3시 20분에 전화하기 리마인더")
	if matched == "" {
		t.Fatal("expected matched reminder time")
	}
	if due.Year() != 2026 || due.Month() != time.May || due.Day() != 25 || due.Hour() != 15 || due.Minute() != 20 {
		t.Fatalf("due=%s matched=%q", due, matched)
	}
}

func TestParseRelativeReminderDue(t *testing.T) {
	now := time.Date(2026, 5, 24, 22, 30, 0, 0, time.Local)
	due, matched := parseReminderDue(now, "20분 뒤에 알려줘")
	if matched == "" {
		t.Fatal("expected matched relative reminder time")
	}
	if !due.Equal(now.Add(20 * time.Minute)) {
		t.Fatalf("due=%s matched=%q", due, matched)
	}
	action := ClassifyArgosAction("20분 뒤에 알려줘")
	if action.Action != "reminder_create" || action.ReminderTitle != "알림" || action.ReminderDue == "" {
		t.Fatalf("action=%#v", action)
	}
	action = ClassifyArgosAction("remind me in 20 minutes")
	if action.Action != "reminder_create" || action.ReminderTitle != "알림" || action.ReminderDue == "" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionNoteSave(t *testing.T) {
	action := ClassifyArgosAction("회의 메모 저장해줘. 내용은 오늘 제품 방향 결정.")
	if action.Action != "note_create" || action.NoteBody != "오늘 제품 방향 결정" {
		t.Fatalf("action=%#v", action)
	}
}

func TestParseCalendarEvent(t *testing.T) {
	now := time.Date(2026, 5, 24, 22, 30, 0, 0, time.Local)
	title, notes, start, end, ok := parseCalendarEvent("내일 오후 3시에 투자자 미팅 일정 추가해줘", now)
	if !ok || !strings.Contains(title, "투자자 미팅") || !strings.Contains(notes, "Signal 요청") {
		t.Fatalf("title=%q notes=%q ok=%t", title, notes, ok)
	}
	if !strings.Contains(start, "2026-05-25T15:00:00") || !strings.Contains(end, "2026-05-25T16:00:00") {
		t.Fatalf("start=%q end=%q", start, end)
	}
}

func TestParseCalendarEventPreservesCalendarWordInTitle(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.Local)
	title, _, start, _, ok := parseCalendarEvent("내일 오후 3시에 Argos 캘린더 결과 포맷 테스트 일정 추가해줘", now)
	if !ok || title != "Argos 캘린더 결과 포맷 테스트" || start == "" {
		t.Fatalf("title=%q start=%q ok=%t", title, start, ok)
	}
}

func TestReminderShortcutInput(t *testing.T) {
	input := reminderShortcutInput("우유 사기", "Signal 요청", "2026-05-25T09:00:00+09:00")
	for _, want := range []string{`"title":"우유 사기"`, `"notes":"Signal 요청"`, `"due":"2026-05-25T09:00:00+09:00"`} {
		if !strings.Contains(input, want) {
			t.Fatalf("missing %q in %s", want, input)
		}
	}
}

func TestReminderShortcutCandidates(t *testing.T) {
	t.Setenv("MESHCLAW_ARGOS_REMINDER_SHORTCUT", "My Reminder Shortcut")
	candidates := reminderShortcutCandidates()
	if len(candidates) == 0 || candidates[0] != "My Reminder Shortcut" || !strings.Contains(strings.Join(candidates, "\n"), "Argos Add Reminder") {
		t.Fatalf("candidates=%#v", candidates)
	}
}

func TestClassifyArgosActionAIHandoff(t *testing.T) {
	action := ClassifyArgosAction("클로드로 넘겨줘: 이 에러 원인 분석")
	if action.Action != "ai_handoff" || action.Provider != "claude" || action.Prompt != "이 에러 원인 분석" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionChatGPTQuestion(t *testing.T) {
	action := ClassifyArgosAction("지피티에게 물어봐: 가나 경제 뉴스 핵심을 한 문장으로 설명해")
	if action.Action != "ai_handoff" || action.Provider != "chatgpt" || action.Prompt != "가나 경제 뉴스 핵심을 한 문장으로 설명해" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionShortcut(t *testing.T) {
	action := ClassifyArgosAction("Argos Morning 단축어 실행")
	if action.Action != "shortcut_run" || action.Shortcut != "Argos Morning" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionSearchCleansCommandWords(t *testing.T) {
	action := ClassifyArgosAction("가나 경제 뉴스 웹에서 가나 경제 뉴스 찾아봐")
	if action.Action != "browser_search" || action.Query != "가나 경제 뉴스" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionSearchCleansEnglishCommandWords(t *testing.T) {
	action := ClassifyArgosAction("search web for Ghana economy news")
	if action.Action != "browser_search" || action.Query != "Ghana economy news" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionVisibleBrowserSearch(t *testing.T) {
	action := ClassifyArgosAction("브라우저에서 가나 경제 뉴스 검색해줘")
	if action.Action != "visible_browser_search" || action.Query != "가나 경제 뉴스" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionWorkDemo(t *testing.T) {
	action := ClassifyArgosAction("작업 데모 보여줘. 검색어는 가나 경제 뉴스")
	if action.Action != "work_demo" || action.Query != "가나 경제 뉴스" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionWorkDemoUsesDefaultQuery(t *testing.T) {
	action := ClassifyArgosAction("작업 데모 보여줘")
	if action.Action != "work_demo" || action.Query != "가나 경제 뉴스" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionMacRunnerCommand(t *testing.T) {
	action := ClassifyArgosAction("아이맥에서 Pages 문서 작성해줘. 제목은 Argos 통합 테스트.")
	if action.Action != "mac_runner_command" || action.Input == "" {
		t.Fatalf("action=%#v", action)
	}
}

func TestArgosPermissionDecisionNormalizesLegacyRunnerCommand(t *testing.T) {
	decision := argosPermissionDecisionFor(ArgosAction{Action: "macbook_command"})
	if !decision.Grantable || decision.Action != "mac_runner_command" || decision.Scope != "mac_runner" {
		t.Fatalf("decision=%#v", decision)
	}
	decision = argosPermissionDecisionFor(ArgosAction{Action: "device_runner_command"})
	if !decision.Grantable || decision.Action != "mac_runner_command" || decision.Scope != "mac_runner" {
		t.Fatalf("decision=%#v", decision)
	}
}

func TestDoctorMacRunnerNoRunnerExplainsLocalRunnerSeparation(t *testing.T) {
	t.Setenv("MESHCLAW_MAC_RUNNERS_FILE", filepath.Join(t.TempDir(), "mac-runners.json"))
	report := DoctorMacRunner(context.Background(), "")
	if report.OK || len(report.NextActions) == 0 {
		t.Fatalf("report=%#v", report)
	}
	found := false
	for _, action := range report.NextActions {
		if strings.Contains(action, "stable Argos UI Runner") && strings.Contains(action, "Mac runner 없이도") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing local runner separation note: %#v", report.NextActions)
	}
}

func TestClassifyArgosActionMacRunnerDoctor(t *testing.T) {
	action := ClassifyArgosAction("맥 실행기 상태 한번 봐줘")
	if action.Action != "mac_runner_doctor" {
		t.Fatalf("action=%#v", action)
	}
	action = ClassifyArgosAction("mac runner imac doctor")
	if action.Action != "mac_runner_doctor" || action.Input != "imac" {
		t.Fatalf("action=%#v", action)
	}
}

func TestClassifyArgosActionDocumentCreateDoesNotUseMacBook(t *testing.T) {
	action := ClassifyArgosAction("Pages 문서 작성해줘. 제목은 Argos 통합 테스트. 내용은 맥북 포커스를 뺏지 않아야 합니다.")
	if action.Action != "document_create" || action.NoteTitle != "Argos 통합 테스트" || !strings.Contains(action.NoteBody, "포커스") {
		t.Fatalf("action=%#v", action)
	}
}

func TestCreateArgosDocumentWritesEditableDOCX(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	result := CreateArgosDocument(context.Background(), "테스트 문서", "본문입니다.")
	if !result.OK || result.URL == "" || result.Markdown == "" || result.DOCX == "" || result.Preview == "" {
		t.Fatalf("result=%#v", result)
	}
	if st, err := os.Stat(result.Markdown); err != nil || st.IsDir() || st.Size() == 0 {
		t.Fatalf("markdown not written: path=%q stat=%#v err=%v", result.Markdown, st, err)
	}
	if st, err := os.Stat(result.DOCX); err != nil || st.IsDir() || st.Size() == 0 {
		t.Fatalf("docx not written: path=%q stat=%#v err=%v", result.DOCX, st, err)
	}
}

func TestCreateArgosDocumentWritesMarkdownTableAsDOCXTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	body := strings.Join([]string{
		"| 일시 | 영역 | 결과 |",
		"| --- | --- | --- |",
		"| 2026-06-02 | 문서 | 업무 보고 작성 |",
	}, "\n")
	result := CreateArgosDocument(context.Background(), "표 테스트", body)
	if !result.OK || result.DOCX == "" {
		t.Fatalf("result=%#v", result)
	}
	xml := readDocxDocumentXML(t, result.DOCX)
	if !strings.Contains(xml, "<w:tbl>") || !strings.Contains(xml, "업무 보고 작성") {
		t.Fatalf("docx should contain a real table, xml=%s", xml)
	}
}

func TestCreateSpreadsheetWritesEditableXLSXAndCSV(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	body := strings.Join([]string{
		"| 구분 | 예산 | 실사용 |",
		"| --- | --- | --- |",
		"| 서버 | 100000 | 80000 |",
	}, "\n")
	result := CreateSpreadsheet(context.Background(), "월간 예산표", body)
	if !result.OK || result.XLSX == "" || result.CSV == "" || result.URL == "" {
		t.Fatalf("result=%#v", result)
	}
	xml := readZipFile(t, result.XLSX, "xl/worksheets/sheet1.xml")
	if !strings.Contains(xml, "서버") || !strings.Contains(xml, "100000") {
		t.Fatalf("xlsx worksheet missing table content: %s", xml)
	}
	csvData, err := os.ReadFile(result.CSV)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(csvData), "서버") {
		t.Fatalf("csv missing row: %s", string(csvData))
	}
	if !strings.Contains(filepath.Base(result.XLSX), "월간-예산표") {
		t.Fatalf("xlsx filename should preserve Korean title: %s", result.XLSX)
	}
}

func readDocxDocumentXML(t *testing.T, path string) string {
	return readZipFile(t, path, "word/document.xml")
}

func readZipFile(t *testing.T, path, name string) string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return string(data)
	}
	t.Fatalf("%s not found in %s", name, path)
	return ""
}

func TestCreatePresentationWritesVerifiedPPTX(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	result := CreatePresentation(context.Background(), "제품 계획", "# 목표\n- 비서 도구 강화\n# 산출물\n- PPTX 생성", "팀 공유", 4, "")
	if !result.OK || result.PPTX == "" || result.Markdown == "" {
		t.Fatalf("result=%#v", result)
	}
	if result.URL == "" {
		t.Fatalf("missing presentation preview html: %#v", result)
	}
	if st, err := os.Stat(result.PPTX); err != nil || st.IsDir() || st.Size() == 0 {
		t.Fatalf("pptx not written: path=%q stat=%#v err=%v", result.PPTX, st, err)
	}
	if st, err := os.Stat(result.URL); err != nil || st.IsDir() || st.Size() == 0 {
		t.Fatalf("preview html not written: path=%q stat=%#v err=%v", result.URL, st, err)
	}
	if err := verifySimplePPTX(result.PPTX, 4); err != nil {
		t.Fatalf("pptx verification failed: %v", err)
	}
	verified := VerifyPresentation(context.Background(), result.PPTX)
	if !verified.OK || !strings.Contains(verified.Stdout, "4 slides") {
		t.Fatalf("verified=%#v", verified)
	}
	edited := EditPresentation(context.Background(), result.PPTX, "추가 계획", "- 검증 추가\n- 공유 준비", "", true)
	if !edited.OK || edited.PPTX == result.PPTX || !strings.Contains(edited.Stdout, "5 slides") {
		t.Fatalf("edited=%#v", edited)
	}
	if _, err := os.Stat(edited.PPTX); err != nil {
		t.Fatalf("edited pptx missing: %v", err)
	}
	if data, err := os.ReadFile(result.Markdown); err != nil || !strings.Contains(string(data), "제품 계획") {
		t.Fatalf("outline missing title: err=%v data=%s", err, string(data))
	}
}

func TestCreatePresentationUsesLanguagePackForEnglishDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	result := CreatePresentation(context.Background(), "Executive Brief", "# Goal\n- Make the decision package usable\n# Next Action\n- Review the attached tracker", "Leadership", 4, "")
	if !result.OK || result.Markdown == "" || result.URL == "" {
		t.Fatalf("result=%#v", result)
	}
	for _, path := range []string{result.Markdown, result.URL} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if containsHangulLocal(string(data)) {
			t.Fatalf("English presentation artifact should not include Korean text: %s\n%s", path, string(data))
		}
	}
}

func TestSaveAutomationResultWritesMarkdownAndHTML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	result := SaveAutomationResult(context.Background(), "터미널 결과", "본문입니다.", "test")
	if !result.OK || result.Markdown == "" || result.URL == "" {
		t.Fatalf("result=%#v", result)
	}
	if st, err := os.Stat(result.Markdown); err != nil || st.IsDir() || st.Size() == 0 {
		t.Fatalf("markdown not written: path=%q stat=%#v err=%v", result.Markdown, st, err)
	}
	if st, err := os.Stat(result.URL); err != nil || st.IsDir() || st.Size() == 0 {
		t.Fatalf("html not written: path=%q stat=%#v err=%v", result.URL, st, err)
	}
}

func containsHangulLocal(value string) bool {
	for _, r := range value {
		if unicode.In(r, unicode.Hangul) {
			return true
		}
	}
	return false
}

func TestRunTerminalTaskSavesOutputArtifact(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	result := RunTerminalTask(context.Background(), "/bin/sh", "printf codex-result", "터미널 테스트", true)
	if !result.OK || result.Stdout != "codex-result" || result.Markdown == "" || result.URL == "" {
		t.Fatalf("result=%#v", result)
	}
	data, err := os.ReadFile(result.Markdown)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "codex-result") {
		t.Fatalf("artifact missing command output: %s", string(data))
	}
}

func TestMapsSearchAndDirectionsReturnUsableURLs(t *testing.T) {
	search := MapsSearch(context.Background(), "서울역", "apple", false)
	if !search.OK || !strings.Contains(search.URL, "maps.apple.com") || !strings.Contains(search.URL, "%EC%84%9C%EC%9A%B8%EC%97%AD") {
		t.Fatalf("search=%#v", search)
	}
	directions := MapsDirections(context.Background(), "강남역", "서울역", "transit", "google", false)
	if !directions.OK || !strings.Contains(directions.URL, "google.com/maps/dir") || !strings.Contains(directions.URL, "travelmode=transit") {
		t.Fatalf("directions=%#v", directions)
	}
}

func TestMacRunnerCommandPermissionIsGrantable(t *testing.T) {
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(t.TempDir(), "permissions.json"))
	action := ClassifyArgosAction("맥에서 Pages 문서 작성해줘. 제목은 Argos 통합 테스트.")
	decision := CheckArgosPermission(action)
	if !decision.Grantable || decision.Scope != "mac_runner" || decision.Label == "" {
		t.Fatalf("decision=%#v", decision)
	}
}

func TestRunMacRunnerCommandUsesSSHExecutor(t *testing.T) {
	dir := t.TempDir()
	ssh := filepath.Join(dir, "ssh")
	if err := os.WriteFile(ssh, []byte("#!/bin/sh\nprintf '%s\\n' '{\"success\":true,\"reply\":\"Mac 실행기에서 처리했습니다.\"}'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MESHCLAW_ARGOS_MAC_RUNNER_SSH_TARGET", "operator@test")
	t.Setenv("MESHCLAW_ARGOS_MAC_RUNNER_PROJECT", "/tmp/Argos Project")
	t.Setenv("MESHCLAW_ARGOS_MAC_RUNNER_PYTHON", "python3")

	result := RunMacRunnerCommand(context.Background(), "Pages 문서 작성해줘")
	if !result.OK || !strings.Contains(result.Stdout, "Mac 실행기에서 처리했습니다") {
		t.Fatalf("result=%#v", result)
	}
	if len(result.Command) != 3 || result.Command[0] != "ssh" || result.Command[1] != "operator@test" {
		t.Fatalf("command=%#v", result.Command)
	}
}

func TestRunMacRunnerCommandUsesSelectedRegistryRunner(t *testing.T) {
	dir := t.TempDir()
	ssh := filepath.Join(dir, "ssh")
	if err := os.WriteFile(ssh, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > "+filepath.Join(dir, "argv")+"\nprintf '%s\\n' '{\"success\":true,\"reply\":\"registry runner\"}'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MESHCLAW_MAC_RUNNERS_FILE", filepath.Join(dir, "mac-runners.json"))
	_, _, err := UpsertMacRunner(MacRunner{
		ID:           "imac",
		SSHTarget:    "operator@imac",
		Project:      "/tmp/Runner Project",
		Python:       "python3.11",
		Capabilities: []string{"pages", "calendar"},
		Enabled:      true,
		Selected:     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	result := RunMacRunnerCommand(context.Background(), "Pages 문서 작성해줘")
	if !result.OK || !strings.Contains(result.Stdout, "registry runner") {
		t.Fatalf("result=%#v", result)
	}
	if len(result.Command) != 3 || result.Command[1] != "operator@imac" {
		t.Fatalf("command=%#v", result.Command)
	}
}

func TestDoctorMacRunnerUsesSelectedRegistryRunner(t *testing.T) {
	dir := t.TempDir()
	ssh := filepath.Join(dir, "ssh")
	fakeSSH := `#!/bin/sh
last=""
for arg in "$@"; do
  last="$arg"
done
case "$last" in
  *uname*) printf 'meshclaw-mac-runner-ok\nDarwin\n15.6.1\n' ;;
  *"test -d"*) printf 'project ok: /tmp/Runner Project\n' ;;
  *"command -v"*) printf 'Python 3.11.9\n' ;;
  *signal_bridge.py*) printf 'local-command bridge present\n' ;;
  *) printf 'ok\n' ;;
esac
`
	if err := os.WriteFile(ssh, []byte(fakeSSH), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MESHCLAW_MAC_RUNNERS_FILE", filepath.Join(dir, "mac-runners.json"))
	_, _, err := UpsertMacRunner(MacRunner{
		ID:        "imac",
		SSHTarget: "operator@imac",
		Project:   "/tmp/Runner Project",
		Python:    "python3.11",
		Enabled:   true,
		Selected:  true,
	})
	if err != nil {
		t.Fatal(err)
	}

	report := DoctorMacRunner(context.Background(), "")
	if !report.OK || report.Runner.ID != "imac" || len(report.Checks) != 4 {
		t.Fatalf("report=%#v", report)
	}
	for _, check := range report.Checks {
		if !check.OK || check.Command[1] != "operator@imac" {
			t.Fatalf("check=%#v", check)
		}
	}
}

func TestDoctorMacRunnerNoRunner(t *testing.T) {
	t.Setenv("MESHCLAW_MAC_RUNNERS_FILE", filepath.Join(t.TempDir(), "mac-runners.json"))
	report := DoctorMacRunner(context.Background(), "")
	if report.OK || len(report.Problems) == 0 || !strings.Contains(report.Problems[0], "no mac runner") {
		t.Fatalf("report=%#v", report)
	}
}

func TestArgosDoMacRunnerDoctorNoRunner(t *testing.T) {
	t.Setenv("MESHCLAW_MAC_RUNNERS_FILE", filepath.Join(t.TempDir(), "mac-runners.json"))
	action := ArgosDo(context.Background(), ArgosRequest{Text: "맥 실행기 상태 봐줘", Execute: true})
	if action.Action != "mac_runner_doctor" || action.Result == nil || action.Result.OK || !strings.Contains(action.Error, "no mac runner") {
		t.Fatalf("action=%#v", action)
	}
	if len(action.NextActions) == 0 {
		t.Fatalf("missing next actions: %#v", action)
	}
}

func TestMacRunnerCommandSavedPath(t *testing.T) {
	stdout := `{"command_result":{"result":{"saved_to":"/Users/example/Documents/New project/outputs/test.pages"}}}`
	if got := macRunnerCommandSavedPath(stdout); got != "/Users/example/Documents/New project/outputs/test.pages" {
		t.Fatalf("saved path=%q", got)
	}
}

func TestMarkdownWorkDemo(t *testing.T) {
	out := markdownWorkDemo("Argos Work Demo\n\nRequest: MeshClaw\nSummary:\n- item")
	if !strings.Contains(out, "# Argos Work Demo") || !strings.Contains(out, "## Summary") {
		t.Fatalf("markdown=%s", out)
	}
}

func TestArgosPermissionGrantAndCheck(t *testing.T) {
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(t.TempDir(), "permissions.json"))
	action := ClassifyArgosAction("Safari 열어줘")
	decision := CheckArgosPermission(action)
	if !decision.Grantable || decision.Allowed {
		t.Fatalf("decision before grant=%#v", decision)
	}
	grant, err := GrantArgosPermission(action, "test", "unit")
	if err != nil {
		t.Fatal(err)
	}
	if grant.Action != "open_app" || grant.Scope != "safari" {
		t.Fatalf("grant=%#v", grant)
	}
	decision = CheckArgosPermission(action)
	if !decision.Allowed || decision.Grant == nil || decision.Grant.ID != grant.ID {
		t.Fatalf("decision after grant=%#v", decision)
	}
}

func TestArgosPermissionVisibleSearchReusesWebSearchGrant(t *testing.T) {
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(t.TempDir(), "permissions.json"))
	search := ClassifyArgosAction("가나 경제 뉴스 웹에서 검색해줘")
	if _, err := GrantArgosPermission(search, "test", "unit"); err != nil {
		t.Fatal(err)
	}
	visible := ClassifyArgosAction("브라우저에서 가나 경제 뉴스 검색해줘")
	decision := CheckArgosPermission(visible)
	if !decision.Allowed || decision.Action != "browser_search" || decision.Scope != "web_search" {
		t.Fatalf("decision=%#v", decision)
	}
}

func TestArgosPermissionRejectsClipboardGrant(t *testing.T) {
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(t.TempDir(), "permissions.json"))
	action := ClassifyArgosAction("클립보드에 secret 복사")
	if action.Action != "clipboard_set" {
		t.Fatalf("action=%#v", action)
	}
	if _, err := GrantArgosPermission(action, "test", "unit"); err == nil {
		t.Fatal("expected clipboard permission grant to fail")
	}
}

func TestArgosPermissionRevoke(t *testing.T) {
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(t.TempDir(), "permissions.json"))
	action := ClassifyArgosAction("open Safari")
	grant, err := GrantArgosPermission(action, "test", "unit")
	if err != nil {
		t.Fatal(err)
	}
	revoked, ok, err := RevokeArgosPermission(grant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || revoked.ID != grant.ID {
		t.Fatalf("revoked=%#v ok=%t", revoked, ok)
	}
	decision := CheckArgosPermission(action)
	if decision.Allowed {
		t.Fatalf("decision after revoke=%#v", decision)
	}
}

func TestArgosPermissionRevokeForAction(t *testing.T) {
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(t.TempDir(), "permissions.json"))
	action := ClassifyArgosAction("open Safari")
	if _, err := GrantArgosPermission(action, "test", "unit"); err != nil {
		t.Fatal(err)
	}
	revoked, ok, err := RevokeArgosPermissionForAction(action)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || revoked.Scope != "safari" {
		t.Fatalf("revoked=%#v ok=%t", revoked, ok)
	}
}

func TestScreenRecordDoesNotFallbackToDirectCaptureByDefault(t *testing.T) {
	t.Setenv("MESHCLAW_ARGOS_UI_RUNNER", "http://127.0.0.1:1")
	t.Setenv("MESHCLAW_ARGOS_DIRECT_SCREEN_RECORD", "")
	result := ScreenRecord(context.Background(), 1, filepath.Join(t.TempDir(), "screen.mov"))
	if result.OK {
		t.Fatalf("expected screen record to fail without a runner")
	}
	if len(result.Command) > 0 && result.Command[0] == "screencapture" {
		t.Fatalf("screen recording should not call screencapture directly by default: %#v", result)
	}
}

func TestRecordingResultMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "screen.mov")
	if err := os.WriteFile(path, []byte("movie"), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	var result Result
	annotateRecordingResult(&result, path, st)
	if result.SizeBytes != 5 || result.SHA256 == "" || result.DeleteAt == "" {
		t.Fatalf("metadata missing: %#v", result)
	}
	if !strings.Contains(result.Retention, "ephemeral") {
		t.Fatalf("retention=%q", result.Retention)
	}
}

func TestFinishScreenRecordingRequiresOutputFile(t *testing.T) {
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	result := finishScreenRecording(cmd, 1, filepath.Join(t.TempDir(), "missing.mov"))
	if result.OK {
		t.Fatalf("recording without an output file should fail: %#v", result)
	}
	if !strings.Contains(result.Error, "no output file") {
		t.Fatalf("error=%q", result.Error)
	}
}
