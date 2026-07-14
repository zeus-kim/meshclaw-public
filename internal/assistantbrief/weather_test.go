package assistantbrief

import "testing"

func TestNormalizeWeatherLocation(t *testing.T) {
	tests := map[string]string{
		"":          "Seoul",
		"   ":       "Seoul",
		"서울 날씨는 ?":  "Seoul",
		"서울 는 ?":    "Seoul",
		"는":         "Seoul",
		"Busan":     "Busan",
		"Tokyo?":    "Tokyo",
		"  Paris  ": "Paris",
		"New York":  "Seoul",
		"서울?":       "서울",
	}
	for input, want := range tests {
		if got := normalizeWeatherLocation(input); got != want {
			t.Fatalf("normalizeWeatherLocation(%q)=%q want %q", input, got, want)
		}
	}
}
