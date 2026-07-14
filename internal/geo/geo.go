package geo

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const defaultLocation = "Seoul"

var koreanParticles = map[string]bool{
	"는": true, "은": true, "이": true, "가": true,
	"을": true, "를": true, "에": true, "의": true,
	"와": true, "과": true, "도": true, "만": true,
}

var knownCities = map[string]string{
	"서울": "Seoul", "seoul": "Seoul",
	"부산": "Busan", "busan": "Busan",
	"대구": "Daegu", "daegu": "Daegu",
	"인천": "Incheon", "incheon": "Incheon",
	"광주": "Gwangju", "gwangju": "Gwangju",
	"대전": "Daejeon", "daejeon": "Daejeon",
	"제주": "Jeju", "제주시": "Jeju", "jeju": "Jeju",
	"남양주": "Namyangju", "namyangju": "Namyangju",
	"방콕": "Bangkok", "bangkok": "Bangkok",
	"도쿄": "Tokyo", "tokyo": "Tokyo",
	"베이징": "Beijing", "beijing": "Beijing",
	"하노이": "Hanoi", "hanoi": "Hanoi",
	"뉴욕": "New York", "new york": "New York",
	"파리": "Paris", "paris": "Paris",
}

type Profile struct {
	Default string            `json:"default"`
	Users   map[string]string `json:"users"`
}

type Resolver struct {
	profilePath string
	httpClient  *http.Client
	mu          sync.Mutex
	ipLocation  string
	ipFetched   time.Time
	ipTTL       time.Duration
}

func DefaultResolver() *Resolver {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".meshclaw", "assistant-location.json")
	return &Resolver{
		profilePath: path,
		httpClient:  http.DefaultClient,
		ipTTL:       time.Hour,
	}
}

func NewResolver(profilePath string, client *http.Client) *Resolver {
	if client == nil {
		client = http.DefaultClient
	}
	return &Resolver{profilePath: profilePath, httpClient: client, ipTTL: time.Hour}
}

// Resolve picks a wttr.in location: explicit override, profile by Signal source, IP geo, then Seoul.
func (r *Resolver) Resolve(ctx context.Context, source, explicit string) string {
	if loc := NormalizeLocation(explicit); loc != "" {
		return loc
	}
	if profile := r.loadProfile(); profile != nil {
		if userLoc := NormalizeLocation(profile.Users[normalizeSource(source)]); userLoc != "" {
			return userLoc
		}
		if def := NormalizeLocation(profile.Default); def != "" {
			return def
		}
	}
	if ipLoc := r.cachedIPLocation(ctx); ipLoc != "" {
		return ipLoc
	}
	return defaultLocation
}

// ExtractExplicitLocation finds a city named in a weather query without keyword stripping.
func ExtractExplicitLocation(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if match := regexp.MustCompile(`(?i)\bin\s+(.+)$`).FindStringSubmatch(text); len(match) == 2 {
		place := cleanExplicitFragment(match[1])
		if loc := NormalizeLocation(place); loc != "" {
			return loc
		}
	}
	lower := strings.ToLower(text)
	for key, canonical := range knownCities {
		if strings.Contains(text, key) || strings.Contains(lower, key) {
			return canonical
		}
	}
	// Single-token place name left after trimming punctuation (not particles).
	fragment := cleanExplicitFragment(text)
	if loc := mapKnownCity(fragment); loc != "" {
		return loc
	}
	return ""
}

// NormalizeLocation validates and canonicalizes a location for wttr.in.
func NormalizeLocation(location string) string {
	location = strings.TrimSpace(location)
	location = strings.TrimRight(location, " ?!?。，.:")
	location = strings.TrimSpace(location)
	if location == "" {
		return ""
	}
	if strings.ContainsAny(location, " \t\n\r?") {
		return ""
	}
	if koreanParticles[location] {
		return ""
	}
	if mapped := mapKnownCity(location); mapped != "" {
		return mapped
	}
	if len(location) > 64 {
		return ""
	}
	return location
}

func mapKnownCity(location string) string {
	key := strings.ToLower(strings.TrimSpace(location))
	if canonical, ok := knownCities[key]; ok {
		return canonical
	}
	if canonical, ok := knownCities[location]; ok {
		return canonical
	}
	return ""
}

func cleanExplicitFragment(value string) string {
	value = strings.Trim(value, " :：,，.。?!")
	value = strings.TrimSpace(value)
	for _, particle := range []string{"는", "은", "이", "가", "을", "를", "에", "의", "날씨", "weather", "forecast"} {
		value = strings.TrimSuffix(value, particle)
		value = strings.TrimSpace(value)
	}
	return value
}

func normalizeSource(source string) string {
	source = strings.TrimSpace(source)
	source = strings.TrimPrefix(source, "uuid:")
	return source
}

func (r *Resolver) loadProfile() *Profile {
	data, err := os.ReadFile(r.profilePath)
	if err != nil {
		return nil
	}
	var profile Profile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil
	}
	return &profile
}

func (r *Resolver) cachedIPLocation(ctx context.Context) string {
	r.mu.Lock()
	if r.ipLocation != "" && time.Since(r.ipFetched) < r.ipTTL {
		loc := r.ipLocation
		r.mu.Unlock()
		return loc
	}
	r.mu.Unlock()

	loc := fetchIPLocation(ctx, r.httpClient)
	if loc == "" {
		return ""
	}
	r.mu.Lock()
	r.ipLocation = loc
	r.ipFetched = time.Now()
	r.mu.Unlock()
	return loc
}

func fetchIPLocation(ctx context.Context, client *http.Client) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://ip-api.com/json/?fields=status,city", nil)
	if err != nil {
		return ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return ""
	}
	var decoded struct {
		Status string `json:"status"`
		City   string `json:"city"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil || decoded.Status != "success" {
		return ""
	}
	return NormalizeLocation(decoded.City)
}
