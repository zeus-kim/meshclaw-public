package reconciler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/meshclaw/meshclaw/internal/nodestate"
)

func TestActualNodeFromReport(t *testing.T) {
	report := nodestate.Report{
		Hostname: "host-a",
		NodeName: "g4",
		System: nodestate.SystemState{
			DiskPct:   72.5,
			MemoryPct: 64.25,
		},
		Inventory: nodestate.InventoryHint{
			PrimaryRole:    "llm-worker",
			SecondaryRoles: []string{"container-host", "llm-worker"},
			Tags:           []string{"gpu", "docker", "gpu"},
		},
		Services: []nodestate.ServiceState{
			{Name: "open-webui", Active: "active", State: "running"},
			{Name: "vsshd", State: "running"},
		},
		Docker: nodestate.DockerState{Containers: []nodestate.DockerContainer{{
			Name:          "open-webui",
			Image:         "ghcr.io/open-webui/open-webui:main",
			State:         "running",
			HealthStatus:  "healthy",
			RestartPolicy: "unless-stopped",
		}}},
	}

	actual := ActualNodeFromReport(report, "ev-g4")
	if actual.ID != "g4" || !actual.Online {
		t.Fatalf("bad identity: %+v", actual)
	}
	if len(actual.Roles) != 2 || actual.Roles[0] != "llm-worker" || actual.Roles[1] != "container-host" {
		t.Fatalf("roles = %#v", actual.Roles)
	}
	if len(actual.Tags) != 2 {
		t.Fatalf("tags not deduplicated: %#v", actual.Tags)
	}
	if actual.Services["open-webui"] != "active" || actual.Services["vsshd"] != "running" {
		t.Fatalf("services = %#v", actual.Services)
	}
	if actual.Containers["open-webui"].State != "running" || actual.Containers["open-webui"].Health != "healthy" || actual.Containers["open-webui"].RestartPolicy != "unless-stopped" {
		t.Fatalf("containers = %#v", actual.Containers)
	}
	if actual.DiskUsedPct == nil || *actual.DiskUsedPct != 72.5 {
		t.Fatalf("disk = %#v", actual.DiskUsedPct)
	}
	if actual.MemoryUsedPct == nil || *actual.MemoryUsedPct != 64.25 {
		t.Fatalf("memory = %#v", actual.MemoryUsedPct)
	}
	if actual.EvidenceID != "ev-g4" {
		t.Fatalf("evidence = %q", actual.EvidenceID)
	}
}

func TestLoadActualNodeReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node.json")
	data, err := json.Marshal(nodestate.Report{
		Hostname: "d1",
		System:   nodestate.SystemState{DiskPct: 88},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	actual, err := LoadActualNodeReport(path)
	if err != nil {
		t.Fatal(err)
	}
	if actual.ID != "d1" || actual.EvidenceID != path {
		t.Fatalf("actual = %+v", actual)
	}
}
