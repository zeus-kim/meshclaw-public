package nodestate

import (
	"context"
	"testing"
	"time"
)

func TestCollectProducesStructuredReport(t *testing.T) {
	report := Collect(context.Background(), Options{
		TopProcesses: 2,
		Timeout:      2 * time.Second,
	})
	if report.Kind != ReportKind {
		t.Fatalf("kind = %q", report.Kind)
	}
	if report.Hostname == "" {
		t.Fatal("hostname is empty")
	}
	if report.System.OS == "" || report.System.Arch == "" || report.System.CPUCount <= 0 {
		t.Fatalf("incomplete system facts: %+v", report.System)
	}
	if report.Version != "3" {
		t.Fatalf("version = %q", report.Version)
	}
	if report.Agent.SchemaVersion == "" || report.Agent.Collector == "" {
		t.Fatalf("missing agent metadata: %+v", report.Agent)
	}
	if report.Inventory.PrimaryRole == "" || report.Inventory.Confidence == "" {
		t.Fatalf("missing inventory hint: %+v", report.Inventory)
	}
	if report.Capacity.CPUHeadroom == "" || report.Capacity.MemoryHeadroom == "" || report.Capacity.DiskHeadroom == "" {
		t.Fatalf("missing capacity state: %+v", report.Capacity)
	}
	if report.AIView.PlainSummary == "" || report.AIView.WhatThisNodeIs == "" || len(report.AIView.RecommendedActions) == 0 {
		t.Fatalf("missing AI view: %+v", report.AIView)
	}
	if len(report.Processes) > 2 {
		t.Fatalf("process limit ignored: %d", len(report.Processes))
	}
}
