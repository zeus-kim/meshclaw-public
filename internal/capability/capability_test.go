package capability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestListMergesConfiguredAndInventoryCapabilities(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_CAPABILITY_FILE", filepath.Join(dir, "capabilities.json"))
	t.Setenv("MESHCLAW_INVENTORY_FILE", filepath.Join(dir, "inventory.json"))

	writeJSON(t, filepath.Join(dir, "inventory.json"), map[string]interface{}{
		"version": 1,
		"nodes": []map[string]interface{}{
			{
				"name":   "g1",
				"role":   "ollama-worker",
				"tags":   []string{"linux", "gpu"},
				"online": true,
			},
			{
				"name":   "s2",
				"role":   "nas",
				"tags":   []string{"linux", "storage"},
				"online": true,
			},
		},
	})
	writeJSON(t, filepath.Join(dir, "capabilities.json"), Store{
		Version: storeVersion,
		Capabilities: []Capability{
			{
				ID:           "custom-api",
				Kind:         KindAPI,
				Provider:     "example",
				Description:  "configured capability",
				Capabilities: []string{"chat"},
				Status:       "available",
				SecretPolicy: SecretUseOnly,
				Policy:       "use-only",
			},
		},
	})

	caps := List()
	for _, id := range []string{"custom-api", "g1-node", "g1-gpu-compute", "s2-storage"} {
		if !hasCapability(caps, id) {
			t.Fatalf("expected capability %s in %#v", id, caps)
		}
	}
}

func TestInitWritesRegistry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_CAPABILITY_FILE", filepath.Join(dir, "capabilities.json"))
	t.Setenv("MESHCLAW_INVENTORY_FILE", filepath.Join(dir, "inventory.json"))
	writeJSON(t, filepath.Join(dir, "inventory.json"), map[string]interface{}{
		"version": 1,
		"nodes": []map[string]interface{}{
			{"name": "c1", "role": "mail-server", "tags": []string{"linux", "mail"}, "online": true},
		},
	})

	caps, err := Init(false)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCapability(caps, "c1-mail") {
		t.Fatalf("expected c1-mail capability in %#v", caps)
	}
	if _, err := os.Stat(Path()); err != nil {
		t.Fatal(err)
	}
}

func TestValidateReportsInvalidRegistry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capabilities.json")
	writeJSON(t, path, Store{
		Version: storeVersion,
		Capabilities: []Capability{
			{
				ID:           "bad",
				Kind:         Kind("unknown"),
				SecretPolicy: SecretPolicy("reveal"),
			},
			{
				ID:       "bad",
				Kind:     KindAPI,
				Provider: "example",
			},
		},
	})

	report := Validate(path)
	if report.Valid {
		t.Fatalf("expected invalid report: %#v", report)
	}
	if len(report.Errors) < 4 {
		t.Fatalf("expected validation errors, got %#v", report.Errors)
	}
	if report.Count != 2 {
		t.Fatalf("normalized count = %d, want 2", report.Count)
	}
}

func TestValidateAcceptsInitializedRegistry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_CAPABILITY_FILE", filepath.Join(dir, "capabilities.json"))
	if _, err := Init(true); err != nil {
		t.Fatal(err)
	}
	report := Validate("")
	if !report.Valid {
		t.Fatalf("expected valid report: %#v", report)
	}
	if report.Count == 0 {
		t.Fatalf("expected capabilities in report")
	}
}

func TestRecommendRanksModelCapabilities(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_CAPABILITY_FILE", filepath.Join(dir, "capabilities.json"))
	t.Setenv("MESHCLAW_INVENTORY_FILE", filepath.Join(dir, "inventory.json"))
	writeJSON(t, filepath.Join(dir, "inventory.json"), map[string]interface{}{
		"version": 1,
		"nodes": []map[string]interface{}{
			{"name": "g1", "role": "ollama-worker", "tags": []string{"linux", "gpu"}, "online": true},
			{"name": "c1", "role": "mail-server", "tags": []string{"linux", "mail"}, "online": true},
		},
	})

	report := Recommend("run local ollama model inference on a gpu node")
	if report.Class != "model" {
		t.Fatalf("class=%q want model", report.Class)
	}
	if len(report.Candidates) == 0 {
		t.Fatalf("expected candidates: %#v", report)
	}
	if report.Candidates[0].Capability.ID != "g1-gpu-compute" {
		t.Fatalf("top candidate=%s want g1-gpu-compute; candidates=%#v", report.Candidates[0].Capability.ID, report.Candidates)
	}
}

func TestRecommendMarksApprovalGatedCapabilities(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capabilities.json")
	t.Setenv("MESHCLAW_CAPABILITY_FILE", path)
	writeJSON(t, path, Store{
		Version: storeVersion,
		Capabilities: []Capability{
			{
				ID:           "provider-create",
				Kind:         KindProvisioner,
				Provider:     "example-cloud",
				Description:  "Create temporary VPS capacity.",
				Capabilities: []string{"provision_server"},
				Status:       "plan_only",
				SecretPolicy: SecretApprovalGated,
				Policy:       "cost-incurring create/delete actions require approval",
			},
		},
	})

	report := Recommend("rent a temporary vps")
	if report.Class != "provision" {
		t.Fatalf("class=%q want provision", report.Class)
	}
	if len(report.Candidates) != 1 {
		t.Fatalf("expected one candidate: %#v", report.Candidates)
	}
	if !report.Candidates[0].ApprovalRequired {
		t.Fatalf("expected approval-required candidate: %#v", report.Candidates[0])
	}
}

func TestRecommendPrefersSpecificRoleCapabilityOverGenericNode(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_INVENTORY_FILE", filepath.Join(dir, "inventory.json"))
	t.Setenv("MESHCLAW_INVENTORY_OVERRIDES_FILE", filepath.Join(dir, "inventory_overrides.json"))
	t.Setenv("MESHCLAW_CAPABILITY_FILE", filepath.Join(dir, "missing-capabilities.json"))
	writeJSON(t, filepath.Join(dir, "inventory.json"), map[string]interface{}{
		"version": 1,
		"nodes": []map[string]interface{}{
			{"name": "c1", "role": "server", "tags": []string{"linux", "vps"}, "online": true},
		},
	})
	writeJSON(t, filepath.Join(dir, "inventory_overrides.json"), map[string]interface{}{
		"version": 1,
		"nodes": []map[string]interface{}{
			{"name": "c1", "role": "mail-server", "tags": []string{"mail", "mox"}},
		},
	})

	report := Recommend("email send approval")
	if len(report.Candidates) < 2 {
		t.Fatalf("expected mail and node candidates: %#v", report.Candidates)
	}
	if report.Candidates[0].Capability.ID != "c1-mail" {
		t.Fatalf("top candidate=%s want c1-mail; candidates=%#v", report.Candidates[0].Capability.ID, report.Candidates)
	}
}

func TestRecommendCloudflareDNSUsesProviderCapability(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_CAPABILITY_FILE", filepath.Join(dir, "missing-capabilities.json"))
	t.Setenv("MESHCLAW_INVENTORY_FILE", filepath.Join(dir, "inventory.json"))
	writeJSON(t, filepath.Join(dir, "inventory.json"), map[string]interface{}{
		"version": 1,
		"nodes": []map[string]interface{}{
			{"name": "c1", "role": "mail-server", "tags": []string{"linux", "mail", "mox"}, "online": true},
		},
	})

	report := Recommend("cloudflare dns change using provider token")
	if report.Class != "api" {
		t.Fatalf("class=%q want api", report.Class)
	}
	if len(report.Candidates) == 0 || report.Candidates[0].Capability.ID != "cloudflare-api" {
		t.Fatalf("top candidate should be cloudflare-api: %#v", report.Candidates)
	}
	if !report.Candidates[0].ApprovalRequired {
		t.Fatalf("expected approval caution for DNS change: %#v", report.Candidates[0])
	}
}

func TestRecommendBrowserScreenshotPrefersController(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_CAPABILITY_FILE", filepath.Join(dir, "missing-capabilities.json"))
	t.Setenv("MESHCLAW_INVENTORY_FILE", filepath.Join(dir, "inventory.json"))
	writeJSON(t, filepath.Join(dir, "inventory.json"), map[string]interface{}{
		"version": 1,
		"nodes": []map[string]interface{}{
			{"name": "c1", "role": "mail-server", "tags": []string{"linux", "mail", "mox"}, "online": true},
		},
	})

	report := Recommend("capture browser screenshot evidence for mail client")
	if report.Class != "automation" {
		t.Fatalf("class=%q want automation", report.Class)
	}
	if len(report.Candidates) == 0 || report.Candidates[0].Capability.ID != "macbook-controller" {
		t.Fatalf("top candidate should be macbook-controller: %#v", report.Candidates)
	}
}

func TestRecommendMailClientInstallPrefersControllerOverMailServer(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_CAPABILITY_FILE", filepath.Join(dir, "missing-capabilities.json"))
	t.Setenv("MESHCLAW_INVENTORY_FILE", filepath.Join(dir, "inventory.json"))
	writeJSON(t, filepath.Join(dir, "inventory.json"), map[string]interface{}{
		"version": 1,
		"nodes": []map[string]interface{}{
			{"name": "c1", "role": "mail-server", "tags": []string{"linux", "mail", "mox"}, "online": true},
		},
	})

	report := Recommend("client-install prepare macmini and MacBook mail clients")
	if report.Class != "automation" {
		t.Fatalf("class=%q want automation", report.Class)
	}
	if len(report.Candidates) == 0 || report.Candidates[0].Capability.ID != "macbook-controller" {
		t.Fatalf("top candidate should be macbook-controller: %#v", report.Candidates)
	}
}

func hasCapability(caps []Capability, id string) bool {
	for _, cap := range caps {
		if cap.ID == id {
			return true
		}
	}
	return false
}

func writeJSON(t *testing.T, path string, value interface{}) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
