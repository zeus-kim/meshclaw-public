package messenger

import (
	"context"
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

func TestAssistantModelToolReplyDoesNotUseDeterministicFastPath(t *testing.T) {
	t.Setenv("MESHCLAW_ASSISTANT_TOOL_LOOP", "0")
	for _, text := range []string{
		"오늘 날씨는?",
		"오늘 주요뉴스",
		"분당에서 광화문까지 출근길 알려줘",
		"새 메일 왔어?",
		"Safari 열어줘",
	} {
		if reply, handled := assistantModelToolReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, text); handled {
			t.Fatalf("%q was handled without model tool loop: %q", text, reply)
		}
	}
}

func TestAssistantToolSystemPromptGuidesModelSelection(t *testing.T) {
	prompt := assistantToolSystemPrompt()
	for _, want := range []string{
		"not by keyword matching",
		"meta questions",
		"ask only for the missing fields",
		"Use search_web for specific web research",
		"Use market_outlook for oil",
		"If the user also asks for an audio/voice message version of the report, use create_voice_report",
		"For 음성파일, voice file, mp3, TTS, voice note, or edge tts requests, use send_tts_voice",
		"For scheduled or recurring delivery to a friend",
		"Use get_news_headlines only for a general current headlines",
		"Never call tools to send, delete, purchase, pay, subscribe, or finalize booking",
		"Prefer one tool call at a time",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAssistantToolDefinitionsDescribeIntentBoundaries(t *testing.T) {
	descriptions := map[string]string{}
	for _, tool := range assistantToolDefinitions() {
		descriptions[tool.Function.Name] = tool.Function.Description
	}
	tests := map[string][]string{
		"get_news_headlines":        {"general current news briefing", "Do not use for web research"},
		"search_web":                {"web research", "Do not use get_news_headlines"},
		"market_outlook":            {"oil/유가/WTI/Brent", "never invent live prices or forecasts"},
		"get_directions":            {"both origin and destination", "ask a concise clarification"},
		"run_mac_action":            {"Calendar, Reminders, Notes, Contacts", "Prefer local macOS apps"},
		"create_document":           {"Obsidian-friendly Markdown", "Do not schedule a calendar event"},
		"create_voice_report":       {"written report document", "TTS audio version", "Do not split this into create_document plus send_tts_voice"},
		"scheduled_delivery_plan":   {"recurring or future Signal delivery", "does not register a job or send anything"},
		"prepare_meeting_materials": {"Obsidian-friendly Markdown brief", "Do not schedule time"},
		"create_presentation":       {"PowerPoint/PPTX", "Obsidian-friendly Markdown outline"},
		"search_mail":               {"A query is required", "ask if the user has not provided one"},
		"find_booking":              {"Requires enough context", "Never confirm, pay, or finalize booking"},
		"search_shopping":           {"Never checkout, subscribe, or pay"},
		"send_tts_voice":            {"Create an audio file", "Do not use create_document for voice-file requests"},
	}
	for name, wants := range tests {
		desc := descriptions[name]
		if desc == "" {
			t.Fatalf("missing tool %q", name)
		}
		for _, want := range wants {
			if !strings.Contains(desc, want) {
				t.Fatalf("%s description missing %q:\n%s", name, want, desc)
			}
		}
	}
}

func TestNormalizeAssistantToolNameMacActionAliases(t *testing.T) {
	for _, name := range []string{
		"set_reminder",
		"create_reminder",
		"add_reminder",
		"reminder_create",
		"add_event",
		"create_event",
		"calendar_event",
		"schedule_event",
		"calendar_add_event",
		"calendar_create_event",
		"calendar_schedule_event",
		"schedule_calendar_event",
		"calendar_update_event",
		"create_calendar_event",
		"add_calendar_event",
		"get_calendar_events",
		"list_calendar_events",
		"reminder_set_alarm",
	} {
		if got := normalizeAssistantToolName(name); got != "run_mac_action" {
			t.Fatalf("%s normalized to %q", name, got)
		}
	}
}

func TestAssistantVoiceReportPlan(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	reply := executeAssistantToolCall(
		ListenOptions{TargetID: "argos-assistant", Mode: "assistant"},
		"오늘 진행상황 보고서를 작성하고 음성 메시지로 보내줘",
		"create_voice_report",
		`{"title":"오늘 진행상황 보고서","body":"## 요약\n- 지도 캡처와 구매 승인 도구를 배포했습니다.\n- TTS 음성 파일 전송을 추가했습니다.","target":"윤","execute":false}`,
	)
	visible := signalReplyVisibleText(reply)
	if !strings.Contains(visible, "음성 보고서 생성 계획") ||
		!strings.Contains(visible, "보고서 제목: 오늘 진행상황 보고서") ||
		!strings.Contains(visible, "보낼 대상: 윤") {
		t.Fatalf("unexpected voice report plan:\n%s", visible)
	}
	if strings.Contains(visible, "문서를 작성했습니다") || len(signalReplyAttachments(reply)) != 0 {
		t.Fatalf("voice report plan should not create document/audio attachments:\n%s", reply)
	}
}

func TestAssistantVoiceReportCallPlanRequiresApproval(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "owner-signal", Channel: "signal", Recipient: "+821012345678", Label: "내게"}); err != nil {
		t.Fatal(err)
	}
	reply := executeAssistantToolCall(
		ListenOptions{TargetID: "argos-assistant", Mode: "assistant"},
		"내게 오늘 보고서를 전화로 읽어줘",
		"create_voice_report",
		`{"title":"오늘 보고서","body":"테스트 보고서입니다.","target":"내게","delivery":"call","execute":false}`,
	)
	visible := signalReplyVisibleText(reply)
	if !strings.Contains(visible, "음성 보고서 생성 계획") ||
		!strings.Contains(visible, "전화 전달: 실제 통화는 approve=true가 필요합니다") {
		t.Fatalf("unexpected voice report call plan:\n%s", visible)
	}
}

func TestAssistantVoiceRequestsRequireToolCall(t *testing.T) {
	for _, text := range []string{
		"오늘의 기도문을 만들어서 edge tts로 파일 만들어줘",
		"윤에게 이 안내문을 음성파일로 보내줘",
		"이 내용을 mp3로 읽어서 보내줘",
		"오늘의 주요뉴스를 음성 파일로 보내줘",
	} {
		if !assistantRequiresToolCall(text) {
			t.Fatalf("voice request should require a tool call: %q", text)
		}
	}
}

func TestAssistantVoiceTargetCurrentRoomAliases(t *testing.T) {
	for _, ref := range []string{"이 방", "현재 대화", "지금 채팅", "this chat", "current room"} {
		if !assistantVoiceTargetIsCurrentRoom(ref) {
			t.Fatalf("expected current room target alias for %q", ref)
		}
	}
	if assistantVoiceTargetIsCurrentRoom("윤") {
		t.Fatal("named contact should not be treated as current room")
	}
}

func TestAssistantScheduledDeliveryRequestsRequireToolCall(t *testing.T) {
	for _, text := range []string{
		"매주 월요일 9시에 윤에게 업무 브리핑 보내줘",
		"매일 아침 8시에 내게 주간 브리핑 보내줘",
		"다음 주 월욜 오후 2시에 장보기 일정 보내줘",
		"정기적으로 보고방에 오늘 요약을 공유해줘",
		"매일 저녁 10시에 내게 내일 일정 알림해줘",
		"매주 수요일 11시에 팀에 회의 노트 전달해줘",
		"매일 아침 7시 알람해줘",
		"매일 아침 8시에 알림줘",
		"매월 둘째 주 월요일마다 회의 내용을 통보해줘",
	} {
		if !assistantRequiresToolCall(text) {
			t.Fatalf("scheduled delivery request should require a tool call: %q", text)
		}
	}

	for _, text := range []string{
		"매일 오전 9시에 회의 일정을 추가해줘",
		"매주 금요일에 회의 일정을 잡아줘",
	} {
		if assistantRequiresToolCall(text) {
			t.Fatalf("non-delivery recurring request should not require scheduled delivery tool path: %q", text)
		}
	}
}

func TestAssistantScheduledDeliveryToolProducesReviewPlan(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	if _, _, err := UpsertTarget(Target{
		ID:        "yun",
		Channel:   "signal",
		Recipient: "+821012345678",
		Label:     "윤",
		Mode:      "assistant",
	}); err != nil {
		t.Fatal(err)
	}
	reply := executeAssistantToolCall(
		ListenOptions{TargetID: "argos-assistant", Mode: "assistant"},
		"매주 월요일 9시에 윤에게 업무 브리핑 보내줘",
		"scheduled_delivery_plan",
		`{"target":"윤","schedule":"매주 월요일 9시","content":"업무 브리핑","content_type":"text","delivery":"voice_note","execute":false}`,
	)
	visible := signalReplyVisibleText(reply)
	if !strings.Contains(visible, "예약 발송 계획을 만들었습니다.") {
		t.Fatalf("expected scheduled delivery review plan: %s", visible)
	}
	if !strings.Contains(visible, "- 상태: review_ready") {
		t.Fatalf("expected review_ready state: %s", visible)
	}
	if !strings.Contains(visible, "- 대상: 윤") {
		t.Fatalf("expected resolved target preview: %s", visible)
	}
}

func TestAssistantRequiredToolFallbackCanPlanScheduledDelivery(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	if _, _, err := UpsertTarget(Target{
		ID:        "yun",
		Channel:   "signal",
		Recipient: "+821012345678",
		Label:     "윤",
		Mode:      "assistant",
	}); err != nil {
		t.Fatal(err)
	}

	reply, handled := assistantRequiredToolFallbackReply(
		ListenOptions{TargetID: "argos-assistant", Mode: "assistant"},
		"매주 월요일 9시에 윤에게 업무 브리핑 보내줘",
	)
	if !handled {
		t.Fatal("expected fallback to handle scheduled delivery request")
	}

	visible := signalReplyVisibleText(reply)
	if !strings.Contains(visible, "예약 발송 계획을 만들었습니다.") ||
		!strings.Contains(visible, "- 주기:") ||
		!strings.Contains(visible, "- 대상: 윤") {
		t.Fatalf("unexpected scheduled delivery fallback reply:\n%s", visible)
	}
}

func TestAssistantScheduledDeliveryCronParityBaseline(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	t.Setenv("MESHCLAW_SCHEDULED_DELIVERIES", filepath.Join(home, ".meshclaw", "scheduled-deliveries.json"))
	if _, _, err := UpsertTarget(Target{
		ID:        "yun",
		Channel:   "signal",
		Recipient: "+821012345678",
		Label:     "윤",
		Mode:      "assistant",
	}); err != nil {
		t.Fatal(err)
	}

	text := "매일 오전 8시에 윤에게 음성 보고 보내줘"
	if !assistantRequiresToolCall(text) {
		t.Fatalf("natural-language scheduled delivery should require a bounded tool call: %q", text)
	}
	schedule := inferAssistantScheduledDeliverySchedule(text)
	if schedule != "매일 오전 8시" && schedule != "매일 오전 8시에" {
		t.Fatalf("unexpected schedule baseline: %q", schedule)
	}

	reply, handled := assistantRequiredToolFallbackReply(
		ListenOptions{TargetID: "argos-assistant", Mode: "assistant"},
		text,
	)
	if !handled {
		t.Fatal("expected scheduled delivery fallback to handle the request")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"예약 발송 계획을 만들었습니다.",
		"- 상태: review_ready",
		"- 대상: 윤",
		"- 형식: voice_report / voice_note",
		"다음 단계는 첫 발송 미리보기를 보여주고, 사용자가 승인하면 예약 등록을 적용하는 것입니다.",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("scheduled delivery parity reply missing %q:\n%s", want, visible)
		}
	}
	if strings.Contains(visible, "registered") || strings.Contains(visible, "sent") {
		t.Fatalf("scheduled delivery fallback must remain plan-only:\n%s", visible)
	}
}

func TestAssistantScheduledDeliveryInferenceHelpers(t *testing.T) {
	if got := inferAssistantScheduledDeliverySchedule("매일 아침 8시에 내게 주간 브리핑 보내줘"); got != "매일 아침 8시" && got != "매일 아침 8시에" {
		t.Fatalf("unexpected schedule inference: %q", got)
	}
	if got := inferAssistantScheduledDeliverySchedule("매일 아침 7시에 알람해줘"); got != "매일 아침 7시" && got != "매일 아침 7시에" {
		t.Fatalf("unexpected schedule inference for alarm form: %q", got)
	}
	if got := inferAssistantScheduledDeliverySchedule("매일 아침 7시에 알림줘"); got != "매일 아침 7시" && got != "매일 아침 7시에" {
		t.Fatalf("unexpected schedule inference for 알림줘 form: %q", got)
	}
	if got := inferAssistantScheduledDeliverySchedule("매주 월요일 9시에 윤에게 업무 브리핑 보내줘"); got != "매주 월요일 9시" && got != "매주 월요일 9시에" {
		t.Fatalf("unexpected schedule inference with recipient: %q", got)
	}
	if got := inferAssistantScheduledDeliverySchedule("다음 주 월욜 오후 2시에 장보기 일정 보내줘"); got == "" {
		t.Fatalf("expected schedule inference for colloquial weekday form, got empty")
	}
	if got := inferAssistantScheduledDeliverySchedule("금주 토욜 오후 2시에 장보기 일정 보내줘"); got == "" {
		t.Fatalf("expected schedule inference for compact week form, got empty")
	}
	if got := inferAssistantScheduledDeliverySchedule("다음주 수욜 오후 2시에 장보기 일정 보내줘"); got != "매주 수요일 오후 2시" && got != "매주 수요일 오후 2시에" {
		t.Fatalf("unexpected schedule inference for compact weekday form: %q", got)
	}
	if got := inferAssistantScheduledDeliverySchedule("다음주 금욜 오후 2시에 장보기 일정 보내줘"); got != "매주 금요일 오후 2시" && got != "매주 금요일 오후 2시에" {
		t.Fatalf("unexpected schedule inference for compact weekday variant: %q", got)
	}
	if got := inferAssistantScheduledDeliveryContent("매일 아침 8시에 내게 주간 브리핑 보내줘"); got != "주간 브리핑 보내줘" && got != "주간 브리핑" {
		t.Fatalf("unexpected content inference: %q", got)
	}
	if got := inferAssistantScheduledDeliveryContent("매일 아침 8시에 윤에게 팀 노트 전달해줘"); got == "" || !strings.Contains(got, "팀 노트") {
		t.Fatalf("unexpected content inference for recipient form: %q", got)
	}
}

func TestAssistantTTSVoicePlanDoesNotCreateDocument(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	reply := executeAssistantToolCall(
		ListenOptions{TargetID: "argos-assistant", Mode: "assistant"},
		"오늘의 기도문을 만들어서 edge tts로 파일 만들어줘",
		"send_tts_voice",
		`{"topic":"오늘의 기도문","execute":false}`,
	)
	visible := signalReplyVisibleText(reply)
	if !strings.Contains(visible, "음성파일 생성 계획") || !strings.Contains(visible, "엔진: edge-tts") {
		t.Fatalf("unexpected voice plan reply:\n%s", visible)
	}
	if strings.Contains(visible, "문서를 작성했습니다") || len(signalReplyAttachments(reply)) != 0 {
		t.Fatalf("voice plan should not create document or attachment:\n%s", reply)
	}
}

func TestAssistantVoiceRequestRedirectsWrongDocumentTool(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	reply := executeAssistantToolCall(
		ListenOptions{TargetID: "argos-assistant", Mode: "assistant"},
		"오늘 작업 보고서를 작성하고 내게 음성 메시지로 보내줘",
		"create_document",
		`{"title":"작업 보고서","body":"테스트 보고서입니다.","execute":false}`,
	)
	visible := signalReplyVisibleText(reply)
	if !strings.Contains(visible, "음성 보고서 생성 계획") ||
		!strings.Contains(visible, "보낼 대상: 내게") {
		t.Fatalf("document tool should redirect to voice report plan:\n%s", visible)
	}
	if strings.Contains(visible, "문서를 작성했습니다") || len(signalReplyAttachments(reply)) != 0 {
		t.Fatalf("redirect should not create document/audio attachments:\n%s", reply)
	}
}

func TestAssistantVoiceRequestRedirectsWrongMacActionTool(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	reply := executeAssistantToolCall(
		ListenOptions{TargetID: "argos-assistant", Mode: "assistant"},
		"윤에게 오늘의 기도문을 만들어서 edge tts로 음성파일 보내줘",
		"run_mac_action",
		`{"prompt":"윤에게 오늘의 기도문을 만들어서 edge tts로 음성파일 보내줘","execute":false}`,
	)
	visible := signalReplyVisibleText(reply)
	if !strings.Contains(visible, "음성파일 생성 계획") ||
		!strings.Contains(visible, "엔진: edge-tts") ||
		!strings.Contains(visible, "보낼 대상: 윤") {
		t.Fatalf("mac action tool should redirect to TTS voice plan:\n%s", visible)
	}
	if strings.Contains(visible, "Mac 작업을 이해하지 못했습니다") || len(signalReplyAttachments(reply)) != 0 {
		t.Fatalf("redirect should not run Mac fallback or attach files:\n%s", reply)
	}
}

func TestResolveAssistantTargetAutoAddsExactContactPhone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	oldSearch := assistantSearchContacts
	assistantSearchContacts = func(ctx context.Context, query string) osauto.Result {
		return osauto.Result{
			OK:     true,
			Stdout: `{"kind":"argos_contacts_helper","ok":true,"count":1,"contacts":[{"name":"윤","phones":["010-1234-5678"]}]}`,
		}
	}
	defer func() { assistantSearchContacts = oldSearch }()

	target, _, err := resolveAssistantVoiceTarget("윤")
	if err != nil {
		t.Fatal(err)
	}
	if target.Label != "윤" || target.Recipient != "+821012345678" || target.Channel != "signal" {
		t.Fatalf("unexpected target: %#v", target)
	}
	store, err := ListTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Targets) != 1 || store.Targets[0].ID != target.ID {
		t.Fatalf("target should be persisted: %#v", store.Targets)
	}
}

func TestResolveAssistantTargetDoesNotAutoAddAmbiguousContacts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	oldSearch := assistantSearchContacts
	assistantSearchContacts = func(ctx context.Context, query string) osauto.Result {
		return osauto.Result{
			OK:     true,
			Stdout: `{"kind":"argos_contacts_helper","ok":true,"count":2,"contacts":[{"name":"윤 A","phones":["010-1111-1111"]},{"name":"윤 B","phones":["010-2222-2222"]}]}`,
		}
	}
	defer func() { assistantSearchContacts = oldSearch }()

	_, candidates, err := resolveAssistantVoiceTarget("윤")
	if err == nil {
		t.Fatal("expected ambiguous contacts to require user choice")
	}
	if len(candidates) != 2 {
		t.Fatalf("expected two candidates, got %#v", candidates)
	}
	store, err := ListTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Targets) != 0 {
		t.Fatalf("ambiguous contacts should not be persisted: %#v", store.Targets)
	}
}

func TestAssistantArtifactToolsCreateRealOutputs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	docReply := executeAssistantToolCall(
		ListenOptions{TargetID: "argos-assistant", Mode: "assistant"},
		"내일 회의용 자료 만들어줘",
		"create_document",
		`{"title":"내일 회의 자료","body":"# 회의 목적\n- 진행 상황 공유\n# 논의할 것\n- 다음 액션"}`,
	)
	docVisible := signalReplyVisibleText(docReply)
	if !strings.Contains(docVisible, "문서를 작성했습니다.") ||
		!strings.Contains(docVisible, "Word나 Pages에서 열 수 있는 문서 파일을 준비했습니다.") ||
		!strings.Contains(docVisible, "Obsidian용 원본") ||
		!strings.Contains(docVisible, "Signal에서 바로 읽기:") ||
		!strings.Contains(docVisible, "흐름도: 요청 --> 작성 --> Obsidian 저장 --> Signal 보고") ||
		!strings.Contains(docVisible, "- 회의 목적") ||
		!strings.Contains(docVisible, "- 진행 상황 공유") ||
		strings.Contains(docVisible, "일정에 필요한 정보") ||
		strings.Contains(docVisible, "open_url") {
		t.Fatalf("unexpected document reply:\n%s", docVisible)
	}
	for _, bad := range []string{"Markdown:", "DOCX:", "/Documents/Argos Vault/", "링크:", "http://", "evidence"} {
		if strings.Contains(docVisible, bad) {
			t.Fatalf("document reply should be natural language, found %q:\n%s", bad, docVisible)
		}
	}
	if attachments := signalReplyAttachments(docReply); len(attachments) == 0 {
		t.Fatalf("document reply should include Signal attachments")
	}

	sheetReply := executeAssistantToolCall(
		ListenOptions{TargetID: "argos-assistant", Mode: "assistant"},
		"이번 달 Argos 비용 예산표를 엑셀로 만들어줘",
		"create_spreadsheet",
		`{"title":"Argos 비용 예산표","body":"| 구분 | 예산 | 실사용 |\n| --- | --- | --- |\n| 서버 | 100000 | 80000 |"}`,
	)
	sheetVisible := signalReplyVisibleText(sheetReply)
	if !strings.Contains(sheetVisible, "표 파일을 작성했습니다.") ||
		!strings.Contains(sheetVisible, "Numbers나 Excel에서 바로 열 수 있는 XLSX 파일을 준비했습니다.") ||
		strings.Contains(sheetVisible, "/Documents/Argos Vault/") ||
		strings.Contains(sheetVisible, "evidence") {
		t.Fatalf("unexpected spreadsheet reply:\n%s", sheetVisible)
	}
	sheetAttachments := signalReplyAttachments(sheetReply)
	if len(sheetAttachments) < 2 {
		t.Fatalf("spreadsheet reply should include XLSX/CSV attachments: %#v", sheetAttachments)
	}
	hasXLSX, hasCSV := false, false
	for _, attachment := range sheetAttachments {
		hasXLSX = hasXLSX || strings.HasSuffix(attachment, ".xlsx")
		hasCSV = hasCSV || strings.HasSuffix(attachment, ".csv")
	}
	if !hasXLSX || !hasCSV {
		t.Fatalf("missing spreadsheet attachments: %#v", sheetAttachments)
	}

	pptReply := executeAssistantToolCall(
		ListenOptions{TargetID: "argos-assistant", Mode: "assistant"},
		"내일 회의용 발표자료 만들어줘",
		"create_presentation",
		`{"title":"내일 회의 발표자료","body":"# 목표\n- 비서 도구 현황 공유\n# 결정\n- 다음 테스트 범위 확정","audience":"팀 회의","slide_count":4}`,
	)
	pptVisible := signalReplyVisibleText(pptReply)
	if !strings.Contains(pptVisible, "발표자료를 작성하고 검증했습니다.") ||
		!strings.Contains(pptVisible, "PowerPoint에서 바로 열 수 있는 PPTX 파일을 만들었습니다.") ||
		!strings.Contains(pptVisible, "Obsidian에서 다듬을 수 있는 발표 outline") ||
		!strings.Contains(pptVisible, "검증:") ||
		strings.Contains(pptVisible, "일정에 필요한 정보") ||
		strings.Contains(pptVisible, "open_url") {
		t.Fatalf("unexpected presentation reply:\n%s", pptVisible)
	}
	for _, bad := range []string{"PPTX:", "Outline:", "Preview:", "/Documents/Argos Vault/", "링크:", "http://", "evidence"} {
		if strings.Contains(pptVisible, bad) {
			t.Fatalf("presentation reply should be natural language, found %q:\n%s", bad, pptVisible)
		}
	}
	if attachments := signalReplyAttachments(pptReply); len(attachments) == 0 {
		t.Fatalf("presentation reply should include Signal attachments")
	}

	meetingReply := executeAssistantToolCall(
		ListenOptions{TargetID: "argos-assistant", Mode: "assistant"},
		"내일 회의 자료를 만들어줘",
		"prepare_meeting_materials",
		`{"title":"내일 회의 자료","body":"개발 현황과 다음 액션을 공유한다.","audience":"내부 팀","slide_count":5}`,
	)
	meetingVisible := signalReplyVisibleText(meetingReply)
	for _, want := range []string{"회의 자료 패키지를 준비했습니다.", "회의 브리프는 Word/Pages 문서로 만들었습니다.", "발표자료는 PPTX로 만들었습니다.", "Obsidian용 브리프 원본", "검증:"} {
		if !strings.Contains(meetingVisible, want) {
			t.Fatalf("meeting reply missing %q:\n%s", want, meetingVisible)
		}
	}
	if strings.Contains(meetingVisible, "일정에 필요한 정보") ||
		strings.Contains(meetingVisible, "open_url") {
		t.Fatalf("meeting materials should not become calendar/url fallback:\n%s", meetingVisible)
	}
	for _, bad := range []string{"브리프 Markdown:", "브리프 DOCX:", "발표 PPTX:", "발표 Outline:", "발표 Preview:", "/Documents/Argos Vault/", "링크:", "http://", "evidence"} {
		if strings.Contains(meetingVisible, bad) {
			t.Fatalf("meeting reply should be natural language, found %q:\n%s", bad, meetingVisible)
		}
	}
	if attachments := signalReplyAttachments(meetingReply); len(attachments) < 2 {
		t.Fatalf("meeting reply should include document and presentation attachments: %#v", attachments)
	}
}

func TestAssistantDocumentReplySignalReadableSmoke(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	reply := executeAssistantToolCall(
		ListenOptions{TargetID: "argos-assistant", Mode: "assistant"},
		"내일 회의용 진행상황 보고서를 Obsidian 문서로 만들어줘. 핵심 성과, 남은 리스크, 다음 액션이 보이게 해줘.",
		"create_document",
		`{"title":"내일 회의용 진행상황 보고서","body":"# 핵심 성과\n- Signal 본문에서 바로 읽는 보고 포맷을 적용했습니다.\n- Obsidian 원본과 iPhone용 HTML 미리보기를 함께 저장합니다.\n# 남은 리스크\n- UI Runner 최신 교체는 macOS 권한 토글이 필요합니다.\n# 다음 액션\n- 실제 Signal 방에서 본문 가독성을 확인합니다."}`,
	)
	visible := signalReplyVisibleText(reply)
	attachments := signalReplyAttachments(reply)
	t.Logf("visible Signal text:\n%s", visible)
	t.Logf("attachment_count=%d", len(attachments))

	for _, want := range []string{
		"Signal에서 바로 읽기:",
		"흐름도: 요청 --> 작성 --> Obsidian 저장 --> Signal 보고",
		"- 핵심 성과",
		"- Signal 본문에서 바로 읽는 보고 포맷을 적용했습니다.",
		"- UI Runner 최신 교체는 macOS 권한 토글이 필요합니다.",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("visible reply missing %q:\n%s", want, visible)
		}
	}
	if strings.Contains(visible, "meshclaw-attachment:") || strings.Contains(visible, "/Documents/Argos Vault/") {
		t.Fatalf("visible reply leaked attachment internals:\n%s", visible)
	}
	if len(attachments) == 0 {
		t.Fatalf("expected document attachments")
	}
}

func TestAssistantExportFailureGivesUsefulFallback(t *testing.T) {
	pptx := filepath.Join(t.TempDir(), "meeting.pptx")
	if err := os.WriteFile(pptx, []byte("pptx"), 0600); err != nil {
		t.Fatal(err)
	}
	reply := formatAssistantExportResult("발표자료", osauto.Result{
		Kind:   "meshclaw_automation_presentation_export",
		Action: "presentation_export",
		PPTX:   pptx,
		URL:    pptx,
		Error:  "LibreOffice/soffice is required for PPTX to PDF export",
	}, evidenceRecordForTest("/tmp/export.json"), nil)
	visible := signalReplyVisibleText(reply)
	if !strings.Contains(visible, "바로 변환하지는 못했습니다") ||
		!strings.Contains(visible, "필요한 PDF 변환 도구") ||
		!strings.Contains(visible, "LibreOffice") ||
		!strings.Contains(visible, "원본 PPTX") ||
		strings.Contains(visible, "is required") ||
		strings.Contains(visible, "evidence:") {
		t.Fatalf("reply=%q visible=%q", reply, visible)
	}
	if attachments := signalReplyAttachments(reply); len(attachments) != 1 || attachments[0] != pptx {
		t.Fatalf("attachments=%#v reply=%q", attachments, reply)
	}

	md := filepath.Join(t.TempDir(), "brief.md")
	if err := os.WriteFile(md, []byte("# brief"), 0600); err != nil {
		t.Fatal(err)
	}
	docReply := formatAssistantExportResult("문서", osauto.Result{
		Kind:     "meshclaw_automation_document_export",
		Action:   "document_export",
		Markdown: md,
		URL:      md,
		Error:    "pandoc is required for pdf export",
	}, evidenceRecordForTest("/tmp/export.json"), nil)
	docVisible := signalReplyVisibleText(docReply)
	if !strings.Contains(docVisible, "필요한 PDF 변환 도구") ||
		!strings.Contains(docVisible, "pandoc") ||
		!strings.Contains(docVisible, "Markdown 원본") ||
		strings.Contains(docVisible, "is required") {
		t.Fatalf("doc reply=%q visible=%q", docReply, docVisible)
	}
	if attachments := signalReplyAttachments(docReply); len(attachments) != 1 || attachments[0] != md {
		t.Fatalf("doc attachments=%#v reply=%q", attachments, docReply)
	}
}

func TestAssistantArtifactRequestDoesNotFallBackToCalendar(t *testing.T) {
	t.Setenv("MESHCLAW_ASSISTANT_TOOL_LOOP", "0")
	for _, text := range []string{
		"내일 회의 자료를 만들어줘",
		"내일 회의용 자료 만들어줘",
		"내일 회의용 발표자료 만들어줘",
	} {
		if reply, handled := assistantInteractiveReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, text); handled {
			t.Fatalf("%q should not be handled by deterministic scenario fallback:\n%s", text, reply)
		}
	}
}

func TestAssistantArtifactRequestDoesNotReturnTemplateWhenToolLoopUnavailable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ASSISTANT_TOOL_LOOP", "0")

	reply := signalReplyVisibleText(assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "내일 회의 자료를 만들어줘"))
	for _, want := range []string{"회의 자료 패키지를 준비했습니다.", "회의 브리프는 Word/Pages 문서로 만들었습니다.", "발표자료는 PPTX로 만들었습니다.", "검증:"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	for _, bad := range []string{"몇 가지 세부 사항이 필요", "**회의 주제**", "**대상**", "**원하는 문서 형식**", "일정에 필요한 정보", "브리프 Markdown:", "발표 PPTX:"} {
		if strings.Contains(reply, bad) {
			t.Fatalf("reply should not be a template clarification containing %q:\n%s", bad, reply)
		}
	}

	pptReply := signalReplyVisibleText(assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "내일 오전 임원 미팅용으로 Argos 진행상황 발표자료를 5장으로 만들어줘. 핵심 성과, 남은 리스크, 다음 액션이 보이게 해줘"))
	for _, want := range []string{"발표자료를 작성하고 검증했습니다.", "PowerPoint에서 바로 열 수 있는 PPTX 파일을 만들었습니다.", "검증: PPTX 파일 구조를 확인했고 5개 슬라이드가 있습니다."} {
		if !strings.Contains(pptReply, want) {
			t.Fatalf("presentation reply missing %q:\n%s", want, pptReply)
		}
	}
	for _, bad := range []string{"정확히 이해하지 못했습니다", "원하시는 작업", "몇 가지 세부 사항이 필요", "PPTX:", "/Documents/Argos Vault/", "evidence"} {
		if strings.Contains(pptReply, bad) {
			t.Fatalf("presentation reply should not contain %q:\n%s", bad, pptReply)
		}
	}
}

func TestAssistantMeetingMinutesCanTargetBriefingRoomDryRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ASSISTANT_TOOL_LOOP", "0")
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant", Mode: "assistant"},
		"오늘 회의록을 작성해서 보고방에 보내줘. 논의: 사용자에게 런타임 설명보다 실제 결과물을 보여준다. 결정: 회의록과 시장조사부터 강화한다.",
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"문서/보고서를 Signal로 보낼 준비를 했습니다.",
		"대상: 보고방",
		"보고방은 one-way/no-reply",
		"보낼 내용:",
		"Argos 회의록 문서/보고서입니다.",
		"Signal에서 바로 읽기:",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("meeting minutes reply missing %q:\n%s", want, visible)
		}
	}
	if strings.ContainsRune(visible, '\uFFFD') {
		t.Fatalf("meeting minutes visible text contains replacement character:\n%s", visible)
	}
	attachments := signalReplyAttachments(reply)
	if len(attachments) == 0 {
		t.Fatalf("meeting minutes should attach created document files; raw=%q", reply)
	}
	for _, attachment := range attachments {
		if strings.HasSuffix(attachment, ".html") {
			t.Fatalf("briefing document send should not attach raw HTML: %#v", attachments)
		}
	}
}

func TestAssistantMarketResearchCanTargetBriefingRoomDryRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ASSISTANT_TOOL_LOOP", "0")
	t.Setenv("MESHCLAW_ASSISTANT_RESEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant", Mode: "assistant"},
		"전기차 충전 인프라 시장조사 보고서를 작성해서 보고방에 보내줘. 내일 회의에서 투자/제품 전략 토픽으로 쓸 수 있게 정리해줘.",
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"문서/보고서를 Signal로 보낼 준비를 했습니다.",
		"대상: 보고방",
		"보고방은 one-way/no-reply",
		"전기차 충전 인프라 시장조사 보고서",
		"Signal에서 바로 읽기:",
		"핵심 결론",
		"내일 회의에서 물어볼 질문",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("market research reply missing %q:\n%s", want, visible)
		}
	}
	if strings.ContainsRune(visible, '\uFFFD') {
		t.Fatalf("market research visible text contains replacement character:\n%s", visible)
	}
	attachments := signalReplyAttachments(reply)
	if len(attachments) == 0 {
		t.Fatalf("market research should attach created document files; raw=%q", reply)
	}
	for _, attachment := range attachments {
		if strings.HasSuffix(attachment, ".html") {
			t.Fatalf("briefing market research send should not attach raw HTML: %#v", attachments)
		}
	}

	chemicalReply := assistantReply(
		ListenOptions{TargetID: "argos-assistant", Mode: "assistant"},
		"최근 화학회사들이 민감하게 반응할만한 뉴스와 시장 이슈를 찾아서 분석 보고서를 작성해서 보고방에 보내줘. 내일 회의에서 사용할 중요한 토픽과 질문을 정해줘.",
	)
	chemicalVisible := signalReplyVisibleText(chemicalReply)
	for _, want := range []string{
		"문서/보고서를 Signal로 보낼 준비를 했습니다.",
		"화학회사 민감 뉴스 분석 보고서",
		"내일 회의 핵심 토픽",
		"PFAS",
	} {
		if !strings.Contains(chemicalVisible, want) {
			t.Fatalf("chemical market research reply missing %q:\n%s", want, chemicalVisible)
		}
	}
	if strings.Contains(chemicalVisible, "보고방에 보낼 예약 문구") || strings.Contains(chemicalVisible, "전화/메시지 초안") {
		t.Fatalf("chemical market research request was hijacked by booking forward:\n%s", chemicalVisible)
	}

	chemicalDeckReply := assistantReply(
		ListenOptions{TargetID: "argos-assistant", Mode: "assistant"},
		"최근 화학회사들이 민감하게 반응할만한 뉴스를 찾아서 분석하고 내일 회의에서 사용할 5장 PPT 자료로 만들어줘.",
	)
	chemicalDeckVisible := signalReplyVisibleText(chemicalDeckReply)
	for _, want := range []string{
		"회의 자료 패키지를 준비했습니다.",
		"회의 브리프는 Word/Pages 문서로 만들었습니다.",
		"발표자료는 PPTX로 만들었습니다.",
		"iPhone Signal에서 PPTX 첨부를 탭",
		"Signal에서 바로 읽기:",
		"화학회사 민감 뉴스 분석",
		"검증: PPTX 파일 구조를 확인했고 5개 슬라이드가 있습니다.",
	} {
		if !strings.Contains(chemicalDeckVisible, want) {
			t.Fatalf("chemical PPT package reply missing %q:\n%s", want, chemicalDeckVisible)
		}
	}
	for _, forbidden := range []string{"meshclaw-attachment:", "/Documents/Argos Vault/", "/.meshclaw/evidence/", "PPTX:", "브리프 DOCX:"} {
		if strings.Contains(chemicalDeckVisible, forbidden) {
			t.Fatalf("chemical PPT package visible reply leaked %q:\n%s", forbidden, chemicalDeckVisible)
		}
	}
	chemicalDeckAttachments := signalReplyAttachments(chemicalDeckReply)
	if !containsAttachmentExt(chemicalDeckAttachments, ".pptx") ||
		!containsAttachmentExt(chemicalDeckAttachments, ".docx") ||
		!containsAttachmentExt(chemicalDeckAttachments, ".md") {
		t.Fatalf("chemical PPT package should attach PPTX/DOCX/MD, got %#v; raw=%q", chemicalDeckAttachments, chemicalDeckReply)
	}
	if containsAttachmentExt(chemicalDeckAttachments, ".html") || containsAttachmentExt(chemicalDeckAttachments, ".htm") {
		t.Fatalf("chemical PPT package should not attach raw HTML by default, got %#v", chemicalDeckAttachments)
	}
}

func TestAssistantRecentArtifactFollowUps(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	opts := ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}

	created := executeAssistantToolCall(
		opts,
		"내일 회의 자료를 만들어줘",
		"prepare_meeting_materials",
		`{"title":"내일 회의 자료","body":"진행 상황과 다음 액션을 공유한다.","audience":"내부 팀","slide_count":4}`,
	)
	if !strings.Contains(signalReplyVisibleText(created), "회의 자료 패키지를 준비했습니다.") {
		t.Fatalf("created reply=%s", signalReplyVisibleText(created))
	}
	state, ok := loadAssistantArtifact(opts)
	if !ok || state.Document.Markdown == "" || state.Presentation.PPTX == "" {
		t.Fatalf("missing recent artifact state: ok=%t state=%#v", ok, state)
	}

	resent, handled := assistantRecentArtifactFallbackReply(opts, "방금 만든 파일 다시 보내줘")
	if !handled {
		t.Fatal("expected resend follow-up to be handled")
	}
	if !strings.Contains(signalReplyVisibleText(resent), "최근 만든 파일을 다시 보냅니다.") {
		t.Fatalf("resent visible=%s", signalReplyVisibleText(resent))
	}
	if !strings.Contains(signalReplyVisibleText(resent), "iPhone Signal에서 PPTX/DOCX/MD 첨부를 탭") {
		t.Fatalf("resent reply should explain mobile opening: %s", signalReplyVisibleText(resent))
	}
	for _, bad := range []string{"meshclaw-attachment:", "/Documents/Argos Vault/", "evidence"} {
		if strings.Contains(signalReplyVisibleText(resent), bad) {
			t.Fatalf("resent visible text leaked %q:\n%s", bad, signalReplyVisibleText(resent))
		}
	}
	if attachments := signalReplyAttachments(resent); len(attachments) < 2 {
		t.Fatalf("expected resent attachments: %#v", attachments)
	}

	pptOnly, handled := assistantRecentArtifactFallbackReply(opts, "그 PPT 다시 보내줘")
	if !handled {
		t.Fatal("expected presentation resend follow-up to be handled")
	}
	pptAttachments := signalReplyAttachments(pptOnly)
	if len(pptAttachments) == 0 {
		t.Fatalf("expected presentation attachments")
	}
	if !strings.Contains(signalReplyVisibleText(pptOnly), "iPhone Signal에서 PPTX/DOCX/MD 첨부를 탭") {
		t.Fatalf("presentation resend should explain mobile opening: %s", signalReplyVisibleText(pptOnly))
	}
	for _, attachment := range pptAttachments {
		if strings.HasSuffix(attachment, ".docx") {
			t.Fatalf("presentation resend should not include document attachment: %#v", pptAttachments)
		}
	}

	pptPDF, handled := assistantRecentArtifactFallbackReply(opts, "그 PPT를 PDF로 보내줘. 안 되면 원본 PPTX를 다시 보내줘")
	if !handled {
		t.Fatal("expected presentation export follow-up to be handled")
	}
	pptPDFVisible := signalReplyVisibleText(pptPDF)
	if strings.Contains(pptPDFVisible, "최근 만든 파일을 다시 보냅니다.") {
		t.Fatalf("pdf export request should not be treated as resend: %s", pptPDFVisible)
	}
	if !strings.Contains(pptPDFVisible, "발표자료를 요청한 형식으로") &&
		!strings.Contains(pptPDFVisible, "발표자료를 요청한 형식으로 내보냈습니다.") {
		t.Fatalf("expected export wording, got=%s", pptPDFVisible)
	}

	docPDF, handled := assistantRecentArtifactFallbackReply(opts, "방금 만든 문서를 PDF로 보내줘. 안 되면 편집 가능한 원본 DOCX를 다시 보내줘")
	if !handled {
		t.Fatal("expected document pdf export follow-up to be handled")
	}
	docPDFVisible := signalReplyVisibleText(docPDF)
	if strings.Contains(docPDFVisible, "문서를 요청한 형식으로 내보냈습니다.") {
		hasPDF := false
		for _, attachment := range signalReplyAttachments(docPDF) {
			if strings.HasSuffix(strings.ToLower(attachment), ".pdf") {
				hasPDF = true
				break
			}
		}
		if !hasPDF {
			t.Fatalf("pdf request with docx fallback should not be treated as docx export: %s", docPDFVisible)
		}
	}
	if !strings.Contains(docPDFVisible, "문서를 요청한 형식으로 바로 변환하지는 못했습니다.") &&
		!strings.Contains(docPDFVisible, "문서를 요청한 형식으로 내보냈습니다.") {
		t.Fatalf("expected document export wording, got=%s", docPDFVisible)
	}

	revised, handled := assistantRecentArtifactFallbackReply(opts, "방금 만든 PPT를 3장으로 줄여줘")
	if !handled {
		t.Fatal("expected revision follow-up to be handled")
	}
	visible := signalReplyVisibleText(revised)
	if !strings.Contains(visible, "최근 발표자료를 기준으로 수정본을 만들었습니다.") ||
		!strings.Contains(visible, "검증:") {
		t.Fatalf("revision visible=%s", visible)
	}
	if attachments := signalReplyAttachments(revised); len(attachments) == 0 {
		t.Fatalf("expected revision attachments")
	}

	docRevised, handled := assistantRecentArtifactFallbackReply(opts, "방금 만든 문서를 더 짧게 다듬어줘")
	if !handled {
		t.Fatal("expected document revision follow-up to be handled")
	}
	docVisible := signalReplyVisibleText(docRevised)
	if !strings.Contains(docVisible, "최근 문서를 기준으로 수정본을 만들었습니다.") ||
		strings.Contains(docVisible, "/Documents/Argos Vault/") ||
		strings.Contains(docVisible, "evidence") ||
		strings.Contains(docVisible, "작업 기록도 저장했습니다") {
		t.Fatalf("document revision visible=%s", docVisible)
	}
	docAttachments := signalReplyAttachments(docRevised)
	if len(docAttachments) == 0 {
		t.Fatal("expected document revision attachments")
	}
	for _, attachment := range docAttachments {
		if strings.HasSuffix(attachment, ".pptx") {
			t.Fatalf("document revision should not include presentation attachment: %#v", docAttachments)
		}
	}
}

func TestAssistantRecentSpreadsheetFollowUps(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	opts := ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}

	created := executeAssistantToolCall(
		opts,
		"이번 달 서버 비용 예산표를 엑셀로 만들어줘",
		"create_spreadsheet",
		`{"title":"Argos 예산표","body":"| 구분 | 예산 | 실사용 | 차이 | 메모 |\n| --- | --- | --- | --- | --- |\n| 서버 | 100000 | 80000 | 20000 | 기본 서버 비용 |\n| 합계 | 100000 | 80000 | 20000 | 자동 합계 |"}`,
	)
	if !strings.Contains(signalReplyVisibleText(created), "표 파일을 작성했습니다.") {
		t.Fatalf("created reply=%s", signalReplyVisibleText(created))
	}
	state, ok := loadAssistantArtifact(opts)
	if !ok || state.Spreadsheet.XLSX == "" || state.Spreadsheet.CSV == "" || state.Kind != "spreadsheet" {
		t.Fatalf("missing spreadsheet state: ok=%t state=%#v", ok, state)
	}

	csvOnly, handled := assistantRecentArtifactFallbackReply(opts, "CSV만 다시 보내줘")
	if !handled {
		t.Fatal("expected CSV-only resend follow-up")
	}
	csvAttachments := signalReplyAttachments(csvOnly)
	if len(csvAttachments) != 1 || !strings.HasSuffix(csvAttachments[0], ".csv") {
		t.Fatalf("expected only CSV attachment: %#v visible=%s", csvAttachments, signalReplyVisibleText(csvOnly))
	}

	xlsxOnly, handled := assistantRecentArtifactFallbackReply(opts, "방금 만든 예산표를 XLSX로 보내줘")
	if !handled {
		t.Fatal("expected XLSX export follow-up")
	}
	xlsxAttachments := signalReplyAttachments(xlsxOnly)
	if len(xlsxAttachments) != 1 || !strings.HasSuffix(xlsxAttachments[0], ".xlsx") {
		t.Fatalf("expected only XLSX attachment: %#v visible=%s", xlsxAttachments, signalReplyVisibleText(xlsxOnly))
	}

	revised, handled := assistantRecentArtifactFallbackReply(opts, "방금 만든 예산표에 GPU 비용 항목 추가해줘")
	if !handled {
		t.Fatal("expected spreadsheet revision follow-up")
	}
	visible := signalReplyVisibleText(revised)
	if !strings.Contains(visible, "최근 표 파일을 기준으로 수정본을 만들었습니다.") {
		t.Fatalf("revision visible=%s", visible)
	}
	if attachments := signalReplyAttachments(revised); len(attachments) < 2 {
		t.Fatalf("expected spreadsheet revision attachments: %#v", attachments)
	} else {
		foundGPU := false
		for _, attachment := range attachments {
			if !strings.HasSuffix(attachment, ".csv") {
				continue
			}
			data, err := os.ReadFile(attachment)
			if err != nil {
				t.Fatalf("read revision csv: %v", err)
			}
			foundGPU = strings.Contains(string(data), "GPU 비용")
		}
		if !foundGPU {
			t.Fatalf("revision CSV should include requested GPU row: %#v", attachments)
		}
	}

	updated, handled := assistantRecentArtifactFallbackReply(opts, "GPU 비용 예산을 500000으로 바꿔줘. 새 파일로 보내줘")
	if !handled {
		t.Fatal("expected spreadsheet budget update follow-up")
	}
	updatedCSV := ""
	for _, attachment := range signalReplyAttachments(updated) {
		if strings.HasSuffix(attachment, ".csv") {
			data, err := os.ReadFile(attachment)
			if err != nil {
				t.Fatalf("read updated csv: %v", err)
			}
			updatedCSV = string(data)
		}
	}
	if !strings.Contains(updatedCSV, "GPU 비용,500000,0,500000,추가 요청") ||
		!strings.Contains(updatedCSV, "합계,600000,80000,520000") {
		t.Fatalf("updated CSV should change GPU budget and recalculate totals:\n%s", updatedCSV)
	}

	deleted, handled := assistantRecentArtifactFallbackReply(opts, "GPU 비용 항목 삭제해줘. 새 파일로 보내줘")
	if !handled {
		t.Fatal("expected spreadsheet delete follow-up")
	}
	deletedCSV := ""
	for _, attachment := range signalReplyAttachments(deleted) {
		if strings.HasSuffix(attachment, ".csv") {
			data, err := os.ReadFile(attachment)
			if err != nil {
				t.Fatalf("read deleted csv: %v", err)
			}
			deletedCSV = string(data)
		}
	}
	if strings.Contains(deletedCSV, "GPU 비용") ||
		!strings.Contains(deletedCSV, "합계,100000,80000,20000") {
		t.Fatalf("deleted CSV should remove GPU row and recalculate totals:\n%s", deletedCSV)
	}

	if target := inferredRecentArtifactTarget("Numbers로 열어줘"); target != "spreadsheet" {
		t.Fatalf("target=%q", target)
	}
	if app := inferredRecentArtifactApp("Numbers로 열어줘"); app != "Numbers" {
		t.Fatalf("app=%q", app)
	}
}

func TestAssistantRecentSpreadsheetUpdateAddsMissingRowWithValue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	opts := ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}

	_ = executeAssistantToolCall(
		opts,
		"이번 달 서버 비용 예산표를 엑셀로 만들어줘",
		"create_spreadsheet",
		`{"title":"Argos 예산표","body":"| 구분 | 예산 | 실사용 | 차이 | 메모 |\n| --- | --- | --- | --- | --- |\n| 서버 | 100000 | 80000 | 20000 | 기본 서버 비용 |\n| 합계 | 100000 | 80000 | 20000 | 자동 합계 |"}`,
	)

	updated, handled := assistantRecentArtifactFallbackReply(opts, "GPU 비용 예산을 500000으로 바꿔줘. 새 파일로 보내줘")
	if !handled {
		t.Fatal("expected missing-row spreadsheet update follow-up")
	}
	updatedCSV := ""
	for _, attachment := range signalReplyAttachments(updated) {
		if strings.HasSuffix(attachment, ".csv") {
			data, err := os.ReadFile(attachment)
			if err != nil {
				t.Fatalf("read updated csv: %v", err)
			}
			updatedCSV = string(data)
		}
	}
	if !strings.Contains(updatedCSV, "GPU 비용,500000,0,500000,추가 요청") ||
		!strings.Contains(updatedCSV, "GPU 비용,500000,0,500000,추가 요청\n합계,600000,80000,520000") {
		t.Fatalf("missing-row update should insert GPU row before totals with requested value:\n%s", updatedCSV)
	}
}

func TestAssistantRecentSpreadsheetTextAndInvoiceUpdates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	opts := ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}

	_ = executeAssistantToolCall(
		opts,
		"이번 주 작업 체크리스트 트래커를 스프레드시트로 만들어줘",
		"create_spreadsheet",
		`{"title":"Argos 트래커","body":"| 항목 | 담당 | 상태 | 마감 | 다음 액션 |\n| --- | --- | --- | --- | --- |\n| 첫 번째 작업 | | 대기 | | 내용 입력 |\n| 두 번째 작업 | | 대기 | | 내용 입력 |"}`,
	)
	statusReply, handled := assistantRecentArtifactFallbackReply(opts, "첫 번째 작업 상태를 완료로 바꿔줘. 새 파일로 보내줘")
	if !handled {
		t.Fatal("expected checklist status update follow-up")
	}
	statusCSV := csvAttachmentText(t, statusReply)
	if !strings.Contains(statusCSV, "첫 번째 작업,,완료,,내용 입력") {
		t.Fatalf("status CSV should update checklist status:\n%s", statusCSV)
	}

	renameReply, handled := assistantRecentArtifactFallbackReply(opts, "첫 번째 작업 이름을 자료 준비로 바꿔줘. 새 파일로 보내줘")
	if !handled {
		t.Fatal("expected checklist row rename follow-up")
	}
	for _, attachment := range signalReplyAttachments(renameReply) {
		if strings.Contains(filepath.Base(attachment), "수정본-수정본") {
			t.Fatalf("revision filename should not repeat 수정본: %#v", signalReplyAttachments(renameReply))
		}
	}
	renameCSV := csvAttachmentText(t, renameReply)
	if !strings.Contains(renameCSV, "자료 준비,,완료,,내용 입력") ||
		strings.Contains(renameCSV, "첫 번째 작업") {
		t.Fatalf("rename CSV should update the row label and preserve status:\n%s", renameCSV)
	}
	assigneeReply, handled := assistantRecentArtifactFallbackReply(opts, "자료 준비 담당을 김대리로 바꿔줘. 새 파일로 보내줘")
	if !handled {
		t.Fatal("expected checklist assignee update follow-up")
	}
	assigneeCSV := csvAttachmentText(t, assigneeReply)
	if !strings.Contains(assigneeCSV, "자료 준비,김대리,완료,,내용 입력") {
		t.Fatalf("assignee CSV should update 담당 and preserve status:\n%s", assigneeCSV)
	}
	dueReply, handled := assistantRecentArtifactFallbackReply(opts, "자료 준비 마감을 내일 오전으로 바꿔줘. 새 파일로 보내줘")
	if !handled {
		t.Fatal("expected checklist deadline update follow-up")
	}
	dueCSV := csvAttachmentText(t, dueReply)
	if !strings.Contains(dueCSV, "자료 준비,김대리,완료,내일 오전,내용 입력") {
		t.Fatalf("deadline CSV should update 마감:\n%s", dueCSV)
	}
	memoReply, handled := assistantRecentArtifactFallbackReply(opts, "자료 준비 다음 액션을 회의 전에 공유로 바꿔줘. 새 파일로 보내줘")
	if !handled {
		t.Fatal("expected checklist next-action update follow-up")
	}
	memoCSV := csvAttachmentText(t, memoReply)
	if !strings.Contains(memoCSV, "자료 준비,김대리,완료,내일 오전,회의 전에 공유") {
		t.Fatalf("next-action CSV should update longer text:\n%s", memoCSV)
	}

	_ = executeAssistantToolCall(
		opts,
		"이번 달 외주 비용 청구서를 엑셀로 만들어줘",
		"create_spreadsheet",
		`{"title":"Argos 청구서","body":"| 항목 | 수량 | 단가 | 금액 | 메모 |\n| --- | --- | --- | --- | --- |\n| 서비스/제품 | 1 | 0 | 0 | 수정 필요 |\n| 합계 | | | 0 | |"}`,
	)
	invoiceReply, handled := assistantRecentArtifactFallbackReply(opts, "서비스/제품 단가를 300000으로 바꿔줘. 새 파일로 보내줘")
	if !handled {
		t.Fatal("expected invoice unit price update follow-up")
	}
	invoiceCSV := csvAttachmentText(t, invoiceReply)
	if !strings.Contains(invoiceCSV, "서비스/제품,1,300000,300000,수정 필요") ||
		!strings.Contains(invoiceCSV, "합계,,,300000,") {
		t.Fatalf("invoice CSV should update unit price and recalculate amount/total:\n%s", invoiceCSV)
	}
	quantityReply, handled := assistantRecentArtifactFallbackReply(opts, "서비스/제품 수량을 3으로 바꿔줘. 새 파일로 보내줘")
	if !handled {
		t.Fatal("expected invoice quantity update follow-up")
	}
	quantityCSV := csvAttachmentText(t, quantityReply)
	if !strings.Contains(quantityCSV, "서비스/제품,3,300000,900000,수정 필요") ||
		!strings.Contains(quantityCSV, "합계,,,900000,") {
		t.Fatalf("invoice CSV should update quantity and recalculate amount/total:\n%s", quantityCSV)
	}
	noteReply, handled := assistantRecentArtifactFallbackReply(opts, "서비스/제품 메모에 \"입금 확인 필요\"라고 적어줘. 새 파일로 보내줘")
	if !handled {
		t.Fatal("expected invoice note update follow-up")
	}
	noteCSV := csvAttachmentText(t, noteReply)
	if !strings.Contains(noteCSV, "서비스/제품,3,300000,900000,입금 확인 필요") ||
		!strings.Contains(noteCSV, "합계,,,900000,") {
		t.Fatalf("invoice CSV should update quoted memo and preserve totals:\n%s", noteCSV)
	}
}

func csvAttachmentText(t *testing.T, reply string) string {
	t.Helper()
	for _, attachment := range signalReplyAttachments(reply) {
		if strings.HasSuffix(attachment, ".csv") {
			data, err := os.ReadFile(attachment)
			if err != nil {
				t.Fatalf("read csv attachment: %v", err)
			}
			return string(data)
		}
	}
	t.Fatalf("missing csv attachment in reply: %s", reply)
	return ""
}

func TestAssistantWorkReportDocumentRequestUsesArtifactFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := evidence.Store("assistant-presentation-create", "argos-assistant", "임원 미팅 발표자료 5장 생성 및 Signal 첨부 완료", map[string]interface{}{"action": "presentation_create"}); err != nil {
		t.Fatalf("seed evidence: %v", err)
	}
	if _, err := evidence.Store("assistant-document-export", "argos-assistant", "업무 보고 문서 PDF 변환 요청 처리", map[string]interface{}{"action": "document_export"}); err != nil {
		t.Fatalf("seed evidence: %v", err)
	}

	reply, handled := assistantRequiredToolFallbackReply(
		ListenOptions{TargetID: "argos-assistant", Mode: "assistant"},
		"이번 주 Argos 작업 내역을 표로 정리해서 한 페이지 업무 보고 문서로 만들어줘. iPhone에서 바로 열고 수정할 수 있게 보내줘",
	)
	if !handled {
		t.Fatal("expected document artifact fallback")
	}
	visible := signalReplyVisibleText(reply)
	if !strings.Contains(visible, "문서를 작성했습니다.") ||
		!strings.Contains(visible, "Word나 Pages에서 열 수 있는 문서 파일을 준비했습니다.") {
		t.Fatalf("unexpected visible reply:\n%s", visible)
	}
	for _, bad := range []string{"MeshClaw 진행상황 요약", "워크스페이스 저장소", "/.meshclaw/", "evidence", "작업 기록도 저장했습니다"} {
		if strings.Contains(visible, bad) {
			t.Fatalf("work report document reply should not expose %q:\n%s", bad, visible)
		}
	}
	attachments := signalReplyAttachments(reply)
	if len(attachments) == 0 {
		t.Fatalf("expected document attachments")
	}
	var markdown string
	for _, attachment := range attachments {
		if strings.HasSuffix(attachment, ".md") {
			data, err := os.ReadFile(attachment)
			if err != nil {
				t.Fatalf("read markdown attachment: %v", err)
			}
			markdown = string(data)
			break
		}
	}
	if !strings.Contains(markdown, "| 일시 | 영역 | 결과 | 다음 액션 |") ||
		!strings.Contains(markdown, "임원 미팅 발표자료 5장 생성") ||
		!strings.Contains(markdown, "변환 도구 설치 후 PDF 재시도") {
		t.Fatalf("work report markdown should include useful table content:\n%s", markdown)
	}
}

func TestAssistantSpreadsheetRequestUsesSpreadsheetFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	reply, handled := assistantRequiredToolFallbackReply(
		ListenOptions{TargetID: "argos-assistant", Mode: "assistant"},
		"이번 달 서버 비용 트래커를 엑셀로 만들어줘. iPhone에서 수정 가능하게 보내줘",
	)
	if !handled {
		t.Fatal("expected spreadsheet artifact fallback")
	}
	visible := signalReplyVisibleText(reply)
	if !strings.Contains(visible, "표 파일을 작성했습니다.") ||
		!strings.Contains(visible, "XLSX 파일을 준비했습니다.") {
		t.Fatalf("unexpected visible reply:\n%s", visible)
	}
	attachments := signalReplyAttachments(reply)
	hasXLSX, hasCSV := false, false
	for _, attachment := range attachments {
		hasXLSX = hasXLSX || strings.HasSuffix(attachment, ".xlsx")
		hasCSV = hasCSV || strings.HasSuffix(attachment, ".csv")
	}
	if !hasXLSX || !hasCSV {
		t.Fatalf("expected XLSX and CSV attachments: %#v", attachments)
	}
}

func TestMailSearchSpreadsheetMarkdown(t *testing.T) {
	lower := strings.ToLower("메일에서 Google Workspace 찾아서 최근 결과를 엑셀 표로 정리해줘")
	if !looksLikeMailSpreadsheetArtifactRequest(lower) {
		t.Fatal("expected mail spreadsheet request routing")
	}
	if !looksLikeAssistantArtifactRequest(lower) {
		t.Fatal("mail spreadsheet request should block Mac open/action fallback")
	}
	body := mailSearchSpreadsheetMarkdown("Google Workspace", []mailadapter.SearchResult{{
		Account: mailadapter.AccountPublic{ID: "work"},
		Messages: []mailadapter.MessageSummary{{
			ID:      "123",
			From:    "Google Workspace <workspace@example.com>",
			Subject: "Google Workspace billing update",
			Date:    time.Date(2026, 6, 2, 10, 30, 0, 0, time.UTC),
			Snippet: "결제 설정 업데이트 안내입니다.",
		}},
	}}, nil)
	for _, want := range []string{
		"| 번호 | 계정 | 일시 | 발신자 | 제목 | 요약 |",
		"| 1 | work |",
		"Google Workspace billing update",
		"결제 설정 업데이트 안내입니다.",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("mail spreadsheet body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "123") {
		t.Fatalf("mail spreadsheet should not expose internal message IDs:\n%s", body)
	}
}

func TestBrowserSearchSpreadsheetMarkdown(t *testing.T) {
	text := "브라우저로 Argos macOS assistant 검색해서 결과를 엑셀 표로 정리해줘"
	lower := strings.ToLower(text)
	if !looksLikeBrowserSpreadsheetArtifactRequest(lower) {
		t.Fatal("expected browser spreadsheet request routing")
	}
	query, ok := extractBrowserSearchSpreadsheetQuery(text)
	if !ok || query != "Argos macOS assistant" {
		t.Fatalf("unexpected browser search spreadsheet query: ok=%v query=%q", ok, query)
	}
	body := browserSearchSpreadsheetMarkdown(query, browserauto.SearchResult{
		Query: query,
		Results: []browserauto.Link{{
			Text: "Argos macOS Assistant",
			URL:  "https://example.com/argos",
		}},
	})
	for _, want := range []string{
		"| 번호 | 제목 | 출처 | 유형 | 활용 메모 | 링크 | 요약 |",
		"| 1 | Argos macOS Assistant | example.com | 웹 결과 | 관련성 확인 필요 | https://example.com/argos | Argos macOS Assistant |",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("browser spreadsheet body missing %q:\n%s", want, body)
		}
	}
}

func TestBrowserSearchSpreadsheetMarkdownUsesFetchedSummary(t *testing.T) {
	query := "Apple Migration Assistant Mac"
	body := browserSearchSpreadsheetMarkdown(query, browserauto.SearchResult{
		Query: query,
		Results: []browserauto.Link{{
			Text: "Transfer from PC to Mac with Migration Assistant - Apple Support",
			URL:  "https://support.apple.com/en-us/102565",
		}},
	}, []browserauto.Page{{
		URL:        "https://support.apple.com/en-us/102565",
		FinalURL:   "https://support.apple.com/en-us/102565",
		StatusCode: 200,
		Text:       "Transfer from PC to Mac with Migration Assistant - Apple Support Transfer from PC to Mac with Migration Assistant Migration Assistant copies contacts, calendars, email accounts, and files from your Windows PC to the appropriate places on your Mac.",
	}})
	if !strings.Contains(body, "Migration Assistant copies contacts") {
		t.Fatalf("browser spreadsheet should use fetched source summary:\n%s", body)
	}
	if strings.Contains(body, "Apple Support Transfer from PC") {
		t.Fatalf("browser spreadsheet summary should clean repeated page title:\n%s", body)
	}
}

func TestBrowserSearchSpreadsheetMarkdownFallsBackToTitle(t *testing.T) {
	query := "Argos macOS assistant"
	body := browserSearchSpreadsheetMarkdown(query, browserauto.SearchResult{
		Query: query,
		Results: []browserauto.Link{{
			Text: "Argos macOS Assistant",
			URL:  "https://example.com/argos",
		}},
	}, []browserauto.Page{{
		URL:        "https://example.com/argos",
		StatusCode: 404,
		Text:       "not found",
	}})
	if !strings.Contains(body, "| 1 | Argos macOS Assistant | example.com | 웹 결과 | 관련성 확인 필요 | https://example.com/argos | Argos macOS Assistant |") {
		t.Fatalf("browser spreadsheet should fall back to title when no usable source page exists:\n%s", body)
	}
}

func TestBrowserSearchReportRequestRouting(t *testing.T) {
	text := "브라우저로 Argos macOS assistant 검색해서 한 페이지 보고서로 정리해줘. 출처를 붙이고 iPhone에서 바로 열 수 있게 보내줘"
	lower := strings.ToLower(text)
	if !looksLikeBrowserReportArtifactRequest(lower) {
		t.Fatal("expected browser report request routing")
	}
	if looksLikeBrowserSpreadsheetArtifactRequest(lower) {
		t.Fatal("report request should not route to spreadsheet")
	}
	query, ok := extractBrowserSearchReportQuery(text)
	if !ok || query != "Argos macOS assistant" {
		t.Fatalf("unexpected browser report query: ok=%v query=%q", ok, query)
	}
}

func TestBrowserSearchSpreadsheetPreservesLongURLRows(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	query := "Argos macOS assistant"
	body := browserSearchSpreadsheetMarkdown(query, browserauto.SearchResult{
		Query: query,
		Results: []browserauto.Link{
			{Text: "Transfer from PC to Mac with Migration Assistant - Apple Support", URL: "https://support.apple.com/en-us/102565"},
			{Text: "GitHub - danyalsaqib/argos3-macos: A parallel, multi-engine simulator ...", URL: "https://github.com/danyalsaqib/argos3-macos"},
			{Text: "GitHub - gveitch1972/argos: Native macOS menu bar app for Xreal Air 2 ...", URL: "https://github.com/gveitch1972/argos"},
			{Text: "Argos Assistant - Argos Automation", URL: "https://argosautomation.com/argos-assistant/"},
			{Text: "Installing Argos Desktop Client for Mac - Argos support", URL: "https://support.sepialine.com/hc/en-us/articles/360022083033-Installing-Argos-Desktop-Client-for-Mac"},
			{Text: "The ARGoS Website", URL: "https://www.argos-sim.info/"},
			{Text: "Home Assistant Companion Docs", URL: "https://companion.home-assistant.io/"},
			{Text: "Macintosh - Argos support", URL: "https://support.sepialine.com/hc/en-us/articles/232073628-Macintosh"},
		},
	})
	result := osauto.CreateSpreadsheet(context.Background(), "웹 검색 결과 Argos macOS assistant", body)
	if !result.OK {
		t.Fatalf("CreateSpreadsheet failed: %+v", result)
	}
	data, err := os.ReadFile(result.CSV)
	if err != nil {
		t.Fatal(err)
	}
	csv := string(data)
	if got := strings.Count(strings.TrimSpace(csv), "\n") + 1; got != 9 {
		t.Fatalf("expected header plus 8 rows, got %d:\n%s", got, csv)
	}
	if !strings.Contains(csv, "Installing Argos Desktop Client for Mac") {
		t.Fatalf("long URL row was not preserved:\n%s", csv)
	}
	if !strings.Contains(csv, "지원/문서") || !strings.Contains(csv, "코드 저장소") {
		t.Fatalf("browser spreadsheet should classify source types:\n%s", csv)
	}
}

func TestRecentArtifactPathSelectionPrefersObsidianMarkdown(t *testing.T) {
	home := t.TempDir()
	md := filepath.Join(home, "doc.md")
	docx := filepath.Join(home, "doc.docx")
	pptx := filepath.Join(home, "deck.pptx")
	for _, path := range []string{md, docx, pptx} {
		if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	state := assistantArtifactState{
		Document:     osauto.Result{Markdown: md, DOCX: docx},
		Presentation: osauto.Result{PPTX: pptx},
	}
	if got := recentArtifactPathForOpen(state, "document", "Obsidian"); got != md {
		t.Fatalf("obsidian path=%q want %q", got, md)
	}
	if got := recentArtifactPathForOpen(state, "presentation", "PowerPoint"); got != pptx {
		t.Fatalf("ppt path=%q want %q", got, pptx)
	}
}

func TestMarketOutlookHelpers(t *testing.T) {
	t.Setenv("MESHCLAW_LANG", "ko")
	if got := marketAssetFromRequest("유가가 어떻게 될것 같아?"); !strings.Contains(got, "crude oil") {
		t.Fatalf("asset=%q", got)
	}
	if got := marketAssetFromRequest("엔비디아 주가 전망 알려줘"); !strings.Contains(got, "Nvidia") {
		t.Fatalf("stock asset=%q", got)
	}
	query := marketOutlookSearchQuery("crude oil WTI Brent", "이번 주")
	for _, want := range []string{"crude oil", "이번 주", "Reuters", "EIA", "OPEC"} {
		if !strings.Contains(query, want) {
			t.Fatalf("query missing %q: %s", want, query)
		}
	}
	fxQuery := marketOutlookSearchQuery("USD/KRW foreign exchange rate", "1개월")
	for _, want := range []string{"USD/KRW", "central bank", "dollar index"} {
		if !strings.Contains(fxQuery, want) {
			t.Fatalf("fx query missing %q: %s", want, fxQuery)
		}
	}
	if strings.Contains(fxQuery, "OPEC") {
		t.Fatalf("fx query should not use oil source terms: %s", fxQuery)
	}
	lines := formatMarketOutlookToolResult(
		"Nvidia NVDA stock",
		"이번 분기",
		"query",
		browserauto.SearchResult{Results: []browserauto.Link{{Text: "Nvidia earnings outlook", URL: "https://example.com/nvda"}}},
		nil,
	)
	visible := strings.Join(lines, "\n")
	for _, want := range []string{"실적 발표", "매수/매도", "다음 Signal 액션", "https://example.com/nvda"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("market outlook result missing %q:\n%s", want, visible)
		}
	}
}

func TestMarketOutlookCanTargetBriefingRoomDryRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}
	lines := formatMarketOutlookToolResult(
		"Nvidia NVDA stock",
		"이번 분기",
		"query",
		browserauto.SearchResult{Results: []browserauto.Link{{Text: "Nvidia earnings outlook", URL: "https://example.com/nvda"}}},
		nil,
	)
	reply := formatAssistantMarketOutlookSendResult(
		ListenOptions{TargetID: "argos-assistant", Mode: "assistant", Execute: false},
		"보고방",
		"Nvidia NVDA stock",
		lines,
		false,
		false,
		"",
		"",
		evidence.Record{},
		nil,
	)
	for _, want := range []string{
		"시장 전망 브리핑을 Signal로 보낼 준비를 했습니다.",
		"대상: 보고방",
		"보고방은 one-way/no-reply",
		"보낼 내용:",
		"실적 발표",
		"실제 매수/매도",
		"다음 Signal 액션",
		"https://example.com/nvda",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("market outlook send dry-run missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "meshclaw-attachment:") || strings.Contains(reply, "Signal로 보냈습니다.") {
		t.Fatalf("market outlook dry-run should not attach or claim sent:\n%s", reply)
	}
	voiceReply := formatAssistantMarketOutlookSendResult(
		ListenOptions{TargetID: "argos-assistant", Mode: "assistant", Execute: false},
		"보고방",
		"Nvidia NVDA stock",
		lines,
		true,
		true,
		"edge-tts",
		"ko-KR-SunHiNeural",
		evidence.Record{},
		nil,
	)
	for _, want := range []string{
		"시장 전망 브리핑을 Signal로 보낼 준비를 했습니다.",
		"MP3 음성 브리핑도 함께 만들 준비가 됐습니다.",
		"보고방은 one-way/no-reply",
		"실제 매수/매도",
	} {
		if !strings.Contains(voiceReply, want) {
			t.Fatalf("market outlook voice dry-run missing %q:\n%s", want, voiceReply)
		}
	}
	if strings.Contains(voiceReply, "meshclaw-attachment:") || strings.Contains(voiceReply, "Signal로 보냈습니다.") {
		t.Fatalf("market outlook voice dry-run should not attach or claim sent:\n%s", voiceReply)
	}
}
