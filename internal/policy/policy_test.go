package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEvaluateConfiguredPolicyOverridesBuiltIn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.json")
	data := `{
  "version": "1",
  "rules": [
    {
      "id": "deny-codex-fleet-scan",
      "subject": "codex",
      "action": "fleet_scan",
      "resource": "server",
      "decision": "deny",
      "reason": "test override"
    }
  ]
}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MESHCLAW_POLICY_FILE", path)

	result := Evaluate(Request{Subject: "codex", Action: "fleet_scan", Resource: "server"})
	if result.Decision != Deny {
		t.Fatalf("decision = %q, want deny", result.Decision)
	}
	if result.Source != "config" || result.RuleID != "deny-codex-fleet-scan" {
		t.Fatalf("result = %#v, want config rule metadata", result)
	}
}

func TestEvaluateUsesDefaultPresetWhenConfigFileIsMissing(t *testing.T) {
	t.Setenv("MESHCLAW_POLICY_FILE", filepath.Join(t.TempDir(), "missing.json"))

	result := Evaluate(Request{Subject: "codex", Action: "fleet_scan", Resource: "server"})
	if result.Decision != Allow {
		t.Fatalf("decision = %q, want allow", result.Decision)
	}
	if result.Source != "config" {
		t.Fatalf("source = %q, want config", result.Source)
	}
}

func TestContextRuleMatchesSubstring(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.json")
	data := `{
  "version": "1",
  "rules": [
    {
      "id": "block-shadow",
      "action": "run_command",
      "resource": "server",
      "context": "/etc/shadow",
      "decision": "deny"
    }
  ]
}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MESHCLAW_POLICY_FILE", path)

	result := Evaluate(Request{Subject: "operator", Action: "run_command", Resource: "server", Context: "cat /etc/shadow"})
	if result.Decision != Deny || result.RuleID != "block-shadow" {
		t.Fatalf("result = %#v, want configured deny", result)
	}
}

func TestDevOpsPresetAllowsLocalReadOnlyButGatesMutatingActions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.json")
	data, err := json.Marshal(PresetConfig("devops"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MESHCLAW_POLICY_FILE", path)

	readOnly := Evaluate(Request{Subject: "local-llm", Action: "fleet_scan", Resource: "server"})
	if readOnly.Decision != Allow {
		t.Fatalf("readOnly decision = %q, want allow", readOnly.Decision)
	}

	mutating := Evaluate(Request{Subject: "local-llm", Action: "run_command", Resource: "server", Context: "systemctl restart nginx"})
	if mutating.Decision != RequireApproval {
		t.Fatalf("mutating decision = %q, want require_approval", mutating.Decision)
	}
}

func TestOperationalMutationsHaveSpecificApprovalReasons(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.json")
	if err := os.WriteFile(path, []byte(`{"version":"1","rules":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MESHCLAW_POLICY_FILE", path)

	cases := []string{"cloudflare_dns_change", "email_send", "account_configure", "account_delete", "provider_token_use", "screenshot_capture", "data_clean_apply", "service_quarantine", "service_remove", "container_restart", "container_recreate", "container_pull_redeploy", "autoheal_apply"}
	for _, action := range cases {
		result := Evaluate(Request{Subject: "meshclaw-runtime", Action: action, Resource: "server"})
		if result.Decision != RequireApproval {
			t.Fatalf("%s decision = %q, want require_approval", action, result.Decision)
		}
		if result.Reason == "unknown action requires policy approval" {
			t.Fatalf("%s used generic reason: %#v", action, result)
		}
	}
}

func TestDocumentationCheckIsReadOnly(t *testing.T) {
	result := Evaluate(Request{Subject: "meshclaw-runtime", Action: "documentation_check", Resource: "mox-docs"})
	if result.Decision != Allow {
		t.Fatalf("documentation_check decision = %q, want allow", result.Decision)
	}
}

func TestServiceDiagnosticsAreReadOnly(t *testing.T) {
	cases := []string{"service_check", "service_audit", "service_triage", "fleet_service_audit", "read_only_diagnosis"}
	for _, action := range cases {
		result := Evaluate(Request{Subject: "codex", Action: action, Resource: "server"})
		if result.Decision != Allow {
			t.Fatalf("%s decision = %q, want allow", action, result.Decision)
		}
	}
}
