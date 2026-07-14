package evidence

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreIndexesEvidenceIntoOpsDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_OPSDB", filepath.Join(home, ".meshclaw", "state"))
	record, err := Store("process-top", "g4", "top processes", map[string]string{"ok": "true"})
	if err != nil {
		t.Fatal(err)
	}
	if record.ID == "" || record.StoredAt == "" {
		t.Fatalf("bad record: %+v", record)
	}
	indexPath := filepath.Join(home, ".meshclaw", "state", "evidence-index.jsonl")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"process-top", "g4", record.ID, "top processes"} {
		if !strings.Contains(text, want) {
			t.Fatalf("index missing %q: %s", want, text)
		}
	}
}
