package messenger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/browserauto"
	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/mailadapter"
	"github.com/meshclaw/meshclaw/internal/osauto"
)

func resetRecentSignalMailContextForTest(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "recent-mail-contexts.json")
	t.Setenv("MESHCLAW_RECENT_MAIL_CONTEXTS", path)
	recentSignalMailContexts.Lock()
	recentSignalMailContexts.items = map[string]recentSignalMailContext{}
	recentSignalMailContexts.Unlock()
	t.Cleanup(func() {
		recentSignalMailContexts.Lock()
		defer recentSignalMailContexts.Unlock()
		recentSignalMailContexts.items = map[string]recentSignalMailContext{}
	})
	return path
}

func resetPendingMailSendForTest(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pending-mail-sends.json")
	t.Setenv("MESHCLAW_PENDING_MAIL_SENDS", path)
	mailSendPending.Lock()
	mailSendPending.items = map[string]pendingMailSend{}
	mailSendPending.Unlock()
	t.Cleanup(func() {
		mailSendPending.Lock()
		defer mailSendPending.Unlock()
		mailSendPending.items = map[string]pendingMailSend{}
	})
	return path
}

func TestParseAssistantToolIntentJSON(t *testing.T) {
	intent, ok := parseAssistantToolIntentJSON(`{"intent":"browser_search","query":"이순신","confidence":0.95}`)
	if !ok {
		t.Fatal("intent was not parsed")
	}
	if intent.Intent != "browser_search" || intent.Query != "이순신" || intent.Confidence != 0.95 {
		t.Fatalf("intent=%#v", intent)
	}
}

func TestParseAssistantToolIntentJSONRejectsMissingPayload(t *testing.T) {
	if intent, ok := parseAssistantToolIntentJSON(`{"intent":"browser_search","confidence":0.95}`); ok {
		t.Fatalf("unexpected intent=%#v", intent)
	}
}

func TestParseAssistantToolIntentJSONAcceptsOpenURL(t *testing.T) {
	intent, ok := parseAssistantToolIntentJSON("```json\n{\"intent\":\"open_url\",\"url\":\"https://example.com\",\"confidence\":0.8}\n```")
	if !ok {
		t.Fatal("intent was not parsed")
	}
	if intent.URL != "https://example.com" {
		t.Fatalf("intent=%#v", intent)
	}
}

func TestParseAssistantToolIntentJSONAcceptsAIHandoff(t *testing.T) {
	intent, ok := parseAssistantToolIntentJSON(`{"intent":"ai_handoff","provider":"codex","prompt":"d1 디스크 정리 계획 세워","confidence":0.9}`)
	if !ok {
		t.Fatal("intent was not parsed")
	}
	if intent.Provider != "codex" || intent.Prompt == "" {
		t.Fatalf("intent=%#v", intent)
	}
}

func TestParseAssistantToolIntentJSONRejectsAIHandoffWithoutPrompt(t *testing.T) {
	if intent, ok := parseAssistantToolIntentJSON(`{"intent":"ai_handoff","provider":"codex","confidence":0.9}`); ok {
		t.Fatalf("unexpected intent=%#v", intent)
	}
}

func TestParseAssistantToolIntentJSONAcceptsAIFrontends(t *testing.T) {
	intent, ok := parseAssistantToolIntentJSON(`{"intent":"ai_frontends","confidence":0.9}`)
	if !ok {
		t.Fatal("intent was not parsed")
	}
	if intent.Intent != "ai_frontends" {
		t.Fatalf("intent=%#v", intent)
	}
}

func TestParseAssistantToolIntentJSONAcceptsArgosAction(t *testing.T) {
	intent, ok := parseAssistantToolIntentJSON(`{"intent":"argos_action","prompt":"Safari 열어줘","confidence":0.9}`)
	if !ok {
		t.Fatal("intent was not parsed")
	}
	if intent.Intent != "argos_action" || intent.Prompt != "Safari 열어줘" {
		t.Fatalf("intent=%#v", intent)
	}
}

func TestArgosPromptFromAssistantToolIntentRoutesToolIntents(t *testing.T) {
	tests := []struct {
		name   string
		intent assistantToolIntent
		want   string
	}{
		{
			name:   "browser search",
			intent: assistantToolIntent{Intent: "browser_search", Query: "가나 경제 뉴스"},
			want:   "검색해줘: 가나 경제 뉴스",
		},
		{
			name:   "browser fetch",
			intent: assistantToolIntent{Intent: "browser_fetch", URL: "https://example.com"},
			want:   "https://example.com 읽고 요약해줘",
		},
		{
			name:   "open url",
			intent: assistantToolIntent{Intent: "open_url", URL: "https://chatgpt.com"},
			want:   "https://chatgpt.com 열어줘",
		},
		{
			name:   "shortcut",
			intent: assistantToolIntent{Intent: "shortcut_run", Name: "Argos Morning", Input: "Seoul"},
			want:   "Argos Morning 단축어 실행해줘. 입력: Seoul",
		},
		{
			name:   "ai handoff",
			intent: assistantToolIntent{Intent: "ai_handoff", Provider: "claude", Prompt: "이 패치 리뷰해"},
			want:   "claude로 넘겨줘: 이 패치 리뷰해",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := argosPromptFromAssistantToolIntent(tt.intent, "")
			if !ok || got != tt.want {
				t.Fatalf("prompt=%q ok=%t, want %q", got, ok, tt.want)
			}
		})
	}
}

func TestFormatSignalArgosActionFailureHidesEvidencePath(t *testing.T) {
	action := osauto.ArgosAction{
		Text:   "Safari 열어줘",
		Action: "open_app",
		App:    "Safari",
		Error:  "permission denied",
	}
	reply := formatSignalArgosAction(action, evidenceRecordForTest("/tmp/evidence.json"), nil)
	if !strings.Contains(reply, "Argos macOS 작업에 실패했습니다") ||
		strings.Contains(reply, "작업 기록도 저장했습니다.") ||
		strings.Contains(reply, "evidence: /tmp/evidence.json") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestFormatSignalArgosActionSuccessIncludesSummary(t *testing.T) {
	action := osauto.ArgosAction{
		Text:   "Safari 열어줘",
		Action: "open_app",
		App:    "Safari",
	}
	reply := formatSignalArgosAction(action, evidenceRecordForTest("/tmp/evidence.json"), nil)
	if !strings.Contains(reply, "한 일: Safari 앱을 열었습니다") || !strings.Contains(reply, "작업: open_app") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestFormatSignalArgosActionBrowserSearchIsNatural(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	doc := filepath.Join(home, "Documents", "Argos Vault", "Work Reports", "argos-search.md")
	if err := os.MkdirAll(filepath.Dir(doc), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(doc, []byte("# report"), 0600); err != nil {
		t.Fatal(err)
	}
	html := strings.TrimSuffix(doc, filepath.Ext(doc)) + ".html"
	if err := os.WriteFile(html, []byte("<html>report</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	action := osauto.ArgosAction{
		Text:       "브라우저로 이순신 검색해서 요약 리포트를 써줘",
		Action:     "visible_browser_search",
		OutputPath: doc,
		Search: &browserauto.SearchResult{Query: "이순신 요약 리포트", Results: []browserauto.Link{
			{Text: "이순신 - 한국민족문화대백과사전", URL: "https://encykorea.aks.ac.kr/Article/E0044900"},
		}},
	}
	reply := formatSignalArgosAction(action, evidenceRecordForTest("/tmp/evidence.json"), nil)
	visible := signalReplyVisibleText(reply)
	if !strings.Contains(visible, "검색해서 리포트 초안을 만들었습니다.") ||
		!strings.Contains(visible, "참고한 출처 후보") ||
		strings.Contains(visible, "작업 문서:") ||
		strings.Contains(visible, "/Documents/Argos Vault/") ||
		strings.Contains(visible, "iPhone 문서 링크") ||
		strings.Contains(visible, "evidence:") {
		t.Fatalf("reply=%q visible=%q", reply, visible)
	}
	attachments := signalReplyAttachments(reply)
	if len(attachments) != 1 || attachments[0] != doc {
		t.Fatalf("attachments=%#v reply=%q", attachments, reply)
	}
}

func TestFormatSignalArgosActionMacRunnerCommandIncludesReply(t *testing.T) {
	action := osauto.ArgosAction{
		Text:   "Pages 문서 작성해줘",
		Action: "mac_runner_command",
		Input:  "Pages 문서 작성해줘",
		Result: &osauto.Result{
			OK:      true,
			Stdout:  `{"success":true,"reply":"문서를 만들었습니다."}`,
			URL:     "/tmp/Signal.pages",
			Preview: "/tmp/Signal.pages.png",
		},
	}
	reply := formatSignalArgosAction(action, evidenceRecordForTest("/tmp/evidence.json"), nil)
	if !strings.Contains(reply, "등록된 Mac 앱 실행기에 작업을 전달했습니다") ||
		!strings.Contains(reply, "Mac 실행기 응답: 문서를 만들었습니다.") ||
		!strings.Contains(reply, "문서 미리보기 이미지: /tmp/Signal.pages.png") ||
		strings.Contains(reply, "evidence: /tmp/evidence.json") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestFormatSignalArgosActionCoupangOpenURLIsUserVisibleCard(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	action := osauto.ArgosAction{
		Text:   "쿠팡에서 생수 500ml 20개 열어줘",
		Action: "open_url",
		URL:    "https://www.coupang.com/np/search?q=%EC%83%9D%EC%88%98+500ml+20%EA%B0%9C",
	}
	reply := formatSignalArgosAction(action, evidenceRecordForTest("/tmp/evidence.json"), nil)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"쿠팡 브라우저 테스트 화면을 열었습니다.",
		"열린 주소: https://www.coupang.com/np/search",
		"지금 화면에서 확인할 것:",
		"쿠팡 계정 로그인 상태",
		"배송지/결제수단이 둘 이상 보이면",
		"다음 Signal 문장:",
		"장바구니 직전 화면까지 준비해",
		"`쿠팡 배송지 2개 확인해` / `쿠팡 결제수단 확인해`",
		"구매 실행 승인",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("Coupang open URL card missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"Argos가 Mac 작업을 처리했습니다", "작업: open_url", "증거:", "evidence:"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("Coupang open URL card should not expose %q:\n%s", unwanted, visible)
		}
	}
}

func TestFormatSignalArgosActionCoupangOpenURLEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	action := osauto.ArgosAction{
		Text:   "open Coupang search",
		Action: "open_url",
		URL:    "https://www.coupang.com/",
	}
	reply := formatSignalArgosAction(action, evidenceRecordForTest("/tmp/evidence.json"), nil)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Opened the Coupang browser-test screen.",
		"Opened URL: https://www.coupang.com/",
		"Check on screen now:",
		"Coupang account login state",
		"If multiple addresses or payment methods are visible",
		"Next Signal messages:",
		"`check Coupang two shipping addresses` / `check Coupang payment method`",
		"purchase execution approved",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("English Coupang open URL card missing %q:\n%s", want, visible)
		}
	}
	if assistantCheckoutPrepContainsHangul(visible) {
		t.Fatalf("English Coupang open URL card should not expose Korean:\n%s", visible)
	}
	for _, unwanted := range []string{"Argos가 Mac 작업을 처리했습니다", "작업: open_url", "evidence:"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("English Coupang open URL card should not expose %q:\n%s", unwanted, visible)
		}
	}
}

func TestFormatSignalArgosActionCalendarList(t *testing.T) {
	start := time.Now().Add(24 * time.Hour)
	end := start.Add(24 * time.Hour)
	eventStart := start.Add(15 * time.Hour)
	eventEnd := eventStart.Add(time.Hour)
	action := osauto.ArgosAction{
		Text:          "내일 일정 뭐 있어?",
		Action:        "calendar_events_list",
		CalendarStart: start.Format(time.RFC3339),
		CalendarEnd:   end.Format(time.RFC3339),
		Result:        &osauto.Result{OK: true, Stdout: fmt.Sprintf(`{"kind":"argos_calendar_list","ok":true,"stdout":"{\"kind\":\"argos_calendar_helper\",\"ok\":true,\"count\":1,\"events\":[{\"title\":\"Argos 회의\",\"start\":\"%s\",\"end\":\"%s\",\"calendar\":\"Home\"}]}"}`, eventStart.Format(time.RFC3339), eventEnd.Format(time.RFC3339))},
	}
	reply := formatSignalArgosAction(action, evidenceRecordForTest("/tmp/evidence.json"), nil)
	visible := signalReplyVisibleText(reply)
	if !strings.Contains(visible, "일정을 확인했습니다.") ||
		!strings.Contains(visible, "일정 1개") ||
		!strings.Contains(visible, "Argos 회의") ||
		!strings.Contains(visible, eventStart.In(time.FixedZone("KST", 9*60*60)).Format("2006-01-02 15:04")) ||
		strings.Contains(visible, "Argos가 Mac 작업을 처리했습니다") ||
		strings.Contains(visible, "작업: calendar_events_list") ||
		strings.Contains(visible, "T") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestFormatSignalArgosActionCalendarCreate(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	action := osauto.ArgosAction{
		Text:          "내일 오후 3시에 Argos 기능 점검 회의 일정 추가해줘",
		Action:        "calendar_event_create",
		CalendarTitle: "Argos 기능 점검 회의",
		CalendarNotes: "Signal 요청: 기능 점검",
		CalendarStart: "2026-06-15T15:00:00+09:00",
		CalendarEnd:   "2026-06-15T16:00:00+09:00",
		Result:        &osauto.Result{OK: true, Stdout: `{"kind":"argos_calendar_create","ok":true,"stdout":"{\"kind\":\"argos_calendar_helper\",\"ok\":true,\"id\":\"evt1\",\"title\":\"Argos 기능 점검 회의\"}"}`},
	}
	reply := formatSignalArgosAction(action, evidenceRecordForTest("/tmp/evidence.json"), nil)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{"일정을 만들었습니다.", "일정: Argos 기능 점검 회의", "시간: 2026-06-15 15:00 ~ 16:00", "메모: Signal 요청: 기능 점검", "내일 일정 뭐 있어?", "승인 대기열 보여줘"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("missing %q\nreply=%q\nvisible=%q", want, reply, visible)
		}
	}
	for _, notWant := range []string{"Argos가 Mac 작업을 처리했습니다", "작업: calendar_event_create", "evidence:", "2026-06-15T15:00:00"} {
		if strings.Contains(visible, notWant) {
			t.Fatalf("unexpected %q\nreply=%q\nvisible=%q", notWant, reply, visible)
		}
	}
}

func TestFormatSignalArgosActionCalendarCreateEnglish(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	action := osauto.ArgosAction{
		Text:          "add a calendar event tomorrow at 3 for Argos review",
		Action:        "calendar_event_create",
		CalendarTitle: "Argos review",
		CalendarStart: "2026-06-15T15:00:00+09:00",
		CalendarEnd:   "2026-06-15T16:00:00+09:00",
		Result:        &osauto.Result{OK: true, Stdout: `{"kind":"argos_calendar_create","ok":true,"title":"Argos review"}`},
	}
	reply := formatSignalArgosAction(action, evidenceRecordForTest("/tmp/evidence.json"), nil)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{"Created a calendar event.", "Event: Argos review", "Time: 2026-06-15 15:00 ~ 16:00", "what is on my calendar tomorrow?", "approval queue"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("missing %q\nreply=%q\nvisible=%q", want, reply, visible)
		}
	}
	for _, notWant := range []string{"일정", "작업: calendar_event_create", "2026-06-15T15:00:00"} {
		if strings.Contains(visible, notWant) {
			t.Fatalf("unexpected %q\nreply=%q\nvisible=%q", notWant, reply, visible)
		}
	}
}

func TestFormatSignalArgosActionCalendarListPermissionGuidance(t *testing.T) {
	action := osauto.ArgosAction{
		Text:          "내일 일정 뭐 있어?",
		Action:        "calendar_events_list",
		CalendarStart: "2026-06-14T00:00:00+09:00",
		CalendarEnd:   "2026-06-15T00:00:00+09:00",
		Error:         "Calendar permission prompt timed out. Open Argos UI Runner once on this Mac and allow Calendar access, then retry.",
	}
	reply := formatSignalArgosAction(action, evidenceRecordForTest("/tmp/evidence.json"), nil)
	for _, want := range []string{"일정 조회가 Mac 권한에서 멈췄습니다.", "시간: 2026-06-14 00:00 ~ 2026-06-15 00:00", "Argos UI Runner.app", "같은 문장"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("calendar permission guidance missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "Argos macOS 작업에 실패했습니다.") {
		t.Fatalf("calendar permission should use specific guidance, got:\n%s", reply)
	}
	if strings.Contains(reply, "Signal dispatcher") || strings.Contains(reply, "2026-06-14T00:00:00") {
		t.Fatalf("calendar permission guidance should stay mobile-friendly:\n%s", reply)
	}
}

func TestFormatSignalArgosActionReminderList(t *testing.T) {
	action := osauto.ArgosAction{
		Text:          "오늘 할 일 뭐 있어?",
		Action:        "reminders_list",
		ReminderStart: "2026-05-26T00:00:00+09:00",
		ReminderEnd:   "2026-05-27T00:00:00+09:00",
		Result:        &osauto.Result{OK: true, Stdout: `{"kind":"argos_reminder_list","ok":true,"stdout":"{\"kind\":\"argos_reminder_helper\",\"ok\":true,\"count\":1,\"reminders\":[{\"title\":\"우유 사기\",\"due\":\"2026-05-26T09:00:00+09:00\",\"calendar\":\"Reminders\"}]}"}`},
	}
	reply := formatSignalArgosAction(action, evidenceRecordForTest("/tmp/evidence.json"), nil)
	visible := signalReplyVisibleText(reply)
	if !strings.Contains(visible, "할 일 목록을 확인했습니다.") ||
		!strings.Contains(visible, "리마인더 1개") ||
		!strings.Contains(visible, "우유 사기") ||
		!strings.Contains(visible, "2026-05-26 09:00") ||
		strings.Contains(visible, "Argos가 Mac 작업을 처리했습니다") ||
		strings.Contains(visible, "작업: reminders_list") ||
		strings.Contains(visible, "T") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestFormatSignalArgosActionReminderCreate(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	action := osauto.ArgosAction{
		Text:          "내일 오전 9시에 계약서 검토 리마인더 추가해줘",
		Action:        "reminder_create",
		ReminderTitle: "계약서 검토",
		ReminderNotes: "NDA 조항 먼저 확인",
		ReminderDue:   "2026-06-15T09:00:00+09:00",
		Result:        &osauto.Result{OK: true, Stdout: `{"kind":"argos_reminder_create","ok":true,"stdout":"{\"kind\":\"argos_reminder_helper\",\"ok\":true,\"id\":\"r1\",\"title\":\"계약서 검토\",\"due\":\"2026-06-15T09:00:00+09:00\"}"}`},
	}
	reply := formatSignalArgosAction(action, evidenceRecordForTest("/tmp/evidence.json"), nil)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{"리마인더를 만들었습니다.", "할 일: 계약서 검토", "알림 시간: 2026-06-15 09:00", "메모: NDA 조항 먼저 확인", "오늘 할 일 뭐 있어?", "승인 대기열 보여줘"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("missing %q\nreply=%q\nvisible=%q", want, reply, visible)
		}
	}
	for _, notWant := range []string{"Argos가 Mac 작업을 처리했습니다", "작업: reminder_create", "evidence:", "2026-06-15T09:00:00"} {
		if strings.Contains(visible, notWant) {
			t.Fatalf("unexpected %q\nreply=%q\nvisible=%q", notWant, reply, visible)
		}
	}
}

func TestFormatSignalArgosActionReminderCreateEnglish(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	action := osauto.ArgosAction{
		Text:          "remind me tomorrow at 9 to review contract",
		Action:        "reminder_create",
		ReminderTitle: "Review contract",
		ReminderDue:   "2026-06-15T09:00:00+09:00",
		Result:        &osauto.Result{OK: true, Stdout: `{"kind":"argos_reminder_create","ok":true,"title":"Review contract","due":"2026-06-15T09:00:00+09:00"}`},
	}
	reply := formatSignalArgosAction(action, evidenceRecordForTest("/tmp/evidence.json"), nil)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{"Created a reminder.", "Task: Review contract", "Reminder time: 2026-06-15 09:00", "what are my tasks today?", "approval queue"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("missing %q\nreply=%q\nvisible=%q", want, reply, visible)
		}
	}
	for _, notWant := range []string{"리마인더", "할 일", "작업: reminder_create", "2026-06-15T09:00:00"} {
		if strings.Contains(visible, notWant) {
			t.Fatalf("unexpected %q\nreply=%q\nvisible=%q", notWant, reply, visible)
		}
	}
}

func TestFormatSignalArgosActionReminderCreatePermissionGuidance(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	action := osauto.ArgosAction{
		Text:          "10분 뒤에 Argos 언어팩 결과 확인 리마인더 추가해줘",
		Action:        "reminder_create",
		ReminderTitle: "Argos 언어팩 결과 확인",
		ReminderDue:   "2026-06-14T12:47:53+09:00",
		Error:         "Reminders permission prompt timed out. Open Argos UI Runner once on this Mac and allow Reminders access, then retry.",
	}
	reply := formatSignalArgosAction(action, evidenceRecordForTest("/tmp/evidence.json"), nil)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{"리마인더 생성이 Mac 권한에서 멈췄습니다.", "할 일: Argos 언어팩 결과 확인", "알림 시간: 2026-06-14 12:47", "Argos UI Runner.app", "같은 문장"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("missing %q\nreply=%q\nvisible=%q", want, reply, visible)
		}
	}
	for _, notWant := range []string{"helper", "dispatcher", "2026-06-14T12:47:53", "작업: reminder_create"} {
		if strings.Contains(visible, notWant) {
			t.Fatalf("unexpected %q\nreply=%q\nvisible=%q", notWant, reply, visible)
		}
	}
}

func TestFormatSignalArgosActionContactsSearch(t *testing.T) {
	action := osauto.ArgosAction{
		Text:         "연락처에서 홍길동 전화번호 찾아줘",
		Action:       "contacts_search",
		ContactQuery: "홍길동",
		Result:       &osauto.Result{OK: true, Stdout: `{"kind":"argos_contacts_search","ok":true,"stdout":"{\"kind\":\"argos_contacts_helper\",\"ok\":true,\"count\":1,\"contacts\":[{\"name\":\"홍길동\",\"phones\":[\"010-1234-5678\"],\"emails\":[\"hong@example.com\"]}]}"}`},
	}
	reply := formatSignalArgosAction(action, evidenceRecordForTest("/tmp/evidence.json"), nil)
	if !strings.Contains(reply, "연락처 1개") || !strings.Contains(reply, "010-1234-5678") || !strings.Contains(reply, "hong@example.com") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestFormatSignalArgosApprovalReminderDeleteIsPerAction(t *testing.T) {
	action := osauto.ClassifyArgosAction("우유 사기 리마인더 삭제해줘")
	decision := osauto.CheckArgosPermission(action)
	reply := formatSignalArgosApprovalRequest(action, decision)
	if !strings.Contains(reply, "매번 확인") || strings.Contains(reply, "항상 허용") {
		t.Fatalf("reply=%q decision=%+v", reply, decision)
	}
}

func evidenceRecordForTest(path string) evidence.Record {
	return evidence.Record{StoredAt: path}
}

func TestParseAssistantToolIntentJSONAcceptsMeshClawReport(t *testing.T) {
	intent, ok := parseAssistantToolIntentJSON(`{"intent":"meshclaw_report","query":"latest evidence","limit":100,"confidence":0.9}`)
	if !ok {
		t.Fatal("intent was not parsed")
	}
	if intent.Intent != "meshclaw_report" || intent.Query != "latest evidence" || intent.Limit != 8 {
		t.Fatalf("intent=%#v", intent)
	}
}

func TestParseAssistantToolIntentJSONRejectsOpenWebUIStatus(t *testing.T) {
	if intent, ok := parseAssistantToolIntentJSON(`{"intent":"openwebui_status","confidence":0.9}`); ok {
		t.Fatalf("legacy Open WebUI intent should not be accepted by Signal assistant: %#v", intent)
	}
}

func TestParseAssistantToolIntentJSONAcceptsMailSummary(t *testing.T) {
	intent, ok := parseAssistantToolIntentJSON(`{"intent":"mail_summary","limit":100,"confidence":0.9}`)
	if !ok {
		t.Fatal("intent was not parsed")
	}
	if intent.Intent != "mail_summary" || intent.Limit != 10 {
		t.Fatalf("intent=%#v", intent)
	}
}

func TestFallbackAssistantToolIntentMailSummary(t *testing.T) {
	intent, ok := fallbackAssistantToolIntent("최근 메일 요약해줘")
	if !ok {
		t.Fatal("expected narrow fallback to classify mail summary")
	}
	if intent.Intent != "mail_summary" || intent.Limit != 10 || intent.Confidence < 0.8 {
		t.Fatalf("intent=%#v", intent)
	}
}

func TestFallbackAssistantToolIntentRecentMailFullList(t *testing.T) {
	for _, input := range []string{"최근 메일 전체 보여줘", "최근 메일 목록 보여줘", "show recent mail"} {
		if isRecentSignalMailReadText(input) {
			t.Fatalf("broad list request should not require recent mail context: %q", input)
		}
		intent, ok := fallbackAssistantToolIntent(input)
		if !ok {
			t.Fatalf("expected broad list fallback for %q", input)
		}
		if intent.Intent != "mail_summary" || intent.Limit != 10 {
			t.Fatalf("intent for %q = %#v", input, intent)
		}
	}
}

func TestSignalMailCategorySearchScheduleFormat(t *testing.T) {
	category, ok := detectSignalMailCategorySearch("오늘 메일에서 일정 관련만 찾아줘")
	if !ok {
		t.Fatal("schedule mail category was not detected")
	}
	if category.Name != "일정/회의 관련" {
		t.Fatalf("category=%#v", category)
	}
	results := []mailadapter.SearchResult{{
		Account: mailadapter.AccountPublic{ID: "work"},
		Messages: []mailadapter.MessageSummary{
			{
				ID:      "promo",
				Subject: "6월 할인 안내",
				From:    "promo@example.com",
				Date:    time.Date(2026, 6, 13, 1, 0, 0, 0, time.UTC),
				Snippet: "이번 주 프로모션입니다.",
			},
			{
				ID:      "meet",
				Subject: "내일 회의 일정 확인",
				From:    "client@example.com",
				Date:    time.Date(2026, 6, 13, 3, 0, 0, 0, time.UTC),
				Snippet: "오전 미팅 가능 시간 확인 부탁드립니다.",
			},
		},
	}}
	items := filterMailCategoryResults(results, category)
	if len(items) != 1 || items[0].Message.ID != "meet" {
		t.Fatalf("items=%#v", items)
	}
	reply := formatSignalMailCategorySearch(category, items, nil, evidence.Record{StoredAt: "/tmp/evidence.json"}, nil, true, 2)
	for _, want := range []string{
		"오늘 메일에서 일정/회의 관련 후보를 확인했습니다.",
		"후보 1개를 찾았습니다.",
		"내일 회의 일정 확인",
		"잡힌 신호:",
		"첫 번째 메일 본문 보여줘",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("category reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"6월 할인 안내", "/tmp/evidence", "작업 기록도 저장했습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("category reply exposed %q:\n%s", bad, reply)
		}
	}

	none := formatSignalMailCategorySearch(category, nil, nil, evidence.Record{}, nil, true, 20)
	if !strings.Contains(none, "20개를 확인했지만 일정/회의 관련 후보는 없습니다.") {
		t.Fatalf("none reply=%s", none)
	}
}

func TestFallbackAssistantToolIntentEmailCheckReadsMail(t *testing.T) {
	for _, input := range []string{"이메일 체크해줘", "메일 확인해줘", "새 메일 왔어?", "check email"} {
		intent, ok := fallbackAssistantToolIntent(input)
		if !ok {
			t.Fatalf("expected mail check fallback for %q", input)
		}
		if intent.Intent != "mail_watch" || intent.Limit != 10 {
			t.Fatalf("intent for %q = %#v", input, intent)
		}
	}
}

func TestFallbackAssistantToolIntentAvoidsMailMutation(t *testing.T) {
	if intent, ok := fallbackAssistantToolIntent("이 메일 삭제해줘"); ok {
		t.Fatalf("mutation should not use narrow fallback: %#v", intent)
	}
}

func TestFormatSignalMailMessageTimeUsesKST(t *testing.T) {
	value := time.Date(2026, 6, 2, 3, 5, 0, 0, time.UTC)
	if got := formatSignalMailMessageTime(value); got != "2026-06-02 12:05" {
		t.Fatalf("time=%q", got)
	}
}

func TestFormatSignalMailMultiAccountSummaryIsConcise(t *testing.T) {
	payload := signalMailMultiSearchResult{
		Intent: "mail_summary",
		Results: []mailadapter.SearchResult{{
			Account: mailadapter.AccountPublic{ID: "one"},
			Messages: []mailadapter.MessageSummary{{
				ID:      "1",
				Subject: "인보이스",
				From:    "billing@example.com",
				Date:    time.Date(2026, 6, 2, 3, 5, 0, 0, time.UTC),
				Snippet: "<SECRET:high_entropy:1>\n<!DOCTYPE html>\n확인 가능한 결제 일정입니다.",
			}},
		}},
	}
	reply := formatSignalMailMultiAccountRead(payload, evidence.Record{}, nil)
	if !strings.Contains(reply, "최근 메일을 확인했습니다.") ||
		!strings.Contains(reply, "2026-06-02 12:05") ||
		!strings.Contains(reply, "billing@example.com") ||
		!strings.Contains(reply, "요약: 확인 가능한 결제 일정입니다.") ||
		!strings.Contains(reply, "첫 번째 메일 본문 보여줘") ||
		strings.Contains(reply, "작업 기록도 저장했습니다") ||
		strings.Contains(reply, "[1]") ||
		strings.Contains(reply, "SECRET") ||
		strings.Contains(reply, "DOCTYPE") ||
		strings.Contains(reply, "=EC=") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantMailPriorityRequestAndFormat(t *testing.T) {
	if !isAssistantMailPriorityRequest("최근 메일 요약하고 답장 필요한 것만 우선순위로 정리해줘") {
		t.Fatal("priority request was not detected")
	}
	if !isAssistantMailPriorityRequest("최근 메일을 확인해서 답장 필요한 것만 우선순위로 정리하고 보고방에 보내줘") {
		t.Fatal("priority request with report-room delivery was not detected")
	}
	if !isAssistantMailPriorityRequest("이메일 체크해서 보고방에 보내줘. 음성으로도 준비해줘") {
		t.Fatal("targeted plain mail check should create a visible mail package")
	}
	if !isAssistantMailPriorityRequest("최근 이메일 요약해서 비서방에 보내줘") {
		t.Fatal("targeted plain mail summary should create a visible mail package")
	}
	if isAssistantMailPriorityRequest("partner@example.com에게 메일 보내줘") {
		t.Fatal("direct mail send should not be treated as a mail priority package")
	}
	if isAssistantMailPriorityRequest("첫 번째 메일 삭제해줘") {
		t.Fatal("destructive mail action should not be treated as a mail priority package")
	}
	if isAssistantMailPriorityRequest("내일 제품회의용 회의자료 패키지를 만들어서 보고방에 보내줘. 내용: Argos 비서는 회의록, 시장조사, 여행계획, 메일 요약을 실제 결과물로 보내야 한다. 5장 PPT와 요약 문서로 준비해줘.") {
		t.Fatal("meeting material package text should not be misrouted into mail priority")
	}
	items := mailPriorityItemsFromResults([]mailadapter.SearchResult{{
		Account: mailadapter.AccountPublic{ID: "work"},
		Messages: []mailadapter.MessageSummary{
			{
				ID:      "low",
				Subject: "주간 뉴스레터",
				From:    "newsletter@example.com",
				Date:    time.Date(2026, 6, 2, 1, 0, 0, 0, time.UTC),
				Snippet: "이번 주 새 소식과 프로모션입니다. unsubscribe",
			},
			{
				ID:      "urgent",
				Subject: "계약서 확인 부탁드립니다",
				From:    "partner@example.com",
				Date:    time.Date(2026, 6, 2, 3, 5, 0, 0, time.UTC),
				Snippet: "내일 오전 회의 전까지 검토 후 회신 부탁드립니다.",
			},
			{
				ID:      "meeting",
				Subject: "회의 일정 문의",
				From:    "client@example.com",
				Date:    time.Date(2026, 6, 2, 2, 0, 0, 0, time.UTC),
				Snippet: "다음 주 미팅 가능 시간을 확인 부탁드립니다.",
			},
			{
				ID:      "security",
				Subject: "계정 보안 확인 필요",
				From:    "security@example.com",
				Date:    time.Date(2026, 6, 2, 2, 30, 0, 0, time.UTC),
				Snippet: "새 로그인 알림입니다. 계정 보안과 인증 상태를 확인해주세요.",
			},
			{
				ID:      "invoice",
				Subject: "인보이스 결제 확인",
				From:    "billing@example.com",
				Date:    time.Date(2026, 6, 2, 2, 15, 0, 0, time.UTC),
				Snippet: "청구 금액과 결제 기한 확인이 필요합니다.",
			},
		},
	}})
	if len(items) != 5 {
		t.Fatalf("items=%d", len(items))
	}
	if items[0].Message.ID != "urgent" || items[0].Bucket != "답장 우선" {
		t.Fatalf("highest priority item=%#v", items[0])
	}
	security := classifyMailPriorityItem("work", mailadapter.MessageSummary{
		ID:      "security",
		Subject: "계정 보안 확인 필요",
		From:    "security@example.com",
		Snippet: "새 로그인 알림입니다. 계정 보안과 인증 상태를 확인해주세요.",
	})
	if !strings.Contains(security.Reason, "보안/계정 확인") || !strings.Contains(security.Action, "발신자와 도메인") {
		t.Fatalf("security mail should get security reason/action: %#v", security)
	}
	invoice := classifyMailPriorityItem("work", mailadapter.MessageSummary{
		ID:      "invoice",
		Subject: "인보이스 결제 확인",
		From:    "billing@example.com",
		Snippet: "청구 금액과 결제 기한 확인이 필요합니다.",
	})
	if !strings.Contains(invoice.Reason, "계약/청구/결제 확인") || !strings.Contains(invoice.Action, "금액") {
		t.Fatalf("invoice mail should get invoice reason/action: %#v", invoice)
	}
	auto := classifyMailPriorityItem("work", mailadapter.MessageSummary{
		ID:      "auto",
		Subject: "Google Play 주문 영수증",
		From:    "googleplay-noreply@google.com",
		Snippet: "주문 내역을 확인하세요.",
	})
	if auto.Bucket == "답장 우선" || strings.Contains(auto.Reason, "회신 요청") {
		t.Fatalf("noreply sender should not be treated as reply-needed: %#v", auto)
	}
	if snippet := cleanSignalMailSnippet(`PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
실제 확인할 내용입니다.`, 120); snippet != "실제 확인할 내용입니다." {
		t.Fatalf("DTD noise was not removed: %q", snippet)
	}
	if snippet := cleanSignalMailSnippet(`=C2=A0 =C2=A0
dGQ+DQogICAgICAgICAgI...
Content-Disposition: inline
=ED=95=9C =ED=98=95 =ED=8E=B8 =EA=B8=B0
https://www.netflix.com/watch/82682323?g=3D948eb231-dac7
읽을 수 있는 문장입니다.`, 120); snippet != "읽을 수 있는 문장입니다." {
		t.Fatalf("encoded noise was not removed: %q", snippet)
	}
	reply := formatAssistantMailPriority(items, nil, evidence.Record{StoredAt: "/tmp/evidence.json"}, nil)
	for _, want := range []string{
		"메일 처리 우선순위입니다.",
		"답장 우선",
		"계약서 확인 부탁드립니다",
		"분류 기준: 회신/일정/계약·청구/보안 후보를 우선",
		"이유: 회신 요청",
		"다음 행동: 본문 확인 후 바로 답장 초안을 만드세요.",
		"계정 보안 확인 필요",
		"이유: 보안/계정 확인",
		"인보이스 결제 확인",
		"이유: 계약/청구/결제 확인",
		"회의 일정 문의",
		"첫 번째 메일 답장 초안 써줘",
		"메일 발송/삭제/이동/첨부 저장은 별도 승인 전에는 실행하지 않았습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("priority reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"/tmp/evidence", "작업 기록도 저장했습니다", "evidence", "뉴스레터 / newsletter@example.com"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("priority reply exposed or over-promoted %q:\n%s", bad, reply)
		}
	}
	if strings.Contains(reply, "주간 뉴스레터") || strings.Contains(reply, "newsletter@example.com") {
		t.Fatalf("priority reply should hide low-value promotional mail:\n%s", reply)
	}
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(t.TempDir(), "targets.json"))
	if _, _, err := UpsertTarget(Target{
		ID:      "argos-briefing",
		Channel: "signal",
		GroupID: "group.argos-briefing",
		Label:   "보고방",
		Mode:    "briefing",
	}); err != nil {
		t.Fatal(err)
	}
	sendPreview := formatAssistantMailPrioritySendResult(ListenOptions{TargetID: "assistant-test"}, "보고방", reply, items, nil, evidence.Record{}, nil)
	for _, want := range []string{"메일 우선순위 보고를 Signal로 보낼 준비", "대상: 보고방", "메일 처리 우선순위입니다.", "답장 우선"} {
		if !strings.Contains(sendPreview, want) {
			t.Fatalf("priority send preview missing %q:\n%s", want, sendPreview)
		}
	}
	if strings.Contains(sendPreview, "예약 문의 문구") || strings.Contains(sendPreview, "전화/메시지 초안") {
		t.Fatalf("priority send preview was misrouted into booking draft:\n%s", sendPreview)
	}

	none := formatAssistantMailPriority([]signalMailPriorityItem{
		classifyMailPriorityItem("work", mailadapter.MessageSummary{
			ID:      "sale",
			Subject: "Up to 80% off ending soon",
			From:    "promo@example.com",
			Snippet: "summer sale newsletter unsubscribe",
		}),
		classifyMailPriorityItem("work", mailadapter.MessageSummary{
			ID:      "receipt",
			Subject: "Google Play 주문 영수증",
			From:    "googleplay-noreply@google.com",
			Snippet: "주문 내역을 확인하세요.",
		}),
	}, nil, evidence.Record{}, nil)
	for _, want := range []string{"지금 바로 답장해야 할 후보는 없습니다", "표시하지 않고 보류했습니다", "메일에서 계약 찾아줘"} {
		if !strings.Contains(none, want) {
			t.Fatalf("none-priority reply missing %q:\n%s", want, none)
		}
	}
	for _, bad := range []string{"Up to 80% off", "Google Play 주문 영수증", "첫 번째 메일 답장 초안"} {
		if strings.Contains(none, bad) {
			t.Fatalf("none-priority reply should not show %q:\n%s", bad, none)
		}
	}
}

func TestInferAssistantSignalTargetRefLanguageNeutralRoomAliases(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"send the executive decision package to the briefing room", "argos-briefing"},
		{"share the deck with the report room", "argos-briefing"},
		{"send this to the assistant room", "argos-assistant"},
		{"send this to the chat room", "argos-chat"},
		{"send the risk brief to the ops report room", "argos-ops"},
		{"보고방에 보내줘", "argos-briefing"},
		{"비서방에 보내줘", "argos-assistant"},
		{"채팅방에 보내줘", "argos-chat"},
	}
	for _, tc := range cases {
		if got := inferAssistantSignalTargetRef(tc.text); got != tc.want {
			t.Fatalf("inferAssistantSignalTargetRef(%q)=%q, want %q", tc.text, got, tc.want)
		}
	}
	for _, text := range []string{
		"create a market research report",
		"prepare a briefing document",
		"make a report and PPT",
	} {
		if got := inferAssistantSignalTargetRef(text); got != "" {
			t.Fatalf("plain artifact request should not infer Signal target: %q -> %q", text, got)
		}
	}
}

func TestFormatSignalMailWatchShowsReadableSummaries(t *testing.T) {
	reply := formatSignalMailWatch(mailadapter.WatchResult{
		Account: mailadapter.AccountPublic{ID: "one"},
		Messages: []mailadapter.MessageSummary{{
			ID:      "99",
			Subject: "회의 일정 확인",
			From:    "partner@example.com",
			Date:    time.Date(2026, 6, 2, 3, 5, 0, 0, time.UTC),
			Snippet: "내일 오후 회의 가능 시간을 확인 부탁드립니다.",
		}},
	}, evidence.Record{StoredAt: "/tmp/evidence.json"}, nil, nil)
	for _, want := range []string{
		"새 메일 확인 결과: 1개",
		"1. 2026-06-02 12:05 회의 일정 확인 / partner@example.com",
		"요약: 내일 오후 회의 가능 시간을 확인 부탁드립니다.",
		"첫 번째 메일 본문 보여줘",
		"발송/삭제/이동/첨부 저장은 별도 승인 전에는 실행하지 않습니다.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("watch reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"[99]", "/tmp/evidence", "작업 기록도 저장했습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("watch reply exposed %q:\n%s", bad, reply)
		}
	}
}

func TestFormatSignalMailSummaryDecodesEUCKRMIMEWords(t *testing.T) {
	payload := signalMailMultiSearchResult{
		Intent: "mail_summary",
		Results: []mailadapter.SearchResult{{
			Account: mailadapter.AccountPublic{ID: "one"},
			Messages: []mailadapter.MessageSummary{{
				ID:      "45",
				Subject: "=?euc-kr?B?KLGksO0pILTZwL0gwda/oSC4uLOqv+Qu?=",
				From:    "Apple <News@InsideApple.Apple.com>",
			}},
		}},
	}
	reply := formatSignalMailMultiAccountRead(payload, evidence.Record{}, nil)
	if strings.Contains(reply, "=?") || strings.Contains(reply, "?=") {
		t.Fatalf("reply exposed undecoded MIME word:\n%s", reply)
	}
	if !strings.Contains(reply, "(광고) 다음 주에 만나요.") || !strings.Contains(reply, "Apple") {
		t.Fatalf("reply did not decode the EUC-KR subject:\n%s", reply)
	}
}

func TestCleanSignalMailHeaderForDisplayDecodesCP949MIMEWords(t *testing.T) {
	subject := cleanSignalMailHeaderForDisplay("=?euc-kr?B?V1dEQzI2wMcguLbB9ri3IMfPt+ewoSC9w8Dbtcu0z7TZLg==?=", "fallback")
	if subject != "WWDC26의 마지막 하루가 시작됩니다." {
		t.Fatalf("subject=%q", subject)
	}
}

func TestFallbackAssistantToolIntentExtractsMailSearchQueryBeforeWatch(t *testing.T) {
	intent, ok := fallbackAssistantToolIntent("메일에서 Google Workspace 찾아서 최근 것 읽어줘")
	if !ok {
		t.Fatal("intent was not detected")
	}
	if intent.Intent != "mail_search" || intent.Query != "Google Workspace" {
		t.Fatalf("intent=%#v", intent)
	}
}

func TestNormalizeAssistantToolIntentCorrectsMailWatchSearchText(t *testing.T) {
	intent := normalizeAssistantToolIntentFromText(assistantToolIntent{Intent: "mail_watch", Limit: 10, Confidence: 0.9}, "메일에서 cloudflare 찾아줘")
	if intent.Intent != "mail_search" || intent.Query != "cloudflare" {
		t.Fatalf("intent=%#v", intent)
	}
}

func TestFormatSignalMailReadRepliesHideInternalTerms(t *testing.T) {
	searchReply := formatSignalMailSearch(mailadapter.SearchResult{
		Account: mailadapter.AccountPublic{ID: "one"},
		Query:   "계약",
		Messages: []mailadapter.MessageSummary{{
			ID:      "42",
			Subject: "계약서 확인",
			From:    "sender@example.com",
			Date:    time.Date(2026, 6, 2, 3, 5, 0, 0, time.UTC),
			Snippet: "첨부 확인 부탁드립니다.",
		}},
	}, evidence.Record{StoredAt: "/tmp/evidence.json"}, nil, nil)
	threadReply := formatSignalMailThread(mailadapter.Message{
		Summary: mailadapter.MessageSummary{ID: "42", From: "sender@example.com", Subject: "계약서 확인"},
		Body:    "첨부 확인 부탁드립니다.",
	}, evidence.Record{StoredAt: "/tmp/evidence.json"}, nil, nil)
	readManyReply := formatSignalMailReadMany(mailadapter.ReadManyResult{
		Messages: []mailadapter.Message{{
			Summary: mailadapter.MessageSummary{ID: "42", From: "sender@example.com", Subject: "계약서 확인"},
			Body:    "첨부 확인 부탁드립니다.",
		}},
	}, evidence.Record{StoredAt: "/tmp/evidence.json"}, nil, nil)
	for _, reply := range []string{searchReply, threadReply, readManyReply} {
		if strings.Contains(reply, "evidence") || strings.Contains(reply, "redaction") || strings.Contains(reply, "/tmp/evidence") || strings.Contains(reply, "작업 기록도 저장했습니다") {
			t.Fatalf("mail reply exposed internal terms:\n%s", reply)
		}
		if !strings.Contains(reply, "민감정보") || !strings.Contains(reply, "작업 기록") && !strings.Contains(reply, "표시했습니다") {
			t.Fatalf("mail reply missing natural safety wording:\n%s", reply)
		}
	}
}

func TestFormatSignalMailThreadIsMobileReadable(t *testing.T) {
	reply := formatSignalMailThread(mailadapter.Message{
		Summary: mailadapter.MessageSummary{
			ID:      "42",
			From:    "sender@example.com",
			Subject: "계약서 확인",
			Date:    time.Date(2026, 6, 2, 3, 5, 0, 0, time.UTC),
		},
		Body: "<html><head><meta charset=\"utf-8\"></head><body style='font-family:Pretendard'>확인 가능한 본문입니다.<br>내일 오후까지 검토 부탁드립니다.</body></html>",
	}, evidence.Record{StoredAt: "/tmp/evidence.json"}, nil, nil)
	for _, want := range []string{
		"메일 본문입니다.",
		"제목: 계약서 확인",
		"보낸 사람: sender@example.com",
		"일시: 2026-06-02 12:05",
		"본문:",
		"확인 가능한 본문입니다.",
		"다음에 할 수 있는 일:",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("thread reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"id:", "[42]", "/tmp/evidence", "<html", "<body", "style=", "Pretendard"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("thread reply exposed %q:\n%s", bad, reply)
		}
	}
}

func TestDirectSignalMailReferenceExtractsAccountAndID(t *testing.T) {
	id, account, ok := directSignalMailReference("user-mail 계정 메일 59 본문 읽어줘")
	if !ok || id != "59" || account != "user-mail" {
		t.Fatalf("id=%q account=%q ok=%t", id, account, ok)
	}

	id, account, ok = directSignalMailReference("59번 메일 자세히 읽어줘")
	if !ok || id != "59" || account != "" {
		t.Fatalf("id=%q account=%q ok=%t", id, account, ok)
	}
}

func TestCleanSignalMailBodySkipsMultilineBodyStyleBlock(t *testing.T) {
	body := `<!DOCTYPE html>
<html lang="ko">
<head>
<title>이번주 신규 매도 기업 Top 5 안내</title>
</head>
<body style='
      margin: 0;
      padding: 0;
      background: #fff;
      font-family:
        "Pretendard", "Apple SD Gothic Neo",
        "Spoqa Han Sans", "Malgun Gothic",
        "맑은 고딕", Arial, Helvetica, sans-serif;
      font-size: 16px;
      color: #222;
      line-height: 1.5;
    '>
<div style="
      font-size: 16px;
      font-weight: 400;
      margin-bottom: 28px;
    ">
안녕하세요, M&amp;A 플랫폼 리스팅입니다.
</div>
<div>홍길동님께서 최근 관심을 보인 기업의 업종과 키워드를 바탕으로 매물을 추천드립니다.</div>
</body>
</html>`
	got := cleanSignalMailBodyForDisplay(body, 500)
	for _, want := range []string{
		"이번주 신규 매도 기업 Top 5 안내",
		"안녕하세요, M&A 플랫폼 리스팅입니다.",
		"매물을 추천드립니다.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("body missing %q:\n%s", want, got)
		}
	}
	for _, bad := range []string{"Pretendard", "Spoqa", "font-size", "font-weight", "margin-bottom", "line-height", "style="} {
		if strings.Contains(got, bad) {
			t.Fatalf("body exposed style noise %q:\n%s", bad, got)
		}
	}
}

func TestFallbackAssistantToolIntentExtractsDirectMailThreadAccount(t *testing.T) {
	intent, ok := fallbackAssistantToolIntent("user-mail 계정 메일 59 본문 읽어줘")
	if !ok {
		t.Fatal("intent was not detected")
	}
	if intent.Intent != "mail_thread" || intent.MessageID != "59" || intent.Account != "user-mail" {
		t.Fatalf("intent=%#v", intent)
	}
}

func TestCleanSignalMailSnippetDropsEncodedNoise(t *testing.T) {
	got := cleanSignalMailSnippet("<SECRET:high_entropy:1>\n=EC=9A=94=EC=B2=AD=ED=95=98=EC=8B=A0\n확인 가능한 문장입니다.", 80)
	if got != "확인 가능한 문장입니다." {
		t.Fatalf("snippet=%q", got)
	}
}

func TestCleanSignalMailSnippetDropsHTMLStyleNoise(t *testing.T) {
	raw := "m, BlinkMacSystemFont, 'Helvetica Neue', Arial, sans-serif; color:#1f2937; = line-height:1.5; background:#f9fafb; } .g-mail-container { max-width:600px;=\n<head> <meta charset=\"utf-8\"/>\n이번 주 신규 매물 3건을 확인하세요. \"Pretendard\", \"Apple SD Gothic Neo\", <body style='font-family:sans-serif'>"
	got := cleanSignalMailSnippet(raw, 120)
	if got != "이번 주 신규 매물 3건을 확인하세요." {
		t.Fatalf("snippet=%q", got)
	}

	raw = "이번주 신규 매도 기업 Top 5 안내\n\"Pretendard\", \"Apple SD Gothic Neo\", Arial, sans-serif"
	got = cleanSignalMailSnippet(raw, 120)
	if got != "이번주 신규 매도 기업 Top 5 안내" {
		t.Fatalf("joined snippet=%q", got)
	}
}

func TestFormatSignalMailThreadDropsRenderingOnlyBody(t *testing.T) {
	reply := formatSignalMailThread(mailadapter.Message{
		Summary: mailadapter.MessageSummary{
			ID:      "101",
			From:    "Google Play <googleplay-noreply@google.com>",
			Subject: "Google Play 주문 영수증(2026. 6. 13.)",
			Date:    time.Date(2026, 6, 13, 9, 8, 0, 0, time.UTC),
		},
		Body: `Cg0KDQo= sparent; border-bottom: 4px solid transparent;">
w.gstatic.com/gumdrop/files/google-play-crm-lockup-ic-h-transparent-w688px-=
<div sty= <td st`,
	}, evidence.Record{}, nil, nil)
	if !strings.Contains(reply, "본문 텍스트는 바로 추출하지 못했습니다.") ||
		!strings.Contains(reply, "구매/결제 영수증 또는 청구 알림입니다") ||
		!strings.Contains(reply, "보통 답장은 필요 없습니다") {
		t.Fatalf("expected fallback body, got:\n%s", reply)
	}
	for _, bad := range []string{"Cg0KDQo", "border-bottom", "transparent", "gstatic", "gumdrop", "<div", "<td", "sty="} {
		if strings.Contains(reply, bad) {
			t.Fatalf("reply exposed rendering noise %q:\n%s", bad, reply)
		}
	}

	reply = formatSignalMailThread(mailadapter.Message{
		Summary: mailadapter.MessageSummary{
			ID:      "102",
			From:    "Google Play <googleplay-noreply@google.com>",
			Subject: "Google Play 주문 영수증",
		},
		Body: "Cg0KDQo=",
	}, evidence.Record{}, nil, nil)
	if strings.Contains(reply, "Cg0KDQo") ||
		!strings.Contains(reply, "본문 텍스트는 바로 추출하지 못했습니다.") ||
		!strings.Contains(reply, "구매/결제 영수증 또는 청구 알림입니다") {
		t.Fatalf("short encoded body was not hidden:\n%s", reply)
	}
}

func TestSignalMailUnreadableFallbackClassifiesCommonMail(t *testing.T) {
	tests := []struct {
		name    string
		summary mailadapter.MessageSummary
		want    string
	}{
		{
			name: "promo",
			summary: mailadapter.MessageSummary{
				From:    "Disney+ <disneyplus@messaging.disneyplus.com>",
				Subject: "(광고) 6월 디즈니+ 구독자 혜택",
			},
			want: "프로모션 또는 서비스 알림입니다",
		},
		{
			name: "listing",
			summary: mailadapter.MessageSummary{
				From:    "Listing <no_reply@listing.co>",
				Subject: "[리스팅] AI 다이어리 앱 외 M&A 매물 제안",
			},
			want: "M&A 매물 제안 또는 시장/영업성 제안 메일입니다",
		},
		{
			name: "account",
			summary: mailadapter.MessageSummary{
				From:    "service-noreply@teamviewer.com",
				Subject: "고객님의 TeamViewer 계정이 삭제되었습니다",
			},
			want: "계정 상태나 보안 관련 알림입니다",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := signalMailUnreadableBodyFallback(tt.summary)
			if !strings.Contains(got, tt.want) {
				t.Fatalf("fallback missing %q:\n%s", tt.want, got)
			}
			if strings.Contains(got, "evidence") || strings.Contains(got, "/Users/") {
				t.Fatalf("fallback exposed internal detail:\n%s", got)
			}
		})
	}
}

func TestFormatSignalMailBriefIsActionable(t *testing.T) {
	reply := formatSignalMailBrief(mailadapter.Message{
		Summary: mailadapter.MessageSummary{
			ID:      "59",
			From:    "Listing <no_reply@listing.co>",
			Subject: "[리스팅] AI 다이어리 앱 외 M&A 매물 제안",
			Date:    time.Date(2026, 6, 13, 0, 2, 0, 0, time.UTC),
		},
		Body: `<!DOCTYPE html>
<html><body style='
font-family: Pretendard;
font-size: 16px;
'>
<div style="
font-weight: 400;
margin-bottom: 28px;
">안녕하세요, M&amp;A 플랫폼 리스팅입니다.</div>
<div>홍길동님께서 최근 관심을 보인 기업의 업종, 키워드, 비즈니스 모델 등을 바탕으로 유사한 조건을 갖춘 매물을 추천드립니다.</div>
<div>인수에 적합한 핵심 조건을 갖춘 기업이니 검토해보세요.</div>
<div>관심 키워드 - 패션 이커머스, 뷰티·화장품, AI·생성형AI, D2C 브랜드</div>
<div>[매물 번호 4689] [175개국 출시] 감성대화형 일기 자동 작성 AI 다이어리 앱 💰 매각 금액: 1.5~2.0억 📊 2024년 매출액: 9.1억 📈 2024년 영업이익: 0.2억</div>
</body></html>`,
	}, evidence.Record{StoredAt: "/tmp/evidence.json"}, nil, nil)
	for _, want := range []string{
		"메일 핵심 요약입니다.",
		"제목: [리스팅] AI 다이어리 앱 외 M&A 매물 제안",
		"핵심:",
		"M&A 플랫폼 리스팅입니다.",
		"매각 금액: 1.5~2.0억",
		"답장 초안",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("brief missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"Pretendard", "font-weight", "style=", "/tmp/evidence", "작업 기록도 저장했습니다"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("brief exposed %q:\n%s", bad, reply)
		}
	}
}

func TestRecentSignalMailFollowupRecognizesOrdinalDraftReply(t *testing.T) {
	if !isRecentSignalMailDraftReplyText("첫 번째 메일에 정중하게 답장 초안 써줘") {
		t.Fatal("expected draft reply follow-up")
	}
	index, ok := recentSignalMailReferenceIndex("두 번째 메일에 답장 초안 써줘", 3)
	if !ok || index != 1 {
		t.Fatalf("index=%d ok=%t", index, ok)
	}
	index, ok = recentSignalMailReferenceIndex("방금 그 메일 답장 초안", 1)
	if !ok || index != 0 {
		t.Fatalf("latest index=%d ok=%t", index, ok)
	}
	if _, ok := recentSignalMailReferenceIndex("여섯 번째 메일 답장 초안", 2); ok {
		t.Fatal("out-of-range ordinal should not match")
	}
}

func TestRecentSignalMailFollowupRecognizesBodyRead(t *testing.T) {
	for _, input := range []string{
		"그 내용을 상세히 보고 해줘",
		"저 시큐리티 업데이트 내용",
		"여기에 나오는 이메일 본문 내용",
		"첫 번째 메일 원문 보여줘",
	} {
		if !isRecentSignalMailReadText(input) {
			t.Fatalf("expected mail body read follow-up: %q", input)
		}
	}
	if isRecentSignalMailReadText("첫 번째 메일에 답장 초안 써줘") {
		t.Fatal("draft reply should not be treated as body read")
	}
}

func TestRecentSignalMailFollowupRecognizesBrief(t *testing.T) {
	for _, input := range []string{
		"이 메일 핵심만 더 짧게",
		"첫 번째 메일 요약해줘",
		"메일 59 핵심만",
		"59번 메일 짧게 정리해줘",
	} {
		if !isRecentSignalMailBriefText(input) {
			t.Fatalf("expected mail brief follow-up: %q", input)
		}
	}
	if isRecentSignalMailBriefText("최근 중요한 메일 요약해줘") {
		t.Fatal("general mail summary should not be treated as a follow-up")
	}
}

func TestRecentSignalMailBodyFollowupUsesStoredSingleMessage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MAIL_CONFIG", filepath.Join(home, "mail.json"))
	resetRecentSignalMailContextForTest(t)
	target := "mail-body-followup"
	rememberRecentSignalMailContext(mailPendingKey(target), recentSignalMailContext{
		Items: []recentSignalMailItem{{
			Account: "missing",
			ID:      "33",
			From:    "ChatGPT <noreply@email.openai.com>",
			Subject: "Action Required: Important security update for OpenAI macOS apps",
		}},
	})
	reply, handled := replyFromRecentSignalMailContext(ListenOptions{TargetID: target, Mode: "assistant"}, "여기에 나오는 이메일 본문 내용")
	if !handled {
		t.Fatal("body follow-up was not handled")
	}
	if strings.Contains(reply, "메일 계정 설정") || strings.Contains(reply, "어떤 내용을 상세히") {
		t.Fatalf("body follow-up routed to wrong answer:\n%s", reply)
	}
	if !strings.Contains(reply, "메일 본문") && !strings.Contains(reply, "본문 조회") {
		t.Fatalf("expected mail body read path, got:\n%s", reply)
	}
}

func TestMailPendingKeyUsesDefaultForEmptySignalTarget(t *testing.T) {
	if got := mailPendingKey(""); got == "" {
		t.Fatal("empty Signal target should still have a stable pending mail key")
	}
	if got := mailPendingKey("  "); got == "" {
		t.Fatal("blank Signal target should still have a stable pending mail key")
	}
	if got := mailPendingKey("assistant-room"); got != "assistant-room" {
		t.Fatalf("mailPendingKey preserved target = %q", got)
	}
}

func TestRecentSignalMailContextStoresMinimalReferences(t *testing.T) {
	ctx := recentSignalMailContextFromSearchResults([]mailadapter.SearchResult{{
		Account: mailadapter.AccountPublic{ID: "one"},
		Messages: []mailadapter.MessageSummary{{
			ID:      "42",
			Subject: "계약서 확인",
			From:    "sender@example.com",
			Snippet: "본문은 저장하지 않습니다",
		}},
	}})
	if len(ctx.Items) != 1 {
		t.Fatalf("items=%#v", ctx.Items)
	}
	item := ctx.Items[0]
	if item.Account != "one" || item.ID != "42" || item.Subject != "계약서 확인" || item.From != "sender@example.com" {
		t.Fatalf("item=%#v", item)
	}
}

func TestRecentSignalMailContextPersistsAcrossProcesses(t *testing.T) {
	path := resetRecentSignalMailContextForTest(t)
	ctx := recentSignalMailContext{
		Items: []recentSignalMailItem{{
			Account: "one",
			ID:      "42",
			Subject: "계약서 확인",
			From:    "sender@example.com",
			Date:    time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC),
		}},
	}
	rememberRecentSignalMailContext("assistant-room-a", ctx)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("stored context was not written: %v", err)
	}
	for _, forbidden := range []string{"본문은 저장하지 않습니다", "\"body\"", "\"Body\""} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("stored context leaked message body marker %q:\n%s", forbidden, string(data))
		}
	}

	recentSignalMailContexts.Lock()
	recentSignalMailContexts.items = map[string]recentSignalMailContext{}
	recentSignalMailContexts.Unlock()

	got, ok := takeRecentSignalMailContext("assistant-room-a", false)
	if !ok || len(got.Items) != 1 {
		t.Fatalf("context was not loaded from disk: got=%#v ok=%t", got, ok)
	}
	item := got.Items[0]
	if item.Account != "one" || item.ID != "42" || item.Subject != "계약서 확인" || item.From != "sender@example.com" {
		t.Fatalf("item=%#v", item)
	}
}

func TestRecentSignalMailContextFallsBackToDefaultKey(t *testing.T) {
	resetRecentSignalMailContextForTest(t)
	ctx := recentSignalMailContext{
		Items: []recentSignalMailItem{{
			Account: "one",
			ID:      "42",
			Subject: "계약서 확인",
			From:    "sender@example.com",
		}},
	}
	rememberRecentSignalMailContext("assistant-room-a", ctx)
	got, ok := takeRecentSignalMailContext("assistant-room-b", false)
	if !ok || len(got.Items) != 1 || got.Items[0].ID != "42" {
		t.Fatalf("context fallback got=%#v ok=%t", got, ok)
	}
}

func TestRecentSignalMailContextPersistentDefaultKey(t *testing.T) {
	resetRecentSignalMailContextForTest(t)
	ctx := recentSignalMailContext{
		Items: []recentSignalMailItem{{
			Account: "one",
			ID:      "42",
			Subject: "계약서 확인",
			From:    "sender@example.com",
		}},
	}
	rememberRecentSignalMailContext("assistant-room-a", ctx)
	recentSignalMailContexts.Lock()
	recentSignalMailContexts.items = map[string]recentSignalMailContext{}
	recentSignalMailContexts.Unlock()

	got, ok := takeRecentSignalMailContext("assistant-room-b", false)
	if !ok || len(got.Items) != 1 || got.Items[0].ID != "42" {
		t.Fatalf("persistent default context got=%#v ok=%t", got, ok)
	}
}

func TestFormatSignalMailDraftShowsBodyWithoutLocalPath(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	reply := formatSignalMailDraft(mailadapter.Draft{
		ID:      "draft-1",
		To:      []string{"sender@example.com"},
		Subject: "Re: 계약서 확인",
		Body:    "<SECRET:high_entropy:1>\n안녕하세요.\n\n확인 후 회신드리겠습니다.\n감사합니다.",
		Path:    "/Users/example/.meshclaw/mail-drafts/draft-1.eml",
	}, evidence.Record{}, nil, nil)
	for _, want := range []string{"답장 초안을 만들었습니다", "아직 보내지 않았습니다", "받는 사람: sender@example.com", "제목: Re: 계약서 확인", "초안:", "안녕하세요.\n\n확인 후 회신드리겠습니다.", "감사합니다.", "이 초안 보내줘"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, forbidden := range []string{"path:", "/Users/", ".eml", "draft:", "SECRET", "high_entropy", "작업 기록도 저장했습니다"} {
		if strings.Contains(reply, forbidden) {
			t.Fatalf("draft reply exposed %q:\n%s", forbidden, reply)
		}
	}
}

func TestFormatSignalMailDraftEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	reply := formatSignalMailDraft(mailadapter.Draft{
		ID:      "draft-1",
		To:      []string{"sender@example.com"},
		Subject: "Re: Contract review",
		Body:    "<SECRET:high_entropy:1>\nHello.\n\nI reviewed this and will follow up soon.\nThank you.",
		Path:    "/Users/example/.meshclaw/mail-drafts/draft-1.eml",
	}, evidence.Record{}, nil, nil)
	for _, want := range []string{"Reply draft created", "It has not been sent", "To: sender@example.com", "Subject: Re: Contract review", "Draft:", "Hello.\n\nI reviewed this", "send this draft", "Actual sending only happens after separate approval"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English reply missing %q:\n%s", want, reply)
		}
	}
	if containsHangul(reply) {
		t.Fatalf("English draft reply should not expose Korean UI text:\n%s", reply)
	}
	for _, forbidden := range []string{"path:", "/Users/", ".eml", "draft:", "SECRET", "high_entropy", "evidence"} {
		if strings.Contains(reply, forbidden) {
			t.Fatalf("draft reply exposed %q:\n%s", forbidden, reply)
		}
	}
}

func TestFormatSignalMailDraftDecodesEncodedSubject(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	reply := formatSignalMailDraft(mailadapter.Draft{
		ID:      "draft-encoded-subject",
		To:      []string{"Apple Developer <developer@insideapple.apple.com>"},
		Subject: "Re: =?euc-kr?Q?=BF=C0=B4=C3=C0=C7_=C1=D6=C0=CE=B0=F8,_Apple_Intel?= =?euc-kr?Q?ligence=BF=CD_=C6=C4?= =?euc-kr?Q?=BF=EE=B5=A5=C0=CC=BC=C7_=B8=F0=B5=A8=C0=D4=B4=CF=B4=D9.?=",
		Body:    "안녕하세요.\n\n확인 후 회신드리겠습니다.",
	}, evidence.Record{}, nil, nil)
	if strings.Contains(reply, "=?") || strings.Contains(reply, "?=") {
		t.Fatalf("draft reply exposed encoded MIME subject:\n%s", reply)
	}
	if !strings.Contains(reply, "제목: Re: 오늘의 주인공, Apple Intelligence와 파운데이션 모델입니다.") {
		t.Fatalf("draft reply did not decode subject:\n%s", reply)
	}
}

func TestFormatSignalMailSendResultUsesLanguagePackAndDecodesSubject(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	reply := formatSignalMailSendResult(mailadapter.SendResult{
		To:       []string{"Apple Developer <developer@insideapple.apple.com>"},
		Subject:  "Re: =?euc-kr?Q?=BF=C0=B4=C3=C0=C7_=C1=D6=C0=CE=B0=F8,_Apple_Intel?= =?euc-kr?Q?ligence=BF=CD_=C6=C4?= =?euc-kr?Q?=BF=EE=B5=A5=C0=CC=BC=C7_=B8=F0=B5=A8=C0=D4=B4=CF=B4=D9.?=",
		Executed: true,
		Status:   "ok",
	}, evidence.Record{}, nil, nil)
	for _, want := range []string{"메일을 보냈습니다.", "받는 사람: Apple Developer <developer@insideapple.apple.com>", "제목: Re: 오늘의 주인공, Apple Intelligence와 파운데이션 모델입니다.", "다음 확인:", "`승인 대기열 보여줘`", "`최근 메일 요약해줘`"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("send reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "=?") || strings.Contains(reply, "?=") || strings.Contains(reply, "/Users/") {
		t.Fatalf("send reply exposed raw implementation detail:\n%s", reply)
	}
}

func TestFormatSignalMailSendResultEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	reply := formatSignalMailSendResult(mailadapter.SendResult{
		To:       []string{"sender@example.com"},
		Subject:  "Re: Contract review",
		Executed: true,
		Status:   "ok",
	}, evidence.Record{}, nil, nil)
	for _, want := range []string{"Mail sent.", "To: sender@example.com", "Subject: Re: Contract review", "Next check:", "`approval queue`", "`summarize recent mail`"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English send reply missing %q:\n%s", want, reply)
		}
	}
	if containsHangul(reply) {
		t.Fatalf("English send reply should not expose Korean UI text:\n%s", reply)
	}
}

func TestFormatSignalMailSendFailureShowsNextCheck(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	reply := formatSignalMailSendResult(mailadapter.SendResult{}, evidence.Record{}, nil, fmt.Errorf("smtp login failed"))
	for _, want := range []string{"메일 발송에 실패했습니다: smtp login failed", "다음 확인:", "초안을 수정", "`승인 대기열 보여줘`"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("send failure reply missing %q:\n%s", want, reply)
		}
	}
}

func TestFormatSignalMailSendFailureEnglishUsesLanguagePack(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "en")
	reply := formatSignalMailSendResult(mailadapter.SendResult{}, evidence.Record{}, nil, fmt.Errorf("smtp login failed"))
	for _, want := range []string{"Mail send failed: smtp login failed", "Next check:", "revise the draft", "`approval queue`"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("English send failure reply missing %q:\n%s", want, reply)
		}
	}
	if containsHangul(reply) {
		t.Fatalf("English send failure reply should not expose Korean UI text:\n%s", reply)
	}
}

func TestMailDraftRevisionShortensPendingDraft(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	temp := t.TempDir()
	t.Setenv("HOME", temp)
	resetPendingMailSendForTest(t)
	t.Setenv("MESHCLAW_MAIL_DRAFT_DIR", filepath.Join(temp, "drafts"))
	target := "assistant-mail-draft-revision"
	key := mailPendingKey(target)
	now := time.Now().UTC()
	rememberPendingMailSend(key, pendingMailSend{
		Draft: mailadapter.Draft{
			ID:      "draft-revision-test",
			Account: "one",
			To:      []string{"sender@example.com"},
			Subject: "Re: M&A 매물 제안",
			Body: strings.Join([]string{
				"안녕하세요.",
				"",
				"제안 주셔서 감사합니다. 보내주신 M&A 매물 자료는 확인했습니다.",
				"검토를 위해 각 매물의 상세 소개서, 최근 매출/영업이익 자료, 사용자·트래픽 지표, 매각 희망 조건을 함께 보내주시면 좋겠습니다.",
				"확인 후 관심 있는 항목이 있으면 다시 연락드리겠습니다.",
				"",
				"감사합니다.",
				"홍길동 드림",
			}, "\n"),
			Status:       "draft",
			Policy:       "not sent; email_send requires approval and evidence",
			ApprovalHint: "Review this draft before sending.",
			CreatedAt:    now,
		},
		CreatedAt: now,
		ExpiresAt: now.Add(time.Minute),
	})

	reply, handled := mailDraftRevisionReply(ListenOptions{TargetID: target, Mode: "assistant"}, "더 짧게")
	if !handled {
		t.Fatal("draft revision was not handled")
	}
	for _, want := range []string{"답장 초안을 만들었습니다", "아직 보내지 않았습니다", "초안:", "제안 주셔서 감사합니다.", "보내려면 `이 초안 보내줘`"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, forbidden := range []string{"/Users/", ".json", "draft:", "상세 기록", "근거 기록"} {
		if strings.Contains(reply, forbidden) {
			t.Fatalf("draft revision exposed %q:\n%s", forbidden, reply)
		}
	}
	pending, ok := peekPendingMailSend(key)
	if !ok {
		t.Fatal("revised draft was not kept pending")
	}
	if strings.Contains(pending.Draft.Body, "최근 매출/영업이익 자료") {
		t.Fatalf("draft was not shortened:\n%s", pending.Draft.Body)
	}
	if !strings.Contains(pending.Draft.Body, "홍길동 드림") {
		t.Fatalf("closing was lost:\n%s", pending.Draft.Body)
	}
}

func TestMailDraftRevisionSurvivesProcessRestartStore(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	temp := t.TempDir()
	t.Setenv("HOME", temp)
	storePath := resetPendingMailSendForTest(t)
	t.Setenv("MESHCLAW_MAIL_DRAFT_DIR", filepath.Join(temp, "drafts"))
	target := "assistant-mail-draft-persist"
	key := mailPendingKey(target)
	now := time.Now().UTC()
	rememberPendingMailSend(key, pendingMailSend{
		Draft: mailadapter.Draft{
			ID:        "draft-persist-test",
			Account:   "one",
			To:        []string{"sender@example.com"},
			Subject:   "Re: 계약서 확인",
			Body:      "안녕하세요.\n\n내용 확인했습니다. 자세한 의견은 내일까지 보내드리겠습니다.\n\n감사합니다.",
			Status:    "draft",
			CreatedAt: now,
		},
		CreatedAt: now,
		ExpiresAt: now.Add(time.Minute),
	})
	if _, err := os.Stat(storePath); err != nil {
		t.Fatalf("pending draft store was not written: %v", err)
	}
	mailSendPending.Lock()
	mailSendPending.items = map[string]pendingMailSend{}
	mailSendPending.Unlock()

	reply, handled := mailDraftRevisionReply(ListenOptions{TargetID: target, Mode: "assistant"}, "더 정중하게")
	if !handled {
		t.Fatal("draft revision was not restored from persistent store")
	}
	for _, want := range []string{"답장 초안을 만들었습니다", "받는 사람: sender@example.com", "제목: Re: 계약서 확인", "실제 발송은 별도 승인 후"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("persistent draft revision missing %q:\n%s", want, reply)
		}
	}
	pending, ok := peekPendingMailSend(key)
	if !ok || pending.Draft.ID != "draft-persist-test" {
		t.Fatalf("persisted pending draft not kept after revision: %#v ok=%t", pending, ok)
	}
}

func TestRecentSignalMailFollowupWithoutContextAsksForSummaryFirst(t *testing.T) {
	resetRecentSignalMailContextForTest(t)
	reply, handled := replyFromRecentSignalMailContext(ListenOptions{TargetID: "missing-mail-context"}, "첫 번째 메일에 답장 초안 써줘")
	if !handled || !strings.Contains(reply, "먼저 `최근 메일 요약해줘`") {
		t.Fatalf("reply=%q handled=%t", reply, handled)
	}
}

func TestRecentSignalMailContextDoesNotStealMailPriorityPackage(t *testing.T) {
	resetRecentSignalMailContextForTest(t)
	text := "최근 메일을 확인해서 답장 필요한 것과 오늘 확인해야 할 것만 우선순위로 정리하고, 바로 읽을 수 있는 요약과 DOCX/MD/PPT/XLSX/CSV/SVG, edge tts 음성 브리핑으로 만들어서 보고방에 보내줘. 메일 발송이나 삭제는 하지 마."
	if !isAssistantMailPriorityRequest(text) {
		t.Fatal("expected mail-priority package request")
	}
	reply, handled := replyFromRecentSignalMailContext(ListenOptions{TargetID: "missing-mail-package-context"}, text)
	if handled {
		t.Fatalf("mail-priority package request should be routed to the mail reader, got reply:\n%s", reply)
	}
}

func TestParseAssistantToolIntentJSONAcceptsMailSearch(t *testing.T) {
	intent, ok := parseAssistantToolIntentJSON(`{"intent":"mail_search","query":"cloudflare","limit":5,"confidence":0.9}`)
	if !ok {
		t.Fatal("intent was not parsed")
	}
	if intent.Query != "cloudflare" || intent.Limit != 5 {
		t.Fatalf("intent=%#v", intent)
	}
}

func TestParseAssistantToolIntentJSONRejectsMailSearchWithoutQuery(t *testing.T) {
	if intent, ok := parseAssistantToolIntentJSON(`{"intent":"mail_search","confidence":0.9}`); ok {
		t.Fatalf("unexpected intent=%#v", intent)
	}
}

func TestParseAssistantToolIntentJSONAcceptsMailDraftReply(t *testing.T) {
	intent, ok := parseAssistantToolIntentJSON(`{"intent":"mail_draft_reply","message_id":"12345","prompt":"정중하게 답장","confidence":0.9}`)
	if !ok {
		t.Fatal("intent was not parsed")
	}
	if intent.MessageID != "12345" || intent.Prompt == "" {
		t.Fatalf("intent=%#v", intent)
	}
}

func TestParseAssistantToolIntentJSONAcceptsMultipleMailIDs(t *testing.T) {
	intent, ok := parseAssistantToolIntentJSON(`{"intent":"mail_thread","message_ids":["123","124,125","123"],"confidence":0.9}`)
	if !ok {
		t.Fatal("intent was not parsed")
	}
	ids := intent.MailIDs()
	if len(ids) != 3 || ids[0] != "123" || ids[1] != "124" || ids[2] != "125" {
		t.Fatalf("ids=%#v", ids)
	}
}

func TestParseAssistantToolIntentJSONAcceptsMailWatch(t *testing.T) {
	intent, ok := parseAssistantToolIntentJSON(`{"intent":"mail_watch","limit":100,"confidence":0.9}`)
	if !ok {
		t.Fatal("intent was not parsed")
	}
	if intent.Limit != 10 {
		t.Fatalf("intent=%#v", intent)
	}
}

func TestParseAssistantToolIntentJSONAcceptsMailMoveApprovalOnly(t *testing.T) {
	intent, ok := parseAssistantToolIntentJSON(`{"intent":"mail_move","message_ids":["11","12"],"target":"Archive","confidence":0.9}`)
	if !ok {
		t.Fatal("intent was not parsed")
	}
	if intent.Target != "Archive" || len(intent.MailIDs()) != 2 {
		t.Fatalf("intent=%#v ids=%#v", intent, intent.MailIDs())
	}
}

func TestParseAssistantToolIntentJSONRejectsMailMoveWithoutTarget(t *testing.T) {
	if intent, ok := parseAssistantToolIntentJSON(`{"intent":"mail_move","message_id":"11","confidence":0.9}`); ok {
		t.Fatalf("unexpected intent=%#v", intent)
	}
}

func TestParseAssistantToolIntentJSONAcceptsMailSendCompose(t *testing.T) {
	intent, ok := parseAssistantToolIntentJSON(`{"intent":"mail_send","to":["operator@example.com"],"subject":"테스트","body":"본문","confidence":0.9}`)
	if !ok {
		t.Fatal("intent was not parsed")
	}
	if len(intent.To) != 1 || intent.Subject != "테스트" || intent.Body != "본문" {
		t.Fatalf("intent=%#v", intent)
	}
}

func TestParseAssistantToolIntentJSONRejectsMailSendWithoutBody(t *testing.T) {
	if intent, ok := parseAssistantToolIntentJSON(`{"intent":"mail_send","to":["operator@example.com"],"subject":"테스트","confidence":0.9}`); ok {
		t.Fatalf("unexpected intent=%#v", intent)
	}
}

func TestMatchSignalMailAccountChoice(t *testing.T) {
	accounts := []mailadapter.AccountPublic{
		{ID: "one", Email: "one@example.com"},
		{ID: "two", Email: "two@example.com"},
	}
	account, ok := matchSignalMailAccountChoice("2", accounts)
	if !ok || account.ID != "two" {
		t.Fatalf("account=%#v ok=%t", account, ok)
	}
	account, ok = matchSignalMailAccountChoice("one@example.com", accounts)
	if !ok || account.ID != "one" {
		t.Fatalf("account=%#v ok=%t", account, ok)
	}
}

func TestChoosePendingMailAccountDoesNotSwallowNewRequest(t *testing.T) {
	key := mailPendingKey("assistant-test")
	t.Cleanup(func() {
		takePendingMailAccountChoice(key, true)
	})
	rememberPendingMailAccountChoice(key, pendingMailAccountChoice{
		Intent:    assistantToolIntent{Intent: "mail_send", To: []string{"to@example.com"}, Subject: "subject", Body: "body"},
		Accounts:  []mailadapter.AccountPublic{{ID: "one", Email: "one@example.com"}, {ID: "two", Email: "two@example.com"}},
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	})

	if reply, ok := choosePendingMailAccount(ListenOptions{TargetID: "assistant-test"}, "delivery tracking"); ok {
		t.Fatalf("pending account choice swallowed a new request: ok=%t reply=%q", ok, reply)
	}
	if _, ok := takePendingMailAccountChoice(key, true); !ok {
		t.Fatal("pending account choice should remain available")
	}
}

func TestChoosePendingMailAccountRepeatsForInvalidChoice(t *testing.T) {
	key := mailPendingKey("assistant-test-invalid")
	t.Cleanup(func() {
		takePendingMailAccountChoice(key, true)
	})
	rememberPendingMailAccountChoice(key, pendingMailAccountChoice{
		Intent:    assistantToolIntent{Intent: "mail_send", To: []string{"to@example.com"}, Subject: "subject", Body: "body"},
		Accounts:  []mailadapter.AccountPublic{{ID: "one", Email: "one@example.com"}, {ID: "two", Email: "two@example.com"}},
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	})

	reply, ok := choosePendingMailAccount(ListenOptions{TargetID: "assistant-test-invalid"}, "3")
	if !ok || !strings.Contains(reply, "메일 계정이 여러 개입니다") {
		t.Fatalf("reply=%q ok=%t", reply, ok)
	}
}

func TestArgosPermissionReplyListAndRevoke(t *testing.T) {
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(t.TempDir(), "permissions.json"))
	if _, err := osauto.GrantArgosPermission(osauto.ClassifyArgosAction("open Safari"), "test", "unit"); err != nil {
		t.Fatal(err)
	}
	reply, ok := argosPermissionReply("권한 목록")
	if !ok || !containsAny(reply, "Safari", "open_app") {
		t.Fatalf("reply=%q ok=%t", reply, ok)
	}
	reply, ok = argosPermissionReply("권한 취소 Safari")
	if !ok || !containsAny(reply, "취소했습니다") {
		t.Fatalf("reply=%q ok=%t", reply, ok)
	}
	grants, err := osauto.ListArgosPermissions()
	if err != nil {
		t.Fatal(err)
	}
	if len(grants) != 0 {
		t.Fatalf("grants=%#v", grants)
	}
}

func TestArgosPermissionReplyHelp(t *testing.T) {
	reply, ok := argosPermissionReply("권한 도움말")
	if !ok || !containsAny(reply, "권한 목록", "권한 취소") {
		t.Fatalf("reply=%q ok=%t", reply, ok)
	}
}

func TestIsClearHistoryCommand(t *testing.T) {
	for _, text := range []string{"기억 지워", "대화 기록 삭제", "/clear", "memory clear"} {
		if !isClearHistoryCommand(text) {
			t.Fatalf("expected clear command for %q", text)
		}
	}
	if isClearHistoryCommand("오늘 날씨 알려줘") {
		t.Fatal("unexpected clear command")
	}
}
