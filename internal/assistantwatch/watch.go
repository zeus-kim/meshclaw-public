package assistantwatch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/browserauto"
)

type Watch struct {
	ID              string    `json:"id"`
	Kind            string    `json:"kind"`
	Query           string    `json:"query"`
	URL             string    `json:"url,omitempty"`
	Site            string    `json:"site,omitempty"`
	ThresholdAmount int       `json:"threshold_amount,omitempty"`
	ThresholdText   string    `json:"threshold_text,omitempty"`
	Cadence         string    `json:"cadence"`
	TargetID        string    `json:"target_id,omitempty"`
	Source          string    `json:"source,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	LastCheckedAt   time.Time `json:"last_checked_at,omitempty"`
	Enabled         bool      `json:"enabled"`
	Status          string    `json:"status"`
}

type Store struct {
	Kind    string  `json:"kind"`
	Path    string  `json:"path"`
	Watches []Watch `json:"watches"`
}

type PriceRequest struct {
	Query           string `json:"query"`
	URL             string `json:"url,omitempty"`
	Site            string `json:"site,omitempty"`
	ThresholdAmount int    `json:"threshold_amount,omitempty"`
	ThresholdText   string `json:"threshold_text,omitempty"`
	Cadence         string `json:"cadence"`
}

type CheckResult struct {
	Kind         string             `json:"kind"`
	Now          time.Time          `json:"now"`
	Total        int                `json:"total"`
	Due          []Watch            `json:"due"`
	Links        []string           `json:"links"`
	Observations []PriceObservation `json:"observations,omitempty"`
	Matched      []PriceObservation `json:"matched,omitempty"`
	Next         []string           `json:"next"`
	StorePath    string             `json:"store_path"`
}

type PriceObservation struct {
	WatchID         string    `json:"watch_id"`
	Query           string    `json:"query"`
	URL             string    `json:"url"`
	ThresholdAmount int       `json:"threshold_amount,omitempty"`
	ThresholdText   string    `json:"threshold_text,omitempty"`
	FoundAmount     int       `json:"found_amount,omitempty"`
	FoundText       string    `json:"found_text,omitempty"`
	Matched         bool      `json:"matched"`
	CheckedAt       time.Time `json:"checked_at"`
	Error           string    `json:"error,omitempty"`
}

func ParsePriceRequest(text string) PriceRequest {
	req := PriceRequest{
		Query:         strings.TrimSpace(text),
		Site:          parseSite(text),
		ThresholdText: parseThresholdText(text),
		Cadence:       parseCadence(text),
	}
	req.ThresholdAmount = parseThresholdAmount(req.ThresholdText)
	req.URL = firstURL(text)
	req.Query = cleanPriceQuery(text)
	if req.URL != "" && req.Query == "" {
		req.Query = req.URL
	}
	return req
}

func CreatePriceWatch(req PriceRequest, targetID, source string, now time.Time) (Watch, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		return Watch{}, fmt.Errorf("price watch query is required")
	}
	if req.Cadence == "" {
		req.Cadence = "6h"
	}
	watch := Watch{
		ID:              stableID("price_alert", req.Query, req.URL, req.ThresholdText, targetID),
		Kind:            "price_alert",
		Query:           req.Query,
		URL:             req.URL,
		Site:            req.Site,
		ThresholdAmount: req.ThresholdAmount,
		ThresholdText:   req.ThresholdText,
		Cadence:         req.Cadence,
		TargetID:        strings.TrimSpace(targetID),
		Source:          strings.TrimSpace(source),
		CreatedAt:       now,
		UpdatedAt:       now,
		Enabled:         true,
		Status:          "watching",
	}
	store, err := Load()
	if err != nil {
		return Watch{}, err
	}
	replaced := false
	for i := range store.Watches {
		if store.Watches[i].ID == watch.ID {
			watch.CreatedAt = store.Watches[i].CreatedAt
			watch.LastCheckedAt = store.Watches[i].LastCheckedAt
			store.Watches[i] = watch
			replaced = true
			break
		}
	}
	if !replaced {
		store.Watches = append(store.Watches, watch)
	}
	return watch, Save(store)
}

func ListWatches(kind string, includeDisabled bool) ([]Watch, error) {
	store, err := Load()
	if err != nil {
		return nil, err
	}
	kind = strings.TrimSpace(kind)
	out := []Watch{}
	for _, watch := range store.Watches {
		if kind != "" && watch.Kind != kind {
			continue
		}
		if !includeDisabled && !watch.Enabled {
			continue
		}
		out = append(out, watch)
	}
	return out, nil
}

func DisableWatch(query string, now time.Time) (Watch, bool, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	query = strings.ToLower(strings.TrimSpace(query))
	store, err := Load()
	if err != nil {
		return Watch{}, false, err
	}
	for i := range store.Watches {
		watch := store.Watches[i]
		if !watch.Enabled {
			continue
		}
		if !watchMatches(watch, query) {
			continue
		}
		watch.Enabled = false
		watch.Status = "disabled"
		watch.UpdatedAt = now
		store.Watches[i] = watch
		return watch, true, Save(store)
	}
	return Watch{}, false, nil
}

func Load() (Store, error) {
	path := Path()
	store := Store{Kind: "meshclaw_assistant_watches", Path: path, Watches: []Watch{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return store, err
	}
	if err := json.Unmarshal(data, &store); err != nil {
		return store, err
	}
	store.Kind = "meshclaw_assistant_watches"
	store.Path = path
	if store.Watches == nil {
		store.Watches = []Watch{}
	}
	return store, nil
}

func Save(store Store) error {
	if store.Path == "" {
		store.Path = Path()
	}
	store.Kind = "meshclaw_assistant_watches"
	if store.Watches == nil {
		store.Watches = []Watch{}
	}
	if err := os.MkdirAll(filepath.Dir(store.Path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(store.Path, append(data, '\n'), 0600)
}

func CheckDue(ctx context.Context, now time.Time) (CheckResult, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	store, err := Load()
	if err != nil {
		return CheckResult{}, err
	}
	result := CheckResult{
		Kind:         "meshclaw_assistant_watch_check",
		Now:          now,
		Total:        len(store.Watches),
		Due:          []Watch{},
		Links:        []string{},
		Observations: []PriceObservation{},
		Matched:      []PriceObservation{},
		StorePath:    store.Path,
	}
	changed := false
	for i := range store.Watches {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		watch := store.Watches[i]
		if !watch.Enabled || !isDue(watch, now) {
			continue
		}
		watch.LastCheckedAt = now
		watch.UpdatedAt = now
		store.Watches[i] = watch
		result.Due = append(result.Due, watch)
		link := SearchLink(watch)
		result.Links = append(result.Links, link)
		if watch.Kind == "price_alert" {
			observation := ObservePrice(ctx, watch, now)
			result.Observations = append(result.Observations, observation)
			if observation.Matched {
				result.Matched = append(result.Matched, observation)
			}
		}
		changed = true
	}
	if changed {
		if err := Save(store); err != nil {
			return result, err
		}
	}
	if len(result.Due) == 0 {
		result.Next = append(result.Next, "No enabled assistant watches are due.")
	} else if len(result.Matched) > 0 {
		result.Next = append(result.Next, "Send the matched price alert to the watch target.")
	} else {
		result.Next = append(result.Next, "No watched price crossed its threshold. Keep evidence only unless the user asks for details.")
	}
	return result, nil
}

func ObservePrice(ctx context.Context, watch Watch, now time.Time) PriceObservation {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	link := SearchLink(watch)
	observation := PriceObservation{
		WatchID:         watch.ID,
		Query:           watch.Query,
		URL:             link,
		ThresholdAmount: watch.ThresholdAmount,
		ThresholdText:   watch.ThresholdText,
		CheckedAt:       now,
	}
	page, err := browserauto.Fetch(ctx, browserauto.FetchOptions{URL: link, MaxBody: 24000, Timeout: 15})
	if err != nil {
		observation.Error = err.Error()
		return observation
	}
	if page.Error != "" {
		observation.Error = page.Error
	}
	candidate, ok := lowestPriceCandidate(page.Text)
	if !ok {
		if observation.Error == "" {
			observation.Error = "no visible price found"
		}
		return observation
	}
	observation.FoundAmount = candidate.Amount
	observation.FoundText = candidate.Text
	observation.Matched = watch.ThresholdAmount > 0 && candidate.Amount <= watch.ThresholdAmount
	return observation
}

func SearchLink(w Watch) string {
	if strings.TrimSpace(w.URL) != "" {
		return w.URL
	}
	query := strings.TrimSpace(w.Query)
	if w.Site != "" {
		query += " " + w.Site
	}
	if w.ThresholdText != "" {
		query += " " + w.ThresholdText
	}
	return "https://www.google.com/search?tbm=shop&q=" + url.QueryEscape(strings.TrimSpace(query))
}

func Path() string {
	if path := strings.TrimSpace(os.Getenv("MESHCLAW_ASSISTANT_WATCHES")); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".meshclaw", "assistant-watches.json")
	}
	return filepath.Join(home, ".meshclaw", "assistant-watches.json")
}

func isDue(w Watch, now time.Time) bool {
	if w.LastCheckedAt.IsZero() {
		return true
	}
	d, err := time.ParseDuration(w.Cadence)
	if err != nil || d <= 0 {
		d = 6 * time.Hour
	}
	return now.Sub(w.LastCheckedAt) >= d
}

func watchMatches(watch Watch, query string) bool {
	if query == "" || query == "최근" || query == "마지막" || query == "latest" || query == "last" {
		return true
	}
	lowerID := strings.ToLower(watch.ID)
	lowerQuery := strings.ToLower(watch.Query)
	return strings.Contains(lowerID, query) || strings.Contains(lowerQuery, query)
}

func stableID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "watch-" + hex.EncodeToString(sum[:8])
}

func parseSite(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "쿠팡") || strings.Contains(lower, "coupang"):
		return "쿠팡"
	case strings.Contains(lower, "네이버") || strings.Contains(lower, "naver"):
		return "네이버쇼핑"
	case strings.Contains(lower, "amazon") || strings.Contains(lower, "아마존"):
		return "Amazon"
	default:
		return ""
	}
}

func parseCadence(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "매시간") || strings.Contains(lower, "hourly"):
		return "1h"
	case strings.Contains(lower, "매일") || strings.Contains(lower, "하루") || strings.Contains(lower, "daily"):
		return "24h"
	case strings.Contains(lower, "아침") || strings.Contains(lower, "오전"):
		return "24h"
	default:
		return "6h"
	}
}

func parseThresholdText(text string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`([0-9][0-9,]*)\s*(만원|천원|원|만|천)\s*(아래|이하|미만|under|below)?`),
		regexp.MustCompile(`(?i)(under|below|less than)\s*([$₩]?\s*[0-9][0-9,]*)`),
	}
	for _, pattern := range patterns {
		if match := pattern.FindString(text); strings.TrimSpace(match) != "" {
			return strings.TrimSpace(match)
		}
	}
	return ""
}

func parseThresholdAmount(value string) int {
	match := regexp.MustCompile(`([0-9][0-9,]*)\s*(만원|천원|원|만|천)?`).FindStringSubmatch(value)
	if len(match) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(strings.ReplaceAll(match[1], ",", ""))
	switch {
	case len(match) >= 3 && (match[2] == "만원" || match[2] == "만"):
		return n * 10000
	case len(match) >= 3 && (match[2] == "천원" || match[2] == "천"):
		return n * 1000
	default:
		return n
	}
}

type priceCandidate struct {
	Amount int
	Text   string
}

func lowestPriceCandidate(text string) (priceCandidate, bool) {
	candidates := extractPriceCandidates(text)
	if len(candidates) == 0 {
		return priceCandidate{}, false
	}
	best := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidate.Amount > 0 && candidate.Amount < best.Amount {
			best = candidate
		}
	}
	return best, true
}

func extractPriceCandidates(text string) []priceCandidate {
	seen := map[int]bool{}
	out := []priceCandidate{}
	add := func(amount int, label string) {
		label = strings.TrimSpace(label)
		if amount <= 0 || amount > 100000000 || seen[amount] {
			return
		}
		seen[amount] = true
		out = append(out, priceCandidate{Amount: amount, Text: label})
	}
	for _, match := range regexp.MustCompile(`₩\s*([0-9][0-9,]*)`).FindAllStringSubmatch(text, -1) {
		add(parseCommaInt(match[1]), match[0])
	}
	for _, match := range regexp.MustCompile(`([0-9][0-9,]*)\s*원`).FindAllStringSubmatch(text, -1) {
		add(parseCommaInt(match[1]), match[0])
	}
	for _, match := range regexp.MustCompile(`([0-9]+)\s*만원`).FindAllStringSubmatch(text, -1) {
		add(parseCommaInt(match[1])*10000, match[0])
	}
	for _, match := range regexp.MustCompile(`([0-9]+)\s*만\s*([0-9]+)\s*천\s*원?`).FindAllStringSubmatch(text, -1) {
		add(parseCommaInt(match[1])*10000+parseCommaInt(match[2])*1000, match[0])
	}
	return out
}

func parseCommaInt(value string) int {
	n, _ := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(value), ",", ""))
	return n
}

func cleanPriceQuery(text string) string {
	value := text
	if threshold := parseThresholdText(text); threshold != "" {
		value = strings.ReplaceAll(value, threshold, " ")
	}
	for _, token := range []string{
		"가격 내려가면", "가격 내려", "가격 알림", "알림", "알려줘", "등록해줘", "만들어줘",
		"아래로", "이하로", "미만으로", "되면", "내려가면", "확인해줘", "봐줘",
		"매일", "매시간", "아침", "오전", "기준", "쿠팡", "네이버쇼핑", "네이버", "아마존",
		"price alert", "price drop", "under", "below", "less than", "notify me", "watch",
	} {
		value = strings.ReplaceAll(value, token, " ")
	}
	value = regexp.MustCompile(`https?://[^\s]+`).ReplaceAllString(value, " ")
	out := strings.Trim(strings.Join(strings.Fields(value), " "), " :：,，.。")
	out = strings.TrimSpace(strings.TrimSuffix(out, " 로"))
	return strings.Trim(out, " :：,，.。")
}

func firstURL(text string) string {
	re := regexp.MustCompile(`https?://[^\s<>"']+`)
	return strings.TrimRight(re.FindString(text), ".,)]}>")
}
