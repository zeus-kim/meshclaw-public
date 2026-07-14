package openwebui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/meshclaw/meshclaw/internal/inventory"
	"github.com/meshclaw/meshclaw/internal/runtime"
)

type ScanReport struct {
	Time        time.Time  `json:"time"`
	Hosts       []HostInfo `json:"hosts"`
	OK          int        `json:"ok"`
	Installed   int        `json:"installed"`
	WithTool    int        `json:"with_tool"`
	WithPipe    int        `json:"with_pipe"`
	WithGateway int        `json:"with_gateway"`
	Failures    int        `json:"failures"`
}

type HostInfo struct {
	Host          string   `json:"host"`
	Status        string   `json:"status"`
	Version       string   `json:"version,omitempty"`
	Name          string   `json:"name,omitempty"`
	Auth          *bool    `json:"auth,omitempty"`
	Runtime       string   `json:"runtime,omitempty"`
	URL           string   `json:"url,omitempty"`
	Gateway       string   `json:"gateway,omitempty"`
	Integration   string   `json:"integration,omitempty"`
	ToolInstalled bool     `json:"tool_installed"`
	PipeInstalled bool     `json:"pipe_installed"`
	Tools         []string `json:"tools,omitempty"`
	Functions     []string `json:"functions,omitempty"`
	Error         string   `json:"error,omitempty"`
}

type remoteScanPayload struct {
	Config struct {
		Status   bool   `json:"status"`
		Name     string `json:"name"`
		Version  string `json:"version"`
		Features struct {
			Auth bool `json:"auth"`
		} `json:"features"`
	} `json:"config"`
	Runtime   string   `json:"runtime"`
	Gateway   string   `json:"gateway"`
	Tools     []string `json:"tools"`
	Functions []string `json:"functions"`
}

func Scan(hosts []string, parallel int) (ScanReport, error) {
	selected, err := selectHosts(hosts)
	if err != nil {
		return ScanReport{}, err
	}
	if parallel <= 0 {
		parallel = 6
	}
	report := ScanReport{Time: time.Now().UTC(), Hosts: make([]HostInfo, len(selected))}
	runner := runtime.NewRunner()
	runner.Timeout = scanTimeout()
	runner.DisableFallback = len(selected) > 1
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for i, host := range selected {
		i, host := i, host
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			report.Hosts[i] = scanHost(runner, host)
		}()
	}
	wg.Wait()
	sort.Slice(report.Hosts, func(i, j int) bool { return report.Hosts[i].Host < report.Hosts[j].Host })
	for _, host := range report.Hosts {
		if host.Status == "ok" {
			report.OK++
		}
		if host.Status == "ok" || host.Status == "missing_tool" || host.Status == "missing_gateway" {
			report.Installed++
		}
		if host.ToolInstalled {
			report.WithTool++
		}
		if host.PipeInstalled {
			report.WithPipe++
		}
		if host.Gateway != "" {
			report.WithGateway++
		}
		if host.Status == "failed" {
			report.Failures++
		}
	}
	return report, nil
}

func scanTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("MESHCLAW_OPENWEBUI_SCAN_TIMEOUT"))
	if raw == "" {
		return 8 * time.Second
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return 8 * time.Second
}

func scanHost(runner runtime.Runner, host string) HostInfo {
	if isLocalHost(host) {
		return scanLocalHost(runner.Timeout, host)
	}
	result := runner.RunEvidence(host, remoteScanScript())
	info := HostInfo{Host: host}
	if !result.Success {
		info.Status = "failed"
		info.Error = trim(result.Stderr+result.Stdout, 1200)
		return info
	}
	return parseScanPayload(host, result.Stdout)
}

func scanLocalHost(timeout time.Duration, host string) HostInfo {
	if timeout <= 0 {
		timeout = scanTimeout()
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-lc", remoteScanScript())
	out, err := cmd.CombinedOutput()
	info := HostInfo{Host: host}
	if ctx.Err() == context.DeadlineExceeded {
		info.Status = "failed"
		info.Error = "local scan timed out after " + timeout.String()
		return info
	}
	if err != nil {
		info.Status = "failed"
		info.Error = trim(err.Error()+": "+string(out), 1200)
		return info
	}
	return parseScanPayload(host, string(out))
}

func parseScanPayload(host, stdout string) HostInfo {
	info := HostInfo{Host: host}
	var payload remoteScanPayload
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
		info.Status = "failed"
		info.Error = "invalid scan JSON: " + trim(stdout, 1200)
		return info
	}
	if !payload.Config.Status || payload.Config.Version == "" {
		info.Status = "not_installed"
		return info
	}
	auth := payload.Config.Features.Auth
	info.Status = "ok"
	info.Version = payload.Config.Version
	info.Name = normalizeDisplayName(payload.Config.Name)
	info.Auth = &auth
	info.Runtime = payload.Runtime
	info.URL = "http://" + host + ":8080"
	info.Gateway = payload.Gateway
	info.Tools = payload.Tools
	info.Functions = payload.Functions
	for _, tool := range payload.Tools {
		if tool == "meshclaw_ops_tool" || tool == "server:mcp:meshclaw" {
			info.ToolInstalled = true
			break
		}
	}
	for _, fn := range payload.Functions {
		if fn == "meshclaw_ops" {
			info.PipeInstalled = true
			break
		}
	}
	switch {
	case info.PipeInstalled && info.ToolInstalled:
		info.Integration = "router-pipe+tool"
	case info.PipeInstalled:
		info.Integration = "router-pipe"
	case info.ToolInstalled:
		info.Integration = "tool"
	default:
		info.Integration = "none"
	}
	if !info.ToolInstalled && !info.PipeInstalled {
		info.Status = "missing_tool"
	}
	if info.Gateway == "" {
		info.Status = "missing_gateway"
	}
	return info
}

func isLocalHost(host string) bool {
	host = normalizeHostName(host)
	if host == "" {
		return false
	}
	names := []string{"localhost", "127.0.0.1", normalizeHostName(os.Getenv("MESHCLAW_NODE")), normalizeHostName(os.Getenv("MESHCLAW_OPENWEBUI_DEFAULT_HOST")), normalizeHostName(os.Getenv("MESHCLAW_ROUTER_DEFAULT_HOST"))}
	if local, err := os.Hostname(); err == nil {
		names = append(names, normalizeHostName(local))
	}
	for _, name := range names {
		if name != "" && host == name {
			return true
		}
	}
	return false
}

func normalizeHostName(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimSuffix(host, ".local")
	if strings.Contains(host, ".") && host != "127.0.0.1" {
		host = strings.Split(host, ".")[0]
	}
	return host
}

func normalizeDisplayName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}
	for {
		next := strings.ReplaceAll(name, "(Open WebUI) (Open WebUI)", "(Open WebUI)")
		if next == name {
			break
		}
		name = next
	}
	return name
}

func selectHosts(hosts []string) ([]string, error) {
	if len(hosts) == 0 {
		nodes := inventory.DefaultNodes()
		out := make([]string, 0, len(nodes))
		for _, node := range nodes {
			out = append(out, node.Name)
		}
		return out, nil
	}
	var out []string
	seen := map[string]bool{}
	for _, host := range hosts {
		for _, part := range strings.Split(host, ",") {
			part = strings.TrimSpace(part)
			if part == "" || seen[part] {
				continue
			}
			if _, ok := inventory.Find(part); !ok {
				return nil, fmt.Errorf("unknown node: %s", part)
			}
			seen[part] = true
			out = append(out, part)
		}
	}
	return out, nil
}

func remoteScanScript() string {
	return `python3 - <<'PY'
import json, os, sqlite3, subprocess, urllib.request

out = {"config": {}, "runtime": "", "gateway": "", "tools": [], "functions": []}
for url in ["http://127.0.0.1:8080/api/config", "http://127.0.0.1:8081/api/config"]:
    try:
        out["config"] = json.loads(urllib.request.urlopen(url, timeout=3).read().decode())
        break
    except Exception:
        pass

def sh(cmd):
    try:
        return subprocess.check_output(cmd, shell=True, stderr=subprocess.DEVNULL, text=True, timeout=3).strip()
    except Exception:
        return ""

if sh("systemctl is-active open-webui.service") == "active":
    out["runtime"] = "systemd"
elif sh("docker ps --format '{{.Names}} {{.Image}}' | grep -i 'open-webui'"):
    out["runtime"] = "docker"

for url in ["http://127.0.0.1:8771/health", "http://127.0.0.1:8766/health", "http://127.0.0.1:8767/health", "http://127.0.0.1:8765/health"]:
    try:
        data = json.loads(urllib.request.urlopen(url, timeout=2).read().decode())
        if data.get("ok") and "meshclaw" in data and "meshclaw" in str(data.get("meshclaw")):
            out["gateway"] = url
            break
    except Exception:
        pass

dbs = [
    "/app/backend/data/webui.db",
    "/home/meshclaw/.local/share/open-webui/webui.db",
    "/home/dell/.local/share/open-webui/webui.db",
    "/var/lib/docker/volumes/open-webui/_data/webui.db",
    "/var/lib/docker/volumes/meshclaw-guard-webui/_data/webui.db",
]
for db in dbs:
    if not os.path.exists(db):
        continue
    try:
        con = sqlite3.connect(db)
        try:
            out["functions"] = [row[0] for row in con.execute("select id from function where coalesce(is_active, 1)=1 and coalesce(is_global, 0)=1")]
        except Exception:
            out["functions"] = [row[0] for row in con.execute("select id from function")]
        try:
            out["tools"] = [row[0] for row in con.execute("select id from tool")]
        except Exception:
            out["tools"] = []
        try:
            row = con.execute("select data from config order by id desc limit 1").fetchone()
            if row:
                data = json.loads(row[0])
                for connection in data.get("tool_server", {}).get("connections", []):
                    if connection.get("type") == "mcp" and connection.get("info", {}).get("id") == "meshclaw":
                        out["tools"].append("server:mcp:meshclaw")
        except Exception:
            pass
        break
    except Exception:
        pass
if not out["tools"]:
    docker_names = sh("docker ps --format '{{.Names}}' 2>/dev/null | grep -E '(^open-webui$|webui)'")
    if not docker_names:
        docker_names = sh("sudo -n docker ps --format '{{.Names}}' 2>/dev/null | grep -E '(^open-webui$|webui)'")
    for name in docker_names.splitlines():
        try:
            code = "import sqlite3,json; con=sqlite3.connect('/app/backend/data/webui.db'); f=[r[0] for r in con.execute('select id from function where coalesce(is_active, 1)=1 and coalesce(is_global, 0)=1')]; t=[r[0] for r in con.execute('select id from tool')]; row=con.execute('select data from config order by id desc limit 1').fetchone(); data=json.loads(row[0]) if row else {}; t += ['server:mcp:meshclaw' for c in data.get('tool_server',{}).get('connections',[]) if c.get('type')=='mcp' and c.get('info',{}).get('id')=='meshclaw']; print(json.dumps({'functions':f,'tools':t}))"
            raw = sh("docker exec %s python -c %s" % (name, json.dumps(code)))
            if not raw:
                raw = sh("sudo -n docker exec %s python -c %s" % (name, json.dumps(code)))
            if raw:
                data = json.loads(raw)
                if isinstance(data, dict):
                    out["functions"] = data.get("functions", [])
                    out["tools"] = data.get("tools", [])
                else:
                    out["functions"] = data
                break
        except Exception:
            pass
print(json.dumps(out, ensure_ascii=False))
PY`
}

func trim(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "...trimmed..."
}
