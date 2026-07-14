package main

import (
	"strings"
	"testing"

	"github.com/meshclaw/meshclaw/internal/mailadapter"
)

func TestMCPMailToolDescriptionsSeparateReadDraftSend(t *testing.T) {
	tools := map[string]mcpTool{}
	for _, tool := range mcpTools() {
		tools[tool.Name] = tool
	}
	checks := map[string][]string{
		"meshclaw_mail_search":      {"READ-ONLY", "summaries"},
		"meshclaw_mail_summarize":   {"READ-ONLY", "summary"},
		"meshclaw_mail_thread":      {"READ-ONLY", "body"},
		"meshclaw_mail_draft_reply": {"DRAFT-ONLY", "never transmits"},
		"meshclaw_mail_compose":     {"DRAFT-ONLY", "never transmits"},
		"meshclaw_mail_send":        {"SEND EMAIL", "approve=true"},
		"meshclaw_mail_delete":      {"DESTRUCTIVE", "approve=true"},
	}
	for name, wants := range checks {
		tool, ok := tools[name]
		if !ok {
			t.Fatalf("missing %s", name)
		}
		for _, want := range wants {
			if !strings.Contains(tool.Description, want) {
				t.Fatalf("%s description missing %q: %s", name, want, tool.Description)
			}
		}
	}
}

func TestSummarizeMailSearchFallback(t *testing.T) {
	summary := summarizeMailSearch(mailSearchResultForTest(), "Cloudflare")
	for _, want := range []string{"2 mail matching Cloudflare item", "Domain removed", "Billing notice"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q: %s", want, summary)
		}
	}
}

func TestSummarizeMailMultiSearchFallback(t *testing.T) {
	result := mcpMailMultiSearchResult{
		Results: []mailadapter.SearchResult{
			{Account: mailadapter.AccountPublic{ID: "personal"}, Messages: []mailadapter.MessageSummary{
				{Subject: "Domain removed", From: "Cloudflare"},
				{Subject: "Billing notice", From: "Cloudflare"},
			}},
			{Account: mailadapter.AccountPublic{ID: "work"}, Messages: []mailadapter.MessageSummary{
				{Subject: "Weekly report", From: "Boss"},
			}},
		},
		Errors: []mcpMailAccountSearchError{{Account: "archive", Error: "login failed"}},
	}
	summary := summarizeMailMultiSearch(result, "")
	for _, want := range []string{"3 recent mail item(s) across 2 account(s), 1 error(s)", "[personal] 2 item", "[work] Weekly report"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q: %s", want, summary)
		}
	}
}

func mailSearchResultForTest() mailadapter.SearchResult {
	return mailadapter.SearchResult{Messages: []mailadapter.MessageSummary{
		{Subject: "Domain removed", From: "Cloudflare"},
		{Subject: "Billing notice", From: "Cloudflare"},
	}}
}
