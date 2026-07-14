package main

import (
	"strings"
	"testing"

	"github.com/meshclaw/meshclaw/internal/logscan"
	"github.com/meshclaw/meshclaw/internal/workflow"
)

func TestLogFindingsSummaryLineShowsHighestAndPatterns(t *testing.T) {
	line := logFindingsSummaryLine(workflow.Report{
		Name:   "analyze-logs",
		Host:   "g4",
		Status: "findings",
		LogFindings: []logscan.Finding{
			{Severity: "high", Pattern: "healthcheck_failure"},
			{Severity: "critical", Pattern: "disk_full"},
		},
	})

	want := "log_findings count=2 highest=critical patterns=healthcheck_failure,disk_full"
	if line != want {
		t.Fatalf("log findings line = %q, want %q", line, want)
	}
}

func TestLogFindingsSummaryLineShowsUnitCandidates(t *testing.T) {
	line := logFindingsSummaryLine(workflow.Report{
		Name:   "analyze-logs",
		Host:   "g4",
		Status: "findings",
		LogFindings: []logscan.Finding{
			{Severity: "high", Pattern: "exec_format_error", UnitCandidates: []string{"open-webui.service", "mine.service"}},
			{Severity: "medium", Pattern: "working_directory_missing", UnitCandidates: []string{"mine.service"}},
		},
	})

	want := "log_findings count=2 highest=high patterns=exec_format_error,working_directory_missing units=open-webui.service,mine.service"
	if line != want {
		t.Fatalf("log findings line = %q, want %q", line, want)
	}
}

func TestLogFindingsSummaryLineEmptyWhenNoFindings(t *testing.T) {
	if line := logFindingsSummaryLine(workflow.Report{}); line != "" {
		t.Fatalf("empty log findings line = %q, want empty", line)
	}
}

func TestLogFindingLineShowsUnitCandidates(t *testing.T) {
	line := logFindingLine(logscan.Finding{
		Severity:       "high",
		Pattern:        "exec_format_error",
		Count:          2,
		Source:         logscan.Source{Type: logscan.SourceHostJournal, Host: "g4", Name: "system"},
		UnitCandidates: []string{"open-webui.service", "mine.service"},
	})

	for _, want := range []string{"pattern=exec_format_error", "source=system", "units=open-webui.service,mine.service"} {
		if !strings.Contains(line, want) {
			t.Fatalf("log finding line missing %q: %s", want, line)
		}
	}
}

func TestLogFindingLineOmitsEmptyUnitCandidates(t *testing.T) {
	line := logFindingLine(logscan.Finding{
		Severity: "high",
		Pattern:  "healthcheck_failure",
		Count:    1,
		Source:   logscan.Source{Type: logscan.SourceContainerLogs, Host: "g4", Container: "api"},
	})

	if strings.Contains(line, "units=") {
		t.Fatalf("log finding line should omit empty unit candidates: %s", line)
	}
}

func TestAutohealHandoffSummaryLineShowsPlanOnlyContract(t *testing.T) {
	line := autohealHandoffSummaryLine(&workflow.AutohealHandoff{
		Decision:         "plan_only_before_operator_approved_apply",
		Confidence:       "high",
		RecommendedTools: []string{"meshclaw_autoheal_plan", "meshclaw_autoheal_container_apply_plan"},
		EvidenceRequired: []string{"stored analyze-logs evidence path", "fresh monitor evidence"},
		RuntimeEvidenceChecklist: []string{
			"docker inspect api image id/name",
			"docker inspect api state.status and state.running",
		},
		StopBefore:      []string{"container apply-plan without logscan evidence"},
		MustNot:         []string{"restart directly"},
		RefreshTriggers: []string{"new critical logscan finding appears"},
	})

	for _, want := range []string{
		"autoheal_handoff decision=plan_only_before_operator_approved_apply",
		"confidence=high",
		"recommended_tools=meshclaw_autoheal_plan,meshclaw_autoheal_container_apply_plan",
		"evidence_required=2",
		"runtime_evidence_checklist=2",
		"stop_before=1",
		"must_not=1",
		"refresh_triggers=1",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("handoff line missing %q: %s", want, line)
		}
	}
}

func TestAutohealHandoffDetailLinesShowRuntimeEvidenceAndStops(t *testing.T) {
	lines := autohealHandoffDetailLines(&workflow.AutohealHandoff{
		EvidenceRequired: []string{
			"stored analyze-logs evidence path from this MCP call",
			"node architecture and deployed binary/image architecture evidence",
			"WorkingDirectory path, mount, and ownership evidence",
			"resolver status and dependency endpoint health evidence",
		},
		StopBefore: []string{
			"building an apply plan without stored analyze-logs evidence",
			"restart/recreate before architecture mismatch is ruled out",
			"service restart before WorkingDirectory and mount evidence are checked",
			"dependent service restart before resolver and endpoint evidence are checked",
		},
	}, 3)

	if len(lines) != 6 {
		t.Fatalf("detail lines len=%d, want 6: %#v", len(lines), lines)
	}
	for _, want := range []string{
		"autoheal_handoff evidence_required: node architecture and deployed binary/image architecture evidence",
		"autoheal_handoff evidence_required: WorkingDirectory path, mount, and ownership evidence",
		"autoheal_handoff stop_before: restart/recreate before architecture mismatch is ruled out",
		"autoheal_handoff stop_before: service restart before WorkingDirectory and mount evidence are checked",
	} {
		if !containsExactLine(lines, want) {
			t.Fatalf("detail lines missing %q: %#v", want, lines)
		}
	}
	for _, line := range lines {
		if strings.Contains(line, "resolver status") || strings.Contains(line, "dependent service restart before resolver") {
			t.Fatalf("detail lines should respect limit: %#v", lines)
		}
	}
}

func TestAutohealHandoffDetailLinesPrioritizeUnitIdentityGate(t *testing.T) {
	lines := autohealHandoffDetailLines(&workflow.AutohealHandoff{
		EvidenceRequired: []string{
			"stored analyze-logs evidence path from this MCP call",
			"redacted log_findings patterns, samples, likely causes, and suggested actions",
			"fresh monitor or agent-collect evidence before any apply step",
			"unit identity evidence with service name and system/user scope",
			"targeted service-check evidence for units: open-webui.service,mine.service",
			"meshclaw_service_check arguments for unit candidates: host=g4 service=open-webui.service|mine.service",
		},
		StopBefore: []string{
			"building an apply plan without stored analyze-logs evidence",
			"requesting operator approval without fresh monitor or agent-collect evidence",
			"executing restart/recreate/rollback actions from logscan evidence alone",
			"service restart before unit identity and scope are confirmed",
		},
	}, 3)

	for _, want := range []string{
		"autoheal_handoff evidence_required: unit identity evidence with service name and system/user scope",
		"autoheal_handoff evidence_required: targeted service-check evidence for units: open-webui.service,mine.service",
		"autoheal_handoff evidence_required: meshclaw_service_check arguments for unit candidates: host=g4 service=open-webui.service|mine.service",
		"autoheal_handoff stop_before: service restart before unit identity and scope are confirmed",
	} {
		if !containsExactLine(lines, want) {
			t.Fatalf("detail lines should prioritize unit identity gate %q: %#v", want, lines)
		}
	}
}

func TestAutohealHandoffDetailLinesPrioritizeContainerLogscanArgs(t *testing.T) {
	lines := autohealHandoffDetailLines(&workflow.AutohealHandoff{
		EvidenceRequired: []string{
			"stored analyze-logs evidence path from this MCP call",
			"redacted log_findings patterns, samples, likely causes, and suggested actions",
			"fresh monitor or agent-collect evidence before any apply step",
			"focused container-logscan evidence for container:api",
			"fresh container runtime evidence for container:api including image, status, health, ports, and restart policy",
			"meshclaw_analyze_logs arguments for focused container logs: host=g4 source=container:api",
		},
		RuntimeEvidenceChecklist: []string{
			"docker inspect api state.status and state.running",
			"docker inspect api state.health when configured",
			"docker inspect api image id/name",
			"docker inspect api hostconfig.restartpolicy",
		},
		StopBefore: []string{
			"building an apply plan without stored analyze-logs evidence",
			"container apply-plan unless the focused container-logscan evidence names container:api",
			"container apply-plan unless fresh runtime inspect/status evidence names container:api",
		},
	}, 2)

	if !containsExactLine(lines, "autoheal_handoff evidence_required: meshclaw_analyze_logs arguments for focused container logs: host=g4 source=container:api") {
		t.Fatalf("detail lines should prioritize focused container logscan args: %#v", lines)
	}
	if !containsExactLine(lines, "autoheal_handoff evidence_required: fresh container runtime evidence for container:api including image, status, health, ports, and restart policy") {
		t.Fatalf("detail lines should prioritize container runtime evidence: %#v", lines)
	}
	if !containsExactLine(lines, "autoheal_handoff runtime_evidence_checklist: docker inspect api state.status and state.running") {
		t.Fatalf("detail lines should prioritize container status checklist: %#v", lines)
	}
	if !containsExactLine(lines, "autoheal_handoff runtime_evidence_checklist: docker inspect api state.health when configured") {
		t.Fatalf("detail lines should prioritize container health checklist: %#v", lines)
	}
	if !containsExactLine(lines, "autoheal_handoff stop_before: container apply-plan unless fresh runtime inspect/status evidence names container:api") {
		t.Fatalf("detail lines should prioritize runtime inspect/status stop gate: %#v", lines)
	}
}

func TestLogFindingSourceLabelPrefersContainerSource(t *testing.T) {
	if got := logFindingSourceLabel(logscan.SourceContainerLogs, "container:api", "api"); got != "container:api" {
		t.Fatalf("source label = %q, want container:api", got)
	}
	if got := logFindingSourceLabel(logscan.SourceHostJournal, "system", ""); got != "system" {
		t.Fatalf("source label = %q, want system", got)
	}
}

func containsExactLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}
