package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/messenger"
)

const defaultDevMemoTarget = "macbook-dev-memo"

var devMemoReadStdin = func() ([]byte, error) {
	return os.ReadFile("/dev/stdin")
}

type devMemoOptions struct {
	Title   string   `json:"title"`
	Body    string   `json:"body"`
	Source  string   `json:"source"`
	Status  string   `json:"status"`
	Next    string   `json:"next,omitempty"`
	Tags    []string `json:"tags,omitempty"`
	Target  string   `json:"target"`
	Send    bool     `json:"send"`
	Execute bool     `json:"execute"`
	JSON    bool     `json:"json"`
	Now     time.Time
}

type devMemoResult struct {
	Path       string                 `json:"path"`
	Text       string                 `json:"text"`
	Target     string                 `json:"target,omitempty"`
	Send       *messenger.SendResult  `json:"send,omitempty"`
	Evidence   evidence.Record        `json:"evidence,omitempty"`
	StoreError string                 `json:"store_error,omitempty"`
	Error      string                 `json:"error,omitempty"`
	ScopeNote  string                 `json:"scope_note"`
	Metadata   map[string]interface{} `json:"metadata"`
}

type devMemoDigestOptions struct {
	Date    string `json:"date"`
	Repo    string `json:"repo,omitempty"`
	Target  string `json:"target"`
	Send    bool   `json:"send"`
	Execute bool   `json:"execute"`
	JSON    bool   `json:"json"`
	Now     time.Time
}

type devMemoEntry struct {
	Path   string `json:"path"`
	Time   string `json:"time,omitempty"`
	Title  string `json:"title"`
	Source string `json:"source,omitempty"`
	Status string `json:"status,omitempty"`
	Next   string `json:"next,omitempty"`
	Body   string `json:"body,omitempty"`
}

type devMemoGitStatus struct {
	Repo      string `json:"repo,omitempty"`
	Branch    string `json:"branch,omitempty"`
	Head      string `json:"head,omitempty"`
	HeadTitle string `json:"head_title,omitempty"`
	Dirty     bool   `json:"dirty"`
	Staged    int    `json:"staged"`
	Modified  int    `json:"modified"`
	Untracked int    `json:"untracked"`
	Error     string `json:"error,omitempty"`
}

func devMemoCommand(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Println(devMemoUsage())
		return nil
	}
	switch args[0] {
	case "add", "write":
		opts, err := parseDevMemoAdd(args[1:])
		if err != nil {
			return err
		}
		result, runErr := runDevMemoAdd(opts)
		if opts.JSON {
			if err := printJSON(result); err != nil {
				return err
			}
			return runErr
		}
		fmt.Printf("dev_memo=%s\n", result.Path)
		if result.Send != nil {
			if result.Send.Executed {
				fmt.Printf("signal_sent=%s\n", result.Target)
			} else {
				fmt.Printf("signal_dry_run=%s\n", result.Target)
			}
		}
		return runErr
	case "digest", "summary", "report":
		opts, err := parseDevMemoDigest(args[1:])
		if err != nil {
			return err
		}
		result, runErr := runDevMemoDigest(opts)
		if opts.JSON {
			if err := printJSON(result); err != nil {
				return err
			}
			return runErr
		}
		fmt.Printf("dev_memo_digest=%s\n", result.Path)
		if result.Send != nil {
			if result.Send.Executed {
				fmt.Printf("signal_sent=%s\n", result.Target)
			} else {
				fmt.Printf("signal_dry_run=%s\n", result.Target)
			}
		}
		return runErr
	default:
		return fmt.Errorf("unknown dev-memo command: %s", args[0])
	}
}

func devMemoUsage() string {
	return `usage:
  meshclaw dev-memo add <text> [--source codex|cursor|claude|ai-studio] [--status done|blocked|note] [--next text] [--title text] [--tag tag[,tag]] [--target macbook-dev-memo] [--send] [--execute] [--json]
  meshclaw dev-memo digest [--date YYYY-MM-DD] [--repo path] [--target macbook-dev-memo] [--send] [--execute] [--json]

Notes:
  Stores a MacBook-only development memo under ~/.meshclaw/dev-memos.
  Digest summarizes Codex/Cursor/Claude/AI Studio memos and current git state into one report.
  Signal sending is opt-in and should target a private MacBook memo target, not Argos/report rooms.`
}

func parseDevMemoAdd(args []string) (devMemoOptions, error) {
	opts := devMemoOptions{Source: "manual", Status: "note", Target: defaultDevMemoTarget, Now: time.Now()}
	parts := []string{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--title":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("--title requires text")
			}
			opts.Title = strings.TrimSpace(args[i])
		case "--body":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("--body requires text")
			}
			opts.Body = strings.TrimSpace(args[i])
		case "--tag", "--tags":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("%s requires a comma-separated value", args[i-1])
			}
			opts.Tags = append(opts.Tags, splitCSV(args[i])...)
		case "--source":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("--source requires a value")
			}
			opts.Source = strings.TrimSpace(args[i])
		case "--status":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("--status requires a value")
			}
			opts.Status = strings.TrimSpace(args[i])
		case "--next":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("--next requires text")
			}
			opts.Next = strings.TrimSpace(args[i])
		case "--target":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("--target requires a messenger target id")
			}
			opts.Target = strings.TrimSpace(args[i])
		case "--send":
			opts.Send = true
		case "--execute":
			opts.Send = true
			opts.Execute = true
		case "--dry-run", "--no-send":
			opts.Send = false
			opts.Execute = false
		case "--json":
			opts.JSON = true
		case "--stdin":
			data, err := devMemoReadStdin()
			if err != nil {
				return opts, err
			}
			opts.Body = strings.TrimSpace(string(data))
		default:
			if strings.HasPrefix(args[i], "--") {
				return opts, fmt.Errorf("unknown dev-memo option: %s", args[i])
			}
			parts = append(parts, args[i])
		}
	}
	if opts.Body == "" {
		opts.Body = strings.TrimSpace(strings.Join(parts, " "))
	}
	if opts.Body == "-" {
		data, err := devMemoReadStdin()
		if err != nil {
			return opts, err
		}
		opts.Body = strings.TrimSpace(string(data))
	}
	if opts.Body == "" {
		return opts, fmt.Errorf("dev-memo text is required")
	}
	if opts.Title == "" {
		opts.Title = defaultDevMemoTitle(opts.Body)
	}
	if opts.Target == "" {
		opts.Target = defaultDevMemoTarget
	}
	if opts.Source == "" {
		opts.Source = "manual"
	}
	if opts.Status == "" {
		opts.Status = "note"
	}
	return opts, nil
}

func runDevMemoAdd(opts devMemoOptions) (devMemoResult, error) {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	text := formatDevMemoSignalText(opts)
	path, err := writeDevMemo(opts, text)
	result := devMemoResult{
		Path:      path,
		Text:      text,
		Target:    opts.Target,
		ScopeNote: "MacBook development memo only. This does not use the Mac mini Argos assistant/reporting rooms unless an operator explicitly configures that target.",
		Metadata: map[string]interface{}{
			"title":   opts.Title,
			"source":  opts.Source,
			"status":  opts.Status,
			"next":    opts.Next,
			"tags":    opts.Tags,
			"send":    opts.Send,
			"execute": opts.Execute,
			"node":    "macbook",
		},
	}
	record, storeErr := evidence.Store("dev-memo", "macbook", opts.Title, map[string]interface{}{
		"path":    path,
		"title":   opts.Title,
		"source":  opts.Source,
		"status":  opts.Status,
		"next":    opts.Next,
		"tags":    opts.Tags,
		"send":    opts.Send,
		"execute": opts.Execute,
	})
	result.Evidence = record
	result.StoreError = errString(storeErr)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	if opts.Send {
		send, sendErr := messenger.Send(messenger.SendOptions{
			TargetID: opts.Target,
			Kind:     "text",
			Text:     text,
			Execute:  opts.Execute,
		})
		result.Send = &send
		if sendErr != nil {
			result.Error = sendErr.Error()
			return result, sendErr
		}
	}
	return result, nil
}

func writeDevMemo(opts devMemoOptions, signalText string) (string, error) {
	root, err := devMemoRoot(opts.Now)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	name := opts.Now.Format("150405") + "-" + devMemoSlug(opts.Source) + "-" + devMemoSlug(opts.Title) + ".md"
	path := filepath.Join(root, name)
	content := strings.Join([]string{
		"# " + opts.Title,
		"",
		"- time: " + opts.Now.Format(time.RFC3339),
		"- node: macbook",
		"- source: " + opts.Source,
		"- status: " + opts.Status,
		"- target: " + opts.Target,
		"- send: " + fmt.Sprintf("%t", opts.Send),
		"",
		signalText,
		"",
	}, "\n")
	if len(opts.Tags) > 0 {
		content = strings.Replace(content, "- send: "+fmt.Sprintf("%t", opts.Send)+"\n", "- send: "+fmt.Sprintf("%t", opts.Send)+"\n- tags: "+strings.Join(opts.Tags, ", ")+"\n", 1)
	}
	if strings.TrimSpace(opts.Next) != "" {
		content = strings.Replace(content, "- send: "+fmt.Sprintf("%t", opts.Send)+"\n", "- send: "+fmt.Sprintf("%t", opts.Send)+"\n- next: "+strings.TrimSpace(opts.Next)+"\n", 1)
	}
	return path, os.WriteFile(path, []byte(content), 0600)
}

func devMemoRoot(now time.Time) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".meshclaw", "dev-memos", now.Format("2006-01-02")), nil
}

func formatDevMemoSignalText(opts devMemoOptions) string {
	lines := []string{
		"🛠️ MeshClaw 개발 메모 — " + opts.Now.Format("01/02 15:04"),
		"",
		opts.Title,
		fmt.Sprintf("source: %s · status: %s", opts.Source, opts.Status),
		"",
		strings.TrimSpace(opts.Body),
	}
	if strings.TrimSpace(opts.Next) != "" {
		lines = append(lines, "", "다음: "+strings.TrimSpace(opts.Next))
	}
	if len(opts.Tags) > 0 {
		lines = append(lines, "", "tags: "+strings.Join(opts.Tags, ", "))
	}
	lines = append(lines, "", "— MacBook")
	return strings.Join(lines, "\n")
}

func parseDevMemoDigest(args []string) (devMemoDigestOptions, error) {
	opts := devMemoDigestOptions{Target: defaultDevMemoTarget, Now: time.Now()}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--date":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("--date requires YYYY-MM-DD")
			}
			opts.Date = strings.TrimSpace(args[i])
		case "--target":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("--target requires a messenger target id")
			}
			opts.Target = strings.TrimSpace(args[i])
		case "--repo":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("--repo requires a path")
			}
			opts.Repo = strings.TrimSpace(args[i])
		case "--send":
			opts.Send = true
		case "--execute":
			opts.Send = true
			opts.Execute = true
		case "--dry-run", "--no-send":
			opts.Send = false
			opts.Execute = false
		case "--json":
			opts.JSON = true
		default:
			return opts, fmt.Errorf("unknown dev-memo digest option: %s", args[i])
		}
	}
	if opts.Date == "" {
		opts.Date = opts.Now.Format("2006-01-02")
	}
	if opts.Target == "" {
		opts.Target = defaultDevMemoTarget
	}
	if opts.Repo == "" {
		if cwd, err := os.Getwd(); err == nil {
			opts.Repo = cwd
		}
	}
	if _, err := time.Parse("2006-01-02", opts.Date); err != nil {
		return opts, fmt.Errorf("invalid --date %q: use YYYY-MM-DD", opts.Date)
	}
	return opts, nil
}

func runDevMemoDigest(opts devMemoDigestOptions) (devMemoResult, error) {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	entries, err := loadDevMemoEntries(opts)
	if err != nil {
		return devMemoResult{Error: err.Error()}, err
	}
	gitStatus := collectDevMemoGitStatus(opts.Repo)
	text := formatDevMemoDigestText(opts, entries, gitStatus)
	path, writeErr := writeDevMemoDigest(opts, text)
	result := devMemoResult{
		Path:      path,
		Text:      text,
		Target:    opts.Target,
		ScopeNote: "MacBook development digest only. It summarizes local Codex/Cursor/Claude/AI Studio work logs and does not touch Argos report rooms by default.",
		Metadata: map[string]interface{}{
			"date":    opts.Date,
			"repo":    gitStatus.Repo,
			"git":     gitStatus,
			"entries": len(entries),
			"send":    opts.Send,
			"execute": opts.Execute,
			"node":    "macbook",
		},
	}
	record, storeErr := evidence.Store("dev-memo-digest", "macbook", fmt.Sprintf("%s entries=%d", opts.Date, len(entries)), map[string]interface{}{"path": path, "date": opts.Date, "repo": gitStatus.Repo, "git": gitStatus, "entries": entries, "send": opts.Send, "execute": opts.Execute})
	result.Evidence = record
	result.StoreError = errString(storeErr)
	if writeErr != nil {
		result.Error = writeErr.Error()
		return result, writeErr
	}
	if opts.Send {
		send, sendErr := messenger.Send(messenger.SendOptions{TargetID: opts.Target, Kind: "text", Text: text, Execute: opts.Execute})
		result.Send = &send
		if sendErr != nil {
			result.Error = sendErr.Error()
			return result, sendErr
		}
	}
	return result, nil
}

func loadDevMemoEntries(opts devMemoDigestOptions) ([]devMemoEntry, error) {
	day, err := time.Parse("2006-01-02", opts.Date)
	if err != nil {
		return nil, err
	}
	root, err := devMemoRoot(day)
	if err != nil {
		return nil, err
	}
	matches, err := filepath.Glob(filepath.Join(root, "*.md"))
	if err != nil {
		return nil, err
	}
	entries := []devMemoEntry{}
	for _, path := range matches {
		if strings.HasPrefix(filepath.Base(path), "digest-") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		entry := parseDevMemoEntry(path, string(data))
		if strings.TrimSpace(entry.Title) != "" {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func parseDevMemoEntry(path, content string) devMemoEntry {
	entry := devMemoEntry{Path: path}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	body := []string{}
	inBody := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") && entry.Title == "" {
			entry.Title = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			continue
		}
		if strings.HasPrefix(trimmed, "- ") && !inBody {
			key, value, ok := strings.Cut(strings.TrimPrefix(trimmed, "- "), ":")
			if ok {
				switch strings.TrimSpace(key) {
				case "time":
					entry.Time = strings.TrimSpace(value)
				case "source":
					entry.Source = strings.TrimSpace(value)
				case "status":
					entry.Status = strings.TrimSpace(value)
				case "next":
					entry.Next = strings.TrimSpace(value)
				}
			}
			continue
		}
		if strings.HasPrefix(trimmed, "🛠️ ") {
			inBody = true
			continue
		}
		if inBody {
			if strings.HasPrefix(trimmed, "tags:") || strings.HasPrefix(trimmed, "— MacBook") || strings.HasPrefix(trimmed, "source:") || trimmed == entry.Title {
				continue
			}
			if strings.TrimSpace(line) != "" {
				body = append(body, strings.TrimSpace(line))
			}
		}
	}
	entry.Body = strings.Join(body, " ")
	if entry.Source == "" {
		entry.Source = "manual"
	}
	if entry.Status == "" {
		entry.Status = "note"
	}
	return entry
}

func writeDevMemoDigest(opts devMemoDigestOptions, text string) (string, error) {
	day, err := time.Parse("2006-01-02", opts.Date)
	if err != nil {
		return "", err
	}
	root, err := devMemoRoot(day)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(root, "digest-"+opts.Now.Format("150405")+".md")
	content := strings.Join([]string{
		"# MeshClaw 개발 요약 " + opts.Date,
		"",
		"- time: " + opts.Now.Format(time.RFC3339),
		"- node: macbook",
		"- target: " + opts.Target,
		"- send: " + fmt.Sprintf("%t", opts.Send),
		"",
		text,
		"",
	}, "\n")
	return path, os.WriteFile(path, []byte(content), 0600)
}

func formatDevMemoDigestText(opts devMemoDigestOptions, entries []devMemoEntry, gitStatus devMemoGitStatus) string {
	lines := []string{
		"🧭 MeshClaw 개발 요약 — " + opts.Date,
		"",
	}
	if gitStatus.Repo != "" || gitStatus.Error != "" {
		lines = append(lines, "Git")
		if gitStatus.Error != "" {
			lines = append(lines, "• 상태 확인 실패: "+gitStatus.Error)
		} else {
			dirty := "clean"
			if gitStatus.Dirty {
				dirty = "dirty"
			}
			lines = append(lines,
				"• repo: "+gitStatus.Repo,
				fmt.Sprintf("• branch: %s · HEAD: %s", firstNonEmpty(gitStatus.Branch, "unknown"), firstNonEmpty(gitStatus.Head, "unknown")),
				"• latest: "+firstNonEmpty(gitStatus.HeadTitle, "unknown"),
				fmt.Sprintf("• status: %s · staged %d · modified %d · untracked %d", dirty, gitStatus.Staged, gitStatus.Modified, gitStatus.Untracked),
			)
		}
		lines = append(lines, "")
	}
	if len(entries) == 0 {
		lines = append(lines, "오늘 기록된 개발 메모가 없습니다.", "", "— MacBook")
		return strings.Join(lines, "\n")
	}
	counts := map[string]int{}
	for _, entry := range entries {
		counts[entry.Source]++
	}
	sources := make([]string, 0, len(counts))
	for source := range counts {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	parts := []string{}
	for _, source := range sources {
		parts = append(parts, fmt.Sprintf("%s %d", source, counts[source]))
	}
	lines = append(lines, fmt.Sprintf("총 %d개 기록 · %s", len(entries), strings.Join(parts, " · ")), "")
	lines = append(lines, "✅ 진행")
	for _, entry := range entries {
		if entry.Status == "blocked" {
			continue
		}
		lines = append(lines, fmt.Sprintf("• [%s] %s", entry.Source, entry.Title))
	}
	blocked := []devMemoEntry{}
	next := []string{}
	for _, entry := range entries {
		if entry.Status == "blocked" {
			blocked = append(blocked, entry)
		}
		if strings.TrimSpace(entry.Next) != "" {
			next = append(next, fmt.Sprintf("• [%s] %s", entry.Source, entry.Next))
		}
	}
	if len(blocked) > 0 {
		lines = append(lines, "", "⚠️ 막힌 점")
		for _, entry := range blocked {
			lines = append(lines, fmt.Sprintf("• [%s] %s", entry.Source, entry.Title))
		}
	}
	if len(next) > 0 {
		lines = append(lines, "", "다음")
		lines = append(lines, next...)
	}
	lines = append(lines, "", "— MacBook")
	return strings.Join(lines, "\n")
}

func collectDevMemoGitStatus(repo string) devMemoGitStatus {
	status := devMemoGitStatus{Repo: strings.TrimSpace(repo)}
	if status.Repo == "" {
		status.Error = "repo path is empty"
		return status
	}
	root, err := gitOutput(status.Repo, "rev-parse", "--show-toplevel")
	if err != nil {
		status.Error = strings.TrimSpace(err.Error())
		return status
	}
	status.Repo = strings.TrimSpace(root)
	branch, err := gitOutput(status.Repo, "branch", "--show-current")
	if err == nil {
		status.Branch = strings.TrimSpace(branch)
	}
	head, err := gitOutput(status.Repo, "rev-parse", "--short", "HEAD")
	if err == nil {
		status.Head = strings.TrimSpace(head)
	}
	title, err := gitOutput(status.Repo, "log", "-1", "--pretty=%s")
	if err == nil {
		status.HeadTitle = strings.TrimSpace(title)
	}
	porcelain, err := gitOutput(status.Repo, "status", "--porcelain")
	if err != nil {
		status.Error = strings.TrimSpace(err.Error())
		return status
	}
	for _, line := range strings.Split(strings.TrimRight(porcelain, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		status.Dirty = true
		if strings.HasPrefix(line, "??") {
			status.Untracked++
			continue
		}
		if len(line) >= 1 && line[0] != ' ' {
			status.Staged++
		}
		if len(line) >= 2 && line[1] != ' ' {
			status.Modified++
		}
	}
	return status
}

func gitOutput(repo string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func defaultDevMemoTitle(body string) string {
	line := strings.TrimSpace(strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")[0])
	line = strings.Trim(line, "#*•- \t")
	if line == "" {
		return "MeshClaw 개발 메모"
	}
	runes := []rune(line)
	if len(runes) > 48 {
		line = string(runes[:48])
	}
	return line
}

func devMemoSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "memo"
	}
	if len(slug) > 64 {
		slug = strings.Trim(slug[:64], "-")
	}
	return slug
}
