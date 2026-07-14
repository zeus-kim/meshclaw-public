package prometheus

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	URL  string `yaml:"url" json:"url"`
	Name string `yaml:"name" json:"name"`
}

type Config struct {
	Default string                  `yaml:"default" json:"default"`
	Servers map[string]ServerConfig `yaml:"servers" json:"servers"`
}

type Client struct {
	config *Config
}

type QueryResult struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []interface{}     `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

type MetricValue struct {
	Instance string  `json:"instance"`
	Node     string  `json:"node"`
	Value    float64 `json:"value"`
	Status   string  `json:"status"`
}

type MetricsResponse struct {
	Metric    string        `json:"metric"`
	Server    string        `json:"server"`
	Timestamp time.Time     `json:"timestamp"`
	Nodes     []MetricValue `json:"nodes"`
	Alerts    []string      `json:"alerts,omitempty"`
}

func NewClient() (*Client, error) {
	config, err := loadConfig()
	if err != nil {
		return nil, err
	}
	return &Client{config: config}, nil
}

func NewClientWithConfig(config *Config) *Client {
	return &Client{config: config}
}

func loadConfig() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(home, ".meshclaw", "prometheus.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultConfig(), nil
		}
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func defaultConfig() *Config {
	return &Config{
		Default: "local",
		Servers: map[string]ServerConfig{
			"local": {
				URL:  "http://localhost:9090",
				Name: "Local Prometheus",
			},
		},
	}
}

func (c *Client) GetConfig() *Config {
	return c.config
}

func (c *Client) SaveConfig() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(home, ".meshclaw", "prometheus.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(c.config)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

func (c *Client) getServerURL(serverName string) (string, error) {
	if serverName == "" {
		serverName = c.config.Default
	}

	server, ok := c.config.Servers[serverName]
	if !ok {
		return "", fmt.Errorf("prometheus server '%s' not found in config", serverName)
	}

	return server.URL, nil
}

func (c *Client) Query(query string, serverName string) (*QueryResult, error) {
	serverURL, err := c.getServerURL(serverName)
	if err != nil {
		return nil, err
	}

	reqURL := fmt.Sprintf("%s/api/v1/query?query=%s", serverURL, url.QueryEscape(query))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("prometheus query failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result QueryResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if result.Status != "success" {
		return nil, fmt.Errorf("prometheus query failed: %s", result.Status)
	}

	return &result, nil
}

func (c *Client) parseResults(result *QueryResult, thresholdHigh float64) *MetricsResponse {
	response := &MetricsResponse{
		Timestamp: time.Now(),
		Nodes:     []MetricValue{},
		Alerts:    []string{},
	}

	for _, r := range result.Data.Result {
		instance := r.Metric["instance"]
		node := extractNodeName(instance)

		var value float64
		if len(r.Value) >= 2 {
			switch v := r.Value[1].(type) {
			case string:
				fmt.Sscanf(v, "%f", &value)
			case float64:
				value = v
			}
		}

		status := "ok"
		if thresholdHigh > 0 && value > thresholdHigh {
			status = "high"
			response.Alerts = append(response.Alerts,
				fmt.Sprintf("%s: %.2f > %.0f threshold", node, value, thresholdHigh))
		}

		response.Nodes = append(response.Nodes, MetricValue{
			Instance: instance,
			Node:     node,
			Value:    value,
			Status:   status,
		})
	}

	sort.Slice(response.Nodes, func(i, j int) bool {
		return response.Nodes[i].Node < response.Nodes[j].Node
	})

	return response
}

func extractNodeName(instance string) string {
	if idx := len(instance) - 5; idx > 0 && instance[idx:] == ":9100" {
		instance = instance[:idx]
	}

	nodeMap := map[string]string{
		"192.0.2.10": "node-a",
		"192.0.2.11": "node-b",
		"192.0.2.12": "node-c",
		"localhost":  "local",
	}

	if name, ok := nodeMap[instance]; ok {
		return name
	}
	return instance
}
