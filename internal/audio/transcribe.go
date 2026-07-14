package audio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type TranscribePlan struct {
	Kind             string    `json:"kind"`
	Action           string    `json:"action"`
	Generated        time.Time `json:"generated"`
	Status           string    `json:"status"`
	Engine           string    `json:"engine"`
	Path             string    `json:"path"`
	Output           string    `json:"output"`
	Model            string    `json:"model"`
	Language         string    `json:"language,omitempty"`
	Task             string    `json:"task"`
	Execute          bool      `json:"execute"`
	Approved         bool      `json:"approved"`
	UserMessage      string    `json:"user_message"`
	ApprovalNote     string    `json:"approval_note"`
	ApprovalRequired bool      `json:"approval_required"`
	ApprovalMissing  bool      `json:"approval_missing,omitempty"`
	Dependency       string    `json:"dependency,omitempty"`
	FileSize         int64     `json:"file_size,omitempty"`
	SHA256           string    `json:"sha256,omitempty"`
	Transcript       string    `json:"transcript,omitempty"`
	Stdout           string    `json:"stdout,omitempty"`
	Stderr           string    `json:"stderr,omitempty"`
	Error            string    `json:"error,omitempty"`
	Next             []string  `json:"next"`
}

func TranscribeLocalWhisper(now time.Time, path, output, model, language, task string, execute, approve bool) (TranscribePlan, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	path = strings.TrimSpace(path)
	output = strings.TrimSpace(output)
	model = firstNonEmpty(strings.TrimSpace(model), "turbo")
	language = strings.TrimSpace(language)
	task = firstNonEmpty(strings.TrimSpace(task), "transcribe")
	plan := TranscribePlan{
		Kind:             "meshclaw_audio_transcribe",
		Action:           "audio_transcribe",
		Generated:        now.UTC(),
		Status:           "review_required",
		Engine:           "local_whisper",
		Path:             path,
		Output:           output,
		Model:            model,
		Language:         language,
		Task:             task,
		Execute:          execute,
		Approved:         approve,
		UserMessage:      "오디오 전사를 로컬 Whisper로 준비했습니다.",
		ApprovalNote:     "Default is plan-only. execute=true requires approve=true because audio can contain private speech and first-run Whisper may download local model files.",
		ApprovalRequired: true,
		Next: []string{
			"Review the audio path and output path before execution.",
			"Run with execute=true and approve=true only when the user asked to transcribe this file.",
		},
	}
	if path == "" {
		plan.Status = "missing_path"
		plan.UserMessage = "전사할 오디오 파일 경로가 필요합니다."
		return plan, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		plan.Status = "path_error"
		plan.Error = err.Error()
		return plan, nil
	}
	if info.IsDir() {
		plan.Status = "path_error"
		plan.Error = "audio path is a directory"
		return plan, nil
	}
	plan.FileSize = info.Size()
	if output == "" {
		output = defaultTranscriptPath(now, path)
		plan.Output = output
	}
	whisper, err := exec.LookPath("whisper")
	if err != nil {
		plan.Status = "needs_dependency"
		plan.Dependency = "whisper"
		plan.UserMessage = "로컬 Whisper CLI를 찾지 못했습니다."
		return plan, nil
	}
	plan.Dependency = whisper
	if !execute {
		plan.Status = "review_ready"
		return plan, nil
	}
	if !approve {
		plan.Status = "approval_required"
		plan.ApprovalMissing = true
		return plan, nil
	}
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		plan.Status = "output_error"
		plan.Error = err.Error()
		return plan, nil
	}
	outDir := filepath.Dir(output)
	base := strings.TrimSuffix(filepath.Base(output), filepath.Ext(output))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	args := []string{path, "--model", model, "--output_format", "txt", "--output_dir", outDir}
	if language != "" {
		args = append(args, "--language", language)
	}
	if task != "" {
		args = append(args, "--task", task)
	}
	cmd := exec.CommandContext(ctx, whisper, args...)
	out, err := cmd.CombinedOutput()
	plan.Stdout = strings.TrimSpace(string(out))
	if err != nil {
		plan.Status = "failed"
		plan.Error = err.Error()
		return plan, nil
	}
	generated := filepath.Join(outDir, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+".txt")
	if generated != output {
		if data, readErr := os.ReadFile(generated); readErr == nil {
			_ = os.WriteFile(output, data, 0644)
		}
	}
	if data, err := os.ReadFile(output); err == nil {
		plan.Transcript = strings.TrimSpace(string(data))
		sum := sha256.Sum256(data)
		plan.SHA256 = hex.EncodeToString(sum[:])
	}
	if plan.Transcript == "" {
		fallback := filepath.Join(outDir, base+".txt")
		if data, err := os.ReadFile(fallback); err == nil {
			plan.Output = fallback
			plan.Transcript = strings.TrimSpace(string(data))
			sum := sha256.Sum256(data)
			plan.SHA256 = hex.EncodeToString(sum[:])
		}
	}
	plan.Status = "completed"
	plan.UserMessage = "오디오 전사를 완료했습니다."
	return plan, nil
}

func defaultTranscriptPath(now time.Time, audioPath string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	base := strings.TrimSuffix(filepath.Base(audioPath), filepath.Ext(audioPath))
	if base == "" {
		base = "audio"
	}
	name := fmt.Sprintf("%s-%s.txt", safeName(base), now.UTC().Format("20060102T150405Z"))
	return filepath.Join(home, ".meshclaw", "transcripts", name)
}

func safeName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "audio"
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "audio"
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
