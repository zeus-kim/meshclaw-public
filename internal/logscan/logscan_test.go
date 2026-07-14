package logscan

import (
	"strings"
	"testing"
)

func TestAnalyzeDetectsOperationalPatterns(t *testing.T) {
	text := strings.Join([]string{
		"kernel: Out of memory: Killed process 1234",
		"docker: back-off restarting failed container",
		"sshd: Failed password for invalid user root from 203.0.113.9",
		`nginx: "GET / HTTP/1.1" 502 bad gateway`,
	}, "\n")
	findings := Analyze(Source{Type: SourceHostJournal, Host: "g4", Name: "journal"}, text, Options{})
	if len(findings) != 4 {
		t.Fatalf("findings = %d, want 4: %+v", len(findings), findings)
	}
	want := map[string]string{
		"oom":          "critical",
		"crash_loop":   "high",
		"auth_failure": "medium",
		"http_5xx":     "medium",
	}
	for _, finding := range findings {
		if want[finding.Pattern] != finding.Severity {
			t.Fatalf("finding %+v does not match expected severities %#v", finding, want)
		}
	}
}

func TestAnalyzeDetectsContainerSelfHealPatternsWithHints(t *testing.T) {
	text := strings.Join([]string{
		"api healthcheck failed: connection refused",
		"nginx listen tcp :8080: bind: address already in use",
		"worker temporary failure in name resolution for postgres",
		"pull access denied for private/app",
		"cache write failed: no space left on device",
		"vllm CUDA out of memory while loading model",
	}, "\n")
	findings := Analyze(Source{Type: SourceContainerLogs, Host: "g4", Container: "api"}, text, Options{})
	want := map[string]string{
		"healthcheck_failure":           "high",
		"port_bind_failure":             "high",
		"dependency_connection_failure": "high",
		"image_pull_failure":            "high",
		"disk_full":                     "critical",
		"gpu_runtime_failure":           "high",
	}
	seen := map[string]Finding{}
	for _, finding := range findings {
		seen[finding.Pattern] = finding
		if finding.LikelyCause == "" || finding.SuggestedAction == "" {
			t.Fatalf("self-heal finding should include cause/action hints: %+v", finding)
		}
	}
	for pattern, severity := range want {
		finding, ok := seen[pattern]
		if !ok {
			t.Fatalf("missing expected pattern %s in %+v", pattern, findings)
		}
		if finding.Severity != severity {
			t.Fatalf("finding %+v does not match expected severity %s", finding, severity)
		}
	}
}

func TestAnalyzeDetectsSystemdRuntimePatternsWithHints(t *testing.T) {
	text := strings.Join([]string{
		"open-webui.service: Failed at step EXEC spawning /opt/openwebui/server: Exec format error",
		"mine.service: Changing to the requested working directory failed: No such file or directory",
		"systemd-resolved.service: DNS query failed: nameserver not responding",
	}, "\n")
	findings := Analyze(Source{Type: SourceHostJournal, Host: "g4", Name: "system"}, text, Options{})
	want := map[string]string{
		"exec_format_error":         "high",
		"working_directory_missing": "high",
		"dns_resolver_failure":      "high",
	}
	seen := map[string]Finding{}
	for _, finding := range findings {
		seen[finding.Pattern] = finding
		if finding.LikelyCause == "" || finding.SuggestedAction == "" {
			t.Fatalf("runtime finding should include cause/action hints: %+v", finding)
		}
	}
	for pattern, severity := range want {
		finding, ok := seen[pattern]
		if !ok {
			t.Fatalf("missing expected pattern %s in %+v", pattern, findings)
		}
		if finding.Severity != severity {
			t.Fatalf("finding %+v does not match expected severity %s", finding, severity)
		}
	}
	if !strings.Contains(seen["exec_format_error"].SuggestedAction, "architecture before restart") {
		t.Fatalf("exec format hint should require architecture evidence before restart: %+v", seen["exec_format_error"])
	}
	if !strings.Contains(seen["working_directory_missing"].SuggestedAction, "NAS/mount availability") {
		t.Fatalf("working directory hint should require mount evidence: %+v", seen["working_directory_missing"])
	}
	if !strings.Contains(seen["dns_resolver_failure"].SuggestedAction, "resolvectl") {
		t.Fatalf("dns resolver hint should require resolver evidence: %+v", seen["dns_resolver_failure"])
	}
	if !containsString(seen["exec_format_error"].UnitCandidates, "open-webui.service") {
		t.Fatalf("exec format finding should capture unit candidate: %+v", seen["exec_format_error"])
	}
	if !containsString(seen["working_directory_missing"].UnitCandidates, "mine.service") {
		t.Fatalf("working directory finding should capture unit candidate: %+v", seen["working_directory_missing"])
	}
	if !containsString(seen["dns_resolver_failure"].UnitCandidates, "systemd-resolved.service") {
		t.Fatalf("dns resolver finding should capture unit candidate: %+v", seen["dns_resolver_failure"])
	}
}

func TestAnalyzeCountsAndRedactsSamples(t *testing.T) {
	text := strings.Join([]string{
		"sshd: Failed password for admin@example.com from 192.0.2.10 token=abc123",
		"sshd: authentication failure for admin@example.com from 192.0.2.11 password=secret",
	}, "\n")
	findings := Analyze(Source{Type: SourceHostJournal, Host: "d1"}, text, Options{MaxSamples: 2})
	if len(findings) != 1 {
		t.Fatalf("findings = %+v", findings)
	}
	finding := findings[0]
	if finding.Pattern != "auth_failure" || finding.Count != 2 {
		t.Fatalf("bad auth finding: %+v", finding)
	}
	if strings.Contains(finding.Sample, "admin@example.com") || strings.Contains(finding.Sample, "192.0.2.") || strings.Contains(finding.Sample, "abc123") || strings.Contains(finding.Sample, "secret") {
		t.Fatalf("sample was not redacted: %s", finding.Sample)
	}
	if !strings.Contains(finding.Sample, "[redacted-email]") || !strings.Contains(finding.Sample, "[redacted-ip]") {
		t.Fatalf("sample missing redaction markers: %s", finding.Sample)
	}
}

func TestCommandSpecs(t *testing.T) {
	journal := HostJournalCommand("")
	if journal.Name != "journalctl" || strings.Join(journal.Args, " ") != "-p err --since 1 hour ago --no-pager" {
		t.Fatalf("bad journal command: %+v", journal)
	}
	docker := DockerLogsCommand("api", 50, "10m")
	if docker.Name != "docker" || strings.Join(docker.Args, " ") != "logs --tail 50 --since 10m api" {
		t.Fatalf("bad docker command: %+v", docker)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
