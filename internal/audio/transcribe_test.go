package audio

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestTranscribeLocalWhisperPlanOnly(t *testing.T) {
	audioPath := fakeAudioFile(t)
	fakeWhisperInPath(t)

	plan, err := TranscribeLocalWhisper(time.Date(2026, 6, 3, 1, 2, 3, 0, time.UTC), audioPath, "", "", "", "", false, false)
	if err != nil {
		t.Fatalf("TranscribeLocalWhisper error = %v", err)
	}
	if plan.Kind != "meshclaw_audio_transcribe" || plan.Action != "audio_transcribe" {
		t.Fatalf("unexpected kind/action: %#v", plan)
	}
	if plan.Status != "review_ready" || plan.Execute || plan.Approved {
		t.Fatalf("plan should default to review-only: %#v", plan)
	}
	if plan.Model != "turbo" || plan.Task != "transcribe" {
		t.Fatalf("unexpected defaults: %#v", plan)
	}
	if plan.Output == "" || filepath.Ext(plan.Output) != ".txt" {
		t.Fatalf("missing transcript output path: %#v", plan)
	}
	if plan.Transcript != "" {
		t.Fatalf("plan-only should not transcribe: %#v", plan)
	}
}

func TestTranscribeLocalWhisperExecutionRequiresApproval(t *testing.T) {
	audioPath := fakeAudioFile(t)
	fakeWhisperInPath(t)

	plan, err := TranscribeLocalWhisper(time.Date(2026, 6, 3, 1, 2, 3, 0, time.UTC), audioPath, "", "", "", "", true, false)
	if err != nil {
		t.Fatalf("TranscribeLocalWhisper error = %v", err)
	}
	if plan.Status != "approval_required" || !plan.ApprovalMissing {
		t.Fatalf("execute should require explicit approval: %#v", plan)
	}
	if plan.Transcript != "" {
		t.Fatalf("approval-required should not transcribe: %#v", plan)
	}
}

func TestTranscribeLocalWhisperMissingDependency(t *testing.T) {
	audioPath := fakeAudioFile(t)
	t.Setenv("PATH", t.TempDir())

	plan, err := TranscribeLocalWhisper(time.Date(2026, 6, 3, 1, 2, 3, 0, time.UTC), audioPath, "", "", "", "", false, false)
	if err != nil {
		t.Fatalf("TranscribeLocalWhisper error = %v", err)
	}
	if plan.Status != "needs_dependency" || plan.Dependency != "whisper" {
		t.Fatalf("missing dependency should be structured: %#v", plan)
	}
}

func fakeAudioFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "voice.mp3")
	if err := os.WriteFile(path, []byte("fake audio"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func fakeWhisperInPath(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	name := "whisper"
	if runtime.GOOS == "windows" {
		name += ".bat"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}
