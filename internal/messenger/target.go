package messenger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Target struct {
	ID        string    `json:"id"`
	Channel   string    `json:"channel"`
	Recipient string    `json:"recipient"`
	GroupID   string    `json:"group_id,omitempty"`
	Label     string    `json:"label,omitempty"`
	Mode      string    `json:"mode,omitempty"`
	Model     string    `json:"model,omitempty"`
	BaseURL   string    `json:"base_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type TargetStore struct {
	Kind    string   `json:"kind"`
	Path    string   `json:"path"`
	Targets []Target `json:"targets"`
}

type SendOptions struct {
	Ref            string
	Step           string
	TargetID       string
	Channel        string
	Execute        bool
	Kind           string
	Text           string
	Attachments    []string
	VoiceNote      bool
	TimeoutSeconds int
}

type SendResult struct {
	Kind               string           `json:"kind"`
	Mode               string           `json:"mode"`
	Target             Target           `json:"target"`
	MessageKind        string           `json:"message_kind"`
	Ref                string           `json:"ref"`
	Step               string           `json:"step,omitempty"`
	Text               string           `json:"text"`
	Attachments        []string         `json:"attachments,omitempty"`
	VoiceNote          bool             `json:"voice_note,omitempty"`
	Command            []string         `json:"command,omitempty"`
	Executed           bool             `json:"executed"`
	Success            bool             `json:"success"`
	ExitCode           int              `json:"exit_code,omitempty"`
	TimeoutSeconds     int              `json:"timeout_seconds,omitempty"`
	TimedOut           bool             `json:"timed_out,omitempty"`
	Stdout             string           `json:"stdout,omitempty"`
	Stderr             string           `json:"stderr,omitempty"`
	Error              string           `json:"error,omitempty"`
	RawSecretsIncluded bool             `json:"raw_secrets_included"`
	GeneratedAt        time.Time        `json:"generated_at"`
	Report             *Report          `json:"report,omitempty"`
	ApprovalRequest    *ApprovalRequest `json:"approval_request,omitempty"`
}

func TargetPath() string {
	if configured := strings.TrimSpace(os.Getenv("MESHCLAW_MESSENGER_TARGETS")); configured != "" {
		return configured
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".meshclaw", "messenger-targets.json")
	}
	return filepath.Join(home, ".meshclaw", "messenger-targets.json")
}

func ListTargets() (TargetStore, error) {
	path := TargetPath()
	store := TargetStore{Kind: "meshclaw_messenger_targets", Path: path, Targets: []Target{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return store, err
	}
	if err := json.Unmarshal(data, &store); err != nil {
		return store, err
	}
	store.Kind = "meshclaw_messenger_targets"
	store.Path = path
	return store, nil
}

func UpsertTarget(target Target) (TargetStore, Target, error) {
	target.ID = sanitizeID(target.ID)
	target.Channel = strings.ToLower(strings.TrimSpace(target.Channel))
	target.Recipient = strings.TrimSpace(target.Recipient)
	target.GroupID = strings.TrimSpace(target.GroupID)
	target.Label = strings.TrimSpace(target.Label)
	target.Mode = strings.ToLower(strings.TrimSpace(target.Mode))
	target.Model = strings.TrimSpace(target.Model)
	target.BaseURL = strings.TrimRight(strings.TrimSpace(target.BaseURL), "/")
	if target.ID == "" {
		return TargetStore{}, Target{}, fmt.Errorf("target id is required")
	}
	if target.Channel == "" {
		target.Channel = "signal"
	}
	if target.Channel != "signal" {
		return TargetStore{}, Target{}, fmt.Errorf("unsupported messenger channel %q", target.Channel)
	}
	if target.Recipient == "" && target.GroupID == "" {
		return TargetStore{}, Target{}, fmt.Errorf("target recipient or group_id is required")
	}
	if target.Recipient != "" && target.GroupID != "" {
		return TargetStore{}, Target{}, fmt.Errorf("target recipient and group_id are mutually exclusive")
	}
	if target.Mode != "" && target.Mode != "guard" && target.Mode != "chat" && target.Mode != "ops" && target.Mode != "briefing" && target.Mode != "assistant" {
		return TargetStore{}, Target{}, fmt.Errorf("target mode must be guard, chat, ops, briefing, or assistant")
	}
	store, err := ListTargets()
	if err != nil {
		return store, Target{}, err
	}
	now := time.Now().UTC()
	target.UpdatedAt = now
	found := false
	for i := range store.Targets {
		if store.Targets[i].ID == target.ID {
			if target.CreatedAt.IsZero() {
				target.CreatedAt = store.Targets[i].CreatedAt
			}
			store.Targets[i] = target
			found = true
			break
		}
	}
	if !found {
		target.CreatedAt = now
		store.Targets = append(store.Targets, target)
	}
	if err := writeTargets(store); err != nil {
		return store, Target{}, err
	}
	return store, target, nil
}

func RemoveTarget(id string) (TargetStore, bool, error) {
	id = sanitizeID(id)
	store, err := ListTargets()
	if err != nil {
		return store, false, err
	}
	out := make([]Target, 0, len(store.Targets))
	removed := false
	for _, target := range store.Targets {
		if target.ID == id {
			removed = true
			continue
		}
		out = append(out, target)
	}
	store.Targets = out
	if err := writeTargets(store); err != nil {
		return store, false, err
	}
	return store, removed, nil
}

func Send(opts SendOptions) (SendResult, error) {
	target, err := findTarget(opts.TargetID)
	if err != nil {
		return SendResult{}, err
	}
	ref := strings.TrimSpace(opts.Ref)
	if ref == "" {
		ref = "latest"
	}
	kind := strings.TrimSpace(opts.Kind)
	if kind == "" {
		kind = "report"
	}
	result := SendResult{
		Kind:               "meshclaw_messenger_send",
		Mode:               "dry-run",
		Target:             target,
		MessageKind:        kind,
		Ref:                ref,
		Step:               strings.TrimSpace(opts.Step),
		GeneratedAt:        time.Now().UTC(),
		RawSecretsIncluded: false,
		Attachments:        cleanAttachments(opts.Attachments),
		VoiceNote:          opts.VoiceNote,
	}
	switch kind {
	case "text":
		result.Text, result.Attachments = normalizeSendTextAttachments(opts.Text, result.Attachments)
	case "report":
		report, err := BuildReport(ReportOptions{Ref: ref, Channel: target.Channel, Audience: target.ID})
		if err != nil {
			return result, err
		}
		if _, err := WriteReport(report); err != nil {
			return result, err
		}
		result.Text = report.Text
		result.Report = &report
		result.Attachments = cleanAttachments(append(result.Attachments, report.Evidence.Report))
	case "approval-request":
		req, err := BuildApprovalRequest(ref, result.Step, target.Channel, target.ID)
		if err != nil {
			return result, err
		}
		result.Text = req.Text
		result.ApprovalRequest = &req
	default:
		return result, fmt.Errorf("unsupported messenger message kind %q", kind)
	}
	result.Command = signalCLICommand(target, result.Text, result.Attachments, result.VoiceNote)
	if !opts.Execute {
		result.Success = true
		return result, nil
	}
	if err := guardLocalSignalSend(target); err != nil {
		result.Error = err.Error()
		return result, err
	}
	result.Mode = "execute"
	timeout := signalSendTimeout(opts.TimeoutSeconds)
	result.TimeoutSeconds = int(timeout.Seconds())
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	completed := exec.CommandContext(ctx, result.Command[0], result.Command[1:]...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	completed.Stdout = &stdout
	completed.Stderr = &stderr
	err = completed.Run()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	if ctx.Err() == context.DeadlineExceeded {
		result.Success = false
		result.TimedOut = true
		result.Error = fmt.Sprintf("signal-cli send timed out after %d seconds", result.TimeoutSeconds)
		return result, fmt.Errorf("%s", result.Error)
	}
	if err != nil {
		result.Success = false
		if exit, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exit.ExitCode()
			if result.Stderr == "" {
				result.Stderr = string(exit.Stderr)
			}
		}
		result.Error = err.Error()
		return result, err
	}
	result.Executed = true
	result.Success = true
	return result, nil
}

func normalizeSendTextAttachments(text string, attachments []string) (string, []string) {
	cleaned := cleanAttachments(attachments)
	if !sendTextHasAttachmentMarker(text) {
		return text, cleaned
	}
	visible := signalReplyVisibleText(text)
	extracted := signalReplyAttachments(text)
	return visible, cleanAttachments(append(cleaned, extracted...))
}

func sendTextHasAttachmentMarker(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		if _, ok := signalReplyAttachmentPath(line); ok {
			return true
		}
	}
	return false
}

func signalSendTimeout(configuredSeconds int) time.Duration {
	seconds := configuredSeconds
	if seconds <= 0 {
		if value := strings.TrimSpace(os.Getenv("MESHCLAW_SIGNAL_SEND_TIMEOUT_SECONDS")); value != "" {
			if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
				seconds = parsed
			}
		}
	}
	if seconds <= 0 {
		seconds = 45
	}
	return time.Duration(seconds) * time.Second
}

func guardLocalSignalSend(target Target) error {
	if strings.ToLower(strings.TrimSpace(target.Channel)) != "signal" {
		return nil
	}
	if localSignalSendOverrideEnabled() {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	if filepath.Clean(home) != "/Users/example" {
		return nil
	}
	if !isArgosUserFacingSignalTarget(target) {
		return nil
	}
	return fmt.Errorf("MacBook Signal sends to Argos user-facing targets are disabled; use the Mac mini Argos Signal runtime instead")
}

func localSignalSendOverrideEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("MESHCLAW_ALLOW_MACBOOK_SIGNAL_SEND")))
	return value == "1" || value == "true" || value == "on" || value == "yes"
}

func isArgosUserFacingSignalTarget(target Target) bool {
	id := sanitizeID(target.ID)
	mode := strings.ToLower(strings.TrimSpace(target.Mode))
	if mode == "assistant" || mode == "briefing" || mode == "ops" {
		return true
	}
	if id == "report-room" || id == "argos-briefing" || id == "argos-assistant-group" {
		return true
	}
	return strings.HasPrefix(id, "argos-")
}

func writeTargets(store TargetStore) error {
	if store.Path == "" {
		store.Path = TargetPath()
	}
	store.Kind = "meshclaw_messenger_targets"
	if err := os.MkdirAll(filepath.Dir(store.Path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(store.Path, append(data, '\n'), 0600)
}

func findTarget(id string) (Target, error) {
	id = sanitizeID(id)
	if id == "" {
		return Target{}, fmt.Errorf("target id is required")
	}
	store, err := ListTargets()
	if err != nil {
		return Target{}, err
	}
	for _, target := range store.Targets {
		if target.ID == id {
			return target, nil
		}
	}
	return Target{}, fmt.Errorf("messenger target %q not found", id)
}

func OneWayReportTargetID(id string) bool {
	target, err := findTarget(id)
	if err != nil {
		return false
	}
	return OneWayReportTarget(target)
}

func OneWayReportTarget(target Target) bool {
	id := sanitizeID(target.ID)
	if id == "argos-briefing" || id == "argos-ops" || id == "report-room" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(target.Mode)) {
	case "briefing", "ops":
		return true
	default:
		return false
	}
}

func signalCLICommand(target Target, message string, attachments []string, voiceNote bool) []string {
	binary := signalCLIBinary()
	args := []string{binary, "send", "-m", message}
	if target.GroupID != "" {
		args = append(args, "-g", target.GroupID)
		args = appendSignalNotifySelf(args)
	} else {
		args = append(args, target.Recipient)
	}
	args = appendSignalAttachments(args, attachments, voiceNote)
	if account := strings.TrimSpace(os.Getenv("MESHCLAW_SIGNAL_ACCOUNT")); account != "" {
		args = []string{binary, "-a", account, "send", "-m", message}
		if target.GroupID != "" {
			args = append(args, "-g", target.GroupID)
			args = appendSignalNotifySelf(args)
		} else {
			args = append(args, target.Recipient)
		}
		args = appendSignalAttachments(args, attachments, voiceNote)
	}
	return args
}

func appendSignalNotifySelf(args []string) []string {
	if !signalNotifySelfEnabled() {
		return args
	}
	return append(args, "--notify-self")
}

func signalNotifySelfEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("MESHCLAW_SIGNAL_NOTIFY_SELF")))
	return value != "0" && value != "false" && value != "off" && value != "no"
}

func appendSignalAttachments(args []string, attachments []string, voiceNote bool) []string {
	clean := cleanAttachments(attachments)
	if len(clean) == 0 {
		return args
	}
	args = append(args, "--attachment")
	args = append(args, clean...)
	if voiceNote {
		args = append(args, "--voice-note")
	}
	return args
}

func cleanAttachments(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path != "" {
			out = append(out, path)
		}
	}
	return out
}

func signalCLIBinary() string {
	if binary := strings.TrimSpace(os.Getenv("MESHCLAW_SIGNAL_CLI")); binary != "" {
		return binary
	}
	for _, path := range []string{"/opt/homebrew/bin/signal-cli", "/usr/local/bin/signal-cli", "/usr/bin/signal-cli"} {
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			return path
		}
	}
	return "signal-cli"
}

func sanitizeID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else if b.Len() > 0 {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
