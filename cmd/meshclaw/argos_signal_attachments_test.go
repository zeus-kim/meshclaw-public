package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meshclaw/meshclaw/internal/osauto"
)

func TestArgosSignalAttachmentsIncludesMarkdownReportNotHTML(t *testing.T) {
	home := t.TempDir()
	doc := filepath.Join(home, "Documents", "Argos Vault", "Work Reports", "argos-search.md")
	if err := os.MkdirAll(filepath.Dir(doc), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(doc, []byte("# report"), 0600); err != nil {
		t.Fatal(err)
	}
	html := doc[:len(doc)-len(filepath.Ext(doc))] + ".html"
	if err := os.WriteFile(html, []byte("<html>report</html>"), 0600); err != nil {
		t.Fatal(err)
	}

	attachments := argosSignalAttachments(osauto.ArgosAction{
		Action:     "visible_browser_search",
		OutputPath: doc,
	})

	if len(attachments) != 1 || attachments[0] != doc {
		t.Fatalf("attachments=%#v", attachments)
	}
}
