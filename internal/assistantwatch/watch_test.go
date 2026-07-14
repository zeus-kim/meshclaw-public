package assistantwatch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestParsePriceRequestKorean(t *testing.T) {
	req := ParsePriceRequest("러닝 벨트 3만원 아래로 가격 내려가면 알려줘")
	if req.Query != "러닝 벨트" || req.ThresholdText == "" || req.ThresholdAmount != 30000 || req.Cadence != "6h" {
		t.Fatalf("req=%#v", req)
	}
}

func TestCreatePriceWatchAndCheckDue(t *testing.T) {
	t.Setenv("MESHCLAW_ASSISTANT_WATCHES", filepath.Join(t.TempDir(), "watches.json"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><h1>러닝 벨트</h1><p>오늘 특가 29,900원</p></body></html>`)
	}))
	defer server.Close()
	now := time.Date(2026, 5, 25, 1, 0, 0, 0, time.UTC)
	watch, err := CreatePriceWatch(ParsePriceRequest("매일 쿠팡 러닝 벨트 3만원 이하 가격 알림 "+server.URL), "argos-assistant", "test", now)
	if err != nil {
		t.Fatal(err)
	}
	if watch.ID == "" || watch.Site != "쿠팡" || watch.Cadence != "24h" {
		t.Fatalf("watch=%#v", watch)
	}
	result, err := CheckDue(context.Background(), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Due) != 1 || len(result.Links) != 1 || result.Links[0] != server.URL {
		t.Fatalf("result=%#v", result)
	}
	if len(result.Matched) != 1 || result.Matched[0].FoundAmount != 29900 {
		t.Fatalf("matched=%#v observations=%#v", result.Matched, result.Observations)
	}
	result, err = CheckDue(context.Background(), now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Due) != 0 {
		t.Fatalf("watch should not be due again before cadence: %#v", result)
	}
}

func TestExtractPriceCandidates(t *testing.T) {
	tests := map[string]int{
		"특가 29,900원":             29900,
		"sale ₩29,900":           29900,
		"오늘만 3만원":                30000,
		"쿠폰가 2만 9천원":             29000,
		"정가 45,000원 할인가 29,900원": 29900,
	}
	for input, want := range tests {
		got, ok := lowestPriceCandidate(input)
		if !ok || got.Amount != want {
			t.Fatalf("lowestPriceCandidate(%q)=%#v ok=%t want=%d", input, got, ok, want)
		}
	}
}

func TestListAndDisableWatch(t *testing.T) {
	t.Setenv("MESHCLAW_ASSISTANT_WATCHES", filepath.Join(t.TempDir(), "watches.json"))
	now := time.Date(2026, 5, 25, 1, 0, 0, 0, time.UTC)
	watch, err := CreatePriceWatch(ParsePriceRequest("러닝 벨트 3만원 이하 가격 알림"), "argos-assistant", "test", now)
	if err != nil {
		t.Fatal(err)
	}
	watches, err := ListWatches("price_alert", false)
	if err != nil || len(watches) != 1 {
		t.Fatalf("watches=%#v err=%v", watches, err)
	}
	disabled, ok, err := DisableWatch("러닝", now.Add(time.Minute))
	if err != nil || !ok || disabled.ID != watch.ID || disabled.Enabled {
		t.Fatalf("disabled=%#v ok=%t err=%v", disabled, ok, err)
	}
	disabled, ok, err = DisableWatch("러닝", now.Add(2*time.Minute))
	if err != nil || ok {
		t.Fatalf("disabled watch should not match again: disabled=%#v ok=%t err=%v", disabled, ok, err)
	}
	watches, err = ListWatches("price_alert", false)
	if err != nil || len(watches) != 0 {
		t.Fatalf("enabled watches should be empty: %#v err=%v", watches, err)
	}
}
