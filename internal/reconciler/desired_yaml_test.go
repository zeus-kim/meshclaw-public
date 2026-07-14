package reconciler

import (
	"strings"
	"testing"
)

func TestParseDesiredStateYAML(t *testing.T) {
	state, err := ParseDesiredStateYAML([]byte(`
schema_version: v3
nodes:
  g4:
    roles: [openwebui-worker, ollama-worker]
    tags:
      - gpu
      - docker
    services:
      open-webui:
        desired: running
      vsshd: active
    containers:
      open-webui:
        desired: running
        image: ghcr.io/open-webui/open-webui:main
        health: healthy
        restart: approval_required
      redis: running
    capacity:
      allow_model_jobs: true
      min_disk_free_pct: 20
      max_memory_used_pct: 85
  d1:
    roles: [storage]
    services:
      docker: running
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2: %#v", len(state.Nodes), state.Nodes)
	}
	if state.SchemaVersion != "v3" {
		t.Fatalf("schema version = %q, want v3", state.SchemaVersion)
	}
	if state.Nodes[0].ID != "d1" || state.Nodes[1].ID != "g4" {
		t.Fatalf("nodes not sorted by id: %#v", state.Nodes)
	}
	g4 := state.Nodes[1]
	if g4.Services["open-webui"] != "running" || g4.Services["vsshd"] != "active" {
		t.Fatalf("services = %#v", g4.Services)
	}
	if g4.Containers["open-webui"].Desired != "running" || g4.Containers["open-webui"].Image == "" || g4.Containers["open-webui"].Health != "healthy" {
		t.Fatalf("open-webui container not parsed: %#v", g4.Containers["open-webui"])
	}
	if g4.Containers["open-webui"].Restart != "approval_required" {
		t.Fatalf("open-webui restart policy not parsed: %#v", g4.Containers["open-webui"])
	}
	if g4.Containers["redis"].Desired != "running" {
		t.Fatalf("redis scalar container not parsed: %#v", g4.Containers["redis"])
	}
	if g4.AllowModelJobs == nil || !*g4.AllowModelJobs {
		t.Fatalf("allow_model_jobs not parsed: %#v", g4.AllowModelJobs)
	}
	if g4.MinDiskFreePct == nil || *g4.MinDiskFreePct != 20 {
		t.Fatalf("min disk not parsed: %#v", g4.MinDiskFreePct)
	}
	if g4.MaxMemoryUsedPct == nil || *g4.MaxMemoryUsedPct != 85 {
		t.Fatalf("max memory not parsed: %#v", g4.MaxMemoryUsedPct)
	}
}

func TestParseDesiredStateYAMLRequiresNodes(t *testing.T) {
	if _, err := ParseDesiredStateYAML([]byte(`nodes: {}`)); err == nil {
		t.Fatal("expected error for empty nodes")
	}
}

func TestParseDesiredStateYAMLAllowsMissingSchemaVersion(t *testing.T) {
	state, err := ParseDesiredStateYAML([]byte(`
nodes:
  g4:
    services:
      docker: running
`))
	if err != nil {
		t.Fatal(err)
	}
	if state.SchemaVersion != "" {
		t.Fatalf("schema version = %q, want empty", state.SchemaVersion)
	}
}

func TestValidateDesiredStateWarnsOnUnsupportedSchemaVersion(t *testing.T) {
	state := DesiredState{
		SchemaVersion: "v9",
		Nodes: []DesiredNode{{
			ID:       "g4",
			Services: map[string]string{"docker": "running"},
		}},
	}
	findings := ValidateDesiredState(state)
	for _, finding := range findings {
		if finding.Field == "schema_version" && finding.Severity == "warning" {
			return
		}
	}
	t.Fatalf("missing schema_version warning: %+v", findings)
}

func TestValidateDesiredStateAllowsContainerHealthOrRestartOnly(t *testing.T) {
	state := DesiredState{Nodes: []DesiredNode{{
		ID: "g4",
		Containers: map[string]DesiredContainer{
			"api":    {Health: "healthy"},
			"worker": {Restart: "approval_required"},
		},
	}}}
	for _, finding := range ValidateDesiredState(state) {
		if finding.Severity == "critical" {
			t.Fatalf("health/restart-only container intent should not be critical: %+v", finding)
		}
	}
}

func TestValidateDesiredStateWarnsOnIgnoredApplyKeys(t *testing.T) {
	state, err := ParseDesiredStateYAML([]byte(`
schema_version: v3
apply: true
nodes:
  g4:
    execute: true
    services:
      docker: running
    containers:
      api:
        desired: running
        auto_apply: true
`))
	if err != nil {
		t.Fatal(err)
	}
	findings := ValidateDesiredState(state)
	var topApply, nodeExecute, containerAutoApply bool
	for _, finding := range findings {
		if finding.Severity != "warning" || !strings.Contains(finding.Message, "does not grant apply, execute, or approval") {
			continue
		}
		switch finding.Field {
		case "apply":
			topApply = true
		case "nodes.g4.execute":
			nodeExecute = finding.NodeID == "g4"
		case "nodes.g4.containers.api.auto_apply":
			containerAutoApply = finding.NodeID == "g4"
		}
	}
	if !topApply || !nodeExecute || !containerAutoApply {
		t.Fatalf("missing ignored apply-key warnings: %+v", findings)
	}
}

func TestValidateDesiredStateFindsWarningsAndCriticals(t *testing.T) {
	tooHigh := 120.0
	state := DesiredState{Nodes: []DesiredNode{{
		ID:    "g4",
		Roles: []string{"gpu", "gpu"},
		Services: map[string]string{
			"open-webui": "restarted",
		},
		Containers: map[string]DesiredContainer{
			"api":    {Desired: "restart", Health: "perfect", Restart: "aggressive"},
			"legacy": {Desired: "absent", Image: "legacy:v1", Health: "healthy"},
		},
		MaxMemoryUsedPct: &tooHigh,
	}, {
		ID: "empty",
	}}}
	findings := ValidateDesiredState(state)
	var duplicateRole, badMemory, badServiceState, badContainerState, badContainerHealth, badContainerRestart, absentConflict, emptyNode bool
	for _, finding := range findings {
		if finding.NodeID == "g4" && finding.Field == "roles" && finding.Severity == "warning" {
			duplicateRole = true
		}
		if finding.NodeID == "g4" && finding.Field == "services.desired" && finding.Severity == "warning" {
			badServiceState = true
		}
		if finding.NodeID == "g4" && finding.Field == "containers.desired" && finding.Severity == "warning" {
			badContainerState = true
		}
		if finding.NodeID == "g4" && finding.Field == "containers.health" && finding.Severity == "warning" {
			badContainerHealth = true
		}
		if finding.NodeID == "g4" && finding.Field == "containers.restart" && finding.Severity == "warning" {
			badContainerRestart = true
		}
		if finding.NodeID == "g4" && finding.Field == "containers.absent" && finding.Severity == "warning" {
			absentConflict = true
		}
		if finding.NodeID == "g4" && finding.Field == "capacity.max_memory_used_pct" && finding.Severity == "critical" {
			badMemory = true
		}
		if finding.NodeID == "empty" && finding.Field == "node" && finding.Severity == "warning" {
			emptyNode = true
		}
	}
	if !duplicateRole || !badMemory || !badServiceState || !badContainerState || !badContainerHealth || !badContainerRestart || !absentConflict || !emptyNode {
		t.Fatalf("missing expected findings: %+v", findings)
	}
}
