package messenger

import (
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/guard"
	"github.com/meshclaw/meshclaw/internal/guardvault"
)

func TestGuardSignalStoresPendingSecretByHandle(t *testing.T) {
	t.Setenv("MESHCLAW_GUARD_VAULT_DIR", t.TempDir())
	t.Setenv("MESHCLAW_SIGNAL_GUARD_BACKEND", "local")
	guardSecretPending.Lock()
	guardSecretPending.items = map[string]pendingGuardSecret{}
	guardSecretPending.Unlock()
	guardCapturePending.Lock()
	guardCapturePending.items = map[string]pendingGuardCapture{}
	guardCapturePending.Unlock()

	target := Target{ID: "argos-guard", Channel: "signal", GroupID: "guard-group", Mode: "guard"}
	raw := "api_key=operator-example-secret-value-1234567890"
	report := guard.Detect("signal-guard", raw)
	first := IncomingMessage{
		Source:         "+821000000000",
		GroupID:        "guard-group",
		Redacted:       report.Redacted,
		raw:            raw,
		SecretDetected: true,
		Intent:         guard.ParseIntent(report.Redacted).Intent,
	}

	reply, handled := guardVaultSignalReply(target, first)
	if !handled {
		t.Fatal("secret ingress was not handled")
	}
	if !strings.Contains(reply, "비밀값을 감지했습니다") {
		t.Fatalf("reply=%q", reply)
	}
	if strings.Contains(reply, "operator-example-secret") {
		t.Fatalf("reply leaked raw secret: %q", reply)
	}

	second := IncomingMessage{
		Source:   first.Source,
		GroupID:  first.GroupID,
		Redacted: "저장 operator-example api-key local",
		Intent:   "credential",
	}
	reply, handled = guardVaultSignalReply(target, second)
	if !handled {
		t.Fatal("store command was not handled")
	}
	if !strings.Contains(reply, "vault://meshclaw/operator-example/api-key") {
		t.Fatalf("reply=%q", reply)
	}
	if strings.Contains(reply, "operator-example-secret") {
		t.Fatalf("reply leaked raw secret: %q", reply)
	}

	entry, err := guardvault.Metadata("operator-example", "api-key")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Handle != "vault://meshclaw/operator-example/api-key" || entry.Backend != "local-aes-gcm" {
		t.Fatalf("entry=%#v", entry)
	}
}

func TestParseGuardStoreCommand(t *testing.T) {
	cmd, ok := parseGuardStoreCommand("저장 operator-example api-key keychain")
	if !ok {
		t.Fatal("command not parsed")
	}
	if cmd.Scope != "operator-example" || cmd.Name != "api-key" || cmd.Backend != "keychain" {
		t.Fatalf("cmd=%#v", cmd)
	}
}

func TestGuardSignalExplicitCaptureStoresNextMessage(t *testing.T) {
	t.Setenv("MESHCLAW_GUARD_VAULT_DIR", t.TempDir())
	t.Setenv("MESHCLAW_SIGNAL_GUARD_BACKEND", "local")
	t.Setenv("MESHCLAW_SIGNAL_GUARD_SECRET_TTL_SECONDS", "60")
	guardSecretPending.Lock()
	guardSecretPending.items = map[string]pendingGuardSecret{}
	guardSecretPending.Unlock()
	guardCapturePending.Lock()
	guardCapturePending.items = map[string]pendingGuardCapture{}
	guardCapturePending.Unlock()

	target := Target{ID: "argos-guard", Channel: "signal", GroupID: "guard-group", Mode: "guard"}
	first := IncomingMessage{
		Source:   "+821000000000",
		GroupID:  "guard-group",
		Redacted: "비밀 입력 cloudflare dns-token local",
		Intent:   "credential",
	}
	reply, handled := guardVaultSignalReply(target, first)
	if !handled {
		t.Fatal("capture command was not handled")
	}
	if !strings.Contains(reply, "다음 60초") || !strings.Contains(reply, "cloudflare/dns-token") {
		t.Fatalf("reply=%q", reply)
	}

	raw := "cf-token-operator-example-secret-value-1234567890"
	second := IncomingMessage{
		Source:   first.Source,
		GroupID:  first.GroupID,
		Redacted: "[REDACTED]",
		raw:      raw,
		Intent:   "credential",
	}
	reply, handled = guardVaultSignalReply(target, second)
	if !handled {
		t.Fatal("captured secret was not handled")
	}
	if !strings.Contains(reply, "vault://meshclaw/cloudflare/dns-token") {
		t.Fatalf("reply=%q", reply)
	}
	if strings.Contains(reply, "cf-token-operator-example-secret") {
		t.Fatalf("reply leaked raw secret: %q", reply)
	}

	entry, err := guardvault.Metadata("cloudflare", "dns-token")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Handle != "vault://meshclaw/cloudflare/dns-token" || entry.Backend != "local-aes-gcm" {
		t.Fatalf("entry=%#v", entry)
	}
}

func TestParseGuardCaptureCommand(t *testing.T) {
	cmd, ok := parseGuardCaptureCommand("비밀 입력 cloudflare dns-token keychain")
	if !ok {
		t.Fatal("command not parsed")
	}
	if cmd.Scope != "cloudflare" || cmd.Name != "dns-token" || cmd.Backend != "keychain" {
		t.Fatalf("cmd=%#v", cmd)
	}
	cmd, ok = parseGuardCaptureCommand("secret github pat local")
	if !ok {
		t.Fatal("english command not parsed")
	}
	if cmd.Scope != "github" || cmd.Name != "pat" || cmd.Backend != "local" {
		t.Fatalf("cmd=%#v", cmd)
	}
}

func TestParseGuardCaptureCommandNaturalKorean(t *testing.T) {
	cmd, ok := parseGuardCaptureCommand("테스트용 토큰을 codex-test natural-token에 로컬로 저장해줘. 다음 메시지에 값만 보낼게.")
	if !ok {
		t.Fatal("command not parsed")
	}
	if cmd.Scope != "codex-test" || cmd.Name != "natural-token" || cmd.Backend != "local" {
		t.Fatalf("cmd=%#v", cmd)
	}
}

func TestGuardSignalSecretTTLDefaultAllowsSignalSyncDelay(t *testing.T) {
	t.Setenv("MESHCLAW_SIGNAL_GUARD_SECRET_TTL_SECONDS", "")
	if got := guardSignalSecretTTL(); got != 5*time.Minute {
		t.Fatalf("ttl=%s", got)
	}
}
