package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunEvidenceUsesVSSHRunManyJSON(t *testing.T) {
	setRuntimeTestInventory(t, "d1")
	binary := fakeVSSH(t, `#!/bin/sh
cat <<'JSON'
[{"target":"d1","result":{"success":true,"command":"printf ok","stdout":"ok\n","stderr":"","exit_code":0,"duration_ms":7}}]
JSON
`)

	result := Runner{Timeout: 10 * time.Second, VSSHBinary: binary, PreferVSSH: true, DisableFallback: true}.RunEvidence("d1", "printf ok")

	if !result.Success {
		t.Fatalf("expected success: %+v", result)
	}
	if result.Transport != "vssh-native" {
		t.Fatalf("transport=%q", result.Transport)
	}
	if result.Stdout != "ok\n" {
		t.Fatalf("stdout=%q", result.Stdout)
	}
	if result.FallbackUsed {
		t.Fatal("did not expect SSH fallback")
	}
}

func TestVSSHJSONParsesPayload(t *testing.T) {
	binary := fakeVSSH(t, `#!/bin/sh
cat <<'JSON'
{"success":true,"data":{"id":"job-1"}}
JSON
`)

	call, err := Runner{Timeout: 10 * time.Second, VSSHBinary: binary}.VSSHJSON("job-start", "d1", "echo ok")
	if err != nil {
		t.Fatal(err)
	}
	if !call.Success {
		t.Fatalf("success=false: %+v", call)
	}
	data := call.Payload["data"].(map[string]interface{})
	if data["id"] != "job-1" {
		t.Fatalf("payload=%v", call.Payload)
	}
}

func TestRunEvidenceDoesNotFallbackOnRemoteCommandFailure(t *testing.T) {
	setRuntimeTestInventory(t, "d1")
	binary := fakeVSSH(t, `#!/bin/sh
cat <<'JSON'
[{"target":"d1","result":{"success":false,"command":"exit 42","stdout":"","stderr":"bad\n","exit_code":42,"duration_ms":7,"error":"remote failed"}}]
JSON
`)

	result := Runner{Timeout: 10 * time.Second, VSSHBinary: binary, PreferVSSH: true}.RunEvidence("d1", "exit 42")

	if result.Success {
		t.Fatalf("expected failure: %+v", result)
	}
	if result.Transport != "vssh-native" {
		t.Fatalf("transport=%q", result.Transport)
	}
	if result.ExitCode != 42 {
		t.Fatalf("exit_code=%d", result.ExitCode)
	}
	if result.FallbackUsed {
		t.Fatal("remote command failure must not fall back to SSH")
	}
	if len(result.Attempts) != 1 {
		t.Fatalf("attempts=%d", len(result.Attempts))
	}
}

func TestNormalizeShellScriptForVSSHArgPreservesCommandBoundaries(t *testing.T) {
	script := `echo "---failed---"
systemctl --failed --no-legend --plain 2>/dev/null || true
echo "---recent-service-errors---"
if command -v journalctl >/dev/null 2>&1; then
  journalctl -p warning..alert -n 20 --no-pager 2>/dev/null | tail -5 || true
fi`

	got := normalizeShellScriptForArg(script)

	for _, want := range []string{
		`echo "---failed---" ; systemctl`,
		`true ; echo "---recent-service-errors---"`,
		`if command -v journalctl >/dev/null 2>&1; then journalctl`,
		`true ; fi ;`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("normalized script missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "then ;") {
		t.Fatalf("normalized script inserted invalid then separator:\n%s", got)
	}
}

func TestNormalizeShellScriptForVSSHArgPreservesCaseSyntax(t *testing.T) {
	script := `while IFS= read -r p; do
  case "$p" in
    *clean*|*final*) continue ;;
  esac
  printf '%s\n' "$p"
done`

	got := normalizeShellScriptForArg(script)

	if strings.Contains(got, `case "$p" in ;`) {
		t.Fatalf("normalized script inserted invalid case separator:\n%s", got)
	}
	if !strings.Contains(got, `case "$p" in *clean*|*final*) continue ;; esac ;`) {
		t.Fatalf("normalized script missing case body separators:\n%s", got)
	}
}

func TestEncodeShellScriptForVSSHAvoidsOuterVariableExpansion(t *testing.T) {
	script := `manifest=/tmp/example
echo "$manifest"`

	got := encodeShellScriptForVSSH(script)

	if strings.Contains(got, "$manifest") {
		t.Fatalf("encoded transport command exposes shell variable to outer shell:\n%s", got)
	}
	if !strings.Contains(got, "base64 -d | /bin/bash") {
		t.Fatalf("encoded transport command should execute through bash stdin:\n%s", got)
	}
}

func fakeVSSH(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vssh")
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func setRuntimeTestInventory(t *testing.T, node string) {
	t.Helper()
	dir := t.TempDir()
	inventoryPath := filepath.Join(dir, "inventory.json")
	overridesPath := filepath.Join(dir, "overrides.json")
	data := `{"version":1,"nodes":[{"name":"` + node + `","user":"tester","source":"unit-test"}]}`
	if err := os.WriteFile(inventoryPath, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overridesPath, []byte(`{"version":1,"nodes":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MESHCLAW_INVENTORY_FILE", inventoryPath)
	t.Setenv("MESHCLAW_INVENTORY_OVERRIDES_FILE", overridesPath)
}
