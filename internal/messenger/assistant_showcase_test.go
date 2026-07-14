package messenger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAssistantShowcaseReportEmphasizesVisibleWork(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	report, err := BuildAssistantShowcaseReport(AssistantShowcaseOptions{
		Now:            time.Date(2026, 6, 13, 7, 30, 0, 0, time.UTC),
		IncludePlanned: true,
		WriteMarkdown:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Argos 기능 쇼케이스", "회의록", "시장조사", "최근 메일", "edge-tts", "구체 업무 시나리오: 300개", "Signal에서 바로 보일 결과 예시", "화학회사 민감 뉴스 분석", "일주일 여행계획", "더 짧게/더 정중하게", "전체 예제 카탈로그"} {
		if !strings.Contains(report.Text, want) {
			t.Fatalf("showcase text missing %q:\n%s", want, report.Text)
		}
	}
	if report.Counts["total"] < 80 || report.Counts["implemented"] == 0 {
		t.Fatalf("unexpected counts: %#v", report.Counts)
	}
	if report.MarkdownPath == "" {
		t.Fatal("markdown path is empty")
	}
	if len(report.Attachments) == 0 || report.Attachments[0] != report.MarkdownPath {
		t.Fatalf("attachments should include markdown first: %#v", report.Attachments)
	}
	data, err := os.ReadFile(report.MarkdownPath)
	if err != nil {
		t.Fatal(err)
	}
	markdown := string(data)
	for _, want := range []string{"## 메일", "## 뉴스/시장조사", "## 문서/파일", "## 음성/TTS", "## 운영/보안"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("showcase markdown missing %q:\n%s", want, markdown)
		}
	}
	if count := strings.Count(markdown, "### "); count < 300 {
		t.Fatalf("showcase should include 300+ concrete scenarios, got %d", count)
	}
	for _, want := range []string{"요청:", "Argos 결과:", "Signal에서 보이는 형태:", "이어서 할 말:"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("showcase markdown missing concrete scenario field %q", want)
		}
	}
}

func TestAssistantShowcaseUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	report, err := BuildAssistantShowcaseReport(AssistantShowcaseOptions{
		Now:            time.Date(2026, 6, 13, 7, 30, 0, 0, time.UTC),
		IncludePlanned: true,
		WriteMarkdown:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Argos capability showcase.", "user-visible work products", "Concrete work scenarios: 300", "Representative work you can feel immediately", "Chemical-company sensitive news", "The full example catalog is attached as Markdown."} {
		if !strings.Contains(report.Text, want) {
			t.Fatalf("English showcase text missing %q:\n%s", want, report.Text)
		}
	}
	for _, unwanted := range []string{"Argos 기능 쇼케이스입니다.", "내부 점검이 아니라", "화학회사 민감 뉴스 분석"} {
		if strings.Contains(report.Text, unwanted) {
			t.Fatalf("English showcase text still contains Korean shell text %q:\n%s", unwanted, report.Text)
		}
	}
	data, err := os.ReadFile(report.MarkdownPath)
	if err != nil {
		t.Fatal(err)
	}
	markdown := string(data)
	for _, want := range []string{"# Argos capability showcase", "## How to use", "## News / market research", "## Ops / security"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("English showcase markdown missing %q:\n%s", want, markdown)
		}
	}
}

func TestAssistantShowcaseAttachmentsSkipRawHTML(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		filepath.Join(dir, "catalog.md"),
		filepath.Join(dir, "preview.html"),
		filepath.Join(dir, "preview.html.png"),
		filepath.Join(dir, "brief.docx"),
	}
	for _, path := range files {
		if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got := uniqueShowcaseAttachments(files)
	if len(got) != 3 {
		t.Fatalf("attachments=%#v", got)
	}
	for _, path := range got {
		if strings.HasSuffix(strings.ToLower(path), ".html") {
			t.Fatalf("raw HTML should not be attached: %#v", got)
		}
	}
	if got[0] != files[0] || got[1] != files[2] || got[2] != files[3] {
		t.Fatalf("unexpected attachment order: %#v", got)
	}
}

func TestAssistantShowcaseSignalRequestCreatesCatalogAttachment(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_ASSISTANT_SHOWCASE_SAMPLES", "0")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")
	if !isAssistantShowcaseRequest("아르고스가 할 수 있는 모든 일을 300개 구체 시나리오로 보여줘") {
		t.Fatal("showcase request was not detected")
	}
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-showcase", Mode: "assistant"}, "아르고스가 할 수 있는 모든 일을 300개 구체 시나리오로 보여줘")
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{"Argos 기능 쇼케이스", "구체 업무 시나리오: 300개", "Signal에서 바로 보일 결과 예시", "화학회사 민감 뉴스 분석", "전체 예제 카탈로그", "모바일에서 바로 열기:", "Markdown 원문: https://argos.example.test/argos/"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("showcase visible reply missing %q:\n%s", want, visible)
		}
	}
	attachments := signalReplyAttachments(reply)
	if len(attachments) == 0 {
		t.Fatalf("showcase reply should attach Markdown catalog; raw reply=%q", reply)
	}
	if !strings.HasSuffix(attachments[0], ".md") {
		t.Fatalf("first showcase attachment should be Markdown, got %#v", attachments)
	}
}

func TestAssistantShowcaseBriefingDryRunPreview(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_ASSISTANT_SHOWCASE_SAMPLES", "0")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-showcase-briefing", Mode: "assistant"}, "기능 쇼케이스를 보고방에 보내줘")
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{"보고방에 보낼 Argos 기능 쇼케이스 미리보기", "실제 Signal listener 실행 모드에서는 보고방으로 바로 전송", "Argos 기능 쇼케이스"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("showcase briefing preview missing %q:\n%s", want, visible)
		}
	}
}

func TestAssistantShowcaseBriefingPreviewUsesLanguagePack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_ASSISTANT_SHOWCASE_SAMPLES", "0")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-showcase-briefing-en", Mode: "assistant"}, "send the scenario catalog to briefing")
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{"Preview of the Argos capability showcase for the briefing room.", "In live Signal listener mode", "Argos capability showcase.", "Open on mobile:", "Markdown source: https://argos.example.test/argos/"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("English showcase briefing preview missing %q:\n%s", want, visible)
		}
	}
	for _, unwanted := range []string{"보고방에 보낼", "실제 Signal listener 실행 모드"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("English showcase briefing preview still contains Korean wrapper %q:\n%s", unwanted, visible)
		}
	}
}

func TestAssistantShowcaseExecuteSendsMobileLinks(t *testing.T) {
	home := t.TempDir()
	signalArgs := filepath.Join(home, "signal-args.txt")
	fakeSignal := filepath.Join(home, "signal-cli")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %q\nprintf 'showcase-signal-246\\n'\n", signalArgs)
	if err := os.WriteFile(fakeSignal, []byte(script), 0700); err != nil {
		t.Fatalf("write fake signal-cli: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_ASSISTANT_SHOWCASE_SAMPLES", "0")
	t.Setenv("MESHCLAW_SIGNAL_CLI", fakeSignal)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, "targets.json"))
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED", "1")
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatalf("upsert briefing target: %v", err)
	}

	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-showcase-execute-en", Mode: "assistant", Execute: true},
		"send the 300 scenario catalog to the briefing room",
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Sent the Argos capability showcase to the briefing room.",
		"Signal ID: showcase-signal-246",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("execute showcase reply missing %q:\n%s", want, visible)
		}
	}
	if containsHangul(visible) {
		t.Fatalf("English execute showcase reply should not expose Korean:\n%s", visible)
	}

	argsData, err := os.ReadFile(signalArgs)
	if err != nil {
		t.Fatalf("read fake signal args: %v", err)
	}
	args := string(argsData)
	for _, want := range []string{
		"Open on mobile:",
		"Markdown source: https://argos.example.test/argos/",
		"--attachment",
		".md",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("fake signal args missing %q:\n%s", want, args)
		}
	}
	if strings.Contains(args, ".html") {
		t.Fatalf("mobile Signal showcase should not attach HTML:\n%s", args)
	}
}
