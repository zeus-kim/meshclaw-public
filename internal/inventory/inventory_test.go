package inventory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_CONFIG_DIR", dir)
	t.Setenv("MESHCLAW_INVENTORY_FILE", "")

	nodes := []Node{
		{Name: "D1", Role: "ai-workload", Tailscale: "100.64.0.1", User: "dell", Tags: []string{"GPU", "linux"}},
	}
	if err := Save(nodes); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 {
		t.Fatalf("loaded %d nodes, want 1", len(loaded))
	}
	if loaded[0].Name != "d1" || loaded[0].Tags[0] != "gpu" || loaded[0].Tags[1] != "linux" {
		t.Fatalf("loaded node not normalized: %#v", loaded[0])
	}
}

func TestDiscoverFallsBackToLegacyNodes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("MESHCLAW_CONFIG_DIR", dir)
	t.Setenv("MESHCLAW_VSSH_BINARY", filepath.Join(dir, "missing-vssh"))
	t.Setenv("PATH", dir)

	if err := os.WriteFile(filepath.Join(dir, "nodes.json"), []byte(`{"nodes":{"d1":{"ip":"100.64.0.1","user":"dell"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	nodes, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("nodes=%d, want 1", len(nodes))
	}
	if nodes[0].Name != "d1" || nodes[0].User != "dell" {
		t.Fatalf("legacy node not loaded: %#v", nodes[0])
	}
}

func TestMergePreservesConfiguredUserAndAddsDiscoveredIP(t *testing.T) {
	merged := Merge(
		[]Node{{Name: "d1", User: "dell", Tags: []string{"linux"}}},
		[]Node{{Name: "d1", Tailscale: "100.64.0.1", Tags: []string{"gpu"}, Source: "tailscale"}},
	)
	if len(merged) != 1 {
		t.Fatalf("merged=%d, want 1", len(merged))
	}
	if merged[0].User != "dell" || merged[0].Tailscale != "100.64.0.1" {
		t.Fatalf("merge lost fields: %#v", merged[0])
	}
	if len(merged[0].Tags) != 2 {
		t.Fatalf("tags=%#v, want linux+gpu", merged[0].Tags)
	}
}

func TestInventoryOverridesRefineDiscoveredRoles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_CONFIG_DIR", dir)
	t.Setenv("MESHCLAW_INVENTORY_OVERRIDES_FILE", filepath.Join(dir, "inventory_overrides.json"))
	if err := os.WriteFile(filepath.Join(dir, "inventory_overrides.json"), []byte(`{
		"version": 1,
		"nodes": [
			{"name":"c1","role":"mail-server","tags":["mail","mox"]}
		]
	}`), 0600); err != nil {
		t.Fatal(err)
	}

	nodes := ApplyOverrides([]Node{{Name: "c1", Role: "server", Tags: []string{"linux", "vps"}, Tailscale: "100.64.0.10"}})
	if len(nodes) != 1 {
		t.Fatalf("nodes=%d, want 1", len(nodes))
	}
	if nodes[0].Role != "mail-server" {
		t.Fatalf("role=%q want mail-server: %#v", nodes[0].Role, nodes[0])
	}
	if !hasTag(nodes[0], "mail") || !hasTag(nodes[0], "mox") || !hasTag(nodes[0], "linux") {
		t.Fatalf("override tags not merged: %#v", nodes[0].Tags)
	}
}

func TestSetAndRemoveOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_INVENTORY_OVERRIDES_FILE", filepath.Join(dir, "overrides.json"))

	nodes, err := SetOverride(Node{Name: "G4", Role: "automation-worker", Tags: []string{"N8N", "gpu"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].Name != "g4" || nodes[0].Role != "automation-worker" {
		t.Fatalf("unexpected overrides: %#v", nodes)
	}
	if !hasTag(nodes[0], "n8n") || !hasTag(nodes[0], "gpu") {
		t.Fatalf("tags not normalized: %#v", nodes[0].Tags)
	}
	loaded, err := LoadOverrides()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].Name != "g4" {
		t.Fatalf("loaded overrides: %#v", loaded)
	}
	remaining, removed, err := RemoveOverride("g4")
	if err != nil {
		t.Fatal(err)
	}
	if !removed || len(remaining) != 0 {
		t.Fatalf("removed=%t remaining=%#v", removed, remaining)
	}
}

func hasTag(node Node, tag string) bool {
	for _, got := range node.Tags {
		if got == tag {
			return true
		}
	}
	return false
}
