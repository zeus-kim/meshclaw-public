package messenger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssistantIndustryScenarioReplyCreatesMobileArtifacts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply, handled := assistantIndustryScenarioReply(ListenOptions{TargetID: "argos-assistant-industry", Mode: "assistant"}, "업종별 회사들의 가상 시나이리오를 더 상세하게 만들어줘. 마케팅 매출 분석 그래프, 광고기획, 인사팀 구인구직, 정부 부처 관공서 일상 업무까지 포함해줘.")
	if !handled {
		t.Fatal("industry scenario request was not handled")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"업종별 Argos 비서 활용 시나리오 패키지",
		"구체 시나리오:",
		"마케팅/영업",
		"매출 분석",
		"ROAS 그래프",
		"광고기획",
		"인사팀",
		"구인/구직",
		"정부 부처/관공서",
		"Mermaid 그래프",
		"SVG 실행 차트",
		"Signal에서 바로 해볼 명령",
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
			t.Fatalf("missing %s attachment in %#v", ext, attachments)
		}
	}
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), ".html") {
			t.Fatalf("HTML should not be attached for mobile Signal package: %#v", attachments)
		}
	}

	markdown := firstAttachmentExt(attachments, ".md")
	if markdown == "" {
		t.Fatalf("missing markdown attachment in %#v", attachments)
	}
	data, err := os.ReadFile(markdown)
	if err != nil {
		t.Fatalf("read markdown attachment: %v", err)
	}
	content := string(data)
	for _, want := range []string{"월별 매출", "광고기획", "채용공고", "민원 분류", "정부 부처", "그래프 처리 방식"} {
		if !strings.Contains(content, want) {
			t.Fatalf("markdown content missing %q", want)
		}
	}
	for _, want := range []string{"전략기획", "재무팀", "법무팀", "고객지원", "제품팀", "IT/보안", "구매팀", "연구개발"} {
		if !strings.Contains(content, want) {
			t.Fatalf("expanded scenario catalog missing %q", want)
		}
	}
	if count := strings.Count(content, "### "); count < 100 {
		t.Fatalf("expected rich scenario catalog, got %d scenario headings", count)
	}
	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		lower := strings.ToLower(attachment)
		if strings.HasSuffix(lower, "-industry-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(lower, "-industry-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("industry package should attach Mermaid graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read industry graph markdown: %v", err)
	}
	for _, want := range []string{"```mermaid", "마케팅팀 매출 분석", "광고기획 캠페인", "인사팀 채용", "관공서 민원/정책"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("industry graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read industry SVG chart: %v", err)
	}
	for _, want := range []string{"<svg", "업종별 Argos", "마케팅", "광고", "인사", "공공", "ROAS"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("industry SVG chart missing %q:\n%s", want, string(svgData))
		}
	}
}

func TestAssistantIndustryScenarioUsesEnglishLanguagePackForArtifacts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply, handled := assistantIndustryScenarioReply(ListenOptions{TargetID: "argos-assistant-industry-en", Mode: "assistant"}, "create detailed industry and department scenarios for companies: marketing revenue charts, advertising planning advice, HR recruiting support, and government agency daily work")
	if !handled {
		t.Fatal("industry scenario request was not handled")
	}
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Argos industry assistant scenario package",
		"Concrete scenarios:",
		"Marketing/sales",
		"Advertising",
		"HR",
		"Government/public agencies",
		"Try these in Signal",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("English visible reply missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"업종별", "보고방", "마케팅/영업", "정부 부처"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("English visible reply leaked Korean text %q:\n%s", unwanted, visible)
		}
	}

	attachments := signalReplyAttachments(reply)
	markdown := firstAttachmentExt(attachments, ".md")
	if markdown == "" {
		t.Fatalf("missing markdown attachment in %#v", attachments)
	}
	data, err := os.ReadFile(markdown)
	if err != nil {
		t.Fatalf("read markdown attachment: %v", err)
	}
	content := string(data)
	for _, want := range []string{"Detailed Industry And Department Scenarios", "Finance", "Legal", "Product", "IT Security", "R&D", "Graph Handling Pattern"} {
		if !strings.Contains(content, want) {
			t.Fatalf("English markdown missing %q:\n%s", want, content)
		}
	}
	for _, unwanted := range []string{"업종", "정부 부처", "채용공고", "민원", "사용자 요청 예문"} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("English markdown leaked Korean text %q:\n%s", unwanted, content)
		}
	}
	if count := strings.Count(content, "### "); count < 100 {
		t.Fatalf("expected expanded English scenario catalog, got %d scenario headings", count)
	}

	graphMarkdown := ""
	graphSVG := ""
	for _, attachment := range attachments {
		lower := strings.ToLower(attachment)
		if strings.HasSuffix(lower, "-industry-graphs.md") {
			graphMarkdown = attachment
		}
		if strings.HasSuffix(lower, "-industry-chart.svg") {
			graphSVG = attachment
		}
	}
	if graphMarkdown == "" || graphSVG == "" {
		t.Fatalf("industry package should attach graph markdown and SVG chart: %#v", attachments)
	}
	graphData, err := os.ReadFile(graphMarkdown)
	if err != nil {
		t.Fatalf("read English industry graph markdown: %v", err)
	}
	for _, want := range []string{"Department Output Flow", "Marketing sales analysis", "Finance operations", "Product team", "Numbers For A Meeting"} {
		if !strings.Contains(string(graphData), want) {
			t.Fatalf("English graph markdown missing %q:\n%s", want, string(graphData))
		}
	}
	for _, unwanted := range []string{"부서별", "관공서", "재무", "제품팀"} {
		if strings.Contains(string(graphData), unwanted) {
			t.Fatalf("English graph markdown leaked Korean text %q:\n%s", unwanted, string(graphData))
		}
	}
	svgData, err := os.ReadFile(graphSVG)
	if err != nil {
		t.Fatalf("read English industry SVG chart: %v", err)
	}
	for _, want := range []string{"<svg", "Marketing", "Ads", "Public", "92 pts"} {
		if !strings.Contains(string(svgData), want) {
			t.Fatalf("English industry SVG missing %q:\n%s", want, string(svgData))
		}
	}
	for _, unwanted := range []string{"마케팅", "광고", "인사", "공공", "점"} {
		if strings.Contains(string(svgData), unwanted) {
			t.Fatalf("English SVG leaked Korean text %q:\n%s", unwanted, string(svgData))
		}
	}
}

func TestAssistantIndustryScenarioEnglishCanSendToBriefingWithLocalizedTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, "targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatalf("upsert briefing target: %v", err)
	}

	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-industry-send-en", Mode: "assistant"},
		"Send the marketing/advertising/HR/public-agency virtual company workflow scenario package to the briefing room with DOCX, PPTX, XLSX, SVG charts, and a voice briefing.",
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Prepared the industry scenario package for Signal delivery.",
		"Target: Briefing room",
		"The briefing room is one-way/no-reply",
		"Argos industry assistant scenario package",
		"Concrete scenarios:",
		"Marketing/sales",
		"Advertising",
		"HR",
		"Government/public agencies",
		"Mobile-openable industry Mermaid graph Markdown",
		"Mobile-openable industry SVG execution chart",
		"edge-tts MP3 industry scenario brief",
		"Try these in Signal:",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("English industry send preview missing %q:\n%s", want, visible)
		}
	}
	if containsHangul(visible) {
		t.Fatalf("English industry send preview should not expose Korean text:\n%s", visible)
	}
	for _, unwanted := range []string{"meshclaw-attachment:", "/.meshclaw/evidence/", "/Documents/Argos Vault/"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("visible reply should not expose raw path %q:\n%s", unwanted, visible)
		}
	}

	attachments := signalReplyAttachments(reply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("English industry send preview missing %s attachment: %#v", ext, attachments)
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
			t.Fatalf("read English industry artifact %s: %v", attachment, err)
		}
		if containsHangul(string(data)) {
			t.Fatalf("English industry artifact should not expose Korean text in %s:\n%s", attachment, string(data))
		}
	}
}

func legacyTestAssistantIndustryScenarioExecuteSendsMobileLinks(t *testing.T) {
	home := t.TempDir()
	signalArgs := filepath.Join(home, "signal-args.txt")
	fakeSignal := filepath.Join(home, "signal-cli")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %q\nprintf 'industry-signal-987\\n'\n", signalArgs)
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
		t.Fatalf("upsert briefing target: %v", err)
	}

	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-industry-execute-en", Mode: "assistant", Execute: true},
		"Create detailed industry and department scenarios for companies and send it to the briefing room with marketing revenue charts, advertising planning advice, HR recruiting support, government agency daily work, PPT, CSV, Mermaid graph, SVG chart, and voice briefing.",
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Sent the industry scenario package to Signal.",
		"Target: Briefing room",
		"Signal ID: industry-signal-987",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("execute industry reply missing %q:\n%s", want, visible)
		}
	}
	if containsHangul(visible) {
		t.Fatalf("English execute industry reply should not expose Korean:\n%s", visible)
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

func TestAssistantIndustryScenarioCanSendToBriefing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, "targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatalf("upsert briefing target: %v", err)
	}

	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-industry-send", Mode: "assistant"},
		"업종별 마케팅/광고/인사/관공서 가상회사 업무 시나리오 패키지를 DOCX/PPTX/XLSX/SVG 그래프와 음성 브리핑까지 보고방에 보내줘.",
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"업종별 시나리오 패키지를 Signal로 보낼 준비를 했습니다.",
		"대상: 보고방",
		"보고방은 one-way/no-reply",
		"업종별 Argos 비서 활용 시나리오 패키지",
		"구체 시나리오:",
		"마케팅/영업",
		"정부 부처/관공서",
		"업종별 Mermaid 그래프",
		"업종별 SVG 실행 차트",
		"Signal에서 바로 해볼 명령",
		"edge-tts MP3 업종별 시나리오 브리핑",
		"모바일에서 바로 열기:",
		"DOCX 보고서: https://argos.example.test/argos/",
		"PPTX 발표자료: https://argos.example.test/argos/",
		"XLSX 분석표: https://argos.example.test/argos/",
		"SVG 차트: https://argos.example.test/argos/",
		"MP3 음성 브리핑: https://argos.example.test/argos/",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("industry scenario send preview missing %q:\n%s", want, visible)
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
			t.Fatalf("industry scenario send preview missing %s attachment: %#v", ext, attachments)
		}
	}
	for _, attachment := range attachments {
		if strings.HasSuffix(strings.ToLower(attachment), ".html") {
			t.Fatalf("HTML should not be attached for mobile Signal package: %#v", attachments)
		}
	}
}

func TestAssistantReplyRoutesIndustryScenarioBeforeGenericShowcase(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-industry-route", Mode: "assistant"}, "업종별 회사 시나리오를 만들어줘. 광고기획, 인사팀, 관공서까지 포함해줘")
	visible := signalReplyVisibleText(reply)
	if !strings.Contains(visible, "업종별 Argos 비서 활용 시나리오 패키지") {
		t.Fatalf("industry scenario route did not produce industry package:\n%s", visible)
	}
	if strings.Contains(visible, "Argos 기능 쇼케이스") {
		t.Fatalf("industry scenario should route before generic showcase:\n%s", visible)
	}
}

func hasAttachmentExt(paths []string, ext string) bool {
	return firstAttachmentExt(paths, ext) != ""
}

func firstAttachmentExt(paths []string, ext string) string {
	ext = strings.ToLower(ext)
	for _, path := range paths {
		if strings.HasSuffix(strings.ToLower(path), ext) {
			return path
		}
	}
	return ""
}
