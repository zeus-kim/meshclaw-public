package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Ported from metalang-mcp-py (crypto_price, weather, unit conversion, util
// generators) — the handful of generic-utility DSL commands that had no
// meshclaw equivalent when metalang was folded in.

var utilHTTPClient = &http.Client{Timeout: 10 * time.Second}

func utilCryptoPrice(coin string) (map[string]interface{}, error) {
	if coin == "" {
		coin = "bitcoin"
	}
	coin = strings.ToLower(strings.TrimSpace(coin))

	u := fmt.Sprintf("https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=usd,krw&include_24hr_change=true", url.QueryEscape(coin))
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "meshclaw/1.0")

	resp, err := utilHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data map[string]map[string]float64
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	info, ok := data[coin]
	if !ok {
		return nil, fmt.Errorf("unknown coin: %s", coin)
	}

	return map[string]interface{}{
		"coin":       coin,
		"usd":        info["usd"],
		"krw":        info["krw"],
		"change_24h": info["usd_24h_change"],
	}, nil
}

func utilWeather(city string) (map[string]interface{}, error) {
	if city == "" {
		city = "Seoul"
	}

	u := fmt.Sprintf("https://wttr.in/%s?format=j1", url.PathEscape(city))
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "curl/7.0")

	resp, err := utilHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data struct {
		CurrentCondition []struct {
			TempC       string `json:"temp_C"`
			FeelsLikeC  string `json:"FeelsLikeC"`
			Humidity    string `json:"humidity"`
			WeatherDesc []struct {
				Value string `json:"value"`
			} `json:"weatherDesc"`
		} `json:"current_condition"`
		NearestArea []struct {
			AreaName []struct {
				Value string `json:"value"`
			} `json:"areaName"`
		} `json:"nearest_area"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	if len(data.CurrentCondition) == 0 {
		return nil, fmt.Errorf("no weather data for %s", city)
	}

	cur := data.CurrentCondition[0]
	desc := ""
	if len(cur.WeatherDesc) > 0 {
		desc = cur.WeatherDesc[0].Value
	}
	area := city
	if len(data.NearestArea) > 0 && len(data.NearestArea[0].AreaName) > 0 {
		area = data.NearestArea[0].AreaName[0].Value
	}

	return map[string]interface{}{
		"area":         area,
		"temp_c":       cur.TempC,
		"feels_like_c": cur.FeelsLikeC,
		"humidity":     cur.Humidity,
		"description":  desc,
	}, nil
}

var unitConversionFactors = map[string]map[string]float64{
	"km":   {"mile": 0.621371, "m": 1000},
	"mile": {"km": 1.60934},
	"kg":   {"lb": 2.20462},
	"lb":   {"kg": 0.453592},
	"cm":   {"inch": 0.393701},
	"inch": {"cm": 2.54},
	"l":    {"gal": 0.264172},
	"gal":  {"l": 3.78541},
	"g":    {"oz": 0.035274},
	"oz":   {"g": 28.3495},
}

func utilConvert(value float64, from, to string) (map[string]interface{}, error) {
	from = strings.ToLower(strings.TrimSpace(from))
	to = strings.ToLower(strings.TrimSpace(to))

	if (from == "celsius" || from == "c") && (to == "fahrenheit" || to == "f") {
		return map[string]interface{}{"value": value*9/5 + 32, "unit": "fahrenheit"}, nil
	}
	if (from == "fahrenheit" || from == "f") && (to == "celsius" || to == "c") {
		return map[string]interface{}{"value": (value - 32) * 5 / 9, "unit": "celsius"}, nil
	}

	table, ok := unitConversionFactors[from]
	if !ok {
		return nil, fmt.Errorf("unsupported source unit: %s", from)
	}
	factor, ok := table[to]
	if !ok {
		return nil, fmt.Errorf("unsupported conversion: %s -> %s", from, to)
	}
	return map[string]interface{}{"value": value * factor, "unit": to}, nil
}

func utilGenerate(kind, input string, length int) (map[string]interface{}, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "uuid":
		return map[string]interface{}{"uuid": utilNewUUID()}, nil
	case "password":
		if length <= 0 {
			length = 16
		}
		pw, err := utilRandomPassword(length)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"password": pw}, nil
	case "hash":
		sum := sha256.Sum256([]byte(input))
		return map[string]interface{}{"sha256": hex.EncodeToString(sum[:])}, nil
	case "base64_encode":
		return map[string]interface{}{"base64": base64.StdEncoding.EncodeToString([]byte(input))}, nil
	case "base64_decode":
		decoded, err := base64.StdEncoding.DecodeString(input)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"decoded": string(decoded)}, nil
	}
	return nil, fmt.Errorf("unsupported kind: %s (use uuid|password|hash|base64_encode|base64_decode)", kind)
}

func utilNewUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func utilRandomPassword(length int) (string, error) {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*"
	result := make([]byte, length)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		result[i] = charset[n.Int64()]
	}
	return string(result), nil
}
