package workflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/meshclaw/meshclaw/internal/logscan"
)

func TestDataCleanPlanCommandPrunesDependencyPaths(t *testing.T) {
	command := dataCleanPlanCommand("/home")

	required := []string{
		`*/site-packages/*`,
		`*/dist-packages/*`,
		`*/.local/lib/python*/*`,
		`*/go/pkg/mod/*`,
		`*/node_modules/*`,
		`min_kb=10240`,
		`jsonl=${manifest%.txt}.jsonl`,
		`"category": category`,
		`"risk": risk`,
		`estimated_reclaim_kb`,
		`sort -hr | head -200`,
	}
	for _, pattern := range required {
		if !strings.Contains(command, pattern) {
			t.Fatalf("cleanup plan command does not prune %q:\n%s", pattern, command)
		}
	}

	forbidden := []string{
		`-name "*checkpoint*"`,
		`-name "*ckpt*"`,
	}
	for _, pattern := range forbidden {
		if strings.Contains(command, pattern) {
			t.Fatalf("cleanup plan command still uses broad file matcher %q:\n%s", pattern, command)
		}
	}
}

func TestDataCleanApplyCommandHasDefenseInDepth(t *testing.T) {
	command := dataCleanApplyCommand("/tmp/meshclaw-data-clean-plan-test.txt")

	required := []string{
		"refuse_dependency_path",
		"refuse_unrecognized_dir",
		"refuse_unrecognized_file",
		`*/site-packages/*`,
		`*.ckpt|*checkpoint*.pt`,
		`structured_plan=`,
		`categories=`,
		`risks=`,
	}
	for _, pattern := range required {
		if !strings.Contains(command, pattern) {
			t.Fatalf("cleanup apply command is missing safety guard %q:\n%s", pattern, command)
		}
	}
}

func TestSummarizeServiceAuditOutputCollapsesRepeatedJournalLines(t *testing.T) {
	output := summarizeServiceAuditOutput(`---failed---
---activating---
---recent-service-errors---
5월 18 18:49:01 d1 systemd[2172]: open-webui.service: Failed with result 'exit-code'.
5월 18 18:49:05 d1 systemd[2172]: open-webui.service: Failed with result 'exit-code'.
5월 18 18:49:08 d1 systemd[2172]: open-webui.service: Failed with result 'exit-code'.
5월 18 18:50:01 d1 systemd[2172]: other.service: Main process exited.`)

	if !strings.Contains(output, "open-webui.service: Failed with result 'exit-code'. (repeated 3 times)") {
		t.Fatalf("repeated service failures were not collapsed:\n%s", output)
	}
	if strings.Count(output, "open-webui.service") != 1 {
		t.Fatalf("collapsed output still repeats open-webui lines:\n%s", output)
	}
	if !strings.Contains(output, "other.service: Main process exited.") {
		t.Fatalf("unique journal line was lost:\n%s", output)
	}
}

func TestServiceCheckCommandCollectsSystemAndUserScopes(t *testing.T) {
	command := serviceCheckCommand("open-webui")

	required := []string{
		"---system-status---",
		"systemctl status",
		"---user-status---",
		"systemctl --user status",
		"journalctl --user -u",
	}
	for _, pattern := range required {
		if !strings.Contains(command, pattern) {
			t.Fatalf("service check command is missing %q:\n%s", pattern, command)
		}
	}
}

func TestSummarizeServiceCheckOutputCollapsesUserLogs(t *testing.T) {
	output := summarizeServiceCheckOutput(`---user-status---
● open-webui.service - Open WebUI Service
     Active: activating (auto-restart) (Result: exit-code)
---user-logs---
5월 18 18:58:17 d1 systemd[2172]: open-webui.service: Main process exited, code=exited, status=203/EXEC
5월 18 18:58:20 d1 systemd[2172]: open-webui.service: Main process exited, code=exited, status=203/EXEC
5월 18 18:58:20 d1 systemd[2172]: open-webui.service: Failed with result 'exit-code'.`)

	if !strings.Contains(output, "status=203/EXEC (repeated 2 times)") {
		t.Fatalf("service-check user logs were not collapsed:\n%s", output)
	}
	if !strings.Contains(output, "Active: activating (auto-restart)") {
		t.Fatalf("service status section was lost:\n%s", output)
	}
}

func TestServiceJournalMessageNormalizesRestartCounters(t *testing.T) {
	first := serviceJournalMessage("5월 18 18:58:17 d1 systemd[2172]: open-webui.service: Scheduled restart job, restart counter is at 41041.")
	second := serviceJournalMessage("5월 18 18:58:20 d1 systemd[2172]: open-webui.service: Scheduled restart job, restart counter is at 41042.")

	if first != second {
		t.Fatalf("restart counter normalization differs:\n%s\n%s", first, second)
	}
	if !strings.Contains(first, "restart counter is at <n>") {
		t.Fatalf("restart counter was not normalized: %s", first)
	}
}

func TestHighestLogSeverityMapsStructuredFindings(t *testing.T) {
	severity := highestLogSeverity([]logscan.Finding{
		{Severity: "medium", Pattern: "auth_failure"},
		{Severity: "critical", Pattern: "oom"},
	})
	if severity != "critical" {
		t.Fatalf("severity = %q, want critical", severity)
	}
	if got := highestLogSeverity([]logscan.Finding{{Severity: "medium", Pattern: "http_5xx"}}); got != "warning" {
		t.Fatalf("medium should map to workflow warning, got %q", got)
	}
}

func TestAnalyzeLogsCommandSupportsContainerSource(t *testing.T) {
	command, source := analyzeLogsCommand("g4", "container:api")

	if source.Type != logscan.SourceContainerLogs || source.Host != "g4" || source.Container != "api" {
		t.Fatalf("bad container log source: %+v", source)
	}
	for _, want := range []string{"'docker'", "'logs'", "'--tail'", "'200'", "'--since'", "'1h'", "'api'"} {
		if !strings.Contains(command, want) {
			t.Fatalf("container command missing %s: %s", want, command)
		}
	}
}

func TestAnalyzeLogsCommandKeepsSystemSourceAsHostJournal(t *testing.T) {
	command, source := analyzeLogsCommand("g4", "system")

	if source.Type != logscan.SourceHostJournal || source.Host != "g4" || source.Name != "system" {
		t.Fatalf("bad system log source: %+v", source)
	}
	if !strings.Contains(command, "journalctl") || strings.Contains(command, "docker logs") {
		t.Fatalf("bad system command: %s", command)
	}
}

func TestReportIncludesLogFindingsWhenPresent(t *testing.T) {
	report := Report{
		Name:   "analyze-logs",
		Host:   "g4",
		Status: "findings",
		Findings: []Finding{{
			Severity: "critical",
			Title:    "Structured logscan patterns found",
		}},
		LogFindings: []logscan.Finding{{
			Severity: "critical",
			Source:   logscan.Source{Type: logscan.SourceHostJournal, Host: "g4", Name: "system"},
			Pattern:  "oom",
			Count:    2,
			Sample:   "kernel: Out of memory",
		}},
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`"log_findings"`, `"pattern":"oom"`, `"count":2`} {
		if !strings.Contains(text, want) {
			t.Fatalf("report JSON missing %s: %s", want, text)
		}
	}
}

func TestAnalyzeLogsAutohealHandoffRoutesContainerFindingsToApplyGate(t *testing.T) {
	handoff := analyzeLogsAutohealHandoff(logscan.Source{Type: logscan.SourceContainerLogs, Host: "g4", Container: "api"}, []logscan.Finding{{
		Severity:        "high",
		Pattern:         "healthcheck_failure",
		LikelyCause:     "container health/readiness probe is failing",
		SuggestedAction: "compare healthcheck command before self-heal",
	}})

	if handoff == nil {
		t.Fatal("container findings should produce autoheal handoff")
	}
	if handoff.Decision != "plan_only_before_operator_approved_apply" {
		t.Fatalf("bad handoff decision: %+v", handoff)
	}
	if handoff.Confidence != "high" {
		t.Fatalf("container high-severity findings should produce high confidence: %+v", handoff)
	}
	for _, want := range []string{"meshclaw_autoheal_plan", "meshclaw_autoheal_container_apply_plan", "meshclaw_autoheal_container_readiness_summary"} {
		if !containsString(handoff.RecommendedTools, want) {
			t.Fatalf("container handoff missing tool %s: %+v", want, handoff)
		}
	}
	if !containsString(handoff.EvidenceRequired, "focused container-logscan evidence for container:api") {
		t.Fatalf("container handoff missing focused logscan evidence: %+v", handoff)
	}
	if !containsString(handoff.EvidenceRequired, "fresh container runtime evidence for container:api including image, status, health, ports, and restart policy") {
		t.Fatalf("container handoff missing runtime evidence: %+v", handoff)
	}
	for _, want := range []string{
		"docker inspect api image id/name",
		"docker inspect api state.status and state.running",
		"docker inspect api state.health when configured",
		"docker inspect api hostconfig.restartpolicy",
		"docker inspect api networksettings.ports",
		"docker ps --filter name=api on host g4",
	} {
		if !containsString(handoff.RuntimeEvidenceChecklist, want) {
			t.Fatalf("container handoff missing runtime checklist item %q: %+v", want, handoff)
		}
	}
	if !containsString(handoff.EvidenceRequired, "meshclaw_analyze_logs arguments for focused container logs: host=g4 source=container:api") {
		t.Fatalf("container handoff missing focused logscan arguments: %+v", handoff)
	}
	if !containsString(handoff.StopBefore, "container apply-plan unless the focused container-logscan evidence names container:api") {
		t.Fatalf("container handoff should stop before apply-plan without focused logscan evidence: %+v", handoff)
	}
	if !containsString(handoff.StopBefore, "container apply-plan unless fresh runtime inspect/status evidence names container:api") {
		t.Fatalf("container handoff should stop before apply-plan without runtime evidence: %+v", handoff)
	}
	if !containsString(handoff.MustNot, "restart or recreate services/containers directly from log output") {
		t.Fatalf("container handoff should forbid direct mutation: %+v", handoff)
	}
}

func TestAnalyzeLogsAutohealHandoffRoutesSystemFindingsToServiceTriage(t *testing.T) {
	handoff := analyzeLogsAutohealHandoff(logscan.Source{Type: logscan.SourceHostJournal, Host: "g4", Name: "system"}, []logscan.Finding{{
		Severity:       "critical",
		Pattern:        "oom",
		UnitCandidates: []string{"open-webui.service", "open-webui.service", "mine.service"},
	}})

	if handoff == nil {
		t.Fatal("system findings should produce autoheal handoff")
	}
	if handoff.Confidence != "medium" {
		t.Fatalf("system critical findings should produce medium handoff confidence until service evidence exists: %+v", handoff)
	}
	for _, want := range []string{"meshclaw_service_check", "meshclaw_autoheal_plan", "meshclaw_reconcile_readiness_summary"} {
		if !containsString(handoff.RecommendedTools, want) {
			t.Fatalf("system handoff missing tool %s: %+v", want, handoff)
		}
	}
	if !containsString(handoff.EvidenceRequired, "targeted service-check evidence for repeated units") {
		t.Fatalf("system handoff missing service-check evidence: %+v", handoff)
	}
	if !containsString(handoff.EvidenceRequired, "unit identity evidence with service name and system/user scope") {
		t.Fatalf("system handoff missing unit identity evidence: %+v", handoff)
	}
	if !containsString(handoff.EvidenceRequired, "targeted service-check evidence for units: open-webui.service,mine.service") {
		t.Fatalf("system handoff missing targeted unit candidate evidence: %+v", handoff)
	}
	if !containsString(handoff.EvidenceRequired, "meshclaw_service_check arguments for unit candidates: host=g4 service=open-webui.service|mine.service") {
		t.Fatalf("system handoff missing service-check unit candidate arguments: %+v", handoff)
	}
	if !containsString(handoff.StopBefore, "executing restart/recreate/rollback actions from logscan evidence alone") {
		t.Fatalf("system handoff should stop before direct restart from logs: %+v", handoff)
	}
	if !containsString(handoff.StopBefore, "service restart before unit identity and scope are confirmed") {
		t.Fatalf("system handoff should stop before restart without unit identity: %+v", handoff)
	}
}

func TestAnalyzeLogsAutohealHandoffAddsRuntimePatternEvidence(t *testing.T) {
	handoff := analyzeLogsAutohealHandoff(logscan.Source{Type: logscan.SourceHostJournal, Host: "g4", Name: "system"}, []logscan.Finding{
		{Severity: "high", Pattern: "exec_format_error"},
		{Severity: "high", Pattern: "working_directory_missing"},
		{Severity: "high", Pattern: "dns_resolver_failure"},
	})

	if handoff == nil {
		t.Fatal("runtime findings should produce autoheal handoff")
	}
	for _, want := range []string{
		"meshclaw_agent_workloads",
		"meshclaw_node_repair_plan",
		"meshclaw_disk_investigate",
		"meshclaw_storage_guardrail_plan",
		"meshclaw_agent_security",
		"meshclaw_service_registry_plan",
	} {
		if !containsString(handoff.RecommendedTools, want) {
			t.Fatalf("runtime handoff missing tool %s: %+v", want, handoff)
		}
	}
	for _, want := range []string{
		"node architecture and deployed binary/image architecture evidence",
		"WorkingDirectory path, mount, and ownership evidence",
		"resolver status and dependency endpoint health evidence",
	} {
		if !containsString(handoff.EvidenceRequired, want) {
			t.Fatalf("runtime handoff missing evidence %s: %+v", want, handoff)
		}
	}
	for _, want := range []string{
		"restart/recreate before architecture mismatch is ruled out",
		"service restart before WorkingDirectory and mount evidence are checked",
		"dependent service restart before resolver and endpoint evidence are checked",
	} {
		if !containsString(handoff.StopBefore, want) {
			t.Fatalf("runtime handoff missing stop-before %s: %+v", want, handoff)
		}
	}
}

func TestServiceQuarantineCommandSupportsUserScope(t *testing.T) {
	command := serviceQuarantineCommand("open-webui")

	required := []string{
		"systemctl --user show",
		"scope=user",
		"systemctl --user disable --now",
		"ExecStart exists and is executable; refusing quarantine",
	}
	for _, pattern := range required {
		if !strings.Contains(command, pattern) {
			t.Fatalf("service quarantine command is missing %q:\n%s", pattern, command)
		}
	}
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
