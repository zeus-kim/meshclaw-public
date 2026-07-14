package aichat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	BaseURL      string  `json:"base_url"`
	APIKey       string  `json:"api_key"`
	Model        string  `json:"model"`
	SystemPrompt string  `json:"system_prompt"`
	MaxTokens    int     `json:"max_tokens"`
	Temperature  float64 `json:"temperature"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type ChatResult struct {
	Content   string
	ToolCalls []ToolCall
}

type Client struct {
	config     Config
	httpClient *http.Client
}

func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".meshclaw", "matrix_ai.json"), nil
}

func DefaultConfig() Config {
	return Config{
		BaseURL:     "http://localhost:11434/v1",
		APIKey:      "ollama",
		Model:       "gemma4:e4b",
		MaxTokens:   4096,
		Temperature: 0.45,
		SystemPrompt: strings.Join([]string{
			"너는 Matrix 운영방의 대화 모델 @ops-ai다.",
			"편하게 대화하되 운영자에게 필요한 밀도로 답한다. 이모지, 감성적인 위로, 장황한 자기소개, 마케팅식 표현은 쓰지 않는다.",
			"MeshClaw 자체인 척하지 말고 MeshClaw 도구봇 @dev와 역할을 구분한다.",
			"너의 역할은 사용자의 생각을 정리하고 서버 운영, 개발 방향, 정책, 워크스페이스 문제를 자연어로 풀어주는 것이다.",
			"실제 서버 명령 실행, 권한 변경, 삭제, 배포, 구매, 외부 API 호출은 직접 했다고 말하지 말고 @dev/MeshClaw 도구 확인 또는 승인이 필요하다고 말한다.",
			"실제 상태, 정책, 워크스페이스, evidence, fleet 조회는 추측하지 않는다. 그런 요청은 MeshClaw 실제 조회 결과를 기준으로 설명한다.",
			"한국어로 답한다. 사용자가 길게 설명해달라고 하면 충분히 길게 답하되, 항목을 나누고 다음 행동을 명확히 제안한다.",
			"답변은 보통 '판단 -> 근거 -> 다음 행동' 순서로 쓴다.",
		}, "\n"),
	}
}

func LoadConfig() (Config, string, error) {
	if path := strings.TrimSpace(os.Getenv("MESHCLAW_MATRIX_AI_CONFIG")); path != "" {
		cfg, err := readConfig(path)
		return cfg, path, err
	}
	path, err := DefaultConfigPath()
	if err != nil {
		return Config{}, "", err
	}
	cfg, err := readConfig(path)
	return cfg, path, err
}

func readConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	cfg = fillDefaults(cfg)
	return cfg, nil
}

func SaveConfig(path string, cfg Config) error {
	if path == "" {
		var err error
		path, err = DefaultConfigPath()
		if err != nil {
			return err
		}
	}
	cfg = fillDefaults(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func fillDefaults(cfg Config) Config {
	defaults := DefaultConfig()
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaults.BaseURL
	}
	if cfg.APIKey == "" {
		cfg.APIKey = defaults.APIKey
	}
	if cfg.Model == "" {
		cfg.Model = defaults.Model
	}
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = defaults.SystemPrompt
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = defaults.MaxTokens
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = defaults.Temperature
	}
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	return cfg
}

func NewClient(cfg Config) *Client {
	return &Client{config: fillDefaults(cfg), httpClient: &http.Client{Timeout: 180 * time.Second}}
}

func (c *Client) CompleteWithTools(ctx context.Context, messages []Message, tools []Tool) (ChatResult, error) {
	return c.chatWithToolsOnce(ctx, messages, tools, c.config.MaxTokens)
}

func (c *Client) ChatWithTools(ctx context.Context, history []Message, input string, tools []Tool) (ChatResult, error) {
	messages := []Message{{Role: "system", Content: c.config.SystemPrompt}}
	messages = append(messages, history...)
	messages = append(messages, Message{Role: "user", Content: input})
	return c.chatWithToolsOnce(ctx, messages, tools, c.config.MaxTokens)
}

func (c *Client) Chat(ctx context.Context, history []Message, input string) (string, error) {
	messages := []Message{{Role: "system", Content: c.config.SystemPrompt}}
	messages = append(messages, history...)
	messages = append(messages, Message{Role: "user", Content: input})
	reply, _, reasoningOnly, err := c.chatOnce(ctx, messages, c.config.MaxTokens, nil)
	if err != nil {
		return "", err
	}
	if reply != "" {
		return reply, nil
	}
	if reasoningOnly {
		retryMessages := append([]Message{}, messages...)
		retryMessages = append(retryMessages, Message{
			Role:    "user",
			Content: "위 답변은 최종 응답이 비어 있었습니다. reasoning을 출력하지 말고, 사용자에게 보낼 최종 답변만 한국어로 작성하세요.",
		})
		retryTokens := c.config.MaxTokens * 2
		if retryTokens < 2048 {
			retryTokens = 2048
		}
		if retryTokens > 8192 {
			retryTokens = 8192
		}
		reply, _, reasoningOnly, err = c.chatOnce(ctx, retryMessages, retryTokens, nil)
		if err != nil {
			return "", err
		}
		if reply != "" {
			return reply, nil
		}
		if reasoningOnly {
			return "", fmt.Errorf("chat backend returned reasoning but no final answer after retry")
		}
	}
	return "", nil
}

func (c *Client) chatWithToolsOnce(ctx context.Context, messages []Message, tools []Tool, maxTokens int) (ChatResult, error) {
	reply, toolCalls, reasoningOnly, err := c.chatOnce(ctx, messages, maxTokens, tools)
	if err != nil {
		return ChatResult{}, err
	}
	if reply != "" || len(toolCalls) > 0 {
		return ChatResult{Content: reply, ToolCalls: toolCalls}, nil
	}
	if reasoningOnly {
		retryMessages := append([]Message{}, messages...)
		retryMessages = append(retryMessages, Message{
			Role:    "user",
			Content: "위 답변은 최종 응답이 비어 있었습니다. reasoning을 출력하지 말고, 사용자에게 보낼 최종 답변만 한국어로 작성하세요.",
		})
		retryTokens := maxTokens * 2
		if retryTokens < 2048 {
			retryTokens = 2048
		}
		if retryTokens > 8192 {
			retryTokens = 8192
		}
		reply, toolCalls, reasoningOnly, err = c.chatOnce(ctx, retryMessages, retryTokens, tools)
		if err != nil {
			return ChatResult{}, err
		}
		if reply != "" || len(toolCalls) > 0 {
			return ChatResult{Content: reply, ToolCalls: toolCalls}, nil
		}
		if reasoningOnly {
			return ChatResult{}, fmt.Errorf("chat backend returned reasoning but no final answer after retry")
		}
	}
	return ChatResult{}, nil
}

func (c *Client) chatOnce(ctx context.Context, messages []Message, maxTokens int, tools []Tool) (reply string, toolCalls []ToolCall, reasoningOnly bool, err error) {
	payload := map[string]interface{}{
		"model":       c.config.Model,
		"messages":    messages,
		"stream":      false,
		"max_tokens":  maxTokens,
		"temperature": c.config.Temperature,
	}
	if len(tools) > 0 {
		payload["tools"] = tools
	}
	if strings.Contains(strings.ToLower(c.config.Model), "gpt-oss") {
		payload["reasoning_effort"] = "low"
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", nil, false, err
	}
	endpoint := c.config.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", nil, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", nil, false, err
	}
	defer resp.Body.Close()
	var decoded struct {
		Choices []struct {
			Message struct {
				Role      string     `json:"role"`
				Content   string     `json:"content"`
				ToolCalls []ToolCall `json:"tool_calls,omitempty"`
				Reasoning string     `json:"reasoning,omitempty"`
				Thinking  string     `json:"thinking,omitempty"`
			} `json:"message"`
		} `json:"choices"`
		Error interface{} `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", nil, false, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, false, fmt.Errorf("chat backend returned %s: %v", resp.Status, decoded.Error)
	}
	if len(decoded.Choices) == 0 {
		return "", nil, false, fmt.Errorf("chat backend returned no choices")
	}
	msg := decoded.Choices[0].Message
	reply = CleanResponse(msg.Content)
	toolCalls = msg.ToolCalls
	if reply == "" && len(toolCalls) == 0 && (msg.Reasoning != "" || msg.Thinking != "") {
		return "", nil, true, nil
	}
	return reply, toolCalls, false, nil
}

func CleanResponse(text string) string {
	text = strings.Map(func(r rune) rune {
		switch {
		case r >= 0x1F300 && r <= 0x1FAFF:
			return -1
		case r >= 0x2600 && r <= 0x27BF:
			return -1
		case r == 0xFE0F:
			return -1
		default:
			return r
		}
	}, text)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
