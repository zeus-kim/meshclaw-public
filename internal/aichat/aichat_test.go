package aichat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCleanResponseRemovesDecorativeEmoji(t *testing.T) {
	input := "### ⚙️ 판단\n안녕하세요 😊\n- 다음 행동"
	got := CleanResponse(input)
	if strings.Contains(got, "⚙") || strings.Contains(got, "😊") {
		t.Fatalf("expected decorative emoji removed, got %q", got)
	}
	if !strings.Contains(got, "판단") || !strings.Contains(got, "다음 행동") {
		t.Fatalf("expected content preserved, got %q", got)
	}
}

func TestChatRetriesReasoningOnlyResponse(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if calls == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"reasoning":"thinking only"}}]}`))
			return
		}
		if payload["max_tokens"].(float64) < 2048 {
			t.Fatalf("retry max_tokens too small: %v", payload["max_tokens"])
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"최종 답변"}}]}`))
	}))
	defer server.Close()

	reply, err := NewClient(Config{BaseURL: server.URL + "/v1", APIKey: "test", Model: "gpt-oss:20b", MaxTokens: 128}).Chat(context.Background(), nil, "ping")
	if err != nil {
		t.Fatal(err)
	}
	if reply != "최종 답변" || calls != 2 {
		t.Fatalf("reply=%q calls=%d", reply, calls)
	}
}
