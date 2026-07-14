package messenger

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestPrewarmChatTargetsOnlyCallsModelBackedConversationTargets(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		calls++
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["model"] != "gpt-oss:20b" {
			t.Fatalf("model=%v", payload["model"])
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ready"}}]}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(dir, "targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "chat", Channel: "signal", GroupID: "group-chat", Mode: "chat", Model: "gpt-oss:20b", BaseURL: server.URL + "/v1"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := UpsertTarget(Target{ID: "briefing", Channel: "signal", GroupID: "group-briefing", Mode: "briefing", Model: "gpt-oss:20b", BaseURL: server.URL + "/v1"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := UpsertTarget(Target{ID: "assistant", Channel: "signal", GroupID: "group-assistant", Mode: "assistant", Model: "gpt-oss:20b", BaseURL: server.URL + "/v1"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := UpsertTarget(Target{ID: "ops", Channel: "signal", GroupID: "group-ops", Mode: "ops"}); err != nil {
		t.Fatal(err)
	}

	results, err := PrewarmChatTargets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("results=%d, want 3: %#v", len(results), results)
	}
	for _, result := range results {
		if !result.OK {
			t.Fatalf("result=%#v", result)
		}
	}
	if calls != 3 {
		t.Fatalf("calls=%d, want 3", calls)
	}
}
