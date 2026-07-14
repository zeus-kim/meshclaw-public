package workers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLinuxWorkerRegistryAddSelectRemove(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_LINUX_WORKERS_FILE", filepath.Join(dir, "linux-workers.json"))

	store, node, err := UpsertLinuxWorker(LinuxWorkerNode{
		ID:        "g4",
		SSHTarget: "operator@g4",
		Role:      "llm-chat-worker",
		Tags:      []string{"gpu", "llm"},
		Enabled:   true,
		Selected:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if node.ID != "g4" || !node.Selected || !containsStringLocal(node.Tags, "g-series") || !containsStringLocal(node.Tags, "no-desktop") {
		t.Fatalf("node=%#v", node)
	}
	if len(store.Nodes) != 1 {
		t.Fatalf("store=%#v", store)
	}

	selected, ok := SelectedLinuxWorker()
	if !ok || selected.ID != "g4" {
		t.Fatalf("selected=%#v ok=%t", selected, ok)
	}

	store, removed, err := RemoveLinuxWorker("g4")
	if err != nil {
		t.Fatal(err)
	}
	if !removed || len(store.Nodes) != 0 {
		t.Fatalf("removed=%t store=%#v", removed, store)
	}
}

func TestDoctorLinuxWorkerUsesSelectedRegistryNode(t *testing.T) {
	dir := t.TempDir()
	ssh := filepath.Join(dir, "ssh")
	fakeSSH := `#!/bin/sh
last=""
for arg in "$@"; do
  last="$arg"
done
case "$last" in
  *uname*) printf 'meshclaw-linux-worker-ok\nLinux g4\n' ;;
  *meshclaw*) printf '1.2.47\n' ;;
  *python3*) printf 'Python 3.11.9\n' ;;
  *nvidia-smi*) printf 'NVIDIA RTX, 24576 MiB\n' ;;
  *ollama*) printf 'gemma3:4b\n' ;;
  *) printf 'ok\n' ;;
esac
`
	if err := os.WriteFile(ssh, []byte(fakeSSH), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MESHCLAW_LINUX_WORKERS_FILE", filepath.Join(dir, "linux-workers.json"))
	_, _, err := UpsertLinuxWorker(LinuxWorkerNode{
		ID:        "g4",
		SSHTarget: "operator@g4",
		Role:      "llm-chat-worker",
		Tags:      []string{"gpu", "llm", "ollama"},
		Enabled:   true,
		Selected:  true,
	})
	if err != nil {
		t.Fatal(err)
	}

	report := DoctorLinuxWorker(context.Background(), "")
	if !report.OK || report.Node.ID != "g4" || len(report.Checks) != 5 {
		t.Fatalf("report=%#v", report)
	}
	for _, check := range report.Checks {
		if !check.OK || check.Command[1] != "operator@g4" {
			t.Fatalf("check=%#v", check)
		}
	}
}

func TestDoctorLinuxWorkerNoNode(t *testing.T) {
	t.Setenv("MESHCLAW_LINUX_WORKERS_FILE", filepath.Join(t.TempDir(), "linux-workers.json"))
	report := DoctorLinuxWorker(context.Background(), "")
	if report.OK || len(report.Problems) == 0 || !strings.Contains(report.Problems[0], "no linux worker") {
		t.Fatalf("report=%#v", report)
	}
}

func TestRunLinuxWorkerJobUsesSelectedNode(t *testing.T) {
	dir := t.TempDir()
	ssh := filepath.Join(dir, "ssh")
	fakeSSH := `#!/bin/sh
printf '%s\n' "$@" > ` + filepath.Join(dir, "argv") + `
printf 'worker job done\n'
`
	if err := os.WriteFile(ssh, []byte(fakeSSH), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MESHCLAW_LINUX_WORKERS_FILE", filepath.Join(dir, "linux-workers.json"))
	_, _, err := UpsertLinuxWorker(LinuxWorkerNode{
		ID:        "g4",
		SSHTarget: "operator@g4",
		Role:      "llm-chat-worker",
		Enabled:   true,
		Selected:  true,
	})
	if err != nil {
		t.Fatal(err)
	}

	result := RunLinuxWorkerJob(context.Background(), LinuxWorkerJobOptions{
		Task:    "뉴스 리서치",
		Command: "printf ok",
		Timeout: 2 * time.Second,
	})
	if !result.OK || result.Worker.ID != "g4" || !strings.Contains(result.Stdout, "worker job done") {
		t.Fatalf("result=%#v", result)
	}
	if len(result.SSHCommand) != 3 || result.SSHCommand[1] != "operator@g4" || result.Command != "printf ok" {
		t.Fatalf("ssh command=%#v command=%q", result.SSHCommand, result.Command)
	}
}

func TestBuildLinuxWorkerTaskCommandUsesNewsResearchScript(t *testing.T) {
	command := buildLinuxWorkerTaskCommand("오늘 AI 뉴스 리서치")
	if !strings.Contains(command, "meshclaw-worker-news-ok") || !strings.Contains(command, "news.google.com/rss") {
		t.Fatalf("expected news research command, got:\n%s", command)
	}
	if !strings.Contains(command, `TASK = "오늘 AI 뉴스 리서치"`) {
		t.Fatalf("task was not embedded safely: %s", command)
	}
}

func TestBuildLinuxWorkerTaskCommandUsesRuntimeCheckForGenericTask(t *testing.T) {
	command := buildLinuxWorkerTaskCommand("런타임 확인")
	if !strings.Contains(command, "meshclaw-worker-job-ok") || strings.Contains(command, "meshclaw-worker-news-ok") {
		t.Fatalf("expected runtime check command, got:\n%s", command)
	}
}

func TestRunLinuxWorkerJobNoNode(t *testing.T) {
	t.Setenv("MESHCLAW_LINUX_WORKERS_FILE", filepath.Join(t.TempDir(), "linux-workers.json"))
	result := RunLinuxWorkerJob(context.Background(), LinuxWorkerJobOptions{Task: "뉴스 리서치"})
	if result.OK || !strings.Contains(result.Error, "no linux worker") {
		t.Fatalf("result=%#v", result)
	}
}
