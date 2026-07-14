package runtime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/meshclaw/meshclaw/internal/inventory"
)

type Runner struct {
	Timeout         time.Duration
	VSSHBinary      string
	PreferVSSH      bool
	DisableFallback bool
}

type Attempt struct {
	Transport string `json:"transport"`
	Target    string `json:"target"`
	Error     string `json:"error,omitempty"`
}

type Evidence struct {
	Success      bool      `json:"success"`
	Host         string    `json:"host"`
	Command      string    `json:"command"`
	Transport    string    `json:"transport"`
	Stdout       string    `json:"stdout"`
	Stderr       string    `json:"stderr"`
	ExitCode     int       `json:"exit_code"`
	DurationMs   int64     `json:"duration_ms"`
	FallbackUsed bool      `json:"fallback_used"`
	Attempts     []Attempt `json:"attempts"`
}

type VSSHCall struct {
	Success    bool                   `json:"success"`
	Args       []string               `json:"args"`
	Stdout     string                 `json:"stdout,omitempty"`
	Stderr     string                 `json:"stderr,omitempty"`
	DurationMs int64                  `json:"duration_ms"`
	Payload    map[string]interface{} `json:"payload,omitempty"`
	Error      string                 `json:"error,omitempty"`
}

func NewRunner() Runner {
	return Runner{Timeout: 45 * time.Second, VSSHBinary: DefaultVSSHBinary(), PreferVSSH: true}
}

func DefaultVSSHBinary() string {
	binary := strings.TrimSpace(os.Getenv("MESHCLAW_VSSH_BINARY"))
	if binary != "" {
		return binary
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidate := home + "/bin/vssh"
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() && st.Mode()&0111 != 0 {
			return candidate
		}
	}
	return "vssh"
}

func (r Runner) Run(host, command string) (string, error) {
	evidence := r.RunEvidence(host, command)
	output := evidence.Stdout
	if evidence.Stderr != "" {
		output += evidence.Stderr
	}
	if evidence.Success {
		return output, nil
	}
	return output, fmt.Errorf("execution failed on %s via %s", host, evidence.Transport)
}

func (r Runner) VSSHJSON(args ...string) (VSSHCall, error) {
	timeout := r.Timeout
	if timeout == 0 {
		timeout = 45 * time.Second
	}
	start := time.Now()
	call := VSSHCall{Args: append([]string{}, args...)}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.VSSHBinary, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		killProcessGroup(cmd)
	}

	call.DurationMs = time.Since(start).Milliseconds()
	call.Stdout = stdout.String()
	call.Stderr = stderr.String()
	if ctx.Err() == context.DeadlineExceeded {
		call.Error = fmt.Sprintf("vssh timed out after %s", timeout)
		return call, fmt.Errorf(call.Error)
	}
	if err != nil {
		call.Error = strings.TrimSpace(stderr.String())
		if call.Error == "" {
			call.Error = err.Error()
		}
		return call, err
	}
	if err := json.Unmarshal(stdout.Bytes(), &call.Payload); err != nil {
		call.Error = fmt.Sprintf("invalid vssh JSON: %v", err)
		return call, fmt.Errorf(call.Error)
	}
	call.Success = true
	if success, ok := call.Payload["success"].(bool); ok {
		call.Success = success
	}
	return call, nil
}

func (r Runner) RunEvidence(host, command string) Evidence {
	timeout := r.Timeout
	if timeout == 0 {
		timeout = 45 * time.Second
	}
	start := time.Now()

	evidence := Evidence{
		Host:     host,
		Command:  command,
		ExitCode: -1,
	}

	if node, ok := inventory.Find(host); ok {
		if r.PreferVSSH {
			vsshEvidence := runVSSHCommand(timeout, r.VSSHBinary, node.Name, command)
			evidence.Attempts = append(evidence.Attempts, Attempt{Transport: "vssh-native", Target: node.Name, Error: vsshEvidence.errString})
			evidence.Transport = "vssh-native"
			evidence.Stdout = vsshEvidence.stdout
			evidence.Stderr = vsshEvidence.stderr
			evidence.ExitCode = vsshEvidence.exitCode
			evidence.DurationMs = time.Since(start).Milliseconds()
			if vsshEvidence.err == nil {
				evidence.Success = true
				evidence.Attempts[len(evidence.Attempts)-1].Error = ""
				return evidence
			}
			if !vsshEvidence.fallbackAllowed || r.DisableFallback {
				return evidence
			}
			evidence.FallbackUsed = true
		}
		targets := sshTargets(node)
		for _, target := range targets {
			sshEvidence := runSSHScript(timeout, target, command)
			evidence.Attempts = append(evidence.Attempts, Attempt{Transport: "ssh", Target: target, Error: sshEvidence.errString})
			evidence.Transport = "ssh"
			evidence.Stdout = sshEvidence.stdout
			evidence.Stderr = sshEvidence.stderr
			evidence.ExitCode = sshEvidence.exitCode
			evidence.DurationMs = time.Since(start).Milliseconds()
			if sshEvidence.err == nil {
				evidence.Success = true
				evidence.Attempts[len(evidence.Attempts)-1].Error = ""
				return evidence
			}
			evidence.FallbackUsed = true
		}
	} else {
		evidence.Attempts = append(evidence.Attempts, Attempt{Transport: "inventory", Target: host, Error: "unknown node"})
	}

	evidence.DurationMs = time.Since(start).Milliseconds()
	return evidence
}

func (r Runner) Status() (string, error) {
	var out bytes.Buffer
	fmt.Fprintf(&out, "%-10s %-15s %-15s %s\n", "NODE", "TAILSCALE", "USER", "ROLE")
	for _, node := range inventory.DefaultNodes() {
		fmt.Fprintf(&out, "%-10s %-15s %-15s %s\n", node.Name, node.Tailscale, node.User, node.Role)
	}
	return out.String(), nil
}

type sshScriptResult struct {
	stdout          string
	stderr          string
	exitCode        int
	err             error
	errString       string
	fallbackAllowed bool
}

func runVSSHCommand(timeout time.Duration, binary, target, command string) sshScriptResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "run-many", target, encodeShellScriptForVSSH(command))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		killProcessGroup(cmd)
	}

	result := sshScriptResult{
		stdout:   stdout.String(),
		stderr:   stderr.String(),
		exitCode: 0,
		err:      err,
	}
	if ctx.Err() == context.DeadlineExceeded {
		result.exitCode = 124
		result.err = fmt.Errorf("vssh timed out after %s", timeout)
		result.errString = result.err.Error()
		result.fallbackAllowed = true
		return result
	}
	if err != nil {
		result.exitCode = commandExitCode(err)
		result.errString = err.Error()
		result.fallbackAllowed = true
		return result
	}

	var responses []struct {
		Target string `json:"target"`
		Result *struct {
			Success    bool   `json:"success"`
			Command    string `json:"command"`
			Stdout     string `json:"stdout"`
			Stderr     string `json:"stderr"`
			ExitCode   int    `json:"exit_code"`
			DurationMs int64  `json:"duration_ms"`
			Error      string `json:"error"`
		} `json:"result"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &responses); err != nil {
		result.exitCode = -1
		result.err = fmt.Errorf("invalid vssh JSON: %w", err)
		result.errString = result.err.Error()
		result.fallbackAllowed = true
		return result
	}
	if len(responses) == 0 {
		result.exitCode = -1
		result.err = fmt.Errorf("empty vssh response")
		result.errString = result.err.Error()
		result.fallbackAllowed = true
		return result
	}
	response := responses[0]
	if response.Error != "" {
		result.exitCode = -1
		result.err = errors.New(response.Error)
		result.errString = response.Error
		result.fallbackAllowed = true
		return result
	}
	if response.Result == nil {
		result.exitCode = -1
		result.err = fmt.Errorf("missing vssh result")
		result.errString = result.err.Error()
		result.fallbackAllowed = true
		return result
	}

	result.stdout = response.Result.Stdout
	result.stderr = response.Result.Stderr
	result.exitCode = response.Result.ExitCode
	if !response.Result.Success {
		msg := response.Result.Error
		if msg == "" {
			msg = fmt.Sprintf("remote command exited with %d", response.Result.ExitCode)
		}
		result.err = errors.New(msg)
		result.errString = msg
		result.fallbackAllowed = false
	}
	return result
}

func encodeShellScriptForVSSH(script string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(script))
	return "printf %s '" + encoded + "' | base64 -d | /bin/bash"
}

func normalizeShellScriptForArg(script string) string {
	lines := strings.Split(script, "\n")
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		parts = append(parts, trimmed)
		if shellLineNeedsSeparator(trimmed) {
			parts = append(parts, ";")
		}
	}
	return strings.Join(parts, " ")
}

func shellLineNeedsSeparator(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if strings.HasSuffix(trimmed, ";") || strings.HasSuffix(trimmed, "&&") || strings.HasSuffix(trimmed, "||") || strings.HasSuffix(trimmed, "|") || strings.HasSuffix(trimmed, "\\") {
		return false
	}
	last := trimmed
	if fields := strings.Fields(trimmed); len(fields) > 0 {
		last = fields[len(fields)-1]
	}
	switch last {
	case "then", "do", "else", "in":
		return false
	default:
		return true
	}
}

func runSSHScript(timeout time.Duration, target, script string) sshScriptResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ssh",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=8",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		target,
		"sh", "-s",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdin = strings.NewReader(script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		killProcessGroup(cmd)
	}

	result := sshScriptResult{
		stdout:   stdout.String(),
		stderr:   stderr.String(),
		exitCode: 0,
		err:      err,
	}
	if ctx.Err() == context.DeadlineExceeded {
		result.exitCode = 124
		result.err = fmt.Errorf("ssh timed out after %s", timeout)
		result.errString = result.err.Error()
		return result
	}
	if err != nil {
		result.exitCode = commandExitCode(err)
		result.errString = err.Error()
	}
	return result
}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

func commandExitCode(err error) int {
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return -1
}

func sshTargets(node inventory.Node) []string {
	targets := []string{}
	add := func(host string) {
		host = strings.TrimSpace(host)
		if host == "" {
			return
		}
		target := host
		if node.User != "" {
			target = node.User + "@" + host
		}
		for _, existing := range targets {
			if existing == target {
				return
			}
		}
		targets = append(targets, target)
	}

	add(node.Tailscale)
	add(node.LAN)
	add(node.WireIP)
	add(node.Name)
	return targets
}
