package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/meshclaw/meshclaw/internal/inventory"
	"github.com/meshclaw/meshclaw/internal/policy"
	"github.com/meshclaw/meshclaw/internal/runtime"
	"github.com/meshclaw/meshclaw/internal/runtimeflow"
)

type SelfReport struct {
	Kind        string      `json:"kind"`
	Status      string      `json:"status"`
	Checks      []SelfCheck `json:"checks"`
	NextActions []string    `json:"next_actions"`
}

type SelfCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	Next   string `json:"next,omitempty"`
}

func Self() SelfReport {
	report := SelfReport{Kind: "meshclaw_doctor", Status: "ok"}
	add := func(name, status, detail, next string) {
		report.Checks = append(report.Checks, SelfCheck{Name: name, Status: status, Detail: detail, Next: next})
		if status == "fail" {
			report.Status = "fail"
		} else if status == "warn" && report.Status == "ok" {
			report.Status = "warn"
		}
	}
	if exe, err := os.Executable(); err == nil {
		add("meshclaw_binary", "ok", exe, "")
	} else {
		add("meshclaw_binary", "fail", err.Error(), "verify meshclaw binary installation or MESHCLAW_BIN")
	}
	checkVSSH(add)
	checkVSSHSecret(add)
	checkMCPConfigs(add)
	nodes := inventory.DefaultNodes()
	if len(nodes) == 0 {
		add("inventory", "warn", "no nodes found", "run meshclaw inventory-discover or configure inventory")
	} else {
		add("inventory", "ok", fmt.Sprintf("nodes=%d", len(nodes)), "")
	}
	if _, path, err := policy.LoadConfig(); err == nil {
		add("policy", "ok", path, "")
	} else {
		add("policy", "fail", err.Error(), "fix policy file or unset MESHCLAW_POLICY_FILE")
	}
	if validation, err := runtimeflow.Validate("fleet-health-demo"); err == nil && validation.Valid {
		add("builtin_workflow", "ok", "fleet-health-demo", "")
	} else if err != nil {
		add("builtin_workflow", "fail", err.Error(), "reinstall MeshClaw or inspect workflow registry")
	} else {
		add("builtin_workflow", "fail", "fleet-health-demo invalid", "run meshclaw workflows validate fleet-health-demo --json")
	}
	checkExampleWorkflows(add)
	if home, err := os.UserHomeDir(); err == nil {
		root := filepath.Join(home, ".meshclaw", "evidence")
		if err := os.MkdirAll(root, 0700); err != nil {
			add("evidence_dir", "fail", err.Error(), "fix ~/.meshclaw permissions")
		} else {
			add("evidence_dir", "ok", root, "")
		}
	} else {
		add("evidence_dir", "fail", err.Error(), "ensure HOME is set")
	}
	add("mcp_server", "ok", "meshclaw mcp command available", "connect Codex/Claude/Cursor to this command")
	switch report.Status {
	case "ok":
		report.NextActions = []string{
			"Run meshclaw workflows validate examples/workflows/fleet-health.json if examples are available.",
			"Run meshclaw run fleet-health-demo --dry-run and inspect meshclaw evidence open latest.",
			"Connect the MCP client to meshclaw mcp.",
		}
	case "warn":
		report.NextActions = []string{
			"Review warning checks before execute mode.",
			"Dry-run workflows are still safe: run meshclaw quickstart --json or meshclaw run fleet-health-demo --dry-run --json.",
			"Pin MCP clients to the intended MeshClaw and VSSH binaries when binary conflicts are reported.",
		}
	default:
		report.NextActions = []string{
			"Fix failed checks before connecting AI clients or running execute mode.",
		}
	}
	return report
}

func checkVSSH(add func(name, status, detail, next string)) {
	effective := runtime.DefaultVSSHBinary()
	resolved, ok := resolveExecutable(effective)
	if !ok {
		add("vssh_binary", "warn", effective+" not executable or not in PATH", "install vssh or set MESHCLAW_VSSH_BINARY")
		return
	}
	add("vssh_binary", "ok", resolved, "")
	if version := commandVersion(resolved); version != "" {
		add("vssh_version", "ok", version, "")
	}
	candidates := vsshCandidates(effective, resolved)
	if len(candidates) <= 1 {
		return
	}
	versions := map[string]string{}
	for _, candidate := range candidates {
		versions[candidate] = commandVersion(candidate)
	}
	if hasVersionConflict(versions) {
		add("vssh_binary_conflict", "warn", formatVersions(versions), "prefer one vssh binary; set MESHCLAW_VSSH_BINARY to the intended version and update PATH/MCP configs")
		return
	}
	add("vssh_binary_conflict", "ok", formatVersions(versions), "")
}

func checkVSSHSecret(add func(name, status, detail, next string)) {
	envSet := strings.TrimSpace(os.Getenv("VSSH_SECRET")) != ""
	homeSet := false
	if home, err := os.UserHomeDir(); err == nil {
		for _, path := range []string{
			filepath.Join(home, ".vssh", "secret"),
			filepath.Join(home, ".config", "vssh", "secret"),
		} {
			if st, err := os.Stat(path); err == nil && !st.IsDir() {
				homeSet = true
				break
			}
		}
	}
	switch {
	case envSet:
		add("vssh_secret", "ok", "env set; value not displayed", "")
	case homeSet:
		add("vssh_secret", "ok", "user secret file found; value not displayed", "")
	default:
		add("vssh_secret", "warn", "no VSSH_SECRET env or known user secret file", "set VSSH_SECRET in MCP config or install a vssh secret file before execute mode")
	}
}

func checkMCPConfigs(add func(name, status, detail, next string)) {
	home, err := os.UserHomeDir()
	if err != nil {
		add("mcp_configs", "warn", err.Error(), "ensure HOME is set")
		return
	}
	configs := []struct {
		name string
		path string
	}{
		{"codex", filepath.Join(home, ".codex", "config.toml")},
		{"claude", filepath.Join(home, ".claude", "mcp.json")},
		{"claude_desktop", filepath.Join(home, ".claude", "claude_desktop_config.json")},
		{"cursor", filepath.Join(home, ".cursor", "mcp.json")},
	}
	found := []string{}
	missing := []string{}
	for _, cfg := range configs {
		data, err := os.ReadFile(cfg.path)
		if err != nil {
			missing = append(missing, cfg.name)
			continue
		}
		text := string(data)
		if strings.Contains(text, "meshclaw") {
			found = append(found, cfg.name)
		} else {
			missing = append(missing, cfg.name+"(no meshclaw)")
		}
	}
	if len(found) == 0 {
		add("mcp_configs", "warn", "no Codex/Claude/Cursor MCP config with meshclaw found", "configure clients to run: meshclaw mcp")
		return
	}
	detail := "configured=" + strings.Join(found, ",")
	if len(missing) > 0 {
		detail += " missing_or_unconfigured=" + strings.Join(missing, ",")
		add("mcp_configs", "warn", detail, "update any client that should use MeshClaw MCP")
		return
	}
	add("mcp_configs", "ok", detail, "")
}

func resolveExecutable(binary string) (string, bool) {
	if path, err := exec.LookPath(binary); err == nil {
		return path, true
	}
	if st, err := os.Stat(binary); err == nil && !st.IsDir() && st.Mode()&0111 != 0 {
		return binary, true
	}
	return "", false
}

func vsshCandidates(effective, resolved string) []string {
	seen := map[string]bool{}
	add := func(path string) {
		if path == "" || seen[path] {
			return
		}
		if st, err := os.Stat(path); err == nil && !st.IsDir() && st.Mode()&0111 != 0 {
			seen[path] = true
		}
	}
	add(effective)
	add(resolved)
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, "bin", "vssh"))
	}
	add("/usr/local/bin/vssh")
	add("/opt/homebrew/bin/vssh")
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		add(filepath.Join(dir, "vssh"))
	}
	out := make([]string, 0, len(seen))
	for path := range seen {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func commandVersion(path string) string {
	out, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func hasVersionConflict(versions map[string]string) bool {
	seen := map[string]bool{}
	for _, version := range versions {
		if version == "" || version == "unknown" {
			continue
		}
		seen[version] = true
	}
	return len(seen) > 1
}

func formatVersions(versions map[string]string) string {
	paths := make([]string, 0, len(versions))
	for path := range versions {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	parts := make([]string, 0, len(paths))
	for _, path := range paths {
		parts = append(parts, path+"="+versions[path])
	}
	return strings.Join(parts, "; ")
}

func checkExampleWorkflows(add func(name, status, detail, next string)) {
	candidates := []string{
		filepath.Join("examples", "workflows", "fleet-health.json"),
		filepath.Join("examples", "workflows", "service-triage-autoheal.json"),
		filepath.Join("examples", "workflows", "model-worker-orchestration.json"),
	}
	checked := 0
	for _, path := range candidates {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		checked++
		validation, err := runtimeflow.Validate(path)
		if err != nil {
			add("example_workflow", "fail", path+": "+err.Error(), "fix or remove invalid example workflow")
			continue
		}
		if !validation.Valid {
			add("example_workflow", "fail", path+" invalid", "run meshclaw workflows validate "+path+" --json")
			continue
		}
	}
	if checked == 0 {
		add("example_workflows", "warn", "examples/workflows not found from current directory", "run from repo root or install packaged examples")
		return
	}
	add("example_workflows", "ok", fmt.Sprintf("validated=%d", checked), "")
}

func FormatSelf(report SelfReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "meshclaw doctor status=%s\n", report.Status)
	for _, check := range report.Checks {
		fmt.Fprintf(&b, "- %s: %s", check.Name, check.Status)
		if check.Detail != "" {
			fmt.Fprintf(&b, " | %s", check.Detail)
		}
		if check.Next != "" {
			fmt.Fprintf(&b, " | next: %s", check.Next)
		}
		b.WriteByte('\n')
	}
	if len(report.NextActions) > 0 {
		b.WriteString("next_actions:\n")
		for _, action := range report.NextActions {
			fmt.Fprintf(&b, "- %s\n", action)
		}
	}
	return b.String()
}

func Check(host string) string {
	node, ok := inventory.Find(host)
	if !ok {
		return fmt.Sprintf("unknown node: %s\n", host)
	}

	runner := runtime.NewRunner()
	out, err := runner.Run(host, "hostname && whoami && uptime")
	if err == nil {
		return fmt.Sprintf("%s OK\npath=vssh-native-first\nremote=%s\nlegacy_wire=%s\n%s", host, preferredRemote(node), node.WireIP, out)
	}

	return fmt.Sprintf(`%s DEGRADED
role=%s
location=%s
tailscale=%s
legacy_wire=%s
lan=%s

execution probe failed:
%v

next checks:
- verify Tailscale reachability and MagicDNS/IP
- verify vsshd is running on the target for sshd-free execution
- if vsshd is unavailable, verify sshd and SSH user/key for fallback
- install the monitoring agent for continuous evidence collection
- use legacy Wire only as fallback
`, node.Name, node.Role, node.Location, node.Tailscale, node.WireIP, node.LAN, err)
}

func preferredRemote(node inventory.Node) string {
	if node.Tailscale != "" {
		return node.Tailscale
	}
	if node.WireIP != "" {
		return node.WireIP
	}
	if node.LAN != "" {
		return node.LAN
	}
	return node.Name
}
