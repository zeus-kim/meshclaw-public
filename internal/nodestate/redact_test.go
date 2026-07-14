package nodestate

import (
	"strings"
	"testing"
)

func TestRedactTextMasksCommonSecrets(t *testing.T) {
	openAIKey := "sk-" + "abcdefghijklmnopqrstuvwxyz012345"
	githubToken := "ghp_" + "abcdefghijklmnopqrstuvwxyz123456"
	input := strings.Join([]string{
		"Authorization: Bearer abcdefghijklmnopqrstuvwxyz012345",
		"OPENAI_API_KEY=" + openAIKey,
		"github=" + githubToken,
		"jwt=eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjMifQ.signaturevalue",
		"password: dragon1234",
	}, "\n")

	got := RedactText(input)
	for _, leaked := range []string{
		"abcdefghijklmnopqrstuvwxyz012345",
		githubToken,
		"dragon1234",
		"signaturevalue",
	} {
		if strings.Contains(got, leaked) {
			t.Fatalf("secret leaked in redacted text: %q\n%s", leaked, got)
		}
	}
	if !strings.Contains(got, "[REDACTED") {
		t.Fatalf("expected redaction marker in %q", got)
	}
}

func TestParseDisk(t *testing.T) {
	var s SystemState
	parseDisk("Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/disk1 1000000 250000 750000 25% /\n", &s)
	if s.DiskPct < 24 || s.DiskPct > 26 {
		t.Fatalf("unexpected disk pct: %.2f", s.DiskPct)
	}
}
