package main

import (
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/nodestate"
)

func TestMonitorStateFromNodeReport(t *testing.T) {
	storedAt := time.Now().UTC()
	state := monitorStateFromNodeReport(nodestate.Report{
		Kind:        nodestate.ReportKind,
		Hostname:    "g4",
		NodeName:    "g4",
		CollectedAt: storedAt,
		System: nodestate.SystemState{
			MemoryPct: 42.5,
			DiskPct:   71.2,
		},
		GPUs: []nodestate.GPUState{{
			Utilization:   50,
			MemoryTotalMB: 100,
			MemoryUsedMB:  25,
		}},
		Services: []nodestate.ServiceState{{
			Name:   "open-webui",
			Active: "active",
			State:  "running",
		}},
	}, storedAt)
	if state == nil {
		t.Fatal("state is nil")
	}
	if state.Name != "g4" || !state.Online {
		t.Fatalf("unexpected state identity: %+v", state)
	}
	if state.Memory != 42.5 || state.Disk != 71.2 {
		t.Fatalf("unexpected metrics: %+v", state)
	}
	if state.GPUUsage != 50 || state.GPUMemory != 25 {
		t.Fatalf("unexpected gpu metrics: %+v", state)
	}
	if len(state.Services) != 1 || state.Services[0] != "open-webui=active/running" {
		t.Fatalf("unexpected services: %#v", state.Services)
	}
}
