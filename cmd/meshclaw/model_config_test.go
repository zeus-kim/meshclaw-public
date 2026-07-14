package main

import (
	"strings"
	"testing"
)

func TestReadModelConfigImportMasksStatusSecret(t *testing.T) {
	cfg, err := readModelConfigImport(strings.NewReader(`{
		"base_url": "https://gateway.example/v1",
		"api_key": "sk-test-1234567890",
		"model": "gpt-4.1",
		"max_tokens": 2048,
		"temperature": 0.2
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "https://gateway.example/v1" || cfg.Model != "gpt-4.1" || cfg.APIKey == "" {
		t.Fatalf("cfg=%#v", cfg)
	}
	if got := maskAPIKey(cfg.APIKey); got == cfg.APIKey || !strings.Contains(got, "...") {
		t.Fatalf("secret was not masked: %q", got)
	}
}

func TestReadModelConfigImportRequiresBaseURLAndModel(t *testing.T) {
	if _, err := readModelConfigImport(strings.NewReader(`{"api_key":"secret"}`)); err == nil {
		t.Fatal("expected missing base_url/model error")
	}
}
