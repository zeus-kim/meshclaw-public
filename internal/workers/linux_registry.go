package workers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"
)

const linuxWorkerStoreVersion = 1

type LinuxWorkerNode struct {
	ID           string    `json:"id"`
	Name         string    `json:"name,omitempty"`
	SSHTarget    string    `json:"ssh_target,omitempty"`
	Address      string    `json:"address,omitempty"`
	User         string    `json:"user,omitempty"`
	Role         string    `json:"role,omitempty"`
	Capabilities []string  `json:"capabilities,omitempty"`
	Tags         []string  `json:"tags,omitempty"`
	Enabled      bool      `json:"enabled"`
	Selected     bool      `json:"selected,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type LinuxWorkerStore struct {
	Kind      string            `json:"kind"`
	Version   int               `json:"version"`
	Path      string            `json:"path"`
	Nodes     []LinuxWorkerNode `json:"nodes"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type LinuxWorkerDoctorReport struct {
	Kind        string             `json:"kind"`
	OK          bool               `json:"ok"`
	Node        LinuxWorkerNode    `json:"node,omitempty"`
	Checks      []LinuxWorkerCheck `json:"checks,omitempty"`
	Problems    []string           `json:"problems,omitempty"`
	NextActions []string           `json:"next_actions,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
}

type LinuxWorkerCheck struct {
	Kind      string    `json:"kind"`
	Action    string    `json:"action"`
	Command   []string  `json:"command,omitempty"`
	OK        bool      `json:"ok"`
	Stdout    string    `json:"stdout,omitempty"`
	Stderr    string    `json:"stderr,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type LinuxWorkerJobOptions struct {
	WorkerID string
	Task     string
	Command  string
	Timeout  time.Duration
}

type LinuxWorkerJobResult struct {
	Kind       string          `json:"kind"`
	OK         bool            `json:"ok"`
	Worker     LinuxWorkerNode `json:"worker,omitempty"`
	Task       string          `json:"task,omitempty"`
	Command    string          `json:"command,omitempty"`
	SSHCommand []string        `json:"ssh_command,omitempty"`
	Stdout     string          `json:"stdout,omitempty"`
	Stderr     string          `json:"stderr,omitempty"`
	Error      string          `json:"error,omitempty"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt time.Time       `json:"finished_at"`
	DurationMS int64           `json:"duration_ms"`
}

func LinuxWorkerStorePath() string {
	if path := strings.TrimSpace(os.Getenv("MESHCLAW_LINUX_WORKERS_FILE")); path != "" {
		return path
	}
	return filepath.Join(workerConfigDir(), "linux-workers.json")
}

func ListLinuxWorkers() (LinuxWorkerStore, error) {
	return loadLinuxWorkerStore()
}

func UpsertLinuxWorker(node LinuxWorkerNode) (LinuxWorkerStore, LinuxWorkerNode, error) {
	node = normalizeLinuxWorker(node)
	if node.ID == "" {
		return LinuxWorkerStore{}, LinuxWorkerNode{}, errors.New("linux worker id is required")
	}
	if node.SSHTarget == "" {
		if node.Address == "" {
			return LinuxWorkerStore{}, LinuxWorkerNode{}, errors.New("linux worker requires --ssh or --host")
		}
		user := firstNonEmptyLocal(node.User, "root")
		node.SSHTarget = user + "@" + node.Address
	}
	store, err := loadLinuxWorkerStore()
	if err != nil {
		return LinuxWorkerStore{}, LinuxWorkerNode{}, err
	}
	now := time.Now().UTC()
	node.UpdatedAt = now
	replaced := false
	for i, existing := range store.Nodes {
		if existing.ID != node.ID {
			continue
		}
		if node.CreatedAt.IsZero() {
			node.CreatedAt = existing.CreatedAt
		}
		store.Nodes[i] = node
		replaced = true
		break
	}
	if !replaced {
		if node.CreatedAt.IsZero() {
			node.CreatedAt = now
		}
		store.Nodes = append(store.Nodes, node)
	}
	if node.Selected {
		for i := range store.Nodes {
			store.Nodes[i].Selected = store.Nodes[i].ID == node.ID
		}
	}
	store = normalizeLinuxWorkerStore(store)
	if err := saveLinuxWorkerStore(store); err != nil {
		return LinuxWorkerStore{}, LinuxWorkerNode{}, err
	}
	saved, _ := findLinuxWorker(store, node.ID)
	return store, saved, nil
}

func RemoveLinuxWorker(id string) (LinuxWorkerStore, bool, error) {
	id = normalizeLinuxWorkerID(id)
	if id == "" {
		return LinuxWorkerStore{}, false, errors.New("linux worker id is required")
	}
	store, err := loadLinuxWorkerStore()
	if err != nil {
		return LinuxWorkerStore{}, false, err
	}
	out := make([]LinuxWorkerNode, 0, len(store.Nodes))
	removed := false
	for _, node := range store.Nodes {
		if node.ID == id {
			removed = true
			continue
		}
		out = append(out, node)
	}
	store.Nodes = out
	store = normalizeLinuxWorkerStore(store)
	if err := saveLinuxWorkerStore(store); err != nil {
		return LinuxWorkerStore{}, removed, err
	}
	return store, removed, nil
}

func SelectLinuxWorker(id string) (LinuxWorkerStore, LinuxWorkerNode, error) {
	id = normalizeLinuxWorkerID(id)
	if id == "" {
		return LinuxWorkerStore{}, LinuxWorkerNode{}, errors.New("linux worker id is required")
	}
	store, err := loadLinuxWorkerStore()
	if err != nil {
		return LinuxWorkerStore{}, LinuxWorkerNode{}, err
	}
	var selected LinuxWorkerNode
	found := false
	now := time.Now().UTC()
	for i := range store.Nodes {
		if store.Nodes[i].ID == id {
			store.Nodes[i].Selected = true
			store.Nodes[i].UpdatedAt = now
			selected = store.Nodes[i]
			found = true
		} else {
			store.Nodes[i].Selected = false
		}
	}
	if !found {
		return LinuxWorkerStore{}, LinuxWorkerNode{}, errors.New("linux worker not found: " + id)
	}
	store = normalizeLinuxWorkerStore(store)
	if err := saveLinuxWorkerStore(store); err != nil {
		return LinuxWorkerStore{}, LinuxWorkerNode{}, err
	}
	return store, selected, nil
}

func SelectedLinuxWorker() (LinuxWorkerNode, bool) {
	store, err := loadLinuxWorkerStore()
	if err != nil {
		return LinuxWorkerNode{}, false
	}
	for _, node := range store.Nodes {
		if node.Selected && node.Enabled {
			return node, true
		}
	}
	for _, node := range store.Nodes {
		if node.Enabled {
			return node, true
		}
	}
	return LinuxWorkerNode{}, false
}

func GetLinuxWorker(id string) (LinuxWorkerNode, bool) {
	store, err := loadLinuxWorkerStore()
	if err != nil {
		return LinuxWorkerNode{}, false
	}
	return findLinuxWorker(store, id)
}

func RunLinuxWorkerJob(ctx context.Context, opts LinuxWorkerJobOptions) LinuxWorkerJobResult {
	started := time.Now().UTC()
	result := LinuxWorkerJobResult{
		Kind:      "meshclaw_linux_worker_job",
		Task:      strings.TrimSpace(opts.Task),
		StartedAt: started,
	}
	var node LinuxWorkerNode
	var ok bool
	if strings.TrimSpace(opts.WorkerID) != "" {
		node, ok = GetLinuxWorker(opts.WorkerID)
	} else {
		node, ok = SelectedLinuxWorker()
	}
	if !ok {
		result.Error = "no linux worker is selected or registered"
		result.FinishedAt = time.Now().UTC()
		result.DurationMS = result.FinishedAt.Sub(started).Milliseconds()
		return result
	}
	result.Worker = node
	if !node.Enabled {
		result.Error = "linux worker is disabled: " + node.ID
		result.FinishedAt = time.Now().UTC()
		result.DurationMS = result.FinishedAt.Sub(started).Milliseconds()
		return result
	}
	if node.SSHTarget == "" {
		result.Error = "linux worker has no SSH target: " + node.ID
		result.FinishedAt = time.Now().UTC()
		result.DurationMS = result.FinishedAt.Sub(started).Milliseconds()
		return result
	}
	command := strings.TrimSpace(opts.Command)
	if command == "" {
		command = buildLinuxWorkerTaskCommand(result.Task)
	}
	result.Command = command
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", node.SSHTarget, command)
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	result.SSHCommand = []string{"ssh", node.SSHTarget, command}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result.Stdout = strings.TrimSpace(stdout.String())
	result.Stderr = strings.TrimSpace(stderr.String())
	result.OK = err == nil
	if err != nil {
		result.Error = err.Error()
		if ctx.Err() != nil {
			result.Error = ctx.Err().Error()
		}
		if result.Stderr != "" {
			result.Error = fmt.Sprintf("%s: %s", result.Error, result.Stderr)
		}
	}
	result.FinishedAt = time.Now().UTC()
	result.DurationMS = result.FinishedAt.Sub(started).Milliseconds()
	return result
}

func DoctorLinuxWorker(ctx context.Context, id string) LinuxWorkerDoctorReport {
	report := LinuxWorkerDoctorReport{Kind: "meshclaw_linux_worker_doctor", CreatedAt: time.Now().UTC()}
	store, err := loadLinuxWorkerStore()
	if err != nil {
		report.Problems = append(report.Problems, "linux worker registry could not be read: "+err.Error())
		report.NextActions = append(report.NextActions, "Check "+LinuxWorkerStorePath()+" permissions.")
		return report
	}
	var node LinuxWorkerNode
	var ok bool
	if strings.TrimSpace(id) != "" {
		node, ok = findLinuxWorker(store, id)
	} else {
		node, ok = SelectedLinuxWorker()
	}
	if !ok {
		report.Problems = append(report.Problems, "no linux worker is selected or registered (등록된 리눅스/g 계열 워커가 없거나 선택되지 않았습니다)")
		report.NextActions = append(report.NextActions, "g 계열 워커를 쓰려면 `meshclaw workers nodes add g4 --ssh user@g4 --role llm-chat-worker --tag g-series,llm,gpu,no-desktop --select`로 등록하세요.")
		return report
	}
	report.Node = node
	if !node.Enabled {
		report.Problems = append(report.Problems, "linux worker is registered but disabled (리눅스 워커가 등록되어 있지만 비활성화되어 있습니다)")
		report.NextActions = append(report.NextActions, "필요하면 `meshclaw workers nodes add "+node.ID+" --ssh "+node.SSHTarget+" --enabled --select`로 다시 활성화하세요.")
		return report
	}
	if node.SSHTarget == "" {
		report.Problems = append(report.Problems, "linux worker has no SSH target (리눅스 워커에 SSH 대상이 없습니다)")
		report.NextActions = append(report.NextActions, "`--ssh user@host`를 넣어 워커 접속 대상을 등록하세요.")
		return report
	}
	checks := []struct {
		name   string
		script string
	}{
		{"ssh_probe", "printf 'meshclaw-linux-worker-ok\\n'; uname -a"},
		{"meshclaw", "command -v meshclaw >/dev/null && (meshclaw --version 2>/dev/null || meshclaw version 2>/dev/null || true)"},
		{"python", "command -v python3 >/dev/null && python3 --version"},
		{"gpu", "command -v nvidia-smi >/dev/null && nvidia-smi --query-gpu=name,memory.total --format=csv,noheader || true"},
		{"ollama", "command -v ollama >/dev/null && ollama list || true"},
	}
	for _, check := range checks {
		result := runLinuxWorkerSSHCheck(ctx, node.SSHTarget, check.name, check.script)
		report.Checks = append(report.Checks, result)
		if !result.OK && check.name != "gpu" && check.name != "ollama" && check.name != "meshclaw" {
			report.Problems = append(report.Problems, check.name+": "+firstNonEmptyLocal(result.Error, result.Stderr, "failed"))
		}
	}
	if len(report.Problems) > 0 {
		report.NextActions = append(report.NextActions, node.SSHTarget+"에서 SSH와 기본 런타임 상태를 확인하세요.")
		report.NextActions = append(report.NextActions, "GPU/Ollama는 선택 기능이지만, g 계열 LLM 워커로 쓸 노드는 설치 상태가 doctor 출력에 보여야 합니다.")
	}
	report.Problems = uniqueLocalStrings(report.Problems)
	report.NextActions = uniqueLocalStrings(report.NextActions)
	report.OK = len(report.Problems) == 0
	return report
}

func buildLinuxWorkerTaskCommand(task string) string {
	task = strings.TrimSpace(task)
	if task == "" {
		task = "worker runtime check"
	}
	if isLinuxWorkerNewsResearchTask(task) {
		return buildLinuxWorkerNewsResearchCommand(task)
	}
	return strings.Join([]string{
		"printf 'meshclaw-worker-job-ok\\n'",
		"printf 'task=%s\\n' " + shellQuoteLocal(task),
		"printf 'host='; hostname",
		"printf 'time='; date -Is 2>/dev/null || date",
		"printf 'uptime='; uptime",
		"printf 'python='; (python3 --version 2>/dev/null || true)",
		"printf 'gpu='; (command -v nvidia-smi >/dev/null && nvidia-smi --query-gpu=name,memory.total --format=csv,noheader 2>/dev/null || true)",
		"printf 'ollama='; (command -v ollama >/dev/null && ollama list 2>/dev/null | sed -n '1,8p' || true)",
	}, "; ")
}

func isLinuxWorkerNewsResearchTask(task string) bool {
	lower := strings.ToLower(strings.TrimSpace(task))
	if lower == "" {
		return false
	}
	return containsAnyLocal(lower,
		"뉴스", "주요뉴스", "헤드라인", "기사", "리서치", "조사", "검색",
		"news", "headline", "headlines", "research", "brief", "briefing",
	)
}

func buildLinuxWorkerNewsResearchCommand(task string) string {
	encodedTask, _ := json.Marshal(task)
	script := fmt.Sprintf(`TASK = %s
import datetime
import html
import socket
import urllib.parse
import urllib.request
import xml.etree.ElementTree as ET

def google_search(query):
    return "https://news.google.com/rss/search?q=" + urllib.parse.quote(query) + "&hl=ko&gl=KR&ceid=KR:ko"

def feed_urls(task):
    task_l = task.lower()
    feeds = [
        ("google-top-ko", "https://news.google.com/rss?hl=ko&gl=KR&ceid=KR:ko"),
        ("google-world-ko", google_search("세계 주요뉴스")),
        ("google-economy-ko", google_search("경제 시장 주요뉴스")),
        ("google-tech-ko", google_search("AI 기술 주요뉴스")),
    ]
    noise = ["뉴스", "주요뉴스", "헤드라인", "기사", "리서치", "조사", "검색", "정리", "요약", "맡겨", "worker", "news", "headline", "headlines", "research", "brief", "briefing"]
    query = task
    for word in noise:
        query = query.replace(word, " ")
    query = " ".join(query.split())
    if query:
        feeds.insert(0, ("google-query", google_search(query)))
    if "경제" in task_l or "시장" in task_l or "econom" in task_l or "market" in task_l:
        feeds.insert(0, ("google-economy-focus", google_search("경제 금융 시장")))
    if "ai" in task_l or "기술" in task_l or "tech" in task_l:
        feeds.insert(0, ("google-tech-focus", google_search("AI 반도체 기술")))
    return feeds[:5]

def text_of(parent, name):
    child = parent.find(name)
    if child is None or child.text is None:
        return ""
    return html.unescape(child.text).strip()

items = []
errors = []
seen = set()
for source, url in feed_urls(TASK):
    try:
        req = urllib.request.Request(url, headers={"User-Agent": "MeshClawWorker/1.0"})
        with urllib.request.urlopen(req, timeout=8) as resp:
            data = resp.read(1024 * 1024)
        root = ET.fromstring(data)
        for item in root.findall(".//item"):
            title = text_of(item, "title")
            link = text_of(item, "link")
            pub = text_of(item, "pubDate")
            if not title:
                continue
            key = title.lower()
            if key in seen:
                continue
            seen.add(key)
            items.append((source, title, link, pub))
            if len(items) >= 12:
                break
    except Exception as exc:
        errors.append(source + ": " + str(exc)[:160])
    if len(items) >= 12:
        break

print("meshclaw-worker-news-ok")
print("task=" + TASK)
print("host=" + socket.gethostname())
print("time=" + datetime.datetime.now(datetime.timezone.utc).astimezone().isoformat(timespec="seconds"))
print("items=" + str(len(items)))
if items:
    print("")
    print("빠른 리서치 결과:")
    for idx, (source, title, link, pub) in enumerate(items[:8], 1):
        print(f"{idx}. {title}")
        meta = source
        if pub:
            meta += " | " + pub
        print("   " + meta)
        if link:
            print("   " + link)
else:
    print("no_items=true")
if errors:
    print("")
    print("fetch_errors:")
    for err in errors[:4]:
        print("- " + err)
`, string(encodedTask))
	return "python3 - <<'PY'\n" + script + "PY"
}

func loadLinuxWorkerStore() (LinuxWorkerStore, error) {
	path := LinuxWorkerStorePath()
	store := LinuxWorkerStore{
		Kind:    "meshclaw_linux_workers",
		Version: linuxWorkerStoreVersion,
		Path:    path,
		Nodes:   []LinuxWorkerNode{},
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return LinuxWorkerStore{}, err
	}
	if err := json.Unmarshal(data, &store); err != nil {
		return LinuxWorkerStore{}, err
	}
	store.Path = path
	return normalizeLinuxWorkerStore(store), nil
}

func saveLinuxWorkerStore(store LinuxWorkerStore) error {
	store = normalizeLinuxWorkerStore(store)
	if err := os.MkdirAll(filepath.Dir(store.Path), 0700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(store.Path, append(payload, '\n'), 0600)
}

func normalizeLinuxWorkerStore(store LinuxWorkerStore) LinuxWorkerStore {
	if store.Kind == "" {
		store.Kind = "meshclaw_linux_workers"
	}
	if store.Version == 0 {
		store.Version = linuxWorkerStoreVersion
	}
	if store.Path == "" {
		store.Path = LinuxWorkerStorePath()
	}
	now := time.Now().UTC()
	seen := map[string]LinuxWorkerNode{}
	selected := ""
	for _, node := range store.Nodes {
		node = normalizeLinuxWorker(node)
		if node.ID == "" {
			continue
		}
		if node.CreatedAt.IsZero() {
			node.CreatedAt = now
		}
		if node.UpdatedAt.IsZero() {
			node.UpdatedAt = node.CreatedAt
		}
		if node.Selected && selected == "" {
			selected = node.ID
		}
		seen[node.ID] = node
	}
	out := make([]LinuxWorkerNode, 0, len(seen))
	for _, node := range seen {
		if selected != "" {
			node.Selected = node.ID == selected
		}
		out = append(out, node)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	store.Nodes = out
	store.UpdatedAt = now
	return store
}

func normalizeLinuxWorker(node LinuxWorkerNode) LinuxWorkerNode {
	node.ID = normalizeLinuxWorkerID(node.ID)
	node.Name = strings.TrimSpace(node.Name)
	node.SSHTarget = strings.TrimSpace(node.SSHTarget)
	node.Address = strings.TrimSpace(node.Address)
	node.User = strings.TrimSpace(node.User)
	node.Role = strings.TrimSpace(node.Role)
	if node.Role == "" {
		node.Role = "headless-worker"
	}
	node.Capabilities = normalizeStringListLocal(node.Capabilities)
	node.Tags = normalizeStringListLocal(append([]string{"linux", "no-desktop"}, node.Tags...))
	if strings.HasPrefix(node.ID, "g") && !containsStringLocal(node.Tags, "g-series") {
		node.Tags = normalizeStringListLocal(append(node.Tags, "g-series"))
	}
	return node
}

func normalizeLinuxWorkerID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	id = strings.ReplaceAll(id, " ", "-")
	return id
}

func findLinuxWorker(store LinuxWorkerStore, id string) (LinuxWorkerNode, bool) {
	id = normalizeLinuxWorkerID(id)
	for _, node := range store.Nodes {
		if node.ID == id {
			return node, true
		}
	}
	return LinuxWorkerNode{}, false
}

func runLinuxWorkerSSHCheck(ctx context.Context, target, name, script string) LinuxWorkerCheck {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", target, script)
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := LinuxWorkerCheck{
		Kind:      "meshclaw_linux_worker_doctor_" + name,
		Action:    "linux_worker_doctor_" + name,
		Command:   []string{"ssh", target, script},
		OK:        err == nil,
		Stdout:    strings.TrimSpace(stdout.String()),
		Stderr:    strings.TrimSpace(stderr.String()),
		CreatedAt: time.Now().UTC(),
	}
	if err != nil {
		result.Error = err.Error()
		if ctx.Err() != nil {
			result.Error = ctx.Err().Error()
		}
		if result.Stderr != "" {
			result.Error = fmt.Sprintf("%s: %s", result.Error, result.Stderr)
		}
	}
	return result
}

func workerConfigDir() string {
	if dir := strings.TrimSpace(os.Getenv("MESHCLAW_HOME")); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".meshclaw"
	}
	return filepath.Join(home, ".meshclaw")
}

func normalizeStringListLocal(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func containsStringLocal(values []string, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == needle {
			return true
		}
	}
	return false
}

func containsAnyLocal(value string, needles ...string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, needle := range needles {
		if needle != "" && strings.Contains(value, strings.ToLower(strings.TrimSpace(needle))) {
			return true
		}
	}
	return false
}

func firstNonEmptyLocal(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func shellQuoteLocal(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func uniqueLocalStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
