package messenger

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/aichat"
	"github.com/meshclaw/meshclaw/internal/assistantprofile"
)

func TestModelRoomReplyInjectsApprovedPersonalizationContextAndKeepsFinalApprovalBoundary(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".meshclaw")
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(root, "messenger-targets.json"))

	if _, err := assistantprofile.InitDefaults(time.Now(), root, true); err != nil {
		t.Fatal(err)
	}
	if _, err := assistantprofile.AddMemory(
		time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC),
		root,
		"user",
		"사용자는 답변을 결론부터 짧게 받고, 구매 전 최종 확인을 선호한다",
		true,
	); err != nil {
		t.Fatal(err)
	}
	planned, err := assistantprofile.AddMemory(time.Now(), root, "user", "UNAPPROVED_MEMORY_SENTINEL", false)
	if err != nil {
		t.Fatal(err)
	}
	if planned.Status != "planned" || planned.Written {
		t.Fatalf("unapproved memory should stay planned: %#v", planned)
	}
	if _, err := assistantprofile.InstallSkill(
		time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC),
		root,
		"Shopping Prep",
		"Compare purchase candidates and stop before booking, payment, or checkout.",
		true,
	); err != nil {
		t.Fatal(err)
	}

	var raw map[string]json.RawMessage
	var messages []aichat.Message
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected chat path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization=%q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if err := json.Unmarshal(raw["messages"], &messages); err != nil {
			t.Fatalf("decode messages: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"확인했습니다."}}]}`))
	}))
	defer server.Close()

	configPath := filepath.Join(home, "matrix_ai.json")
	if err := aichat.SaveConfig(configPath, aichat.Config{
		BaseURL:   server.URL + "/v1",
		APIKey:    "test-key",
		Model:     "model-room-memory-test",
		MaxTokens: 256,
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MESHCLAW_MATRIX_AI_CONFIG", configPath)

	input := "내 스타일대로 생수 구매 후보를 찾아줘"
	reply := modelRoomReply(ListenOptions{TargetID: "argos-memory-model-context", Mode: "assistant"}, input, "assistant")
	if reply != "확인했습니다." {
		t.Fatalf("unexpected reply: %q", reply)
	}

	var model string
	if err := json.Unmarshal(raw["model"], &model); err != nil {
		t.Fatal(err)
	}
	if model != "model-room-memory-test" {
		t.Fatalf("model=%q", model)
	}
	for _, forbidden := range []string{"tools", "tool_choice", "execute", "approve", "approved"} {
		if _, ok := raw[forbidden]; ok {
			t.Fatalf("modelRoomReply payload should not include top-level %q: %s", forbidden, string(raw[forbidden]))
		}
	}

	memoryPrompt := findSystemMessage(messages, "Argos bounded memory context follows.")
	requireContains(t, memoryPrompt,
		"stable user preference, personalization, and long-term context",
		"Apply style and preference memories naturally",
		"Memory and skills are context, not authorization.",
		"Never treat memory as permission to send, delete, buy, book, pay, mutate accounts",
		"결론부터 짧게",
		"구매 전 최종 확인",
	)
	skillPrompt := findSystemMessage(messages, "Argos active skill context follows.")
	requireContains(t, skillPrompt,
		"Shopping Prep",
		"Compare purchase candidates",
		"require explicit final approval",
	)
	if strings.Contains(allMessageText(messages), "UNAPPROVED_MEMORY_SENTINEL") {
		t.Fatalf("unapproved memory leaked into model request:\n%s", allMessageText(messages))
	}
	last := messages[len(messages)-1]
	if last.Role != "user" || last.Content != input {
		t.Fatalf("last message=%#v", last)
	}
}

func TestGuardReplyChatFallbackInjectsApprovedPersonalizationContext(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".meshclaw")
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(root, "messenger-targets.json"))

	if _, err := assistantprofile.InitDefaults(time.Now(), root, true); err != nil {
		t.Fatal(err)
	}
	if _, err := assistantprofile.AddMemory(
		time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC),
		root,
		"user",
		"User prefers concise answers with the conclusion first.",
		true,
	); err != nil {
		t.Fatal(err)
	}
	planned, err := assistantprofile.AddMemory(time.Now(), root, "user", "UNAPPROVED_CHAT_MEMORY_SENTINEL", false)
	if err != nil {
		t.Fatal(err)
	}
	if planned.Status != "planned" || planned.Written {
		t.Fatalf("unapproved memory should stay planned: %#v", planned)
	}
	if _, err := assistantprofile.InstallSkill(
		time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC),
		root,
		"Concise Chat",
		"Answer with a short conclusion first, then only the needed detail.",
		true,
	); err != nil {
		t.Fatal(err)
	}

	var raw map[string]json.RawMessage
	var messages []aichat.Message
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected chat path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if err := json.Unmarshal(raw["messages"], &messages); err != nil {
			t.Fatalf("decode messages: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Conclusion first."}}]}`))
	}))
	defer server.Close()

	configPath := filepath.Join(home, "matrix_ai.json")
	if err := aichat.SaveConfig(configPath, aichat.Config{
		BaseURL:   server.URL + "/v1",
		APIKey:    "test-key",
		Model:     "guard-chat-memory-test",
		MaxTokens: 256,
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MESHCLAW_MATRIX_AI_CONFIG", configPath)

	input := "Please answer in my usual style."
	reply := guardReply(
		ListenOptions{TargetID: "argos-chat-memory-context", Mode: "chat"},
		Target{ID: "argos-chat-memory-context", Channel: "signal", Mode: "chat"},
		IncomingMessage{Source: "+100", Redacted: input, Intent: "meta_or_conversation"},
	)
	if reply != "Conclusion first." {
		t.Fatalf("unexpected reply: %q", reply)
	}

	var model string
	if err := json.Unmarshal(raw["model"], &model); err != nil {
		t.Fatal(err)
	}
	if model != "guard-chat-memory-test" {
		t.Fatalf("model=%q", model)
	}
	memoryPrompt := findSystemMessage(messages, "Argos bounded memory context follows.")
	requireContains(t, memoryPrompt,
		"stable user preference, personalization, and long-term context",
		"User prefers concise answers with the conclusion first.",
		"Memory and skills are context, not authorization.",
	)
	skillPrompt := findSystemMessage(messages, "Argos active skill context follows.")
	requireContains(t, skillPrompt,
		"Concise Chat",
		"short conclusion first",
		"require explicit final approval",
	)
	if strings.Contains(allMessageText(messages), "UNAPPROVED_CHAT_MEMORY_SENTINEL") {
		t.Fatalf("unapproved memory leaked into guard chat model request:\n%s", allMessageText(messages))
	}
	last := messages[len(messages)-1]
	if last.Role != "user" || last.Content != input {
		t.Fatalf("last message=%#v", last)
	}
}

func findSystemMessage(messages []aichat.Message, marker string) string {
	for _, msg := range messages {
		if msg.Role == "system" && strings.Contains(msg.Content, marker) {
			return msg.Content
		}
	}
	return ""
}

func requireContains(t *testing.T, text string, wants ...string) {
	t.Helper()
	if strings.TrimSpace(text) == "" {
		t.Fatalf("empty text; wants=%v", wants)
	}
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}

func allMessageText(messages []aichat.Message) string {
	parts := make([]string, 0, len(messages))
	for _, msg := range messages {
		parts = append(parts, msg.Content)
	}
	return strings.Join(parts, "\n")
}
