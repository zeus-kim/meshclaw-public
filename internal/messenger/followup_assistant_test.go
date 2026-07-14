package messenger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meshclaw/meshclaw/internal/evidence"
)

func TestIsAssistantFollowUp(t *testing.T) {
	followUps := []string{
		"한글로",
		"한글로 다시 알려줘",
		"영어로 번역해줘",
		"더 자세히",
		"1번 자세히",
		"3번 자세히 설명해줘",
		"두번째 기사 더 알려줘",
		"원문",
		"출처 보여줘",
		"방금 그거 다시 말해줘",
	}
	for _, msg := range followUps {
		if !isAssistantFollowUp(msg) {
			t.Errorf("expected %q to be treated as a follow-up", msg)
		}
	}

	fresh := []string{
		"오늘 주요뉴스",
		"오늘 주요뉴스 5개",
		"서울 오늘 날씨 알려줘",
		"내일 일정 추가해줘 강남역 약속",
		"브라우저에서 경제 뉴스 검색해줘",
		"오늘 할 일 뭐 있어?",
	}
	for _, msg := range fresh {
		if isAssistantFollowUp(msg) {
			t.Errorf("expected %q NOT to be treated as a follow-up", msg)
		}
	}
}

func TestFollowUpNewsIndex(t *testing.T) {
	cases := map[string]int{
		"1번 자세히":        1,
		"2번 더 알려줘":      2,
		"3번 자세히 설명해줘":   3,
		"item 4 detail": 4,
		"두번째 기사":        2,
		"세 번째 자세히":      3,
	}
	for msg, want := range cases {
		got, ok := followUpNewsIndex(msg)
		if !ok || got != want {
			t.Errorf("followUpNewsIndex(%q) = (%d,%v), want (%d,true)", msg, got, ok, want)
		}
	}

	noIndex := []string{"한글로", "더 자세히", "출처", "오늘 주요뉴스"}
	for _, msg := range noIndex {
		if idx, ok := followUpNewsIndex(msg); ok {
			t.Errorf("followUpNewsIndex(%q) unexpectedly returned index %d", msg, idx)
		}
	}
}

func TestBareNewsFollowUpIndex(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	payload := fastNewsBriefingEvidence{
		Kind: "signal_fast_news",
		Text: "news",
		Items: []fastNewsBriefingEvidenceItem{
			{Title: "A", Link: "https://a"},
			{Title: "B", Link: "https://b"},
			{Title: "C", Link: "https://c"},
			{Title: "D", Link: "https://d"},
			{Title: "E", Link: "https://e"},
		},
	}
	if _, err := evidence.Store("news-brief", "rss", "signal_fast_news", payload); err != nil {
		t.Fatal(err)
	}

	if idx, ok := bareNewsFollowUpIndex("5"); !ok || idx != 5 {
		t.Fatalf("bareNewsFollowUpIndex(5) = (%d,%v), want (5,true)", idx, ok)
	}
	if _, ok := bareNewsFollowUpIndex("9"); ok {
		t.Fatal("expected 9 to be out of range")
	}
	if !isAssistantFollowUp("5") {
		t.Fatal("expected bare 5 to be treated as follow-up when news evidence exists")
	}
}

func TestAssistantFollowUpBareDigitReply(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	payload := fastNewsBriefingEvidence{
		Kind: "signal_fast_news",
		Text: "news",
		Items: []fastNewsBriefingEvidenceItem{
			{Title: "첫 번째 기사", Link: "https://a", Description: "요약 A"},
			{Title: "두 번째 기사", Link: "https://b", Description: "요약 B"},
		},
	}
	if _, err := evidence.Store("news-brief", "rss", "signal_fast_news", payload); err != nil {
		t.Fatal(err)
	}

	opts := ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}
	for _, msg := range []string{"1", "1번", "1번 자세히"} {
		reply, handled := assistantFollowUpReply(opts, msg)
		if !handled {
			t.Fatalf("assistantFollowUpReply(%q) was not handled", msg)
		}
		if !strings.Contains(reply, "첫 번째 기사") {
			t.Fatalf("assistantFollowUpReply(%q) = %q", msg, reply)
		}
	}
}

func TestAssistantReplyMailOrdinalDraftDoesNotFallThroughToNews(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	payload := fastNewsBriefingEvidence{
		Kind: "signal_fast_news",
		Text: "news",
		Items: []fastNewsBriefingEvidenceItem{
			{Title: "첫 번째 기사", Link: "https://a", Description: "요약 A"},
		},
	}
	if _, err := evidence.Store("news-brief", "rss", "signal_fast_news", payload); err != nil {
		t.Fatal(err)
	}

	reply := assistantReply(ListenOptions{TargetID: "mail-ordinal-no-context", Mode: "assistant"}, "첫 번째 메일에 정중하게 답장 초안 써줘")
	if strings.Contains(reply, "1번 뉴스") || strings.Contains(reply, "첫 번째 기사") {
		t.Fatalf("mail ordinal draft request fell through to news reply: %q", reply)
	}
	if !strings.Contains(reply, "최근 메일 목록") {
		t.Fatalf("expected mail-context guidance, got: %q", reply)
	}
}

func TestAssistantScenarioMailOrdinalDraftDoesNotFallThroughToNews(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	payload := fastNewsBriefingEvidence{
		Kind: "signal_fast_news",
		Text: "news",
		Items: []fastNewsBriefingEvidenceItem{
			{Title: "첫 번째 기사", Link: "https://a", Description: "요약 A"},
		},
	}
	if _, err := evidence.Store("news-brief", "rss", "signal_fast_news", payload); err != nil {
		t.Fatal(err)
	}

	reply, handled := assistantScenarioReply(ListenOptions{TargetID: "mail-ordinal-scenario-no-context", Mode: "assistant"}, "첫 번째 메일에 정중하게 답장 초안 써줘")
	if !handled {
		t.Fatal("expected mail draft follow-up to be handled before news scenario")
	}
	if strings.Contains(reply, "1번 뉴스") || strings.Contains(reply, "첫 번째 기사") {
		t.Fatalf("mail ordinal draft request fell through to scenario news reply: %q", reply)
	}
	if !strings.Contains(reply, "최근 메일 목록") {
		t.Fatalf("expected mail-context guidance, got: %q", reply)
	}
}

func TestLoadLatestNewsRecordFromAssistantNews(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if _, err := evidence.Store("assistant-news", "argos-assistant", "signal_fast_news", map[string]interface{}{
		"text": "오늘 주요뉴스",
		"news": []map[string]string{
			{"title": "테스트 헤드라인", "link": "https://example.com", "feed_title": "테스트", "description": "본문 요약"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	record, path, ok := loadLatestNewsRecord()
	if !ok {
		t.Fatal("expected assistant-news evidence to load")
	}
	if !strings.Contains(path, "assistant-news") {
		t.Fatalf("unexpected evidence path: %s", path)
	}
	if len(record.Payload.Items) != 1 || record.Payload.Items[0].Title != "테스트 헤드라인" {
		t.Fatalf("unexpected items: %+v", record.Payload.Items)
	}
	_ = filepath.Base(path)
}

func TestAssistantFollowUpProductionHome(t *testing.T) {
	home := strings.TrimSpace(os.Getenv("MESHCLAW_VERIFY_HOME"))
	if home == "" {
		t.Skip("set MESHCLAW_VERIFY_HOME to run production verification")
	}
	t.Setenv("HOME", home)
	opts := ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}
	reply, handled := assistantFollowUpReply(opts, "1")
	if !handled {
		t.Fatal("expected bare 1 to resolve against stored news evidence")
	}
	if strings.Contains(reply, "무엇을 의미") || strings.Contains(reply, "모르겠") {
		t.Fatalf("model confusion reply: %q", reply)
	}
	if !strings.Contains(reply, "번 뉴스") {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestIsNewsDocumentRequest(t *testing.T) {
	if !isNewsDocumentRequest("오늘의 주요뉴스를 문서로 작성해서 보내줘") {
		t.Fatal("expected news document request")
	}
	if isNewsDocumentRequest("오늘 주요뉴스") {
		t.Fatal("plain news request should not match document intent")
	}
	if isNewsDocumentRequest("오늘의 주요뉴스를 음성 파일로 보내줘") {
		t.Fatal("voice news request should not match document intent")
	}
	if !isNewsVoiceRequest("오늘의 주요뉴스를 음성 파일로 보내줘") {
		t.Fatal("expected news voice request")
	}
}
