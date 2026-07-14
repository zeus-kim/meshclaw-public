package messenger

import (
	"archive/zip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/guard"
	"github.com/meshclaw/meshclaw/internal/osauto"
	"github.com/meshclaw/meshclaw/internal/runtimeflow"
)

func TestBuildReportFromLatestEvidence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := filepath.Join(home, ".meshclaw", "evidence", "2026-05-22", "demo")
	if err := os.MkdirAll(bundle, 0700); err != nil {
		t.Fatal(err)
	}
	result := runtimeflow.Result{
		Success:        true,
		Workflow:       "demo",
		Mode:           runtimeflow.DryRun,
		GeneratedAt:    time.Now().UTC(),
		BundleDir:      bundle,
		EvidenceBundle: bundle,
		Summary: runtimeflow.Summary{
			Total:            2,
			Succeeded:        1,
			Skipped:          1,
			ApprovalRequired: 1,
		},
		Steps: []runtimeflow.ExecutionResult{
			{Success: true, Workflow: "demo", Step: "inspect", Title: "Inspect", Status: "ok", PolicyDecision: "allow"},
			{
				Success:          true,
				Workflow:         "demo",
				Step:             "send-mail",
				Title:            "Send mail",
				Status:           "approval_pending",
				Action:           "email_send",
				Resource:         "mail",
				ApprovalRequired: true,
				PolicyDecision:   "require_approval",
				PolicyReason:     "sending real email requires approval",
				Skipped:          true,
				SkipReason:       "approval required before execution",
			},
		},
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "execution.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	latest := filepath.Join(home, ".meshclaw", "evidence", "latest")
	if err := os.Symlink(bundle, latest); err != nil {
		t.Fatal(err)
	}

	report, err := BuildReport(ReportOptions{Ref: "latest", Channel: "signal", Audience: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Decision != "approval_required" {
		t.Fatalf("decision=%q", report.Decision)
	}
	if len(report.ApprovalNeeded) != 1 || report.ApprovalNeeded[0].Step != "send-mail" {
		t.Fatalf("approval=%#v", report.ApprovalNeeded)
	}
	if report.Redaction.RawSecretsIncluded {
		t.Fatal("report must not include raw secrets")
	}
	if report.Text == "" {
		t.Fatal("expected formatted messenger text")
	}
	if filepath.Base(report.Evidence.Report) != "messenger-report.md" {
		t.Fatalf("report evidence path=%q", report.Evidence.Report)
	}
	if strings.Contains(report.Text, bundle) || strings.Contains(report.Text, "report.html") {
		t.Fatalf("visible report text should not expose local report paths:\n%s", report.Text)
	}
}

func TestWriteReportCreatesObsidianMarkdown(t *testing.T) {
	home := seedLatestBundle(t)
	t.Setenv("HOME", home)
	report, err := BuildReport(ReportOptions{Ref: "latest", Channel: "signal", Audience: "argos-briefing"})
	if err != nil {
		t.Fatal(err)
	}
	jsonPath, err := WriteReport(report)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(jsonPath) != "messenger-report.json" {
		t.Fatalf("jsonPath=%q", jsonPath)
	}
	mdPath := report.Evidence.Report
	data, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"type: meshclaw-argos-report", "tags:", "# demo is ready", "## Summary", "## Evidence"} {
		if !strings.Contains(text, want) {
			t.Fatalf("markdown missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(mdPath, "report.html") {
		t.Fatalf("markdown report path should not be html: %s", mdPath)
	}
}

func TestFormatSignalMeshClawReportIncludesArgosRecording(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	recording := filepath.Join(home, ".meshclaw", "recordings", "argos-test.mov")
	if _, err := evidence.Store("browser_search", "argos", "가나 경제 뉴스", map[string]interface{}{
		"action":         "browser_search",
		"query":          "가나 경제 뉴스",
		"recording_path": recording,
	}); err != nil {
		t.Fatal(err)
	}

	text := formatSignalMeshClawReport("Argos 방금 한 작업 보고", 10)
	if !strings.Contains(text, "recording="+recording) {
		t.Fatalf("report missing recording path:\n%s", text)
	}
	if !strings.Contains(text, "action=browser_search") || !strings.Contains(text, "query=가나 경제 뉴스") {
		t.Fatalf("report missing argos detail:\n%s", text)
	}
}

func TestIsAssistantMeshClawReportRequest(t *testing.T) {
	for _, input := range []string{"Argos work report", "방금 한 작업 보고", "mac status", "report"} {
		if !isAssistantMeshClawReportRequest(strings.ToLower(input)) {
			t.Fatalf("expected report request for %q", input)
		}
	}
	if isAssistantMeshClawReportRequest("open safari") {
		t.Fatal("open safari should not be treated as a report")
	}
	if isAssistantMeshClawReportRequest("브라우저로 이순신 검색해서 요약 리포트를 써줘") {
		t.Fatal("browser research report should not be treated as MeshClaw progress report")
	}
	if isAssistantMeshClawReportRequest("이번 주 Argos 작업 내역을 표로 정리해서 한 페이지 업무 보고 문서로 만들어줘") {
		t.Fatal("document creation request should not be treated as MeshClaw progress report")
	}
	if isAssistantMeshClawReportRequest("노드들 상태 확인해봐") {
		t.Fatal("fleet status should not be treated as MeshClaw progress report")
	}
}

func TestIsAssistantFleetStatusRequest(t *testing.T) {
	for _, input := range []string{"노드들 상태 확인해봐", "자세히 설명해줘 노드별로", "내 노드 이름들을 몰라?", "server node status"} {
		if !isAssistantFleetStatusRequest(strings.ToLower(input)) {
			t.Fatalf("expected fleet status request for %q", input)
		}
	}
	if isAssistantFleetStatusRequest("브라우저로 이순신 검색해서 요약 리포트를 써줘") {
		t.Fatal("browser research should not be treated as fleet status")
	}
}

func TestSignalReplyAttachmentsExtractsExistingRecording(t *testing.T) {
	dir := t.TempDir()
	recording := filepath.Join(dir, "argos.mov")
	if err := os.WriteFile(recording, []byte("movie"), 0600); err != nil {
		t.Fatal(err)
	}
	reply := "Argos가 Mac 작업을 처리했습니다: open_app\n작업 화면 기록: " + recording + "\n작업 화면 기록: /missing.mov"
	attachments := signalReplyAttachments(reply)
	if len(attachments) != 1 || attachments[0] != recording {
		t.Fatalf("attachments=%#v", attachments)
	}
}

func TestSignalReplyAttachmentsExtractsDocument(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "argos.md")
	if err := os.WriteFile(doc, []byte("# Argos"), 0600); err != nil {
		t.Fatal(err)
	}
	preview := filepath.Join(dir, "argos.html")
	if err := os.WriteFile(preview, []byte("<html>Argos</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	image := preview + ".png"
	if err := os.WriteFile(image, []byte("png"), 0600); err != nil {
		t.Fatal(err)
	}
	reply := "Argos가 Mac 작업을 처리했습니다: work_demo\n작업 문서: " + doc
	attachments := signalReplyAttachments(reply)
	if len(attachments) != 2 || attachments[0] != image || attachments[1] != doc {
		t.Fatalf("attachments=%#v", attachments)
	}
}

func TestSignalReplyHiddenAttachmentMarkerIsNotVisible(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "meeting.docx")
	if err := os.WriteFile(doc, []byte("docx"), 0600); err != nil {
		t.Fatal(err)
	}
	reply := strings.Join([]string{
		"회의 자료 패키지를 준비했습니다.",
		"PowerPoint 링크: http://example.test/deck.pptx",
		"meshclaw-attachment: " + doc,
		"작업 기록도 저장했습니다.",
	}, "\n")
	attachments := signalReplyAttachments(reply)
	if len(attachments) != 1 || attachments[0] != doc {
		t.Fatalf("attachments=%#v", attachments)
	}
	visible := signalReplyVisibleText(reply)
	if strings.Contains(visible, "meshclaw-attachment:") || strings.Contains(visible, doc) {
		t.Fatalf("hidden attachment marker leaked into visible reply:\n%s", visible)
	}
	if !strings.Contains(visible, "회의 자료 패키지를 준비했습니다.") {
		t.Fatalf("visible reply lost natural language text:\n%s", visible)
	}
}

func TestSignalReplyLogTextHidesAttachmentMarkersAndLocalPaths(t *testing.T) {
	reply := strings.Join([]string{
		"문서를 작성했습니다.",
		"첨부는 Signal 파일로 함께 보냅니다.",
		"meshclaw-attachment: /Users/argos/Documents/Argos Vault/Inbox/report.docx",
		"추가 기록: /tmp/argos/evidence.json",
	}, "\n")

	logText := signalReplyLogText(reply)
	for _, bad := range []string{"meshclaw-attachment:", "/Users/", "/tmp/"} {
		if strings.Contains(logText, bad) {
			t.Fatalf("log text leaked %q:\n%s", bad, logText)
		}
	}
	if !strings.Contains(logText, "문서를 작성했습니다.") || !strings.Contains(logText, "[local path redacted]") {
		t.Fatalf("log text lost summary or redaction marker:\n%s", logText)
	}
}

func TestSignalReplyVoiceNoteOnlyForSingleAudioAttachment(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "voice.mp3")
	doc := filepath.Join(dir, "report.docx")
	if err := os.WriteFile(audio, []byte("audio"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(doc, []byte("docx"), 0600); err != nil {
		t.Fatal(err)
	}
	if !signalReplyVoiceNote(signalReplyAttachments("meshclaw-attachment: " + audio)) {
		t.Fatal("single audio attachment should be sent as a Signal voice note")
	}
	if signalReplyVoiceNote(signalReplyAttachments("meshclaw-attachment: " + doc + "\nmeshclaw-attachment: " + audio)) {
		t.Fatal("mixed report attachments should not be sent as one Signal voice note")
	}
	if signalReplyVoiceNote(signalReplyAttachments("meshclaw-attachment: " + doc)) {
		t.Fatal("non-audio attachment should not be sent as a Signal voice note")
	}
}

func TestParseSignalReceiveKeepsAttachmentOnlyMessage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	attachmentDir := filepath.Join(home, ".meshclaw", "signal-attachments")
	if err := os.MkdirAll(attachmentDir, 0700); err != nil {
		t.Fatal(err)
	}
	stored := filepath.Join(attachmentDir, "signal-attachment.txt")
	if err := os.WriteFile(stored, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	raw := `{"envelope":{"sourceNumber":"+100","timestamp":1770000000000,"dataMessage":{"message":"","attachments":[{"contentType":"text/plain","filename":"note.txt","storedFilename":"signal-attachment.txt","size":5}],"groupInfo":{"groupId":"group-1"}}}}` + "\n"

	events := parseSignalReceive([]byte(raw), Target{GroupID: "group-1"})
	if len(events) != 1 {
		t.Fatalf("events=%#v", events)
	}
	event := events[0]
	if event.Redacted != "첨부 파일을 받았습니다." {
		t.Fatalf("redacted=%q", event.Redacted)
	}
	if len(event.Attachments) != 1 {
		t.Fatalf("attachments=%#v", event.Attachments)
	}
	attachment := event.Attachments[0]
	if attachment.Path != stored || attachment.Filename != "note.txt" || attachment.ContentType != "text/plain" {
		t.Fatalf("attachment=%#v", attachment)
	}
	if attachment.Text != "hello" {
		t.Fatalf("attachment text=%q", attachment.Text)
	}
}

func TestSignalSelfEchoIgnoresArgosReplies(t *testing.T) {
	cases := []IncomingMessage{
		{Name: "Argos", Source: "+100", Redacted: "승인 기록에 실패했습니다: step \"기록에\" not found in workflow email-orchestration-demo"},
		{Name: "", Source: "+100", Redacted: "주차 정보 확인 링크입니다.\nhttps://www.google.com/maps/search/?api=1&query=parking"},
		{Name: "", Source: "+100", Redacted: "지도에서 주차/영업정보/후기를 확인하고, 장소가 맞으면 전화나 공식 페이지 확인까지 이어갈 수 있습니다."},
	}
	for _, event := range cases {
		if !isSignalSelfEcho(event) {
			t.Fatalf("expected self echo: %#v", event)
		}
	}
	if isSignalSelfEcho(IncomingMessage{Name: "Example User", Source: "+200", Redacted: "오늘 보고서 보여줘"}) {
		t.Fatal("human request should not be treated as self echo")
	}
}

func TestSignalSelfEchoIgnoresConfiguredBotNumber(t *testing.T) {
	t.Setenv("MESHCLAW_SIGNAL_BOT_NUMBER", "+15550001111")
	if !isSignalSelfEcho(IncomingMessage{Name: "Assistant", Source: "+15550001111", Redacted: "hello"}) {
		t.Fatal("configured bot number should be ignored")
	}
}

func TestParseSignalReceiveFallsBackToSignalCLIAttachmentDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	signalDir := filepath.Join(home, ".local", "share", "signal-cli", "attachments")
	if err := os.MkdirAll(signalDir, 0700); err != nil {
		t.Fatal(err)
	}
	stored := filepath.Join(signalDir, "signal-image.png")
	if err := os.WriteFile(stored, []byte("not really an image"), 0600); err != nil {
		t.Fatal(err)
	}
	raw := `{"envelope":{"sourceNumber":"+100","timestamp":1770000000000,"dataMessage":{"message":"image","attachments":[{"contentType":"text/plain","filename":"signal-image.png","storedFilename":"signal-image.png","size":19}],"groupInfo":{"groupId":"group-1"}}}}` + "\n"

	events := parseSignalReceive([]byte(raw), Target{GroupID: "group-1"})
	if len(events) != 1 {
		t.Fatalf("events=%#v", events)
	}
	if got := events[0].Attachments[0].Path; got != stored {
		t.Fatalf("path=%q want %q", got, stored)
	}
}

func TestParseSignalReceiveFallsBackToSignalCLIAttachmentBySize(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	signalDir := filepath.Join(home, ".local", "share", "signal-cli", "attachments")
	if err := os.MkdirAll(signalDir, 0700); err != nil {
		t.Fatal(err)
	}
	body := []byte("Argos attachment text")
	stored := filepath.Join(signalDir, "random-signal-name.txt")
	if err := os.WriteFile(stored, body, 0600); err != nil {
		t.Fatal(err)
	}
	raw := `{"envelope":{"sourceNumber":"+100","timestamp":1770000000000,"dataMessage":{"message":"summarize attachment","attachments":[{"contentType":"text/plain","filename":"original-name.txt","size":21}],"groupInfo":{"groupId":"group-1"}}}}` + "\n"

	events := parseSignalReceive([]byte(raw), Target{GroupID: "group-1"})
	if len(events) != 1 {
		t.Fatalf("events=%#v", events)
	}
	attachment := events[0].Attachments[0]
	if attachment.Path != stored || attachment.Text != string(body) {
		t.Fatalf("attachment=%#v want path=%q text=%q", attachment, stored, string(body))
	}
}

func TestParseSignalReceiveChoosesNewestMatchingAttachmentByTime(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	signalDir := filepath.Join(home, ".local", "share", "signal-cli", "attachments")
	if err := os.MkdirAll(signalDir, 0700); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(signalDir, "old-random.txt")
	newPath := filepath.Join(signalDir, "new-random.txt")
	body := []byte("fresh Argos attachment text")
	if err := os.WriteFile(oldPath, body, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, body, 0600); err != nil {
		t.Fatal(err)
	}
	eventTime := time.Date(2026, 5, 24, 21, 25, 30, 0, time.UTC)
	if err := os.Chtimes(oldPath, eventTime.Add(-20*time.Minute), eventTime.Add(-20*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newPath, eventTime.Add(2*time.Second), eventTime.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	raw := `{"envelope":{"sourceNumber":"+100","timestamp":` + strconv.FormatInt(eventTime.UnixMilli(), 10) + `,"dataMessage":{"message":"summarize attachment","attachments":[{"contentType":"text/plain","filename":"original-name.txt","size":27}],"groupInfo":{"groupId":"group-1"}}}}` + "\n"

	events := parseSignalReceive([]byte(raw), Target{GroupID: "group-1"})
	if len(events) != 1 {
		t.Fatalf("events=%#v", events)
	}
	attachment := events[0].Attachments[0]
	if attachment.Path != newPath || attachment.Text != string(body) {
		t.Fatalf("attachment=%#v want path=%q text=%q", attachment, newPath, string(body))
	}
}

func TestParseSignalReceiveFallsBackToRecentAttachmentWhenSizeMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	signalDir := filepath.Join(home, ".local", "share", "signal-cli", "attachments")
	if err := os.MkdirAll(signalDir, 0700); err != nil {
		t.Fatal(err)
	}
	stored := filepath.Join(signalDir, "random-signal-name.txt")
	body := []byte("recent text without size")
	if err := os.WriteFile(stored, body, 0600); err != nil {
		t.Fatal(err)
	}
	eventTime := time.Date(2026, 5, 24, 21, 25, 30, 0, time.UTC)
	if err := os.Chtimes(stored, eventTime.Add(3*time.Second), eventTime.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	raw := `{"envelope":{"sourceNumber":"+100","timestamp":` + strconv.FormatInt(eventTime.UnixMilli(), 10) + `,"dataMessage":{"message":"summarize attachment","attachments":[{"contentType":"text/plain","filename":"original-name.txt"}],"groupInfo":{"groupId":"group-1"}}}}` + "\n"

	events := parseSignalReceive([]byte(raw), Target{GroupID: "group-1"})
	if len(events) != 1 {
		t.Fatalf("events=%#v", events)
	}
	attachment := events[0].Attachments[0]
	if attachment.Path != stored || attachment.Text != string(body) {
		t.Fatalf("attachment=%#v want path=%q text=%q", attachment, stored, string(body))
	}
}

func TestParseSignalReceiveReadsDocxAttachment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	attachmentDir := filepath.Join(home, ".meshclaw", "signal-attachments")
	if err := os.MkdirAll(attachmentDir, 0700); err != nil {
		t.Fatal(err)
	}
	stored := filepath.Join(attachmentDir, "signal-doc.docx")
	if err := writeMinimalDocx(stored, "Argos DOCX attachment", "second paragraph"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(stored)
	if err != nil {
		t.Fatal(err)
	}
	raw := `{"envelope":{"sourceNumber":"+100","timestamp":1770000000000,"dataMessage":{"message":"summarize attachment","attachments":[{"contentType":"application/vnd.openxmlformats-officedocument.wordprocessingml.document","filename":"report.docx","storedFilename":"signal-doc.docx","size":` + strconv.FormatInt(info.Size(), 10) + `}],"groupInfo":{"groupId":"group-1"}}}}` + "\n"

	events := parseSignalReceive([]byte(raw), Target{GroupID: "group-1"})
	if len(events) != 1 {
		t.Fatalf("events=%#v", events)
	}
	attachment := events[0].Attachments[0]
	if attachment.Path != stored || !strings.Contains(attachment.Text, "Argos DOCX attachment") || !strings.Contains(attachment.Text, "second paragraph") {
		t.Fatalf("attachment=%#v", attachment)
	}
	if reply := directSignalAttachmentReply(events[0]); !strings.Contains(reply, "Argos DOCX attachment") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestLatestSignalAttachmentContextIncludesImageOCRResult(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	event := IncomingMessage{
		Redacted: "이 이미지 봐줘",
		Attachments: []SignalAttachment{{
			ContentType: "image/png",
			Filename:    "screen.png",
			Path:        filepath.Join(home, "screen.png"),
			Text:        "Gemini가 답변한 화면",
		}},
	}

	storeLatestSignalAttachmentContext("argos-briefing", event)
	contextText := latestSignalAttachmentContext("argos-briefing")
	if !strings.Contains(contextText, "최근 Signal 이미지/첨부 컨텍스트") ||
		!strings.Contains(contextText, "screen.png") ||
		!strings.Contains(contextText, "Gemini가 답변한 화면") {
		t.Fatalf("context=%q", contextText)
	}
}

func TestDirectSignalAttachmentReplyReturnsCode(t *testing.T) {
	event := IncomingMessage{
		Redacted: "Reply with the code only.",
		Attachments: []SignalAttachment{{
			ContentType: "image/png",
			Text:        "ARGOS OCR TEST\nSignal image context works\ncode: BRIEFING-7429",
		}},
	}
	if got := directSignalAttachmentReply(event); got != "BRIEFING-7429" {
		t.Fatalf("got=%q", got)
	}
}

func TestDirectSignalAttachmentReplyReturnsOCRError(t *testing.T) {
	event := IncomingMessage{
		Redacted: "What is in this image?",
		Attachments: []SignalAttachment{{
			ContentType: "image/png",
			Error:       "image text recognition timed out",
		}},
	}
	got := directSignalAttachmentReply(event)
	if !strings.Contains(got, "내용을 읽지 못했습니다") {
		t.Fatalf("got=%q", got)
	}
}

func TestDirectSignalAttachmentReplySummarizesTextAttachment(t *testing.T) {
	event := IncomingMessage{
		Redacted: "첨부 내용 요약해줘",
		Attachments: []SignalAttachment{{
			ContentType: "text/markdown",
			Filename:    "report.md",
			Text:        "# Report\n\nfirst point\nsecond point\nthird point\nfourth point\nfifth point\nsixth point",
		}},
	}
	got := directSignalAttachmentReply(event)
	if !strings.Contains(got, "첨부 내용 요약입니다") || !strings.Contains(got, "first point") || strings.Contains(got, "sixth point") {
		t.Fatalf("got=%q", got)
	}
}

func TestDirectSignalAttachmentReplyUsesImageDescription(t *testing.T) {
	event := IncomingMessage{
		Redacted: "이 이미지 뭐야?",
		Attachments: []SignalAttachment{{
			ContentType: "image/png",
			Description: "브라우저에서 Gemini 답변 화면이 보이고, 사용자는 그 답이 환각인지 묻고 있습니다.",
			Text:        "Gemini can make mistakes",
		}},
	}
	got := directSignalAttachmentReply(event)
	if !strings.Contains(got, "이미지 분석입니다") ||
		!strings.Contains(got, "Gemini 답변 화면") ||
		!strings.Contains(got, "읽은 텍스트") {
		t.Fatalf("got=%q", got)
	}
}

func TestDirectLatestSignalAttachmentReplyUsesStoredImageContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	storeLatestSignalAttachmentContext("argos-briefing", IncomingMessage{
		Redacted: "Vision retest",
		Attachments: []SignalAttachment{{
			ContentType: "image/png",
			Filename:    "screen.png",
			Description: "브라우저에서 Gemini 답변 화면이 보입니다.",
			Text:        "Gemini can make mistakes",
		}},
	})

	got := directLatestSignalAttachmentReply("argos-briefing", "저 위 이미지 못읽어?")
	if !strings.Contains(got, "이미지 분석입니다") ||
		!strings.Contains(got, "Gemini 답변 화면") ||
		!strings.Contains(got, "읽은 텍스트") {
		t.Fatalf("got=%q", got)
	}
}

func TestDirectLatestSignalAttachmentReplyIgnoresUnrelatedSummaryRequest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	storeLatestSignalAttachmentContext("argos-briefing", IncomingMessage{
		Redacted: "image",
		Attachments: []SignalAttachment{{
			ContentType: "image/png",
			Text:        "old image text",
		}},
	})

	if got := directLatestSignalAttachmentReply("argos-briefing", "오늘 뉴스 요약해줘"); got != "" {
		t.Fatalf("got=%q", got)
	}
}

func TestPrepareSignalMailFromLatestAttachmentCreatesDraft(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MAIL_ACCOUNTS", filepath.Join(home, "mail-accounts.json"))
	t.Setenv("MESHCLAW_MAIL_DRAFT_DIR", filepath.Join(home, "mail-drafts"))
	if err := os.WriteFile(filepath.Join(home, "mail-accounts.json"), []byte(`{
  "accounts": [
    {
      "id": "personal",
      "backend": "imap",
      "email": "sender@example.com",
      "host": "imap.example.com",
      "username": "sender@example.com",
      "password_env": "MAIL_PASSWORD",
      "mailbox": "INBOX",
      "tls": true
    }
  ]
}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MAIL_PASSWORD", "test-password")
	storeLatestSignalAttachmentContext("argos-assistant", IncomingMessage{
		Redacted: "PDF 첨부",
		Attachments: []SignalAttachment{{
			ContentType: "application/pdf",
			Filename:    "brief.pdf",
			Text:        "Argos PDF live test\ncode: PDF-LIVE-2026",
		}},
	})

	reply, handled := prepareSignalMailFromLatestAttachment(ListenOptions{TargetID: "argos-assistant"}, "이 첨부를 operator@example.com 에게 메일로 보내줘")
	if !handled {
		t.Fatal("expected latest attachment mail request to be handled")
	}
	if !strings.Contains(reply, "메일 초안을 만들었습니다") ||
		!strings.Contains(reply, "operator@example.com") ||
		!strings.Contains(reply, "PDF-LIVE-2026") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestPrepareSignalMailFromLatestAttachmentRequiresContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	reply, handled := prepareSignalMailFromLatestAttachment(ListenOptions{TargetID: "argos-assistant"}, "이 첨부를 operator@example.com 에게 메일로 보내줘")
	if !handled || !strings.Contains(reply, "최근 첨부 컨텍스트를 찾지 못했습니다") {
		t.Fatalf("handled=%t reply=%q", handled, reply)
	}
}

func TestGuardReplyRoutesAttachmentMailBeforeAttachmentSummary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MAIL_ACCOUNTS", filepath.Join(home, "mail-accounts.json"))
	t.Setenv("MESHCLAW_MAIL_DRAFT_DIR", filepath.Join(home, "mail-drafts"))
	if err := os.WriteFile(filepath.Join(home, "mail-accounts.json"), []byte(`{
  "accounts": [
    {
      "id": "personal",
      "backend": "imap",
      "email": "sender@example.com",
      "host": "imap.example.com",
      "username": "sender@example.com",
      "password_env": "MAIL_PASSWORD",
      "mailbox": "INBOX",
      "tls": true
    }
  ]
}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MAIL_PASSWORD", "test-password")
	storeLatestSignalAttachmentContext("bot-signal", IncomingMessage{
		Redacted: "PDF 첨부",
		Attachments: []SignalAttachment{{
			ContentType: "application/pdf",
			Filename:    "brief.pdf",
			Text:        "Argos PDF live test\ncode: PDF-LIVE-2026",
		}},
	})

	reply := guardReply(ListenOptions{TargetID: "bot-signal", Mode: "guard"}, Target{}, IncomingMessage{
		Source:   "+100",
		Redacted: "이 첨부를 operator@example.com 에게 메일로 보내줘",
	})
	if !strings.Contains(reply, "메일 초안을 만들었습니다") || strings.Contains(reply, "첨부에서 읽은 텍스트입니다") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestGuardReplyRoutesPersonalAssistantAction(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	reply := guardReply(ListenOptions{TargetID: "bot-signal", Mode: "guard"}, Target{}, IncomingMessage{
		Source:   "+100",
		Redacted: "내일 오전 9시에 우유 사기 리마인더 추가해줘",
	})
	if !strings.Contains(reply, "Mac 작업 실행 권한이 필요합니다") ||
		!strings.Contains(reply, "reminder_create") ||
		!strings.Contains(reply, "우유 사기") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestAssistantModeUsesRawSecretLocallyWithoutRepeatingIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	raw := `내 비밀번호는 "홍길동" 이야 메모장에 적어넣어줘.`
	report := guard.Detect("signal-guard", raw)
	reply := guardReply(ListenOptions{TargetID: "bot-signal", Mode: "assistant"}, Target{}, IncomingMessage{
		Source:         "+100",
		Redacted:       report.Redacted,
		raw:            raw,
		SecretDetected: true,
		Intent:         guard.ParseIntent(report.Redacted).Intent,
	})
	if strings.Contains(reply, "알려드릴 수 없습니다") || strings.Contains(reply, "보안상의 이유") || strings.Contains(reply, "Guard") {
		t.Fatalf("assistant should not refuse local secret input: %q", reply)
	}
	if strings.Contains(reply, "홍길동") {
		t.Fatalf("reply repeated raw secret: %q", reply)
	}
}

func TestAssistantModeCanUseVaultCaptureFlow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	target := Target{ID: "bot-signal", Channel: "signal", GroupID: "assistant-group", Mode: "assistant"}
	first := guardReply(ListenOptions{TargetID: "bot-signal", Mode: "assistant"}, target, IncomingMessage{
		Source:   "+100",
		GroupID:  "assistant-group",
		Redacted: "비밀 입력 operator-example api-key local",
	})
	if !strings.Contains(first, "비밀값 저장 준비 완료") ||
		!strings.Contains(first, "저장 대상: operator-example/api-key") ||
		!strings.Contains(first, "backend: local") ||
		strings.Contains(first, "Guard") ||
		strings.Contains(first, "주의:") {
		t.Fatalf("unexpected first reply: %q", first)
	}

	raw := "operator-example-secret-value-1234567890"
	report := guard.Detect("signal-guard", raw)
	second := guardReply(ListenOptions{TargetID: "bot-signal", Mode: "assistant"}, target, IncomingMessage{
		Source:         "+100",
		GroupID:        "assistant-group",
		Redacted:       report.Redacted,
		raw:            raw,
		SecretDetected: true,
		Intent:         guard.ParseIntent(report.Redacted).Intent,
	})
	if !strings.Contains(second, "저장 완료") {
		t.Fatalf("unexpected second reply: %q", second)
	}
	if strings.Contains(second, "operator-example-secret") || strings.Contains(second, "Guard") || strings.Contains(second, "Signal 앱에 남은") {
		t.Fatalf("assistant vault reply leaked detail: %q", second)
	}
}

func TestGuardReplyAcceptsPendingArgosApproval(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ARGOS_PERMISSIONS", filepath.Join(home, "permissions.json"))
	rememberPendingArgosAction(argosPendingKey("bot-signal"), pendingArgosAction{
		Action: osauto.ArgosAction{
			Action:        "open_app",
			App:           "Safari",
			Text:          "Safari 열어줘",
			ReminderTitle: "",
		},
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	})
	reply := guardReply(ListenOptions{TargetID: "bot-signal", Mode: "guard"}, Target{}, IncomingMessage{
		Source:   "+100",
		Redacted: "실행",
	})
	if strings.Contains(reply, "Guard 채널") {
		t.Fatalf("approval should not fall through to guard help: %q", reply)
	}
	if !strings.Contains(reply, "Argos") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestCallSignalVisionModelOpenAICompatiblePayload(t *testing.T) {
	var sawImage bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["model"] != "vision-test" {
			t.Fatalf("model=%v", payload["model"])
		}
		data, _ := json.Marshal(payload)
		sawImage = strings.Contains(string(data), "data:image/png;base64,")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"화면에 로그인 폼과 오류 메시지가 보입니다."}}]}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	image := filepath.Join(dir, "screen.png")
	if err := os.WriteFile(image, []byte("fake png bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MESHCLAW_SIGNAL_VISION_BASE_URL", server.URL)
	t.Setenv("MESHCLAW_SIGNAL_VISION_API_KEY", "test-key")
	got, err := callSignalVisionModel(image, "vision-test", "error")
	if err != nil {
		t.Fatal(err)
	}
	if !sawImage || !strings.Contains(got, "로그인 폼") {
		t.Fatalf("sawImage=%t got=%q", sawImage, got)
	}
}

func TestMaybeSignalImageStatusReply(t *testing.T) {
	t.Setenv("MESHCLAW_SIGNAL_VISION_MODEL", "")
	t.Setenv("MESHCLAW_SIGNAL_VISION_BASE_URL", "")
	reply, handled := maybeSignalImageStatusReply("비전 상태 알려줘")
	if !handled {
		t.Fatal("expected image status request to be handled")
	}
	if !strings.Contains(reply, "Signal 이미지 이해 상태입니다") ||
		!strings.Contains(reply, "OCR") ||
		!strings.Contains(reply, "이미지 설명 모델") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestMaybeSignalAttachmentStatusReply(t *testing.T) {
	t.Setenv("MESHCLAW_SIGNAL_VISION_MODEL", "")
	t.Setenv("MESHCLAW_SIGNAL_VISION_BASE_URL", "")
	reply, handled := maybeSignalAttachmentStatusReply("첨부 상태 알려줘")
	if !handled {
		t.Fatal("expected attachment status request to be handled")
	}
	for _, want := range []string{"Signal 첨부 이해 상태입니다", "이미지", "DOCX", "PDF 텍스트", "PDF 이미지 OCR fallback"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("missing %q in:\n%s", want, reply)
		}
	}
}

func TestSignalPDFThumbnailCandidatesSortsLargestImageFirst(t *testing.T) {
	dir := t.TempDir()
	small := filepath.Join(dir, "small.png")
	large := filepath.Join(dir, "large.jpg")
	if err := os.WriteFile(small, []byte("small"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(large, []byte(strings.Repeat("x", 100)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte(strings.Repeat("x", 200)), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := signalPDFThumbnailCandidates(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != large || got[1] != small {
		t.Fatalf("got=%#v", got)
	}
}

func TestSignalImageStatusReadsDispatcherPlist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"gemma3:4b"}]}`))
	}))
	defer server.Close()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_SIGNAL_VISION_MODEL", "")
	t.Setenv("MESHCLAW_SIGNAL_VISION_BASE_URL", "")
	plist := strings.Join([]string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<plist version="1.0">`,
		`<dict>`,
		`<key>EnvironmentVariables</key>`,
		`<dict>`,
		`<key>MESHCLAW_SIGNAL_VISION_BASE_URL</key>`,
		`<string>` + server.URL + `</string>`,
		`<key>MESHCLAW_SIGNAL_VISION_MODEL</key>`,
		`<string>gemma3:4b</string>`,
		`</dict>`,
		`</dict>`,
		`</plist>`,
	}, "\n")
	path := filepath.Join(home, "Library", "LaunchAgents", "ai.meshclaw.argos-signal-dispatcher.plist")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(plist), 0600); err != nil {
		t.Fatal(err)
	}

	report := signalImageStatus()
	if !report.VisionConfigured || !report.VisionReachable || report.VisionSource != "signal-dispatcher-plist" ||
		report.VisionModel != "gemma3:4b" || report.VisionBaseURL != server.URL {
		t.Fatalf("report=%#v", report)
	}
}

func TestCleanupSignalReplyAttachmentsDeletesOnlyMeshClawRecordings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	recording := filepath.Join(home, ".meshclaw", "recordings", "argos.mov")
	if err := os.MkdirAll(filepath.Dir(recording), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recording, []byte("movie"), 0600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(home, "outside.mov")
	if err := os.WriteFile(outside, []byte("movie"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupSignalReplyAttachments([]string{recording, outside})
	if _, err := os.Stat(recording); !os.IsNotExist(err) {
		t.Fatalf("expected recording to be removed, err=%v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file should remain: %v", err)
	}
}

func TestStripArgosRecordingWords(t *testing.T) {
	got := stripArgosRecordingWords("녹화하면서 가나 경제 뉴스 검색해줘")
	if got != "가나 경제 뉴스 검색해줘" {
		t.Fatalf("got=%q", got)
	}
}

func TestFormatSignalArgosActionTreatsRecordingFailureAsWarning(t *testing.T) {
	action := osauto.ArgosAction{
		Action:        "open_app",
		App:           "Safari",
		RecordingPath: "/tmp/argos-test.mov",
		Result:        &osauto.Result{OK: true, Action: "open_app"},
		Recording:     &osauto.Result{OK: false, Action: "screen_record", Error: "exit status 1"},
	}
	text := formatSignalArgosAction(action, evidence.Record{}, nil)
	if strings.Contains(text, "Argos macOS 작업에 실패") {
		t.Fatalf("recording warning should not fail primary action:\n%s", text)
	}
	if !strings.Contains(text, "Argos가 Mac 작업을 처리했습니다.") || !strings.Contains(text, "작업: open_app") || !strings.Contains(text, "화면 기록 경고: exit status 1") {
		t.Fatalf("missing success or recording warning:\n%s", text)
	}
}

func TestFormatSignalArgosReminderShortcut(t *testing.T) {
	report := osauto.ReminderShortcutReport{
		Kind:          "meshclaw_argos_reminder_shortcut",
		ExpectedNames: []string{"Argos Add Reminder"},
		InputSchema:   `{"title":"string"}`,
		ExampleInput:  `{"title":"우유 사기"}`,
		SetupSteps:    []string{"Shortcuts 앱에서 `Argos Add Reminder`를 만드세요."},
		TestCommand:   "meshclaw automation shortcut-run 'Argos Add Reminder'",
	}
	text := formatSignalArgosReminderShortcut(report, evidence.Record{}, nil)
	if !strings.Contains(text, "설치 필요") || !strings.Contains(text, "Argos Add Reminder") || !strings.Contains(text, "테스트 명령") {
		t.Fatalf("reply=%s", text)
	}
}

func TestIsArgosMacDoctorRequest(t *testing.T) {
	for _, input := range []string{"Argos doctor", "맥 녹화 권한 확인", "화면 기록 권한 체크", "캘린더 권한 확인해줘", "리마인더 권한 상태 봐줘", "맥 권한 확인해줘"} {
		if !isArgosMacDoctorRequest(strings.ToLower(input)) {
			t.Fatalf("expected argos doctor request for %q", input)
		}
	}
	if isArgosMacDoctorRequest("Argos work report") {
		t.Fatal("work report should not be treated as argos doctor")
	}
}

func TestFormatSignalArgosMacPermissionSummaryKeepsCalendarBrief(t *testing.T) {
	report := osauto.ArgosMacDoctorReport{
		Kind: "meshclaw_argos_macos_doctor",
		OK:   true,
		Calendar: osauto.CalendarAutomationReport{
			Kind:     "meshclaw_argos_calendar_automation",
			OK:       true,
			Provider: "Argos UI Runner calendar helper",
		},
		UIRunnerInstall: osauto.UIRunnerInstallReport{
			Kind:          "meshclaw_argos_ui_runner_install",
			OK:            true,
			InstalledPath: "/Users/argos/Applications/Argos UI Runner.app",
			StagedPath:    "/Users/argos/Applications/Argos UI Runner.app.next",
		},
		ScreenRecording: osauto.Result{OK: true},
	}
	text := formatSignalArgosMacPermissionSummary("캘린더 권한 확인해줘", report, evidence.Record{}, nil)
	for _, want := range []string{"Argos Mac 권한 상태입니다.", "캘린더: 정상", "Argos UI Runner calendar helper"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	for _, notWant := range []string{"Runner 설치 상태", "staged Runner", "화면 기록:"} {
		if strings.Contains(text, notWant) {
			t.Fatalf("permission summary should stay brief, found %q in:\n%s", notWant, text)
		}
	}
	if got := len(strings.Split(text, "\n")); got > 6 {
		t.Fatalf("permission summary too long (%d lines):\n%s", got, text)
	}
}

func TestFormatSignalArgosMacPermissionSummaryShowsGeneralBrief(t *testing.T) {
	report := osauto.ArgosMacDoctorReport{
		Kind: "meshclaw_argos_macos_doctor",
		OK:   true,
		Calendar: osauto.CalendarAutomationReport{
			Kind: "meshclaw_argos_calendar_automation",
			OK:   true,
		},
		ReminderShortcut: osauto.ReminderShortcutReport{
			Kind: "meshclaw_argos_reminder_shortcut",
			OK:   true,
		},
		Contacts: osauto.ContactsAutomationReport{
			Kind: "meshclaw_argos_contacts_automation",
			OK:   true,
		},
		ScreenRecording: osauto.Result{OK: true},
	}
	text := formatSignalArgosMacPermissionSummary("맥 권한 확인해줘", report, evidence.Record{}, nil)
	for _, want := range []string{"캘린더: 정상", "리마인더: 정상", "연락처: 정상", "화면 기록: 정상"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	for _, notWant := range []string{"Runner 설치 상태", "경로:", "단축어:"} {
		if strings.Contains(text, notWant) {
			t.Fatalf("general permission summary should stay compact, found %q in:\n%s", notWant, text)
		}
	}
	if got := len(strings.Split(text, "\n")); got > 7 {
		t.Fatalf("general permission summary too long (%d lines):\n%s", got, text)
	}
}

func TestIsArgosMacSetupRequest(t *testing.T) {
	for _, input := range []string{"Argos setup", "아르고스 맥 셋업", "macos setup"} {
		if !isArgosMacSetupRequest(strings.ToLower(input)) {
			t.Fatalf("expected argos setup request for %q", input)
		}
	}
	if isArgosMacSetupRequest("Argos work report") {
		t.Fatal("work report should not be treated as argos setup")
	}
}

func TestFormatSignalArgosMacDoctorShowsRecordingNextActions(t *testing.T) {
	report := osauto.ArgosMacDoctorReport{
		Kind:            "meshclaw_argos_macos_doctor",
		OK:              false,
		OS:              "darwin",
		User:            "odt",
		PermissionsPath: "/Users/argos/.meshclaw/argos-permissions.json",
		Grants:          []osauto.ArgosPermissionGrant{{Action: "open_app", Scope: "safari"}},
		Shortcuts:       osauto.Result{OK: true, Command: []string{"shortcuts", "list"}},
		ScreenRecording: osauto.Result{OK: false, Command: []string{"screencapture"}, Error: "exit status 1"},
		Problems:        []string{"Screen recording test failed: exit status 1"},
		NextActions:     []string{"Grant Screen Recording permission and restart signal-dispatcher."},
	}
	text := formatSignalArgosMacDoctor(report, evidence.Record{}, nil)
	for _, want := range []string{"Argos Mac 비서 진단: 점검 필요", "저장된 실행 권한: 1개", "화면 기록: 점검 필요", "Grant Screen Recording"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}

func TestFormatSignalArgosMacSetupShowsUserAction(t *testing.T) {
	report := osauto.ArgosMacSetupReport{
		Kind:                 "meshclaw_argos_macos_setup",
		OK:                   false,
		UIRunnerURL:          "http://127.0.0.1:48292",
		AccessibilityRequest: osauto.Result{OK: false, Command: []string{"POST", "http://127.0.0.1:48292/request-accessibility"}, Error: "Accessibility permission is still not granted"},
		OpenAccessibility:    osauto.Result{OK: true, Command: []string{"open", "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"}},
		OpenScreenRecording:  osauto.Result{OK: true, Command: []string{"open", "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture"}},
		Doctor: osauto.ArgosMacDoctorReport{
			Kind:            "meshclaw_argos_macos_doctor",
			Grants:          []osauto.ArgosPermissionGrant{{Action: "browser_search", Scope: "web_search"}},
			ScreenRecording: osauto.Result{OK: false, Command: []string{"screencapture"}, Error: "exit status 1"},
		},
		Problems:    []string{"화면 기록 테스트 실패: exit status 1"},
		NextActions: []string{"시스템 설정에서 권한을 허용하세요."},
	}
	text := formatSignalArgosMacSetup(report, evidence.Record{}, nil)
	for _, want := range []string{"Argos Mac 비서 셋업: 사용자 조치 필요", "손쉬운 사용 권한 요청", "손쉬운 사용 설정 화면: 열림", "화면 기록 설정 화면: 열림", "권한 허용 후 `Argos doctor`"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}

func TestParseSignalReceiveSyncSentMessage(t *testing.T) {
	raw := `{"envelope":{"sourceNumber":"+100","timestamp":1770000000000,"syncMessage":{"sentMessage":{"timestamp":1770000000001,"message":"아르고스 상태","groupInfo":{"groupId":"group-console"}}}}}` + "\n"
	events := parseSignalReceive([]byte(raw), Target{GroupID: "group-console"})
	if len(events) != 1 {
		t.Fatalf("events=%#v", events)
	}
	if !events[0].SelfSync {
		t.Fatalf("expected self sync event: %#v", events[0])
	}
	if events[0].Redacted != "아르고스 상태" || events[0].Timestamp != 1770000000001 {
		t.Fatalf("event=%#v", events[0])
	}
	t.Setenv("MESHCLAW_SIGNAL_SELF_NUMBER", "+100")
	if !isSignalSelfEcho(events[0]) {
		t.Fatal("single-account console should be disabled by default")
	}
	t.Setenv("MESHCLAW_SIGNAL_SINGLE_ACCOUNT_CONSOLE", "1")
	if isSignalSelfEcho(events[0]) {
		t.Fatal("explicit single-account console command should not be treated as self echo")
	}
	events[0].Redacted = "브리핑 결과입니다."
	if !isSignalSelfEcho(events[0]) {
		t.Fatal("non-console self sync should remain ignored to prevent loops")
	}
}

func TestBuildApprovalRequest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := filepath.Join(home, ".meshclaw", "evidence", "2026-05-22", "demo")
	if err := os.MkdirAll(bundle, 0700); err != nil {
		t.Fatal(err)
	}
	result := runtimeflow.Result{
		Success:        true,
		Workflow:       "demo",
		Mode:           runtimeflow.DryRun,
		GeneratedAt:    time.Now().UTC(),
		BundleDir:      bundle,
		EvidenceBundle: bundle,
		Summary:        runtimeflow.Summary{Total: 1, Skipped: 1, ApprovalRequired: 1},
		Steps: []runtimeflow.ExecutionResult{{
			Success:          true,
			Workflow:         "demo",
			Step:             "rotate-key",
			Title:            "Rotate key",
			Status:           "approval_pending",
			Action:           "secret_rotate",
			Resource:         "vault://meshclaw/service/key",
			ApprovalRequired: true,
			StrongApproval:   true,
			PolicyDecision:   "require_approval",
			PolicyReason:     "secret rotation requires approval",
			Skipped:          true,
			SkipReason:       "approval required before execution",
		}},
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "execution.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}

	req, err := BuildApprovalRequest(bundle, "rotate-key", "signal", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if !req.Strong || req.Step != "rotate-key" {
		t.Fatalf("request=%#v", req)
	}
	if req.ApprovalCLI == "" || req.Text == "" {
		t.Fatalf("missing approval text: %#v", req)
	}
}

func writeMinimalDocx(path string, paragraphs ...string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	zw := zip.NewWriter(file)
	defer zw.Close()
	if err := writeZipFile(zw, "[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`); err != nil {
		return err
	}
	body := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`
	for _, paragraph := range paragraphs {
		body += `<w:p><w:r><w:t>` + htmlEscapeTest(paragraph) + `</w:t></w:r></w:p>`
	}
	body += `</w:body></w:document>`
	return writeZipFile(zw, "word/document.xml", body)
}

func writeZipFile(zw *zip.Writer, name, body string) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(body))
	return err
}

func htmlEscapeTest(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}
