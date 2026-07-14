package monitor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNodeStateFromVSSHFacts(t *testing.T) {
	state := nodeStateFromVSSHFacts("d1", "100.64.0.1", &vsshServerInfo{
		Memory: &vsshMemoryInfo{Total: 1000, Used: 250},
		Load:   &vsshLoadInfo{Load1: 1.5, CPUs: 4},
		Disk: []vsshDiskInfo{
			{MountPoint: "/data", UsePercent: "80%"},
			{MountPoint: "/", UsePercent: "42%"},
		},
		GPU: []vsshGPUInfo{{Utilization: 33, MemoryUsed: 2, MemoryTotal: 4}},
	})

	if !state.Online {
		t.Fatal("expected online state")
	}
	if state.Memory != 25 {
		t.Fatalf("memory=%f", state.Memory)
	}
	if state.Disk != 42 {
		t.Fatalf("disk=%f", state.Disk)
	}
	if state.CPU != 1.5 {
		t.Fatalf("cpu=%f", state.CPU)
	}
	if state.GPUUsage != 33 {
		t.Fatalf("gpu=%f", state.GPUUsage)
	}
	if state.GPUMemory != 50 {
		t.Fatalf("gpu memory=%f", state.GPUMemory)
	}
}

func TestRootDiskPercentFallsBackToFirstDisk(t *testing.T) {
	got := rootDiskPercent([]vsshDiskInfo{{MountPoint: "/mnt", UsePercent: "77%"}})
	if got != 77 {
		t.Fatalf("got=%f", got)
	}
}

func TestParseVSSHStatus(t *testing.T) {
	output := []byte("\x1b[92m●\x1b[0m c1 10.98.144.211 0.04 9% 8% 65 days\n\x1b[91m○\x1b[0m s2 10.98.217.98 0.46 38% 65% 50 days\n")
	states := parseVSSHStatus(output, map[string]string{"c1": "10.98.144.211", "s2": "100.82.76.16"})
	if len(states) != 2 {
		t.Fatalf("states=%d", len(states))
	}
	if !states["c1"].Online || states["c1"].Memory != 9 || states["c1"].Disk != 8 {
		t.Fatalf("c1 state = %+v", states["c1"])
	}
	if states["s2"].IP != "10.98.217.98" {
		t.Fatalf("s2 ip = %q", states["s2"].IP)
	}
	if states["s2"].Online || states["s2"].Error == "" {
		t.Fatalf("s2 state = %+v", states["s2"])
	}
}

func TestOnlineStatusFallbackUsesStatusState(t *testing.T) {
	statusStates := map[string]*NodeState{
		"s2": {Name: "s2", IP: "10.99.62.28", Online: true},
	}

	state, ok := onlineStatusFallback("s2", "100.82.76.16", "auth failed", statusStates)
	if !ok {
		t.Fatal("expected status fallback")
	}
	if !state.Online {
		t.Fatalf("state = %+v", state)
	}
	if state.IP != "10.99.62.28" {
		t.Fatalf("ip = %q", state.IP)
	}
	if state.Error != "vssh facts unavailable: auth failed" {
		t.Fatalf("error = %q", state.Error)
	}
}

func TestVSSHFactsHasMetrics(t *testing.T) {
	if vsshFactsHasMetrics(&vsshServerInfo{}) {
		t.Fatal("empty facts should not count as metric-bearing")
	}
	if !vsshFactsHasMetrics(&vsshServerInfo{Memory: &vsshMemoryInfo{Total: 1000, Used: 100}}) {
		t.Fatal("memory facts should count as metric-bearing")
	}
	if !vsshFactsHasMetrics(&vsshServerInfo{Load: &vsshLoadInfo{Load1: 0.5}}) {
		t.Fatal("load facts should count as metric-bearing")
	}
	if !vsshFactsHasMetrics(&vsshServerInfo{Disk: []vsshDiskInfo{{MountPoint: "/", UsePercent: "8%"}}}) {
		t.Fatal("disk facts should count as metric-bearing")
	}
}

func TestAlternateVSSHTargetsDefaultPort(t *testing.T) {
	t.Setenv("MESHCLAW_VSSH_ALT_PORTS", "")
	got := alternateVSSHTargets("s2", "10.99.62.28")
	want := []string{"s2:48292", "10.99.62.28:48292"}
	if len(got) != len(want) {
		t.Fatalf("targets = %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("targets = %#v", got)
		}
	}
}

func TestShouldTryAlternateVSSHPort(t *testing.T) {
	if !shouldTryAlternateVSSHPort("vtls handshake: tls: first record does not look like a TLS handshake") {
		t.Fatal("expected TLS failure to use alternate port")
	}
	if !shouldTryAlternateVSSHPort("auth failed") {
		t.Fatal("expected auth failure to use alternate port")
	}
	if shouldTryAlternateVSSHPort("empty vssh facts result") {
		t.Fatal("empty facts should use status fallback, not alternate port")
	}
}

func TestDefaultConfigKeepsSSHUsersWhenConfigOmitsSSHUsers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, ".meshclaw")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "nodes.json"), []byte(`{"nodes":{"d1":{"ip":"100.64.0.1","user":"dell"}}}`), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	if cfg.SSHUsers == nil {
		t.Fatal("SSHUsers is nil")
	}
	if cfg.SSHUsers["d1"] != "dell" {
		t.Fatalf("d1 user = %q, want dell", cfg.SSHUsers["d1"])
	}
}
