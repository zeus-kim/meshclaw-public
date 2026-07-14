package nodestate

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/meshclaw/meshclaw/internal/logscan"
)

func TestParseSSListenersMarksPublicPorts(t *testing.T) {
	out := `Netid State  Recv-Q Send-Q Local Address:Port Peer Address:Port Process
tcp   LISTEN 0      4096   0.0.0.0:8080       0.0.0.0:*     users:(("web",pid=1,fd=3))
tcp   LISTEN 0      128    127.0.0.1:5432     0.0.0.0:*     users:(("postgres",pid=2,fd=7))
tcp   LISTEN 0      128    [::]:443           [::]:*        users:(("nginx",pid=3,fd=8))`
	listeners := parseSSListeners(out)
	if len(listeners) != 3 {
		t.Fatalf("listeners len = %d", len(listeners))
	}
	if !listeners[0].Public || listeners[1].Public || !listeners[2].Public {
		t.Fatalf("unexpected public flags: %#v", listeners)
	}
}

func TestDockerPortsMarksPublicMappings(t *testing.T) {
	ports := dockerPorts("web", "0.0.0.0:8080->80/tcp, 127.0.0.1:5432->5432/tcp")
	if len(ports) != 2 {
		t.Fatalf("ports len = %d", len(ports))
	}
	if !ports[0].Public || ports[1].Public {
		t.Fatalf("unexpected docker public flags: %#v", ports)
	}
}

func TestParseDockerInspect(t *testing.T) {
	out := `/web	healthy	2	false	0	2026-06-23T00:00:00.000000000Z	unless-stopped
/worker	unhealthy	7	true	137	0001-01-01T00:00:00Z	on-failure
/plain	none	0	false	0	2026-06-23T01:00:00Z	no`
	containers := parseDockerInspect(out)
	if len(containers) != 3 {
		t.Fatalf("containers len = %d", len(containers))
	}
	web := containers["web"]
	if web.HealthStatus != "healthy" || web.RestartPolicy != "unless-stopped" || web.RestartCount != 2 || web.OOMKilled || web.ExitCode != 0 || web.StartedAt == "" {
		t.Fatalf("unexpected web inspect: %#v", web)
	}
	worker := containers["worker"]
	if worker.HealthStatus != "unhealthy" || worker.RestartPolicy != "on-failure" || worker.RestartCount != 7 || !worker.OOMKilled || worker.ExitCode != 137 || worker.StartedAt != "" {
		t.Fatalf("unexpected worker inspect: %#v", worker)
	}
}

func TestMergeDockerInspectAndWarnings(t *testing.T) {
	state := DockerState{
		Available: true,
		Containers: []DockerContainer{
			{Name: "web", State: "running"},
			{Name: "worker", State: "exited"},
			{Name: "db", State: "running"},
		},
	}
	mergeDockerInspect(&state, map[string]DockerContainer{
		"web":    {Name: "web", HealthStatus: "unhealthy", RestartPolicy: "unless-stopped", RestartCount: 1, StartedAt: "2026-06-23T00:00:00Z"},
		"worker": {Name: "worker", HealthStatus: "none", RestartCount: 8, OOMKilled: true, ExitCode: 137},
	})
	if state.Containers[0].HealthStatus != "unhealthy" || state.Containers[0].RestartPolicy != "unless-stopped" || state.Containers[0].StartedAt == "" {
		t.Fatalf("web inspect not merged: %#v", state.Containers[0])
	}
	if !state.Containers[1].OOMKilled || state.Containers[1].RestartCount != 8 || state.Containers[1].ExitCode != 137 {
		t.Fatalf("worker inspect not merged: %#v", state.Containers[1])
	}
	warnings := dockerHealthWarnings(state)
	if len(warnings) != 2 {
		t.Fatalf("warnings = %#v, want two", warnings)
	}
	if warnings[0] != "docker container web is unhealthy" {
		t.Fatalf("unexpected first warning: %#v", warnings)
	}
}

func TestSecurityFindingsFromOpsState(t *testing.T) {
	c := collector{}
	findings := c.securityFindings(Report{
		System: SystemState{OS: "linux"},
		Network: NetworkState{Listeners: []NetworkListener{{
			Address: "0.0.0.0",
			Port:    "8080",
			Public:  true,
		}}},
		Docker:   DockerState{Ports: []DockerPort{{Mapping: "0.0.0.0:8080->80/tcp", Public: true}}},
		Firewall: FirewallState{Warnings: []string{"ufw inactive"}},
	})
	if len(findings) < 3 {
		t.Fatalf("expected network/docker/firewall findings: %#v", findings)
	}
}

func TestLogStateIncludesStructuredFindings(t *testing.T) {
	state := LogState{
		RecentErrorCount: 1,
		Samples:          []string{"kernel: Out of memory"},
		LogFindings: []logscan.Finding{{
			Severity: "critical",
			Source:   logscan.Source{Type: logscan.SourceHostJournal, Name: "journal"},
			Pattern:  "oom",
			Count:    1,
			Sample:   "kernel: Out of memory",
		}},
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`"log_findings"`, `"pattern":"oom"`, `"count":1`} {
		if !strings.Contains(text, want) {
			t.Fatalf("log state JSON missing %s: %s", want, text)
		}
	}
}
