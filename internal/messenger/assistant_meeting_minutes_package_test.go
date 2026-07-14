package messenger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssistantMeetingMinutesPackageTargetsBriefingWithArtifacts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", home+"/targets.json")
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-meeting-send", Mode: "assistant"}, "오늘 회의록을 작성해서 보고방에 보내줘. 논의: 사용자는 내부 점검보다 실제 결과물을 원한다. 결정: 회의록과 시장조사부터 강화한다. 할 일: 민수는 시장조사 보고서 초안 작성, 마감: 금요일. 지연은 보고방 결과물 샘플 확인, 마감: 내일. 리스크: 증거 경로만 보내면 아무도 보지 않는다. 음성으로도 준비해줘.")
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"회의록 패키지를 Signal로 보낼 준비를 했습니다.",
		"문서/보고서를 Signal로 보낼 준비를 했습니다.",
		"대상: 보고방",
		"보고방은 one-way/no-reply",
		"Argos 회의록 문서/보고서입니다.",
		"Signal에서 바로 읽기:",
		"결정사항:",
		"회의록과 시장조사부터 강화한다",
		"담당/마감 액션아이템:",
		"민수",
		"금요일",
		"리스크:",
		"증거 경로만 보내면 아무도 보지 않는다",
		"다음 Signal 액션:",
		"담당자 확인 요청:",
		"메일 후속 초안 요청:",
		"리마인더 초안 요청:",
		"캘린더 초안 요청:",
		"아직 보내지 마",
		"아직 생성하지 마",
		"아직 등록하지 마",
		"보고방 업데이트 요청:",
		"실행 중지선: 실제 메일 발송, 캘린더 등록, 리마인더 생성은 별도 승인 전 중지",
		"Mermaid 액션 흐름",
		"SVG 담당/마감 액션 보드",
		"edge-tts MP3 회의록 브리핑",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("meeting minutes package reply missing %q:\n%s", want, visible)
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
			t.Fatalf("meeting minutes package missing %s attachment: %#v", ext, attachments)
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
	for _, want := range []string{"## 결정사항", "## 할 일", "## 담당/마감 표", "## 다음 Signal 액션:", "담당자 확인 요청:", "메일 후속 초안 요청:", "리마인더 초안 요청:", "캘린더 초안 요청:", "아직 보내지 마", "아직 생성하지 마", "아직 등록하지 마", "보고방 업데이트 요청:", "실행 중지선: 실제 메일 발송, 캘린더 등록, 리마인더 생성은 별도 승인 전 중지", "## 리스크", "## 다음 회의 안건", "회의록과 시장조사부터 강화한다", "민수", "금요일"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("meeting markdown missing %q:\n%s", want, string(data))
		}
	}
	csv := firstAttachmentExt(attachments, ".csv")
	csvData, err := os.ReadFile(csv)
	if err != nil {
		t.Fatalf("read csv attachment: %v", err)
	}
	for _, want := range []string{"시장조사 보고서 초안 작성", "민수", "금요일", "보고방 결과물 샘플 확인", "지연", "내일"} {
		if !strings.Contains(string(csvData), want) {
			t.Fatalf("meeting csv missing %q:\n%s", want, string(csvData))
		}
	}
	graphMarkdown := ""
	actionBoard := ""
	for _, attachment := range attachments {
		lower := strings.ToLower(attachment)
		if strings.HasSuffix(lower, "-action-flow.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(lower, "-action-board.svg") {
			actionBoard = attachment
		}
	}
	if graphMarkdown == "" || actionBoard == "" {
		t.Fatalf("meeting package should attach action flow markdown and SVG board: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read action flow markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "회의 요약", "액션아이템", "시장조사 보고서 초안 작성", "보고방 진행 업데이트"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("action flow markdown missing %q:\n%s", want, string(graphData))
		}
	}
	boardData, err := os.ReadFile(actionBoard)
	if err != nil {
		t.Fatalf("read action board SVG: %v", err)
	}
	for _, want := range []string{"<svg", "Argos 회의록 패키지", "할 일", "담당", "마감", "민수", "금요일", "대기"} {
		if !strings.Contains(string(boardData), want) {
			t.Fatalf("action board SVG missing %q:\n%s", want, string(boardData))
		}
	}
}

func TestAssistantMeetingMinutesPackageEnglishUsesLanguagePack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", home+"/targets.json")
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-meeting-send-en", Mode: "assistant"},
		"write meeting minutes and send them to the briefing room. notes: decision: ship user-visible outputs first. action: owner: Mina, draft the customer-ready report, due: Friday. risk: status-only reports do not create value. voice briefing",
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Prepared the meeting-minutes package for Signal delivery.",
		"Prepared the document/report for Signal delivery.",
		"Target: Briefing room",
		"Read directly in Signal:",
		"Decisions:",
		"ship user-visible outputs first",
		"Action items with owners/deadlines:",
		"Mina",
		"Friday",
		"Risks:",
		"status-only reports do not create value",
		"Next Signal actions:",
		"Owner follow-up:",
		"Mail follow-up draft request:",
		"Reminder draft request:",
		"Calendar draft request:",
		"do not send it yet",
		"do not create it yet",
		"Briefing update request:",
		"Execution stop line: actual email sending, calendar creation, and reminder creation stay stopped until separate approval.",
		"Mobile-openable Mermaid action flow",
		"Mobile-openable SVG owner/deadline action board",
		"edge-tts MP3 meeting-minutes brief",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("English meeting minutes package reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"Target: 보고방", "대상:", "보고방은", "문서/보고서", "회의록 문서", "Signal에서 바로 읽기", "담당/마감", "아르고스 회의록"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("English visible reply should not expose Korean text %q:\n%s", unwanted, visible)
		}
	}

	attachments := signalReplyAttachments(reply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("English meeting minutes package missing %s attachment: %#v", ext, attachments)
		}
	}
	reportMarkdown := ""
	report := ""
	graphMarkdown := ""
	actionBoard := ""
	for _, attachment := range attachments {
		lower := strings.ToLower(attachment)
		if strings.HasSuffix(lower, ".md") && !strings.HasSuffix(lower, "-action-flow.md") {
			data, err := os.ReadFile(attachment)
			if err != nil {
				t.Fatalf("read English markdown candidate: %v", err)
			}
			if strings.Contains(string(data), "## Owner and Deadline Table") {
				reportMarkdown = attachment
				report = string(data)
			}
		}
		if strings.HasSuffix(lower, "-action-flow.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(lower, "-action-board.svg") {
			actionBoard = attachment
		}
	}
	if reportMarkdown == "" || graphMarkdown == "" || actionBoard == "" {
		t.Fatalf("English meeting package should attach action flow markdown and SVG board: %#v", attachments)
	}
	for _, want := range []string{"## Decisions", "## Action Items", "## Owner and Deadline Table", "## Next Signal actions:", "Owner follow-up:", "Mail follow-up draft request:", "Reminder draft request:", "Calendar draft request:", "do not send it yet", "do not create it yet", "draft calendar events only", "Briefing update request:", "Execution stop line: actual email sending, calendar creation, and reminder creation stay stopped until separate approval.", "## Risks", "## Next Meeting Agenda", "ship user-visible outputs first", "Mina", "Friday"} {
		if !strings.Contains(report, want) {
			t.Fatalf("English report markdown missing %q:\n%s", want, report)
		}
	}
	if containsHangul(report) {
		t.Fatalf("English report markdown should not expose Korean text:\n%s", report)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read English action flow markdown: %v", err)
	}
	graph := string(graphData)
	for _, want := range []string{"action flow", "Meeting summary", "Action items", "Briefing-room progress update", "Owner: Mina", "Due: Friday"} {
		if !strings.Contains(graph, want) {
			t.Fatalf("English action flow markdown missing %q:\n%s", want, graph)
		}
	}
	if containsHangul(graph) {
		t.Fatalf("English action flow markdown should not expose Korean text:\n%s", graph)
	}
	boardData, err := os.ReadFile(actionBoard)
	if err != nil {
		t.Fatalf("read English action board SVG: %v", err)
	}
	board := string(boardData)
	for _, want := range []string{"<svg", "Argos meeting minutes package", "Task", "Owner", "Due", "Pending", "Mina", "Friday"} {
		if !strings.Contains(board, want) {
			t.Fatalf("English action board SVG missing %q:\n%s", want, board)
		}
	}
	if containsHangul(board) {
		t.Fatalf("English action board SVG should not expose Korean text:\n%s", board)
	}
}

func TestAssistantMeetingMinutesPackageExecuteSendsToBriefingWithArtifacts(t *testing.T) {
	home := t.TempDir()
	signalArgs := filepath.Join(home, "signal-args.txt")
	fakeSignal := filepath.Join(home, "signal-cli")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %q\nprintf 'signal-test-123\\n'\n", signalArgs)
	if err := os.WriteFile(fakeSignal, []byte(script), 0700); err != nil {
		t.Fatalf("write fake signal-cli: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_SIGNAL_CLI", fakeSignal)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", home+"/targets.json")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-meeting-execute-en", Mode: "assistant", Execute: true},
		"write meeting minutes and send them to the briefing room. notes: decision: turn the marketing meeting into a concrete execution pack. action: owner: Mina, draft campaign budget changes, due: Friday. action: owner: Alex, prepare the client-facing follow-up, due: tomorrow. risk: teams ignore status-only updates. voice briefing",
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Sent the meeting-minutes package to Signal.",
		"Target: Briefing room",
		"Signal ID: signal-test-123",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("execute reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"대상:", "Signal에서 바로 읽기", "meshclaw-attachment:", "/Documents/Argos Vault/"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("execute reply should not expose %q:\n%s", unwanted, visible)
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
		t.Fatalf("mobile Signal package should not attach HTML:\n%s", args)
	}
}

func TestAssistantReplyMeetingMinutesWinsOverShoppingActionItem(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, "targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-meeting-shopping-regression", Mode: "assistant"},
		"오늘 제품/마케팅 회의 메모입니다. 논의: 사용자는 내부 상태보다 실제 결과물을 보고 싶어한다. 결정: 다음 데모는 회의록, 시장조사, 쇼핑 준비처럼 바로 열리는 산출물을 우선 보여준다. 할 일: 민수는 화학회사 시장조사 보고서 초안을 작성, 마감: 금요일. 지연은 쿠팡 실구매 테스트 체크리스트를 정리, 마감: 내일. 리스크: 상태 보고만 보내면 사용자가 체감하지 못한다. 이걸 회의록, 액션아이템 표, PPT, Mermaid 흐름도, SVG 액션보드, edge tts 음성 브리핑으로 만들어서 보고방에 보내줘.",
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"회의록 패키지를 Signal로 보낼 준비를 했습니다.",
		"결정사항:",
		"다음 데모는 회의록, 시장조사, 쇼핑 준비처럼 바로 열리는 산출물을 우선 보여준다",
		"민수",
		"지연",
		"쿠팡 실구매 테스트 체크리스트",
		"리스크:",
		"상태 보고만 보내면 사용자가 체감하지 못한다",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("meeting minutes should win over shopping action item, missing %q:\n%s", want, visible)
		}
	}
	if strings.Contains(visible, "쿠팡 구매 전 준비 카드") {
		t.Fatalf("meeting minutes request was stolen by Coupang checkout prep:\n%s", visible)
	}
}

func TestAssistantReplyMarketingMeetingMemoWinsOverDailyAgenda(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_NO_FETCH", "1")
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, "targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-meeting-daily-regression", Mode: "assistant"},
		"오늘 마케팅 회의 메모입니다. 결정: 다음 주 광고 캠페인은 화학회사 리서치 보고서의 규제 리스크 토픽을 중심으로 B2B 의사결정자용 메시지를 테스트한다. 할 일: 민수는 규제 리스크별 광고 문구 5개 작성, 마감: 내일. 지연은 경쟁사 랜딩페이지 3개 비교표 작성, 마감: 화요일. Alex는 ROAS 가설과 예산 배분안을 정리, 마감: 수요일. 리스크: 실무자는 상태 보고가 아니라 바로 열 수 있는 자료를 원한다. 이걸 회의록, 액션아이템 표, 발표자료, 음성 브리핑으로 만들고 보고방에 보내줘.",
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"회의록 패키지를 Signal로 보낼 준비를 했습니다.",
		"다음 주 광고 캠페인은 화학회사 리서치 보고서의 규제 리스크 토픽",
		"민수",
		"지연",
		"Alex",
		"ROAS 가설과 예산 배분안",
		"실무자는 상태 보고가 아니라 바로 열 수 있는 자료를 원한다",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("marketing meeting memo should route to meeting minutes, missing %q:\n%s", want, visible)
		}
	}
	if strings.Contains(visible, "하루 실행계획 패키지") {
		t.Fatalf("meeting memo was hijacked by daily agenda package:\n%s", visible)
	}
}

func TestAssistantMeetingMinutesActionItemsExtractOwnerAndDue(t *testing.T) {
	items := assistantMeetingMinutesActionItems("할 일: 민수는 시장조사 보고서 초안 작성, 마감: 금요일. 담당: 지연, 보고방 결과물 샘플 확인, 마감: 내일.")
	if len(items) != 2 {
		t.Fatalf("items=%#v", items)
	}
	if items[0].Owner != "민수" || items[0].Due != "금요일" || !strings.Contains(items[0].Task, "시장조사 보고서 초안 작성") {
		t.Fatalf("first item=%#v", items[0])
	}
	if items[1].Owner != "지연" || items[1].Due != "내일" || !strings.Contains(items[1].Task, "보고방 결과물 샘플 확인") {
		t.Fatalf("second item=%#v", items[1])
	}
}

func TestAssistantMeetingMinutesPackageRouting(t *testing.T) {
	if !isAssistantMeetingMinutesPackageRequest("오늘 회의록을 작성해서 보고방에 보내줘. 결정: 결과물 중심으로 보낸다.") {
		t.Fatal("expected targeted meeting minutes request to route to package")
	}
	if !isAssistantMeetingMinutesPackageRequest("회의록을 PPT와 액션아이템 표와 음성으로 만들어줘. 결정: 결과물 중심.") {
		t.Fatal("expected artifact-rich meeting minutes request to route to package")
	}
	if !isAssistantMeetingMinutesPackageRequest("오늘 제품/마케팅 회의 메모입니다. 결정: 내일 쿠팡 실구매 테스트는 결제 전 확인 단계까지 진행합니다. 할 일: 김팀장은 예산안을 작성합니다. 이걸 회의록, 액션아이템 표, PPT, Mermaid 흐름도, SVG 액션보드, edge tts 음성 브리핑으로 만들어서 보고방에 보내줘.") {
		t.Fatal("meeting-minutes package should own meeting requests even when Coupang appears as one action item")
	}
	if isAssistantMeetingMinutesPackageRequest("이 회의 메모를 결정사항, 할 일, 리스크가 보이는 회의록으로 만들어줘") {
		t.Fatal("plain meeting minutes request should stay on department workflow")
	}
	if isAssistantMeetingMinutesPackageRequest("내일 제품회의용 회의자료 패키지를 만들어서 보고방에 보내줘. 5장 PPT와 요약 문서로 준비해줘.") {
		t.Fatal("meeting materials package should not be misrouted into meeting minutes package")
	}
}
