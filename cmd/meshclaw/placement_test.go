package main

import (
	"testing"

	"github.com/meshclaw/meshclaw/internal/capability"
	"github.com/meshclaw/meshclaw/internal/monitor"
	"github.com/meshclaw/meshclaw/internal/workflow"
)

func TestScorePlacementUsesCapabilityFallbackOnlyWhenRelevant(t *testing.T) {
	inventoryReport := fleetInventoryReport{
		Hosts: []workflow.SoftwareInventoryReport{
			{Host: "gpu1", Status: "failed", Error: "inventory unavailable"},
			{Host: "store1", Status: "failed", Error: "inventory unavailable"},
		},
	}
	monitorOut := monitorCheckOutput{States: map[string]*monitor.NodeState{
		"gpu1":   {Name: "gpu1", Online: true, Disk: 20, Memory: 20},
		"store1": {Name: "store1", Online: true, Disk: 20, Memory: 20},
	}}
	caps := []capability.Capability{
		{ID: "gpu1-gpu-compute", Host: "gpu1", Status: "available", Capabilities: []string{"gpu", "model_worker"}},
		{ID: "store1-storage", Host: "store1", Status: "available", Capabilities: []string{"storage", "backup_candidate"}},
	}

	report := scorePlacement("GPU inference", inventoryReport, monitorOut, caps)
	if len(report.Candidates) != 1 {
		t.Fatalf("expected one GPU fallback candidate, got %#v", report.Candidates)
	}
	if report.Candidates[0].Host != "gpu1" {
		t.Fatalf("expected gpu1 candidate, got %#v", report.Candidates[0])
	}
	for _, rejected := range report.Rejected {
		if rejected.Host == "gpu1" {
			t.Fatalf("gpu1 should not be rejected: %#v", report.Rejected)
		}
	}
}
