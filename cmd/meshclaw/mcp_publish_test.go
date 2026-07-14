package main

import (
	"testing"

	"github.com/meshclaw/meshclaw/internal/evidence"
)

func TestMCPToolsExposeUnifiedPublishTools(t *testing.T) {
	tools := map[string]mcpTool{}
	for _, tool := range mcpTools() {
		tools[tool.Name] = tool
	}
	for _, name := range []string{"meshclaw_news_document", "meshclaw_argos_research"} {
		tool, ok := tools[name]
		if !ok {
			t.Fatalf("missing MCP tool %s", name)
		}
		if tool.Description == "" {
			t.Fatalf("%s has empty description", name)
		}
	}
	if _, ok := tools["meshclaw_argos_research"].InputSchema.Properties["query"]; !ok {
		t.Fatalf("meshclaw_argos_research should require a query property")
	}
	for _, name := range []string{"meshclaw_news_document", "meshclaw_argos_research"} {
		target, ok := tools[name].InputSchema.Properties["target"]
		if !ok {
			t.Fatalf("%s should expose publication target", name)
		}
		if target.Default != "macmini" {
			t.Fatalf("%s target default=%v, want macmini", name, target.Default)
		}
		if _, ok := tools[name].InputSchema.Properties["record_artifact"]; !ok {
			t.Fatalf("%s should expose record_artifact", name)
		}
		if _, ok := tools[name].InputSchema.Properties["mission_id"]; !ok {
			t.Fatalf("%s should expose mission_id", name)
		}
	}
}

func TestMCPSurfaceIncludesUnifiedPublishTools(t *testing.T) {
	guide := mcpSurfaceGuide()
	defaultTools, ok := guide["default_tools"].([]string)
	if !ok {
		t.Fatalf("default_tools type = %T, want []string", guide["default_tools"])
	}
	seen := map[string]bool{}
	for _, name := range defaultTools {
		seen[name] = true
	}
	for _, name := range []string{"meshclaw_news_document", "meshclaw_argos_research"} {
		if !seen[name] {
			t.Fatalf("default_tools missing %s", name)
		}
	}
}

func TestParseRemoteMCPResponsePayload(t *testing.T) {
	stdout := `{"jsonrpc":"2.0","result":{"content":[{"type":"text","text":"{\"target\":\"local\",\"ok\":true}"}]},"id":1}` + "\n"
	resp, payload, err := parseRemoteMCPResponse(stdout)
	if err != nil {
		t.Fatalf("parseRemoteMCPResponse error: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected remote error: %#v", resp.Error)
	}
	if payload["target"] != "local" || payload["ok"] != true {
		t.Fatalf("payload=%#v", payload)
	}
}

func TestPublicationTargetDefaultsToMacmini(t *testing.T) {
	want := "macmini"
	if isMacminiRuntime() {
		want = "local"
	}
	if got := publicationTarget(map[string]interface{}{}); got != want {
		t.Fatalf("empty target=%q, want %s", got, want)
	}
	if got := publicationTarget(map[string]interface{}{"target": "local"}); got != "local" {
		t.Fatalf("local target=%q, want local", got)
	}
}

func TestRecordPublicationArtifactCanBeSkipped(t *testing.T) {
	got := recordPublicationArtifact(
		map[string]interface{}{"record_artifact": false},
		"macmini",
		"doc",
		"title",
		"/Users/argos/Documents/Argos Vault/Work Reports/foo.md",
		"http://127.0.0.1/foo",
		evidence.Record{ID: "ev-001"},
	)
	if got["recorded"] != false || got["skipped"] != true {
		t.Fatalf("recordPublicationArtifact skip = %#v", got)
	}
}

func TestRemotePayloadHelpers(t *testing.T) {
	remote := map[string]interface{}{
		"payload": map[string]interface{}{
			"report": map[string]interface{}{"path": "/tmp/report.md"},
			"links":  []interface{}{"http://127.0.0.1/report"},
		},
	}
	payload := remotePayload(remote)
	if got := nestedString(payload, "report", "path"); got != "/tmp/report.md" {
		t.Fatalf("nestedString=%q", got)
	}
	if got := firstRemoteLink(payload); got != "http://127.0.0.1/report" {
		t.Fatalf("firstRemoteLink=%q", got)
	}
}
