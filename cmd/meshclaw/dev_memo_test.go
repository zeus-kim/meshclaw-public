package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseDevMemoAddDefaultsToNoSend(t *testing.T) {
	opts, err := parseDevMemoAdd([]string{"배포", "완료", "--source", "codex", "--status", "done", "--next", "다음 정시 브리핑 확인", "--tag", "deploy,macbook"})
	if err != nil {
		t.Fatalf("parseDevMemoAdd error = %v", err)
	}
	if opts.Send || opts.Execute {
		t.Fatalf("dev memo should default to no-send: send=%t execute=%t", opts.Send, opts.Execute)
	}
	if opts.Target != defaultDevMemoTarget {
		t.Fatalf("target = %q, want %q", opts.Target, defaultDevMemoTarget)
	}
	if len(opts.Tags) != 2 || opts.Tags[0] != "deploy" || opts.Tags[1] != "macbook" {
		t.Fatalf("tags = %#v", opts.Tags)
	}
	if opts.Source != "codex" || opts.Status != "done" || opts.Next == "" {
		t.Fatalf("metadata = source=%q status=%q next=%q", opts.Source, opts.Status, opts.Next)
	}
}

func TestRunDevMemoAddWritesMarkdownOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	opts := devMemoOptions{
		Title:  "MacBook 개발 로그",
		Body:   "local-ai LaunchAgent는 no-send 상태로 유지.",
		Source: "codex",
		Status: "done",
		Target: defaultDevMemoTarget,
		Now:    time.Date(2026, 5, 31, 23, 45, 0, 0, time.Local),
	}
	result, err := runDevMemoAdd(opts)
	if err != nil {
		t.Fatalf("runDevMemoAdd error = %v", err)
	}
	if result.Send != nil {
		t.Fatalf("send result should be nil for default no-send")
	}
	wantDir := filepath.Join(home, ".meshclaw", "dev-memos", "2026-05-31")
	if !strings.HasPrefix(result.Path, wantDir) {
		t.Fatalf("path = %q, want under %q", result.Path, wantDir)
	}
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("read memo: %v", err)
	}
	text := string(data)
	for _, want := range []string{"# MacBook 개발 로그", "node: macbook", "source: codex", "status: done", "send: false", "local-ai LaunchAgent"} {
		if !strings.Contains(text, want) {
			t.Fatalf("memo missing %q:\n%s", want, text)
		}
	}
}

func TestDevMemoAddStdinStoresDevWorkerMarkdownSafely(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	latest := strings.Join([]string{
		"# MeshClaw dev worker latest",
		"",
		"- stamp: 20260612T010203Z",
		"- source: dev-worker-loop",
		"- status: pass",
		"- send: true",
		"",
		"## Check Summary",
		"",
		"- go-test-focused: pass",
		"- local-lite-smoke: pass",
		"",
		"next: keep dev-memo digest wired into automation records",
	}, "\n")
	oldReadStdin := devMemoReadStdin
	devMemoReadStdin = func() ([]byte, error) {
		return []byte(latest), nil
	}
	t.Cleanup(func() {
		devMemoReadStdin = oldReadStdin
	})
	opts, err := parseDevMemoAdd([]string{"--stdin", "--source", "dev-worker-loop", "--status", "done", "--next", "review latest worker handoff"})
	if err != nil {
		t.Fatalf("parseDevMemoAdd error = %v", err)
	}
	if opts.Send || opts.Execute {
		t.Fatalf("--stdin dev worker memo should default to no-send: send=%t execute=%t", opts.Send, opts.Execute)
	}
	if opts.Title != "MeshClaw dev worker latest" {
		t.Fatalf("title = %q, want dev-worker markdown heading", opts.Title)
	}
	result, err := runDevMemoAdd(devMemoOptions{
		Title:  opts.Title,
		Body:   opts.Body,
		Source: opts.Source,
		Status: opts.Status,
		Next:   opts.Next,
		Target: opts.Target,
		Send:   opts.Send,
		Now:    time.Date(2026, 6, 12, 10, 2, 3, 0, time.Local),
	})
	if err != nil {
		t.Fatalf("runDevMemoAdd error = %v", err)
	}
	if result.Send != nil {
		t.Fatalf("send result should be nil for default no-send")
	}
	if !strings.Contains(result.ScopeNote, "MacBook development memo only") ||
		!strings.Contains(result.ScopeNote, "Argos assistant/reporting rooms unless an operator explicitly configures") {
		t.Fatalf("scope_note does not describe MacBook-only scope: %q", result.ScopeNote)
	}
	if result.Metadata["source"] != "dev-worker-loop" || result.Metadata["status"] != "done" || result.Metadata["send"] != false || result.Metadata["node"] != "macbook" {
		t.Fatalf("metadata was not kept safe: %#v", result.Metadata)
	}
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("read memo: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"# MeshClaw dev worker latest",
		"- source: dev-worker-loop",
		"- status: done",
		"- send: false",
		"- next: review latest worker handoff",
		"## Check Summary",
		"next: keep dev-memo digest wired into automation records",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("memo missing %q:\n%s", want, text)
		}
	}
}

func TestParseDevMemoExecuteImpliesSend(t *testing.T) {
	opts, err := parseDevMemoAdd([]string{"중요 변경", "--execute"})
	if err != nil {
		t.Fatalf("parseDevMemoAdd error = %v", err)
	}
	if !opts.Send || !opts.Execute {
		t.Fatalf("--execute should imply send: send=%t execute=%t", opts.Send, opts.Execute)
	}
}

func TestRunDevMemoDigestSummarizesSources(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	now := time.Date(2026, 5, 31, 23, 55, 0, 0, time.Local)
	_, err := runDevMemoAdd(devMemoOptions{
		Title:  "Codex 배포 검증",
		Body:   "macmini local-ai 브리핑 경로 확인.",
		Source: "codex",
		Status: "done",
		Next:   "정시 브리핑 결과 확인",
		Target: defaultDevMemoTarget,
		Now:    now.Add(-10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("runDevMemoAdd codex: %v", err)
	}
	_, err = runDevMemoAdd(devMemoOptions{
		Title:  "Cursor 문서 정리 대기",
		Body:   "공개 설치 UX 문서가 아직 남음.",
		Source: "cursor",
		Status: "blocked",
		Target: defaultDevMemoTarget,
		Now:    now.Add(-5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("runDevMemoAdd cursor: %v", err)
	}
	repo := initDevMemoTestRepo(t)
	result, err := runDevMemoDigest(devMemoDigestOptions{Date: "2026-05-31", Repo: repo, Target: defaultDevMemoTarget, Now: now})
	if err != nil {
		t.Fatalf("runDevMemoDigest error = %v", err)
	}
	if result.Send != nil {
		t.Fatalf("digest should default to no-send")
	}
	for _, want := range []string{"Git", "branch:", "status: clean", "총 2개 기록", "codex 1", "cursor 1", "[codex] Codex 배포 검증", "⚠️ 막힌 점", "정시 브리핑 결과 확인"} {
		if !strings.Contains(result.Text, want) {
			t.Fatalf("digest missing %q:\n%s", want, result.Text)
		}
	}
	if _, err := os.Stat(result.Path); err != nil {
		t.Fatalf("digest file missing: %v", err)
	}
}

func TestRunDevMemoDigestIncludesDevWorkerLoopMemo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	now := time.Date(2026, 6, 12, 10, 20, 0, 0, time.Local)
	_, err := runDevMemoAdd(devMemoOptions{
		Title:  "MeshClaw dev worker loop 20260612T012000Z",
		Body:   "# MeshClaw dev worker latest\n\n- source: dev-worker-loop\n- status: pass\n\n## Check Summary\n\n- go-test-focused: pass",
		Source: "dev-worker-loop",
		Status: "done",
		Next:   "feed latest worker handoff into next dev-memo digest",
		Target: defaultDevMemoTarget,
		Now:    now.Add(-2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("runDevMemoAdd dev-worker-loop: %v", err)
	}
	_, err = runDevMemoAdd(devMemoOptions{
		Title:  "Cursor follow-up",
		Body:   "docs handoff still needs review.",
		Source: "cursor",
		Status: "blocked",
		Next:   "resolve docs review",
		Target: defaultDevMemoTarget,
		Now:    now.Add(-1 * time.Minute),
	})
	if err != nil {
		t.Fatalf("runDevMemoAdd cursor: %v", err)
	}
	repo := initDevMemoTestRepo(t)
	result, err := runDevMemoDigest(devMemoDigestOptions{Date: "2026-06-12", Repo: repo, Target: defaultDevMemoTarget, Now: now})
	if err != nil {
		t.Fatalf("runDevMemoDigest error = %v", err)
	}
	if result.Send != nil {
		t.Fatalf("digest should default to no-send")
	}
	for _, want := range []string{
		"총 2개 기록",
		"dev-worker-loop 1",
		"cursor 1",
		"[dev-worker-loop] MeshClaw dev worker loop 20260612T012000Z",
		"⚠️ 막힌 점",
		"[cursor] Cursor follow-up",
		"[dev-worker-loop] feed latest worker handoff into next dev-memo digest",
		"[cursor] resolve docs review",
	} {
		if !strings.Contains(result.Text, want) {
			t.Fatalf("digest missing %q:\n%s", want, result.Text)
		}
	}
}

func TestCollectDevMemoGitStatusCountsDirtyFiles(t *testing.T) {
	repo := initDevMemoTestRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("new\n"), 0600); err != nil {
		t.Fatal(err)
	}
	status := collectDevMemoGitStatus(repo)
	if status.Error != "" {
		t.Fatalf("git status error = %s", status.Error)
	}
	if !status.Dirty || status.Modified != 1 || status.Untracked != 1 {
		t.Fatalf("unexpected status: %#v", status)
	}
	if status.Head == "" || status.HeadTitle != "initial commit" {
		t.Fatalf("head = %q title = %q", status.Head, status.HeadTitle)
	}
}

func initDevMemoTestRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "meshclaw@example.invalid")
	runGit(t, repo, "config", "user.name", "MeshClaw Test")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("initial\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "tracked.txt")
	runGit(t, repo, "commit", "-m", "initial commit")
	return repo
}

func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
