package messenger

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meshclaw/meshclaw/internal/aichat"
)

func TestAssistantMemoryIntentUsesModelForNaturalCreateWithoutKeyword(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ASSISTANT_MEMORY_INTENT_MODEL", "1")
	t.Setenv("MESHCLAW_ASSISTANT_MEMORY_INTENT_TIMEOUT_MS", "1500")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		all := ""
		for _, msg := range payload.Messages {
			all += msg.Content + "\n"
		}
		if !strings.Contains(all, "pending_memory_approval") || !strings.Contains(all, "앞으로 답은 결론부터 짧게 해줘") {
			t.Fatalf("classifier prompt missing required context:\n%s", all)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"intent\":\"create\",\"scope\":\"assistant\",\"memory_text\":\"답변은 결론부터 짧게 한다\",\"confidence\":0.93,\"reason\":\"stable assistant response-style preference\"}"}}]}`))
	}))
	defer server.Close()

	configPath := filepath.Join(home, "matrix_ai.json")
	config := map[string]interface{}{
		"base_url":    server.URL + "/v1",
		"api_key":     "test",
		"model":       "memory-intent-test",
		"max_tokens":  128,
		"temperature": 0,
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MESHCLAW_MATRIX_AI_CONFIG", configPath)

	reply := assistantReply(
		ListenOptions{TargetID: "argos-assistant-memory-intent-model", Mode: "assistant"},
		"앞으로 답은 결론부터 짧게 해줘",
	)
	for _, want := range []string{
		"Argos memory에 저장할까요",
		"범위: assistant",
		"답변은 결론부터 짧게 한다",
		"저장해",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
}

func TestGuardReplyRoutesMemoryIntentBeforeConversationalFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ASSISTANT_MEMORY_INTENT_MODEL", "1")
	t.Setenv("MESHCLAW_ASSISTANT_MEMORY_INTENT_TIMEOUT_MS", "1500")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"intent\":\"create\",\"scope\":\"assistant\",\"category\":\"assistant_style\",\"is_stable_memory\":true,\"is_one_off_task\":false,\"memory_text\":\"답변은 결론부터 짧게 한다\",\"confidence\":0.93}"}}]}`))
	}))
	defer server.Close()

	configPath := filepath.Join(home, "matrix_ai.json")
	config := map[string]interface{}{
		"base_url":   server.URL + "/v1",
		"api_key":    "test",
		"model":      "memory-intent-test",
		"max_tokens": 128,
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MESHCLAW_MATRIX_AI_CONFIG", configPath)

	reply := guardReply(
		ListenOptions{TargetID: "argos-assistant-memory-signal-route", Mode: "assistant"},
		Target{ID: "argos-assistant-memory-signal-route", Channel: "signal", Mode: "assistant"},
		IncomingMessage{Source: "+100", Redacted: "앞으로 답은 결론부터 짧게 해줘"},
	)
	for _, want := range []string{
		"Argos memory에 저장할까요",
		"범위: assistant",
		"답변은 결론부터 짧게 한다",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "알겠습니다") {
		t.Fatalf("memory intent should not be swallowed by conversational fallback:\n%s", reply)
	}
}

func TestAssistantMemoryIntentFallbackStillSupportsExplicitRecall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ASSISTANT_MEMORY_INTENT_MODEL", "0")
	reply := assistantReply(ListenOptions{TargetID: "argos-assistant-memory-intent-fallback", Mode: "assistant"}, "기억 확인")
	if !strings.Contains(reply, "현재 승인된 Argos memory") && !strings.Contains(reply, "아직 승인된 assistant memory가 없습니다") {
		t.Fatalf("fallback recall not handled:\n%s", reply)
	}
}

func TestAssistantMemoryIntentRejectsSmallModelTaskCreate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_ASSISTANT_MEMORY_INTENT_MODEL", "1")
	t.Setenv("MESHCLAW_ASSISTANT_MEMORY_INTENT_TIMEOUT_MS", "1500")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"intent\":\"create\",\"scope\":\"user\",\"category\":\"schedule\",\"is_stable_memory\":false,\"is_one_off_task\":true,\"memory_text\":\"내일 아침 8시에 뉴스 알려줘\",\"confidence\":0.95,\"reason\":\"schedule/reminder request, not long-term memory\"}"}}]}`))
	}))
	defer server.Close()

	configPath := filepath.Join(home, "matrix_ai.json")
	config := map[string]interface{}{
		"base_url":   server.URL + "/v1",
		"api_key":    "test",
		"model":      "small-memory-intent-test",
		"max_tokens": 128,
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MESHCLAW_MATRIX_AI_CONFIG", configPath)

	if intent, ok := classifyAssistantMemoryIntent("내일 아침 8시에 뉴스 알려줘", false); ok {
		t.Fatalf("task-like model create should be rejected, got %#v", intent)
	}
}

func TestAssistantMemoryIntentRepairsSmallModelUpdateWithoutTarget(t *testing.T) {
	intent, ok := normalizeAssistantMemoryIntent(assistantMemoryIntent{
		Kind:       "update",
		Scope:      "assistant",
		MemoryText: "답변은 결론부터 짧게 한다",
		Confidence: 0.85,
	}, "앞으로 답은 결론부터 짧게 해줘", false)
	if !ok {
		t.Fatal("expected targetless stable update to be repaired")
	}
	if intent.Kind != assistantMemoryIntentCreate {
		t.Fatalf("intent.Kind=%q, want create", intent.Kind)
	}
}

func TestParseAssistantMemoryIntentJSONToleratesSmallModelFieldTypes(t *testing.T) {
	intent, ok := parseAssistantMemoryIntentJSON(`Here is JSON:
{"intent":"remember","scope":"assistant","memory_text":"답변은 결론부터 짧게 한다","confidence":"high","is_stable_memory":"yes","is_one_off_task":"no"}`)
	if !ok {
		t.Fatal("expected loose JSON parse to succeed")
	}
	intent, ok = normalizeAssistantMemoryIntent(intent, "앞으로 답은 결론부터 짧게 해줘", false)
	if !ok {
		t.Fatal("expected normalized loose intent to be accepted")
	}
	if intent.Kind != assistantMemoryIntentCreate || intent.Scope != "assistant" {
		t.Fatalf("unexpected normalized intent: %#v", intent)
	}
}

func TestAssistantMemoryIntentModelConfigUsesDedicatedOverrides(t *testing.T) {
	t.Setenv("MESHCLAW_ASSISTANT_MEMORY_INTENT_BASE_URL", "http://g4:11434/v1")
	t.Setenv("MESHCLAW_ASSISTANT_MEMORY_INTENT_API_KEY", "memory-key")
	t.Setenv("MESHCLAW_ASSISTANT_MEMORY_INTENT_MODEL_NAME", "gemma3:4b")

	cfg := assistantMemoryIntentModelConfig(aichat.Config{
		BaseURL: "http://localhost:11434/v1",
		APIKey:  "ollama",
		Model:   "gemma4:e4b",
	})
	if cfg.BaseURL != "http://g4:11434/v1" || cfg.APIKey != "memory-key" || cfg.Model != "gemma3:4b" {
		t.Fatalf("unexpected dedicated memory intent config: %#v", cfg)
	}
}
