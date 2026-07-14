package geo

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizeLocationRejectsParticles(t *testing.T) {
	for _, input := range []string{"", "는", "은", "서울 는 ?", "New York"} {
		got := NormalizeLocation(input)
		switch input {
		case "서울 는 ?":
			if got != "" {
				t.Fatalf("NormalizeLocation(%q)=%q want empty", input, got)
			}
		case "New York":
			if got != "" {
				t.Fatalf("NormalizeLocation(%q)=%q want empty", input, got)
			}
		default:
			if got != "" {
				t.Fatalf("NormalizeLocation(%q)=%q want empty", input, got)
			}
		}
	}
	if got := NormalizeLocation("Busan"); got != "Busan" {
		t.Fatalf("got=%q", got)
	}
	if got := NormalizeLocation("서울"); got != "Seoul" {
		t.Fatalf("got=%q", got)
	}
}

func TestExtractExplicitLocation(t *testing.T) {
	tests := map[string]string{
		"오늘 서울 날씨 알려줘":                "Seoul",
		"부산 비 와?":                     "Busan",
		"weather in Tokyo":            "Tokyo",
		"오늘 뭐 입고 나가":                  "",
		"what should I wear in Paris": "Paris",
		"오늘 날씨는?":                     "",
		"날씨 알려줘":                      "",
	}
	for input, want := range tests {
		if got := ExtractExplicitLocation(input); got != want {
			t.Fatalf("ExtractExplicitLocation(%q)=%q want %q", input, got, want)
		}
	}
}

func TestResolverUsesProfileBySource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "assistant-location.json")
	if err := os.WriteFile(path, []byte(`{"default":"Seoul","users":{"+821086215273":"Busan"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	r := NewResolver(path, http.DefaultClient)
	got := r.Resolve(context.Background(), "+821086215273", "")
	if got != "Busan" {
		t.Fatalf("got=%q want Busan", got)
	}
	got = r.Resolve(context.Background(), "+821086215273", "Tokyo")
	if got != "Tokyo" {
		t.Fatalf("explicit override got=%q", got)
	}
}

func TestResolverUsesCachedIPWhenNoProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")
	r := NewResolver(path, http.DefaultClient)
	r.mu.Lock()
	r.ipLocation = "Incheon"
	r.ipFetched = time.Now()
	r.mu.Unlock()

	got := r.Resolve(context.Background(), "+999", "")
	if got != "Incheon" {
		t.Fatalf("got=%q want Incheon", got)
	}
}
