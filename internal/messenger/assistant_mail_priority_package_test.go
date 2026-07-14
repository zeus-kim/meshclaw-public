package messenger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/mailadapter"
)

func TestAssistantMailPriorityPackageTargetsBriefingWithArtifacts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "ko")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", home+"/targets.json")
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}

	items := mailPriorityItemsFromResults([]mailadapter.SearchResult{{
		Account: mailadapter.AccountPublic{ID: "work"},
		Messages: []mailadapter.MessageSummary{
			{
				ID:      "urgent",
				Subject: "내일 계약서 검토 후 회신 부탁드립니다",
				From:    "partner@example.com",
				Date:    time.Date(2026, 6, 13, 1, 30, 0, 0, time.UTC),
				Snippet: "내일 오전 미팅 전에 계약서 조건을 검토하고 회신 부탁드립니다.",
			},
			{
				ID:      "schedule",
				Subject: "광고기획 회의 일정 문의",
				From:    "marketing@example.com",
				Date:    time.Date(2026, 6, 13, 2, 30, 0, 0, time.UTC),
				Snippet: "다음 주 캠페인 회의 가능 시간을 확인 부탁드립니다.",
			},
			{
				ID:      "promo",
				Subject: "주간 뉴스레터",
				From:    "newsletter@example.com",
				Date:    time.Date(2026, 6, 13, 3, 30, 0, 0, time.UTC),
				Snippet: "이번 주 할인 소식과 프로모션입니다. unsubscribe",
			},
		},
	}})
	replyText := formatAssistantMailPriority(items, nil, evidence.Record{}, nil)
	reply := formatAssistantMailPriorityPackageSendResult(
		ListenOptions{TargetID: "argos-assistant-mail-package"},
		"보고방",
		"최근 메일을 확인해서 답장 필요한 것만 우선순위로 정리하고 보고방에 보내줘. 음성으로도 준비해줘.",
		replyText,
		items,
		nil,
		evidence.Record{},
		nil,
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"메일 우선순위 보고를 Signal로 보낼 준비를 했습니다.",
		"대상: 보고방",
		"보고방은 one-way/no-reply",
		"메일 우선순위 보고 패키지를 만들었습니다.",
		"메일 처리 우선순위입니다.",
		"답장 우선",
		"답장 초안 방향:",
		"초안 방향: 수신 확인, 요청받은 검토/일정/자료 항목에 대한 답",
		"답장/발송 승인 초안:",
		"`답장 초안 승인`: 첫 번째 답장 우선 메일의 본문을 읽고 답장 초안만 작성",
		"`메일 발송 승인`: 최종 문안과 수신자 확인 후 해당 답장 1건만 발송",
		"삭제, 이동, 첨부 저장, 캘린더 초대, 리마인더 생성은 별도 승인 전 중지",
		"일정 조율 메일을 캘린더 후보 초안으로 바꿔줘. 아직 초대하지 마",
		"첫 번째 답장 마감 리마인더 초안만 만들어줘. 아직 생성하지 마",
		"SVG 메일 처리 보드",
		"edge-tts MP3 메일 브리핑",
		"메일 발송/삭제/이동/첨부 저장은 별도 승인 전에는 실행하지 않았습니다.",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("mail priority package reply missing %q:\n%s", want, visible)
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
			t.Fatalf("mail priority package missing %s attachment: %#v", ext, attachments)
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
	for _, want := range []string{"## 답장 우선", "## 확인 필요", "## 답장 초안 방향", "## 답장/발송 승인 초안", "## 보류", "내일 계약서 검토 후 회신 부탁드립니다", "광고기획 회의 일정 문의", "메일 미리보기 기준", "`답장 초안 승인`", "`메일 발송 승인`", "삭제, 이동, 첨부 저장, 캘린더 초대, 리마인더 생성은 별도 승인 전 중지", "캘린더 후보 초안으로만", "리마인더 초안으로만", "실제 초대/등록은 별도 승인 전에는 하지 않습니다", "실제 생성은 별도 승인 전에는 하지 않습니다"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("mail priority markdown missing %q:\n%s", want, string(data))
		}
	}
	board := firstAttachmentExt(attachments, ".svg")
	boardData, err := os.ReadFile(board)
	if err != nil {
		t.Fatalf("read mail board SVG: %v", err)
	}
	for _, want := range []string{"<svg", "메일 처리 우선순위 패키지", "답장 우선", "확인 필요", "보류", "먼저 처리할 메일", "내일 계약서", "광고기획"} {
		if !strings.Contains(string(boardData), want) {
			t.Fatalf("mail priority board missing %q:\n%s", want, string(boardData))
		}
	}
}

func TestAssistantPlainMailCheckRoutesToPrioritySummary(t *testing.T) {
	for _, input := range []string{"이메일 체크해줘", "메일 확인해줘", "check email"} {
		if !isAssistantMailPriorityRequest(input) {
			t.Fatalf("plain mail check should route to priority summary: %q", input)
		}
	}
	if isAssistantMailPriorityRequest("새 메일 왔어?") {
		t.Fatal("new-mail existence check should stay on mail watch")
	}
}

func TestAssistantMailPriorityPackageEnglishCanSendToBriefing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_LANG", "en")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")
	t.Setenv("MESHCLAW_EDGE_TTS", fakeEdgeTTS(t))
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, "targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}

	items := mailPriorityItemsFromResults([]mailadapter.SearchResult{{
		Account: mailadapter.AccountPublic{ID: "work"},
		Messages: []mailadapter.MessageSummary{
			{
				ID:      "urgent",
				Subject: "Please review the contract and reply by tomorrow",
				From:    "partner@example.com",
				Date:    time.Date(2026, 6, 13, 1, 30, 0, 0, time.UTC),
				Snippet: "Please review the contract terms and respond before tomorrow morning's meeting.",
			},
			{
				ID:      "schedule",
				Subject: "Question about next week's campaign meeting schedule",
				From:    "marketing@example.com",
				Date:    time.Date(2026, 6, 13, 2, 30, 0, 0, time.UTC),
				Snippet: "Please confirm available time for next week's campaign meeting.",
			},
			{
				ID:      "promo",
				Subject: "Weekly newsletter",
				From:    "newsletter@example.com",
				Date:    time.Date(2026, 6, 13, 3, 30, 0, 0, time.UTC),
				Snippet: "This week's discount news and promotions. unsubscribe",
			},
		},
	}})
	replyText := formatAssistantMailPriority(items, nil, evidence.Record{}, nil)
	if containsHangul(replyText) {
		t.Fatalf("English mail priority summary should not expose Korean:\n%s", replyText)
	}
	reply := formatAssistantMailPriorityPackageSendResult(
		ListenOptions{TargetID: "argos-assistant-mail-package-en"},
		"argos-briefing",
		"check recent mail, prioritize replies, draft next actions, and send a mail-priority package with voice to the briefing room",
		replyText,
		items,
		nil,
		evidence.Record{},
		nil,
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Prepared the mail-priority report for Signal delivery.",
		"Target: Briefing room",
		"The briefing room is one-way/no-reply",
		"Created a mail-priority report package.",
		"Mail handling priority.",
		"Reply first",
		"Reply draft direction:",
		"Draft direction: acknowledge receipt",
		"Reply/send approval drafts:",
		"`reply draft approved`: read the first reply-first mail body and draft the reply only",
		"`mail send approved`: after final wording and recipient confirmation, send only that one reply",
		"Deletion, moving, attachment saving, calendar invites, and reminder creation stay stopped until separate approval.",
		"turn schedule-related mail into draft calendar options only; do not invite yet",
		"draft a follow-up reminder for the first reply deadline only; do not create it yet",
		"Mobile-openable SVG mail handling board",
		"edge-tts MP3 mail brief",
		"No mail sending, deletion, moving, or attachment saving was executed without separate approval.",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("English mail priority package reply missing %q:\n%s", want, visible)
		}
	}
	if containsHangul(visible) {
		t.Fatalf("English mail priority visible text should not expose Korean:\n%s", visible)
	}
	for _, unwanted := range []string{"meshclaw-attachment:", "/Documents/Argos Vault/", "/.meshclaw/evidence/"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("visible reply should not expose raw path %q:\n%s", unwanted, visible)
		}
	}

	attachments := signalReplyAttachments(reply)
	for _, ext := range []string{".docx", ".md", ".pptx", ".xlsx", ".csv", ".svg", ".mp3"} {
		if !hasAttachmentExt(attachments, ext) {
			t.Fatalf("English mail priority package missing %s attachment: %#v", ext, attachments)
		}
	}
	for _, attachment := range attachments {
		lower := strings.ToLower(attachment)
		if strings.HasSuffix(lower, ".html") {
			t.Fatalf("HTML should not be attached for mobile Signal package: %#v", attachments)
		}
		reportMarkdown := ""
		if !(strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".csv") || strings.HasSuffix(lower, ".svg")) {
			continue
		}
		data, err := os.ReadFile(attachment)
		if err != nil {
			t.Fatalf("read English mail artifact %s: %v", attachment, err)
		}
		if containsHangul(string(data)) {
			t.Fatalf("English mail priority artifact should not expose Korean in %s:\n%s", attachment, string(data))
		}
		if strings.HasSuffix(lower, ".md") {
			for _, want := range []string{"Reply/Send Approval Drafts", "`reply draft approved`", "`mail send approved`", "Deletion, moving, attachment saving, calendar invites, and reminder creation stay stopped until separate approval."} {
				if !strings.Contains(string(data), want) {
					t.Fatalf("English mail priority markdown missing %q:\n%s", want, string(data))
				}
			}
			if strings.Contains(string(data), "## Next Execution") {
				reportMarkdown = string(data)
			}
		}
		if reportMarkdown != "" {
			for _, want := range []string{"draft calendar options only", "actual invites or calendar creation wait for separate approval", "draft reminders only", "actual reminder creation waits for separate approval"} {
				if !strings.Contains(reportMarkdown, want) {
					t.Fatalf("English mail priority report markdown missing %q:\n%s", want, reportMarkdown)
				}
			}
		}
	}
}

func TestAssistantMailPriorityPackageExecuteSendsToBriefingWithArtifacts(t *testing.T) {
	home := t.TempDir()
	signalArgs := filepath.Join(home, "signal-args.txt")
	fakeSignal := filepath.Join(home, "signal-cli")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %q\nprintf 'mail-signal-456\\n'\n", signalArgs)
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

	items := mailPriorityItemsFromResults([]mailadapter.SearchResult{{
		Account: mailadapter.AccountPublic{ID: "work"},
		Messages: []mailadapter.MessageSummary{
			{
				ID:      "urgent",
				Subject: "Please review the contract and reply by tomorrow",
				From:    "partner@example.com",
				Date:    time.Date(2026, 6, 13, 1, 30, 0, 0, time.UTC),
				Snippet: "Please review the contract terms and respond before tomorrow morning's meeting.",
			},
			{
				ID:      "schedule",
				Subject: "Question about next week's campaign meeting schedule",
				From:    "marketing@example.com",
				Date:    time.Date(2026, 6, 13, 2, 30, 0, 0, time.UTC),
				Snippet: "Please confirm available time for next week's campaign meeting.",
			},
		},
	}})
	replyText := formatAssistantMailPriority(items, nil, evidence.Record{}, nil)
	reply := formatAssistantMailPriorityPackageSendResult(
		ListenOptions{TargetID: "argos-assistant-mail-package-execute-en", Execute: true},
		"argos-briefing",
		"check recent mail, prioritize replies, and send a mail-priority package with voice to the briefing room",
		replyText,
		items,
		nil,
		evidence.Record{},
		nil,
	)
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"Sent the mail-priority report to Signal.",
		"Target: Briefing room",
		"Signal ID: mail-signal-456",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("execute mail reply missing %q:\n%s", want, visible)
		}
	}
	if containsHangul(visible) {
		t.Fatalf("English execute mail reply should not expose Korean:\n%s", visible)
	}
	for _, unwanted := range []string{"meshclaw-attachment:", "/Documents/Argos Vault/", "/.meshclaw/evidence/"} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("execute mail reply should not expose %q:\n%s", unwanted, visible)
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
