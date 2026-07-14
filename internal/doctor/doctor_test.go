package doctor

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSelfDoctorReportsRuntimeChecks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_POLICY_FILE", filepath.Join(home, "missing-policy.json"))
	t.Setenv("MESHCLAW_VSSH_BINARY", filepath.Join(home, "missing-vssh"))

	report := Self()

	if report.Kind != "meshclaw_doctor" {
		t.Fatalf("kind=%q", report.Kind)
	}
	if report.Status == "" {
		t.Fatalf("status is empty: %#v", report)
	}
	for _, name := range []string{"meshclaw_binary", "vssh_binary", "inventory", "policy", "builtin_workflow", "evidence_dir", "mcp_server"} {
		if !hasCheck(report, name) {
			t.Fatalf("missing check %s: %#v", name, report.Checks)
		}
	}
	if len(report.NextActions) == 0 {
		t.Fatalf("missing next actions")
	}
}

func TestFormatSelf(t *testing.T) {
	text := FormatSelf(SelfReport{
		Kind:   "meshclaw_doctor",
		Status: "ok",
		Checks: []SelfCheck{
			{Name: "meshclaw_binary", Status: "ok", Detail: "/tmp/meshclaw"},
		},
		NextActions: []string{"Run dry-run."},
	})
	if text == "" || !strings.Contains(text, "meshclaw doctor status=ok") || !strings.Contains(text, "meshclaw_binary") {
		t.Fatalf("unexpected text: %s", text)
	}
}

func hasCheck(report SelfReport, name string) bool {
	for _, check := range report.Checks {
		if check.Name == name {
			return true
		}
	}
	return false
}
