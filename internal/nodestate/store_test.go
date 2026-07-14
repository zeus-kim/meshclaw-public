package nodestate

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreAndListReports(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	report := Report{
		Kind:        ReportKind,
		Version:     "1",
		Hostname:    "test-host",
		NodeName:    "test-host",
		CollectedAt: time.Now().UTC(),
		System:      SystemState{OS: "linux", Arch: "amd64", CPUCount: 4, MemoryPct: 10, DiskPct: 20},
	}
	stored, err := Store(report)
	if err != nil {
		t.Fatal(err)
	}
	if stored.StatePath == "" {
		t.Fatal("state path is empty")
	}
	if _, err := os.Stat(filepath.Join(home, ".meshclaw", "state", "nodes", "test-host.json")); err != nil {
		t.Fatal(err)
	}
	reports, err := List(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("reports len = %d", len(reports))
	}
	if reports[0].Report.NodeName != "test-host" {
		t.Fatalf("node = %q", reports[0].Report.NodeName)
	}
}

func TestStoreWithHistoryAndTail(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for i := 0; i < 3; i++ {
		_, err := StoreWithOptions(Report{
			Kind:        ReportKind,
			Version:     "1",
			Hostname:    "history-host",
			NodeName:    "history-host",
			CollectedAt: time.Now().UTC(),
			System:      SystemState{OS: "linux", Arch: "amd64", CPUCount: 4, MemoryPct: float64(i)},
		}, StoreOptions{AppendHistory: true, MaxHistory: 2})
		if err != nil {
			t.Fatal(err)
		}
	}
	history, err := TailHistory("history-host", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("history len = %d", len(history))
	}
	if history[0].Report.System.MemoryPct != 1 || history[1].Report.System.MemoryPct != 2 {
		t.Fatalf("history was not trimmed to latest entries: %#v", history)
	}
}
