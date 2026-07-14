package messenger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meshclaw/meshclaw/internal/browserauto"
	"github.com/meshclaw/meshclaw/internal/publish"
)

func TestAssistantBrowserResearchPackageCreatesMobileArtifacts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_ASSISTANT_RESEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-research-package", Mode: "assistant"}, "브라우저로 전기차 보조금 검색해서 출처 있는 보고서와 표와 PPT로 정리해줘")
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"전기차 보조금 웹 리서치 패키지",
		"검색 쿼리:",
		"출처 확인 상태: 검색 후보 0개 / 원문 읽음 0개 / 공식 후보 0개 / 읽기 실패 0개",
		"실제 웹 검색이 비활성화되어 있어",
		"검색 실행 상태: 실제 웹 검색 비활성화",
		"핵심 결론",
		"먼저 공식/1차 자료를 확인하고",
		"회의에서 바로 볼 질문:",
		"최근 변화가 가격, 예산, 수요, 규제, 고객 행동 중 어디에 직접 영향을 주는가?",
		"내일 회의 결정 옵션:",
		"선택 B: 현재 확인된 방향성만 반영해 작은 실험이나 제한된 실행을 먼저 시작합니다.",
		"24시간 실행계획:",
		"회의 후 실행 초안:",
		"`담당자/마감 초안 승인`",
		"`후속 리마인더 초안 승인`",
		"실제 메일 발송, 캘린더 등록, 리마인더 생성, 외부 공유는 별도 승인 전 중지합니다.",
		"다음 액션",
		"공식자료, 규제기관, 회사 공시, 보도자료를 우선 확인합니다.",
		"XLSX/CSV",
		"SVG 출처 소스맵",
		"이어서 바로 시킬 수 있는 명령",
		"모바일에서 바로 열기:",
		"DOCX 보고서: https://argos.example.test/argos/",
		"PPTX 발표자료: https://argos.example.test/argos/",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("visible reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"meshclaw-attachment:", "/.meshclaw/evidence/", "/Documents/Argos Vault/"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("visible reply should not expose raw path %q:\n%s", unwanted, visible)
		}
	}

	attachments := signalReplyAttachments(reply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("research package missing %s attachment: %#v", ext, attachments)
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
	content := string(data)
	for _, want := range []string{"# 전기차 보조금 웹 리서치 패키지 보고서", "출처 확인 상태: 검색 후보 0개 / 원문 읽음 0개 / 공식 후보 0개 / 읽기 실패 0개", "검색 실행 상태: 실제 웹 검색 비활성화", "## 바로 사용할 회의 토픽", "## 내일 회의 결정 옵션", "## 24시간 실행계획", "## 회의 후 실행 초안", "`담당자/마감 초안 승인`", "`후속 리마인더 초안 승인`", "실제 메일 발송, 캘린더 등록, 리마인더 생성, 외부 공유는 별도 승인 전 중지합니다.", "## 다음 액션", "전기차 보조금"} {
		if !strings.Contains(content, want) {
			t.Fatalf("markdown content missing %q:\n%s", want, content)
		}
	}
	sourceMap := firstAttachmentExt(attachments, ".svg")
	svgData, err := os.ReadFile(sourceMap)
	if err != nil {
		t.Fatalf("read source map SVG: %v", err)
	}
	for _, want := range []string{"<svg", "전기차 보조금 웹 리서치 패키지", "검색 후보", "원문 읽음", "회의 전 먼저 열 출처"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("source map SVG missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantBrowserResearchPackageDoesNotStealCoupangFlow(t *testing.T) {
	if isAssistantBrowserResearchPackageRequest("쿠팡에서 생수 500ml 20개 로켓배송으로 가격 리뷰 좋은 후보 3개 비교표 만들어줘") {
		t.Fatal("Coupang shopping requests must stay on the shopping/browser-prep flow")
	}
}

func TestAssistantMarketResearchMeetingPackageDoesNotRequireBrowserWord(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_ASSISTANT_RESEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")

	request := "최근 업계 뉴스를 시장조사로 분석해서 회의자료 패키지 만들어줘. Markdown/DOCX/PPTX와 표까지 준비해줘"
	if !isAssistantBrowserResearchPackageRequest(request) {
		t.Fatal("market meeting-material package should route to research package without browser wording")
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-research-package", Mode: "assistant"}, request)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"웹 리서치 패키지",
		"회의에서 바로 볼 질문:",
		"DOCX",
		"PPTX",
		"XLSX/CSV",
		"모바일에서 바로 열기:",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("meeting package reply missing %q:\n%s", want, visible)
		}
	}
	attachments := signalReplyAttachments(reply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("meeting package missing %s attachment: %#v", ext, attachments)
		}
	}
}

func TestAssistantBrowserResearchPackageQueryKeepsRequestLanguage(t *testing.T) {
	ko := assistantBrowserResearchPackageQuery("브라우저로 전기차 보조금 검색해서 출처 있는 보고서와 표로 정리해줘")
	for _, want := range []string{"최신 2026", "공식 출처 데이터"} {
		if !strings.Contains(ko, want) {
			t.Fatalf("Korean query missing %q: %s", want, ko)
		}
	}
	for _, unwanted := range []string{"latest 2026", "official sources data"} {
		if strings.Contains(ko, unwanted) {
			t.Fatalf("Korean query should not include English booster %q: %s", unwanted, ko)
		}
	}

	en := assistantBrowserResearchPackageQuery("web search EV subsidy report table")
	for _, want := range []string{"latest 2026", "official sources data"} {
		if !strings.Contains(en, want) {
			t.Fatalf("English query missing %q: %s", want, en)
		}
	}
	if containsHangul(en) {
		t.Fatalf("English query should not include Korean boosters: %s", en)
	}
}

func TestAssistantBrowserResearchPackageQueryCleansKoreanParticles(t *testing.T) {
	query := assistantBrowserResearchPackageQuery("브라우저로 최근 화장품 브랜드 숏폼 광고 성과와 소비자 반응을 검색해서 출처 있는 보고서와 표와 PPT로 정리해서 비서방에 보내줘.")
	for _, unwanted := range []string{"성과와", "반응을", "비서방"} {
		if strings.Contains(query, unwanted) {
			t.Fatalf("Korean query should remove particles/delivery target %q: %s", unwanted, query)
		}
	}
	for _, want := range []string{"성과", "소비자", "반응", "공식 출처 데이터"} {
		if !strings.Contains(query, want) {
			t.Fatalf("Korean query missing cleaned term %q: %s", want, query)
		}
	}
}

func TestAssistantBrowserResearchVisibleSourceNotesAreLimited(t *testing.T) {
	report := publish.ResearchReport{SourcePages: []browserauto.Page{
		{Title: "Official notice", Text: "첫 번째 원문은 규제 변화와 비용 영향, 예산 확인 항목을 설명합니다."},
		{Title: "Company update", Text: "두 번째 원문은 경쟁사 대응과 고객 반응을 설명합니다."},
		{Title: "Market article", Text: "세 번째 원문은 배경 설명으로만 사용합니다."},
	}}
	lines := assistantBrowserResearchVisibleSourceNoteLines(report, 2)
	if len(lines) != 2 {
		t.Fatalf("expected two visible source notes, got %d: %#v", len(lines), lines)
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"Official notice", "첫 번째 원문", "Company update", "두 번째 원문"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("visible source notes missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "Market article") {
		t.Fatalf("visible source notes should be limited to two lines:\n%s", joined)
	}
	for _, line := range lines {
		if len([]rune(line)) > 180 {
			t.Fatalf("visible source note should stay compact on mobile (%d runes): %s", len([]rune(line)), line)
		}
	}
}

func TestAssistantBrowserResearchVisibleSourceNotesFilterIrrelevantExcerpts(t *testing.T) {
	report := publish.ResearchReport{
		Query: "전기차 배터리 원자재 가격 보조금 정책 변화 공식 출처 데이터",
		SourcePages: []browserauto.Page{
			{Title: "전기차 보조금 최신 기준", Text: "기초연금은 어르신들의 안정적인 삶을 돕기 위해 마련된 사회보장 제도입니다."},
			{Title: "전기차 보조금 정책 변화", URL: "https://example.gov/ev", Text: "전기차 구매 보조금 정책은 배터리 성능 기준과 가격 구간에 따라 지원 금액이 달라집니다."},
		},
	}
	lines := assistantBrowserResearchVisibleSourceNoteLines(report, 2)
	joined := strings.Join(lines, "\n")
	if len(lines) != 1 {
		t.Fatalf("expected only one relevant source note, got %d:\n%s", len(lines), joined)
	}
	if strings.Contains(joined, "기초연금") {
		t.Fatalf("irrelevant source excerpt should not be shown:\n%s", joined)
	}
	if !strings.Contains(joined, "배터리 성능") {
		t.Fatalf("relevant source excerpt missing:\n%s", joined)
	}
	if !strings.Contains(joined, "공식 문서") {
		t.Fatalf("visible source note should show source type:\n%s", joined)
	}
}

func TestAssistantBrowserResearchVisibleSourceTypeUsesURLForOfficial(t *testing.T) {
	official := assistantBrowserResearchVisibleSourceType("정부 정책 변화", "https://example.gov/notice")
	if official != "공식 문서" {
		t.Fatalf("expected official source type for gov URL, got %q", official)
	}
	generic := assistantBrowserResearchVisibleSourceType("정부 정책 변화", "https://example.com/post")
	if generic == "공식 문서" {
		t.Fatalf("generic URL should not become official only because title says government: %q", generic)
	}
}

func TestAssistantBrowserResearchPackageCanAttachVoiceBrief(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_ASSISTANT_RESEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-research-voice", Mode: "assistant"}, "브라우저로 전기차 보조금 검색해서 출처 있는 보고서와 표를 만들고 뉴스 방송 원고와 edge tts 음성으로 보내줘")
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"전기차 보조금 웹 리서치 패키지",
		"edge-tts MP3 음성파일",
		"음성 원고 미리보기",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("visible voice reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"edge tts latest", "뉴스 방송 latest", "고 원고", "원고와 latest", " -.\n"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("voice instructions should not leak into search query or script (%q):\n%s", unwanted, visible)
		}
	}
	if strings.Contains(assistantBrowserResearchPackageQuery("브라우저로 전기차 보조금 검색해서 출처 있는 보고서와 표를 만들고 뉴스 방송 원고와 edge tts 음성으로 보내줘"), "고 원고") {
		t.Fatalf("voice instructions should not leak into search query:\n%s", visible)
	}
	attachments := signalReplyAttachments(reply)
	if !hasAttachmentExt(attachments, ".mp3") {
		t.Fatalf("voice research package missing mp3 attachment: %#v", attachments)
	}
	for _, ext := range []string{".docx", ".md", ".xlsx", ".csv", ".svg"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("voice research package missing %s attachment: %#v", ext, attachments)
		}
	}
}

func TestAssistantBrowserResearchPackageCanSendToBriefing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_ASSISTANT_RESEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", home+"/targets.json")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatalf("upsert briefing target: %v", err)
	}

	request := "브라우저로 최근 화학회사들이 민감하게 반응할 뉴스를 검색해서 출처 있는 보고서와 표와 PPT로 정리해서 보고방에 보내줘. 내일 회의 토픽과 음성 브리핑도 준비해줘."
	query := assistantBrowserResearchPackageQuery(request)
	for _, unwanted := range []string{"보고방", "비서방", "회의", "브리핑", "준비", "official sources data"} {
		if strings.Contains(query, unwanted) {
			t.Fatalf("delivery instructions should not leak into research query %q: %s", unwanted, query)
		}
	}
	if !strings.Contains(query, "공식 출처 데이터") {
		t.Fatalf("Korean research query should use Korean source booster: %s", query)
	}

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-research-send", Mode: "assistant"}, request)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"웹 리서치 패키지를 Signal로 보낼 준비를 했습니다.",
		"대상: 보고방",
		"보고방은 one-way/no-reply",
		"화학 산업 웹 리서치 패키지",
		"출처 확인 상태: 검색 후보 0개 / 원문 읽음 0개 / 공식 후보 0개 / 읽기 실패 0개",
		"핵심 결론",
		"회의에서 바로 볼 질문:",
		"PFAS/REACH/환경규제 변화",
		"나프타·원유·천연가스",
		"내일 회의 결정 옵션:",
		"원료 가격 민감 품목의 견적 유효기간을 줄이고",
		"24시간 실행계획:",
		"회의 후 실행 초안:",
		"`담당자/마감 초안 승인`",
		"`후속 리마인더 초안 승인`",
		"다음 액션",
		"SVG 출처 소스맵",
		"edge-tts MP3 음성파일",
		"모바일에서 바로 열기:",
		"DOCX 보고서: https://argos.example.test/argos/",
		"MP3 음성 브리핑: https://argos.example.test/argos/",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("research send preview missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"meshclaw-attachment:", "/.meshclaw/evidence/", "/Documents/Argos Vault/"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("visible reply should not expose raw path %q:\n%s", unwanted, visible)
		}
	}
	attachments := signalReplyAttachments(reply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("research send preview missing %s attachment: %#v", ext, attachments)
		}
	}
}

func TestAssistantBrowserResearchPackageQueryRemovesAssistantRoomTarget(t *testing.T) {
	query := assistantBrowserResearchPackageQuery("브라우저로 전기차 보조금 검색해서 출처 있는 보고서와 표와 PPT로 정리해서 비서방에 보내줘.")
	for _, unwanted := range []string{"비서방", "보내줘"} {
		if strings.Contains(query, unwanted) {
			t.Fatalf("assistant-room delivery target leaked into query %q: %s", unwanted, query)
		}
	}
	if !strings.Contains(query, "공식 출처 데이터") {
		t.Fatalf("Korean source booster missing after delivery cleanup: %s", query)
	}
}

func TestAssistantBrowserResearchPackageEnglishCanSendToBriefing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_ASSISTANT_RESEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", home+"/targets.json")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatalf("upsert briefing target: %v", err)
	}

	request := "web search chemical industry PFAS regulation feedstock price competitor response sources report table PPT voice briefing send to briefing room"
	query := assistantBrowserResearchPackageQuery(request)
	for _, unwanted := range []string{"web search", "report", "table", "ppt", "voice", "briefing room", "send"} {
		if strings.Contains(strings.ToLower(query), unwanted) {
			t.Fatalf("English delivery/artifact instruction leaked into research query %q: %s", unwanted, query)
		}
	}

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-research-send-en", Mode: "assistant"}, request)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Prepared the web research package for Signal delivery.",
		"Target: Briefing room",
		"The briefing room is one-way/no-reply",
		"Chemical industry web research package",
		"Search query:",
		"Source check: 0 search candidates / 0 pages read / 0 official candidates / 0 read failures",
		"Key Conclusions",
		"Meeting questions to use now:",
		"PFAS, REACH, or environmental regulation changes",
		"Decision options for tomorrow's meeting:",
		"feedstock-sensitive products",
		"24-hour execution plan:",
		"Post-meeting execution drafts:",
		"`owner/deadline draft approved`",
		"`follow-up reminder draft approved`",
		"Actual email sending, calendar entry creation, reminder creation, and external sharing stay stopped until separate approval.",
		"Next Actions",
		"Mobile-openable SVG source map",
		"edge-tts MP3 audio with a broadcast-style script",
		"Useful follow-up commands:",
		"Open on mobile:",
		"DOCX report: https://argos.example.test/argos/",
		"MP3 voice brief: https://argos.example.test/argos/",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("English research send preview missing %q:\n%s", want, visible)
		}
	}
	if containsHangul(visible) {
		t.Fatalf("English research send preview should not expose Korean text:\n%s", visible)
	}
	for _, unwanted := range []string{"meshclaw-attachment:", "/.meshclaw/evidence/", "/Documents/Argos Vault/"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("visible reply should not expose raw path %q:\n%s", unwanted, visible)
		}
	}

	attachments := signalReplyAttachments(reply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("English research send preview missing %s attachment: %#v", ext, attachments)
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
			t.Fatalf("read English research artifact %s: %v", attachment, err)
		}
		if containsHangul(string(data)) {
			t.Fatalf("English research artifact should not expose Korean text in %s:\n%s", attachment, string(data))
		}
	}
}

func TestAssistantBrowserResearchPackageExecuteSendsMobileLinks(t *testing.T) {
	home := t.TempDir()
	signalArgs := filepath.Join(home, "signal-args.txt")
	fakeSignal := filepath.Join(home, "signal-cli")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %q\nprintf 'research-signal-321\\n'\n", signalArgs)
	if err := os.WriteFile(fakeSignal, []byte(script), 0700); err != nil {
		t.Fatalf("write fake signal-cli: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_ASSISTANT_RESEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_SIGNAL_CLI", fakeSignal)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, "targets.json"))
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_HOST", "argos.example.test")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatalf("upsert briefing target: %v", err)
	}

	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-research-execute-en", Mode: "assistant", Execute: true},
		"web search chemical industry PFAS regulation feedstock price competitor response sources report table PPT voice briefing send to briefing room",
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Sent the web research package to Signal.",
		"Target: Briefing room",
		"Signal ID: research-signal-321",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("execute research reply missing %q:\n%s", want, visible)
		}
	}
	if containsHangul(visible) {
		t.Fatalf("English execute research reply should not expose Korean:\n%s", visible)
	}

	argsData, err := os.ReadFile(signalArgs)
	if err != nil {
		t.Fatalf("read fake signal args: %v", err)
	}
	args := string(argsData)
	for _, want := range []string{
		"Open on mobile:",
		"DOCX report: https://argos.example.test/argos/",
		"PPTX deck: https://argos.example.test/argos/",
		"XLSX table: https://argos.example.test/argos/",
		"SVG chart: https://argos.example.test/argos/",
		"MP3 voice brief: https://argos.example.test/argos/",
		"--attachment",
		".docx",
		".pptx",
		".xlsx",
		".svg",
		".mp3",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("fake signal args missing %q:\n%s", want, args)
		}
	}
	if strings.Contains(args, ".html") {
		t.Fatalf("mobile Signal package should not attach HTML:\n%s", args)
	}
}

func fakeEdgeTTS(t *testing.T) string {
	t.Helper()
	path := t.TempDir() + "/edge-tts"
	script := `#!/bin/sh
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--write-media" ]; then
    shift
    out="$1"
  fi
  shift
done
if [ -z "$out" ]; then
  exit 2
fi
mkdir -p "$(dirname "$out")"
printf 'fake mp3' > "$out"
`
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatalf("write fake edge-tts: %v", err)
	}
	return path
}
