package osauto

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

const macRunnerStoreVersion = 1

type MacRunner struct {
	ID           string    `json:"id"`
	Name         string    `json:"name,omitempty"`
	SSHTarget    string    `json:"ssh_target,omitempty"`
	Project      string    `json:"project,omitempty"`
	Python       string    `json:"python,omitempty"`
	URL          string    `json:"url,omitempty"`
	Capabilities []string  `json:"capabilities,omitempty"`
	Enabled      bool      `json:"enabled"`
	Selected     bool      `json:"selected,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type MacRunnerStore struct {
	Kind      string      `json:"kind"`
	Version   int         `json:"version"`
	Path      string      `json:"path"`
	Runners   []MacRunner `json:"runners"`
	UpdatedAt time.Time   `json:"updated_at"`
}

type MacRunnerDoctorReport struct {
	Kind        string    `json:"kind"`
	OK          bool      `json:"ok"`
	Runner      MacRunner `json:"runner,omitempty"`
	Checks      []Result  `json:"checks,omitempty"`
	Problems    []string  `json:"problems,omitempty"`
	NextActions []string  `json:"next_actions,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func MacRunnerStorePath() string {
	if path := strings.TrimSpace(os.Getenv("MESHCLAW_MAC_RUNNERS_FILE")); path != "" {
		return path
	}
	return filepath.Join(defaultMeshClawDir(), "mac-runners.json")
}

func ListMacRunners() (MacRunnerStore, error) {
	store, err := loadMacRunnerStore()
	if err != nil {
		return MacRunnerStore{}, err
	}
	return store, nil
}

func UpsertMacRunner(runner MacRunner) (MacRunnerStore, MacRunner, error) {
	runner = normalizeMacRunner(runner)
	if runner.ID == "" {
		return MacRunnerStore{}, MacRunner{}, errors.New("mac runner id is required")
	}
	if runner.SSHTarget == "" && runner.URL == "" {
		return MacRunnerStore{}, MacRunner{}, errors.New("mac runner requires --ssh or --url")
	}
	store, err := loadMacRunnerStore()
	if err != nil {
		return MacRunnerStore{}, MacRunner{}, err
	}
	now := time.Now().UTC()
	runner.UpdatedAt = now
	replaced := false
	for i, existing := range store.Runners {
		if existing.ID != runner.ID {
			continue
		}
		if runner.CreatedAt.IsZero() {
			runner.CreatedAt = existing.CreatedAt
		}
		if runner.Name == "" {
			runner.Name = existing.Name
		}
		store.Runners[i] = runner
		replaced = true
		break
	}
	if !replaced {
		if runner.CreatedAt.IsZero() {
			runner.CreatedAt = now
		}
		store.Runners = append(store.Runners, runner)
	}
	if runner.Selected {
		for i := range store.Runners {
			store.Runners[i].Selected = store.Runners[i].ID == runner.ID
		}
	}
	store = normalizeMacRunnerStore(store)
	if err := saveMacRunnerStore(store); err != nil {
		return MacRunnerStore{}, MacRunner{}, err
	}
	selected, _ := findMacRunner(store, runner.ID)
	return store, selected, nil
}

func RemoveMacRunner(id string) (MacRunnerStore, bool, error) {
	id = normalizeMacRunnerID(id)
	if id == "" {
		return MacRunnerStore{}, false, errors.New("mac runner id is required")
	}
	store, err := loadMacRunnerStore()
	if err != nil {
		return MacRunnerStore{}, false, err
	}
	out := make([]MacRunner, 0, len(store.Runners))
	removed := false
	for _, runner := range store.Runners {
		if runner.ID == id {
			removed = true
			continue
		}
		out = append(out, runner)
	}
	store.Runners = out
	store = normalizeMacRunnerStore(store)
	if err := saveMacRunnerStore(store); err != nil {
		return MacRunnerStore{}, removed, err
	}
	return store, removed, nil
}

func SelectMacRunner(id string) (MacRunnerStore, MacRunner, error) {
	id = normalizeMacRunnerID(id)
	if id == "" {
		return MacRunnerStore{}, MacRunner{}, errors.New("mac runner id is required")
	}
	store, err := loadMacRunnerStore()
	if err != nil {
		return MacRunnerStore{}, MacRunner{}, err
	}
	found := false
	var selected MacRunner
	now := time.Now().UTC()
	for i := range store.Runners {
		if store.Runners[i].ID == id {
			store.Runners[i].Selected = true
			store.Runners[i].UpdatedAt = now
			selected = store.Runners[i]
			found = true
		} else {
			store.Runners[i].Selected = false
		}
	}
	if !found {
		return MacRunnerStore{}, MacRunner{}, errors.New("mac runner not found: " + id)
	}
	store = normalizeMacRunnerStore(store)
	if err := saveMacRunnerStore(store); err != nil {
		return MacRunnerStore{}, MacRunner{}, err
	}
	return store, selected, nil
}

func SelectedMacRunner() (MacRunner, bool) {
	store, err := loadMacRunnerStore()
	if err != nil {
		return MacRunner{}, false
	}
	for _, runner := range store.Runners {
		if runner.Selected && runner.Enabled {
			return runner, true
		}
	}
	for _, runner := range store.Runners {
		if runner.Enabled {
			return runner, true
		}
	}
	return MacRunner{}, false
}

func DoctorMacRunner(ctx context.Context, id string) MacRunnerDoctorReport {
	report := MacRunnerDoctorReport{
		Kind:      "meshclaw_mac_runner_doctor",
		CreatedAt: time.Now().UTC(),
	}
	store, err := loadMacRunnerStore()
	if err != nil {
		report.Problems = append(report.Problems, "mac runner registry could not be read: "+err.Error())
		report.NextActions = append(report.NextActions, "Check "+MacRunnerStorePath()+" permissions.")
		return report
	}
	var runner MacRunner
	var ok bool
	if strings.TrimSpace(id) != "" {
		runner, ok = findMacRunner(store, id)
	} else {
		runner, ok = SelectedMacRunner()
	}
	if !ok {
		report.Problems = append(report.Problems, "no mac runner is selected or registered (등록된 Mac 실행기가 없거나 선택되지 않았습니다)")
		report.NextActions = append(report.NextActions, "macmini의 stable Argos UI Runner는 별도 기능입니다. Calendar/Reminders/Contacts 같은 로컬 비서 기능은 Mac runner 없이도 동작할 수 있습니다.")
		report.NextActions = append(report.NextActions, "Mac 실행기를 쓰려면 `meshclaw argos runners add <id> --ssh user@host --project <path> --select`로 등록하세요.")
		return report
	}
	report.Runner = runner
	if !runner.Enabled {
		report.Problems = append(report.Problems, "mac runner is registered but disabled (Mac 실행기가 등록되어 있지만 비활성화되어 있습니다)")
		report.NextActions = append(report.NextActions, "필요하면 `meshclaw argos runners add "+runner.ID+" --ssh "+runner.SSHTarget+" --enabled --select`로 다시 활성화하세요.")
		return report
	}
	if runner.SSHTarget == "" {
		report.Problems = append(report.Problems, "mac runner has no SSH target (Mac 실행기에 SSH 대상이 없습니다)")
		report.NextActions = append(report.NextActions, "`--ssh user@host`를 넣어 실행기 접속 대상을 등록하세요.")
		return report
	}
	project := firstNonEmpty(runner.Project, "/Users/example/Documents/New project")
	python := firstNonEmpty(runner.Python, "python3")
	checks := []struct {
		name   string
		script string
	}{
		{"ssh_probe", "printf 'meshclaw-mac-runner-ok\\n'; uname -s; sw_vers -productVersion 2>/dev/null || true"},
		{"project_dir", "test -d " + shellQuote(project) + " && printf 'project ok: " + project + "\\n'"},
		{"python", "command -v " + shellQuote(python) + " >/dev/null && " + shellQuote(python) + " --version"},
		{"local_command", "test -f " + shellQuote(filepath.Join(project, "scripts", "signal_bridge.py")) + " && printf 'local-command bridge present\\n'"},
	}
	for _, check := range checks {
		result := runMacRunnerSSHCheck(ctx, runner.SSHTarget, check.name, check.script)
		report.Checks = append(report.Checks, result)
		if !result.OK {
			report.Problems = append(report.Problems, check.name+": "+firstNonEmpty(result.Error, result.Stderr, "failed"))
		}
	}
	if len(report.Problems) > 0 {
		report.NextActions = append(report.NextActions, runner.SSHTarget+"에 Mac 실행기 프로젝트를 설치하거나 업데이트하세요.")
		report.NextActions = append(report.NextActions, "SSH, 프로젝트 폴더, Python, local-command bridge 점검이 모두 통과할 때까지 실행기를 비활성 상태로 두세요.")
	}
	report.Problems = uniqueLocalStrings(report.Problems)
	report.NextActions = uniqueLocalStrings(report.NextActions)
	report.OK = len(report.Problems) == 0
	return report
}

func loadMacRunnerStore() (MacRunnerStore, error) {
	path := MacRunnerStorePath()
	store := MacRunnerStore{
		Kind:    "meshclaw_mac_runners",
		Version: macRunnerStoreVersion,
		Path:    path,
		Runners: []MacRunner{},
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return MacRunnerStore{}, err
	}
	if err := json.Unmarshal(data, &store); err != nil {
		return MacRunnerStore{}, err
	}
	store.Path = path
	return normalizeMacRunnerStore(store), nil
}

func saveMacRunnerStore(store MacRunnerStore) error {
	store = normalizeMacRunnerStore(store)
	if err := os.MkdirAll(filepath.Dir(store.Path), 0700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(store.Path, append(payload, '\n'), 0600)
}

func normalizeMacRunnerStore(store MacRunnerStore) MacRunnerStore {
	if store.Kind == "" {
		store.Kind = "meshclaw_mac_runners"
	}
	if store.Version == 0 {
		store.Version = macRunnerStoreVersion
	}
	if store.Path == "" {
		store.Path = MacRunnerStorePath()
	}
	now := time.Now().UTC()
	seen := map[string]MacRunner{}
	selected := ""
	for _, runner := range store.Runners {
		runner = normalizeMacRunner(runner)
		if runner.ID == "" {
			continue
		}
		if runner.CreatedAt.IsZero() {
			runner.CreatedAt = now
		}
		if runner.UpdatedAt.IsZero() {
			runner.UpdatedAt = runner.CreatedAt
		}
		if runner.Selected && selected == "" {
			selected = runner.ID
		}
		seen[runner.ID] = runner
	}
	out := make([]MacRunner, 0, len(seen))
	for _, runner := range seen {
		if selected != "" {
			runner.Selected = runner.ID == selected
		}
		out = append(out, runner)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	store.Runners = out
	store.UpdatedAt = now
	return store
}

func normalizeMacRunner(runner MacRunner) MacRunner {
	runner.ID = normalizeMacRunnerID(runner.ID)
	runner.Name = strings.TrimSpace(runner.Name)
	runner.SSHTarget = strings.TrimSpace(runner.SSHTarget)
	runner.Project = strings.TrimSpace(runner.Project)
	runner.Python = strings.TrimSpace(runner.Python)
	runner.URL = strings.TrimSpace(runner.URL)
	runner.Capabilities = normalizeStringList(runner.Capabilities)
	return runner
}

func normalizeMacRunnerID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	id = strings.ReplaceAll(id, " ", "-")
	return id
}

func normalizeStringList(values []string) []string {
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

func findMacRunner(store MacRunnerStore, id string) (MacRunner, bool) {
	id = normalizeMacRunnerID(id)
	for _, runner := range store.Runners {
		if runner.ID == id {
			return runner, true
		}
	}
	return MacRunner{}, false
}

func runMacRunnerSSHCheck(ctx context.Context, target, name, script string) Result {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", target, script)
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := runCommandWithContext(ctx, cmd)
	result := Result{
		Kind:      "meshclaw_mac_runner_doctor_" + name,
		Action:    "mac_runner_doctor_" + name,
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
