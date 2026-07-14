package guardvault

import (
	"os"
	"strings"
	"testing"
)

func TestPutListMetadataWithoutRawValue(t *testing.T) {
	t.Setenv("MESHCLAW_GUARD_VAULT_DIR", t.TempDir())
	entry, err := Put(PutOptions{Scope: "cloudflare", Name: "dns-token", Kind: "api-token", Value: []byte("super-secret-value")})
	if err != nil {
		t.Fatal(err)
	}
	if entry.Handle != "vault://meshclaw/cloudflare/dns-token" {
		t.Fatalf("handle=%s", entry.Handle)
	}
	if strings.Contains(entry.Fingerprint, "super-secret") {
		t.Fatalf("fingerprint leaked value: %s", entry.Fingerprint)
	}
	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("list len=%d", len(list))
	}
	meta, err := MetadataByHandle(entry.Handle)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Fingerprint != entry.Fingerprint || !meta.RawAvailable {
		t.Fatalf("metadata mismatch: %+v", meta)
	}
}

func TestDelete(t *testing.T) {
	t.Setenv("MESHCLAW_GUARD_VAULT_DIR", t.TempDir())
	entry, err := Put(PutOptions{Scope: "mail", Name: "app-password", Value: []byte("secret")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Delete("mail", "app-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(entryPath(Root(), entry.Scope, entry.Name)); !os.IsNotExist(err) {
		t.Fatalf("entry still exists or unexpected err: %v", err)
	}
}

func TestUseEnvRedactsOutput(t *testing.T) {
	t.Setenv("MESHCLAW_GUARD_VAULT_DIR", t.TempDir())
	entry, err := Put(PutOptions{Scope: "test", Name: "token", Value: []byte("secret-token")})
	if err != nil {
		t.Fatal(err)
	}
	result, err := UseEnv(entry.Handle, "TOKEN", []string{"sh", "-c", "printf '%s' \"$TOKEN\""})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%s", result.ExitCode, result.Stderr)
	}
	if strings.Contains(result.Stdout, "secret-token") {
		t.Fatalf("stdout leaked secret: %s", result.Stdout)
	}
	if !strings.Contains(result.Stdout, "[REDACTED_SECRET]") {
		t.Fatalf("stdout not redacted: %s", result.Stdout)
	}
	meta, err := MetadataByHandle(entry.Handle)
	if err != nil {
		t.Fatal(err)
	}
	if meta.LastUsedAt == nil {
		t.Fatalf("last used was not updated")
	}
}

func TestResolveEnv(t *testing.T) {
	t.Setenv("MESHCLAW_GUARD_VAULT_DIR", t.TempDir())
	entry, err := Put(PutOptions{Scope: "provider", Name: "token", Kind: "api-token", Value: []byte("secret-token")})
	if err != nil {
		t.Fatal(err)
	}

	env, entries, err := ResolveEnv(map[string]string{"PROVIDER_TOKEN": entry.Handle})
	if err != nil {
		t.Fatal(err)
	}
	if env["PROVIDER_TOKEN"] != "secret-token" {
		t.Fatalf("env did not resolve secret")
	}
	if len(entries) != 1 || entries[0].Handle != entry.Handle {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestResolveEnvRejectsInvalidName(t *testing.T) {
	t.Setenv("MESHCLAW_GUARD_VAULT_DIR", t.TempDir())
	entry, err := Put(PutOptions{Scope: "provider", Name: "token", Kind: "api-token", Value: []byte("secret-token")})
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = ResolveEnv(map[string]string{"1TOKEN": entry.Handle})
	if err == nil {
		t.Fatal("expected invalid env name error")
	}
	if !strings.Contains(err.Error(), "env name") {
		t.Fatalf("error = %v", err)
	}
}

func TestBackendsExposeExternalManagers(t *testing.T) {
	backends := Backends()
	seen := map[string]BackendStatus{}
	for _, backend := range backends {
		seen[backend.ID] = backend
	}
	for _, id := range []string{"local-aes-gcm", "apple-keychain", "pass", "1password", "bitwarden"} {
		if seen[id].ID == "" {
			t.Fatalf("missing backend %s", id)
		}
	}
	if !seen["local-aes-gcm"].Available || !seen["local-aes-gcm"].ReadWrite {
		t.Fatalf("local backend should always be available: %+v", seen["local-aes-gcm"])
	}
}
