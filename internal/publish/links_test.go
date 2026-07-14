package publish

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizePublicBase(t *testing.T) {
	got := SanitizePublicBase("Argos 뉴스 문서 2026/05/31.md")
	if got != "argos-2026-05-31-md" {
		t.Fatalf("SanitizePublicBase = %q", got)
	}
}

func TestTokenizedPublicName(t *testing.T) {
	got := TokenizedPublicName("/tmp/Argos 뉴스.md", []byte("hello"))
	if got == "" {
		t.Fatal("empty tokenized name")
	}
	if got[:6] != "argos-" || got[len(got)-3:] != ".md" {
		t.Fatalf("TokenizedPublicName = %q, want argos-* .md", got)
	}
}

func TestWritePublicFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path, ok := WritePublicFile("../report.html", []byte("<html>ok</html>"))
	if !ok {
		t.Fatal("WritePublicFile returned false")
	}
	want := filepath.Join(home, ".meshclaw", "public", "argos", "report.html")
	if path != want {
		t.Fatalf("path=%q, want %q", path, want)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("written file missing: %v", err)
	}
}

func TestLinksForPublishedUsesPublicBaseURLBeforePortFallback(t *testing.T) {
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_BASE_URL", "https://argos.example.test/argos")
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_HOST", "argos.example.test")

	links := LinksForPublished("/tmp/report deck.pptx")
	if len(links) == 0 {
		t.Fatal("no links")
	}
	if links[0] != "https://argos.example.test/argos/report%20deck.pptx" {
		t.Fatalf("first link=%q", links[0])
	}
	if !containsString(links, "http://argos.example.test:48303/argos/report%20deck.pptx") {
		t.Fatalf("expected port fallback in links: %#v", links)
	}
}

func TestPublicBaseURLsDerivesFromDashboardURL(t *testing.T) {
	t.Setenv("MESHCLAW_ARGOS_DASHBOARD_URL", "https://argos.zeus.kim/argos/dashboard.html")
	got := PublicBaseURLs()
	if len(got) != 1 || got[0] != "https://argos.zeus.kim/argos" {
		t.Fatalf("PublicBaseURLs=%#v", got)
	}
}

func TestPublicBaseURLsSpecialCasesArgosHost(t *testing.T) {
	t.Setenv("MESHCLAW_ARGOS_PUBLIC_HOST", "argos.zeus.kim")
	got := PublicBaseURLs()
	if len(got) == 0 || got[0] != "https://argos.zeus.kim/argos" {
		t.Fatalf("PublicBaseURLs=%#v", got)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}
