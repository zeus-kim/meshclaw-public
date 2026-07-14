package assistantcatalog

import "testing"

func TestCatalogHasEnoughCommonAssistantScenarios(t *testing.T) {
	got := Catalog()
	if len(got) < 100 {
		t.Fatalf("Catalog() returned %d scenarios, want at least 100", len(got))
	}
	seen := map[string]bool{}
	for _, scenario := range got {
		if scenario.ID == "" || scenario.Category == "" || scenario.Title == "" || scenario.Example == "" || scenario.Adapter == "" || scenario.Risk == "" {
			t.Fatalf("scenario has missing required fields: %#v", scenario)
		}
		if seen[scenario.ID] {
			t.Fatalf("duplicate scenario id: %s", scenario.ID)
		}
		seen[scenario.ID] = true
	}
	for _, id := range []string{
		"assistant_identity",
		"assistant_capability_menu",
		"voice_morning_brief",
		"voice_news_brief",
		"voice_weather_brief",
		"tts_read_text",
		"restaurant_reserve",
		"place_map_link",
		"music_play",
		"youtube_music_play",
		"radio_play",
		"iphone_audio_handoff",
		"ott_recommend_today",
		"ott_subscription_manage",
		"product_browser_buy",
		"product_payment_pause",
		"mail_send",
		"mail_summary",
		"mail_search",
		"mail_draft_reply",
		"mail_action_items",
		"calendar_create",
		"monitor_price",
		"monitor_news_topic",
		"ppt_meeting_pack",
		"ppt_market_research",
		"ppt_mobile_resend",
		"ops_voice_report",
		"ops_security_digest",
		"approval_purchase",
	} {
		if _, ok := ByID(id); !ok {
			t.Fatalf("missing common scenario %q", id)
		}
	}
}

func TestCatalogIncludesConcretePresentationScenarios(t *testing.T) {
	for _, id := range []string{
		"ppt_meeting_pack",
		"ppt_market_research",
		"ppt_sales_brief",
		"ppt_ops_review",
		"ppt_travel_plan",
		"ppt_mobile_resend",
	} {
		scenario, ok := ByID(id)
		if !ok {
			t.Fatalf("missing presentation scenario %q", id)
		}
		if scenario.Category != "files" {
			t.Fatalf("%s category = %q, want files", id, scenario.Category)
		}
		if scenario.Risk != RiskWrite || !scenario.RequiresApproval {
			t.Fatalf("%s should be an approval-gated file write scenario: %#v", id, scenario)
		}
		if scenario.Example == "" || scenario.Adapter == "" {
			t.Fatalf("%s should have concrete example and adapter: %#v", id, scenario)
		}
	}
}

func TestMailSummaryScenarioUsesDefaultRecentMailExample(t *testing.T) {
	scenario, ok := ByID("mail_summary")
	if !ok {
		t.Fatal("missing mail_summary")
	}
	if scenario.Example != "최근 메일 요약해줘" {
		t.Fatalf("mail_summary example = %q", scenario.Example)
	}
	if scenario.RequiresApproval {
		t.Fatalf("read-only mail summary should not require approval: %#v", scenario)
	}
}

func TestMailDraftReplyScenarioUsesRecentMailFollowupExample(t *testing.T) {
	scenario, ok := ByID("mail_draft_reply")
	if !ok {
		t.Fatal("missing mail_draft_reply")
	}
	if scenario.Example != "첫 번째 메일에 답장 초안 써줘" {
		t.Fatalf("mail_draft_reply example = %q", scenario.Example)
	}
	if scenario.RequiresApproval {
		t.Fatalf("draft-only reply should not require send approval: %#v", scenario)
	}
}

func TestByCategoryReturnsStableCatalogOrder(t *testing.T) {
	got := ByCategory("mail")
	if len(got) < 3 {
		t.Fatalf("ByCategory(mail) returned %d scenarios, want at least 3", len(got))
	}
	wantIDs := []string{"mail_accounts", "mail_summary", "mail_search"}
	for i, wantID := range wantIDs {
		if got[i].ID != wantID {
			t.Fatalf("ByCategory(mail)[%d].ID = %q, want %q", i, got[i].ID, wantID)
		}
	}

	got[0].ID = "mutated"
	scenario, ok := ByID("mail_accounts")
	if !ok {
		t.Fatal("missing mail_accounts")
	}
	if scenario.ID != "mail_accounts" {
		t.Fatalf("ByCategory returned mutable catalog backing data: %#v", scenario)
	}
}

func TestSignalExamplesReturnsRepresentativeExamples(t *testing.T) {
	got := SignalExamples("mail", 3)
	want := []string{
		"연결된 이메일 뭐 있어?",
		"최근 메일 요약해줘",
		"101.band 관련 메일 찾아줘",
	}
	if len(got) != len(want) {
		t.Fatalf("SignalExamples(mail, 3) length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SignalExamples(mail, 3)[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if got := SignalExamples("mail", 0); len(got) != 0 {
		t.Fatalf("SignalExamples(mail, 0) = %#v, want empty", got)
	}
}

func TestEveryCategoryHasSignalExamples(t *testing.T) {
	categories := map[string]bool{}
	for _, scenario := range Catalog() {
		categories[scenario.Category] = true
	}
	for category := range categories {
		examples := SignalExamples(category, 3)
		if len(examples) == 0 {
			t.Fatalf("category %q has no signal examples", category)
		}
		for _, example := range examples {
			if example == "" {
				t.Fatalf("category %q returned empty signal example: %#v", category, examples)
			}
		}
	}
}

func TestRiskyScenariosRequireApproval(t *testing.T) {
	allowedStatus := map[string]bool{
		"implemented": true,
		"partial":     true,
		"planned":     true,
	}
	for _, scenario := range Catalog() {
		if !allowedStatus[scenario.Status] {
			t.Fatalf("scenario has unsupported status: %#v", scenario)
		}
		switch scenario.Risk {
		case RiskWrite:
			if !scenario.RequiresApproval {
				t.Fatalf("write scenario must require approval: %#v", scenario)
			}
		case RiskExternal:
			if !scenario.RequiresApproval {
				t.Fatalf("external scenario must require approval: %#v", scenario)
			}
		case RiskMoney:
			if !scenario.RequiresApproval {
				t.Fatalf("money scenario must require approval: %#v", scenario)
			}
		}
		if isDestructiveScenario(scenario) && !scenario.RequiresApproval {
			t.Fatalf("destructive scenario must require approval: %#v", scenario)
		}
	}
}

func isDestructiveScenario(scenario Scenario) bool {
	switch scenario.ID {
	case "reminder_delete", "calendar_delete", "mail_delete", "restaurant_cancel", "ott_cancel_before_confirm", "archive_cleanup":
		return true
	default:
		return false
	}
}
