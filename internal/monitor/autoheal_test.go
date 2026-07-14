package monitor

import (
	"strings"
	"testing"

	"github.com/meshclaw/meshclaw/internal/runtime"
)

func TestApplyRuntimeResultAddsTransportAndExitCode(t *testing.T) {
	action := &HealAction{}
	applyRuntimeResult(action, runtime.Evidence{
		Success:   false,
		Transport: "vssh-native",
		ExitCode:  42,
		Stderr:    "failed",
	})

	if action.Success {
		t.Fatal("expected failed action")
	}
	if action.Transport != "vssh-native" {
		t.Fatalf("transport=%q", action.Transport)
	}
	if action.ExitCode != 42 {
		t.Fatalf("exit=%d", action.ExitCode)
	}
	if !strings.Contains(action.Result, "failed via vssh-native exit=42") {
		t.Fatalf("result=%q", action.Result)
	}
}

func TestFormatRuntimeResultSuccessWithoutOutput(t *testing.T) {
	got := formatRuntimeResult(runtime.Evidence{Success: true, Transport: "vssh-native"})
	if got != "completed via vssh-native" {
		t.Fatalf("got=%q", got)
	}
}
