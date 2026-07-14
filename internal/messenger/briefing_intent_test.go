package messenger

import "testing"

func TestParseBriefingIntentJSON(t *testing.T) {
	intent, ok := parseBriefingIntentJSON(`{"intent":"weather_current","location":"서울","confidence":0.91}`)
	if !ok {
		t.Fatal("intent was not parsed")
	}
	if intent.Intent != "weather_current" || intent.Location != "Seoul" || intent.Confidence != 0.91 {
		t.Fatalf("intent=%#v", intent)
	}
}

func TestParseBriefingIntentJSONExtractsObject(t *testing.T) {
	intent, ok := parseBriefingIntentJSON("```json\n{\"intent\":\"meta_or_conversation\",\"confidence\":0.95}\n```")
	if !ok {
		t.Fatal("intent was not parsed")
	}
	if intent.Intent != "meta_or_conversation" || intent.Confidence != 0.95 {
		t.Fatalf("intent=%#v", intent)
	}
}

func TestParseBriefingIntentJSONRejectsUnknownIntent(t *testing.T) {
	if intent, ok := parseBriefingIntentJSON(`{"intent":"weatherish","confidence":0.9}`); ok {
		t.Fatalf("unexpected intent=%#v", intent)
	}
}

func TestNormalizeBriefingLocation(t *testing.T) {
	cases := map[string]string{
		"":      "Seoul",
		"서울":    "Seoul",
		"hanoi": "Hanoi",
		"방콕":    "Bangkok",
	}
	for input, want := range cases {
		if got := normalizeBriefingLocation(input); got != want {
			t.Fatalf("normalizeBriefingLocation(%q)=%q, want %q", input, got, want)
		}
	}
}
