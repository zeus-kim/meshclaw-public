package messenger

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/aichat"
)

const (
	assistantMemoryIntentNone     = "none"
	assistantMemoryIntentCreate   = "create"
	assistantMemoryIntentRecall   = "recall"
	assistantMemoryIntentSnapshot = "snapshot"
	assistantMemoryIntentApprove  = "approve"
	assistantMemoryIntentUpdate   = "update"
	assistantMemoryIntentDelete   = "delete"
)

type assistantMemoryIntent struct {
	Kind         string `json:"intent"`
	Scope        string `json:"scope,omitempty"`
	MemoryText   string `json:"memory_text,omitempty"`
	Target       string `json:"target,omitempty"`
	Category     string `json:"category,omitempty"`
	StableMemory *bool  `json:"is_stable_memory,omitempty"`
	OneOffTask   *bool  `json:"is_one_off_task,omitempty"`
	Confidence   float64
	Reason       string `json:"reason,omitempty"`
	Source       string `json:"-"`
}

func assistantMemoryIntentReply(opts ListenOptions, text string) (string, bool) {
	pendingKey := assistantMemoryPendingKey(opts.TargetID)
	_, hasPending := takePendingAssistantMemory(pendingKey, false)
	intent, ok := classifyAssistantMemoryIntent(text, hasPending)
	if !ok || intent.Kind == assistantMemoryIntentNone {
		return "", false
	}
	switch intent.Kind {
	case assistantMemoryIntentApprove:
		if !hasPending {
			return "", false
		}
		return approvePendingAssistantMemory(opts)
	case assistantMemoryIntentRecall:
		return assistantMemoryRecallReply(), true
	case assistantMemoryIntentSnapshot:
		return assistantMemorySnapshotReply(text), true
	case assistantMemoryIntentCreate:
		return assistantMemoryPlanFromIntent(opts, intent), true
	case assistantMemoryIntentUpdate, assistantMemoryIntentDelete:
		return assistantMemoryMutationIntentReply(intent), true
	default:
		return "", false
	}
}

func classifyAssistantMemoryIntent(text string, hasPendingApproval bool) (assistantMemoryIntent, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return assistantMemoryIntent{}, false
	}
	if intent, ok := classifyAssistantMemoryIntentWithModel(trimmed, hasPendingApproval); ok {
		return intent, true
	}
	return classifyAssistantMemoryIntentFallback(trimmed, hasPendingApproval)
}

func classifyAssistantMemoryIntentWithModel(text string, hasPendingApproval bool) (assistantMemoryIntent, bool) {
	if !assistantMemoryIntentModelEnabled() {
		return assistantMemoryIntent{}, false
	}
	cfg, _, err := aichat.LoadConfig()
	if err != nil {
		cfg = aichat.DefaultConfig()
	}
	cfg = assistantMemoryIntentModelConfig(cfg)
	cfg.SystemPrompt = strings.Join([]string{
		"Classify long-term assistant memory intent. Return JSON only.",
		"Fields: intent, scope, category, is_stable_memory, is_one_off_task, memory_text, target, confidence, reason.",
		"intent: none|create|recall|snapshot|approve|update|delete.",
		"Use create for a stable future user preference, user fact, assistant style, or reusable procedure.",
		"Use recall when the user asks what the assistant remembers. Use snapshot for memory architecture/status.",
		"Use approve only when pending_memory_approval=true and the user confirms the pending write.",
		"Use update/delete only when the user points to an existing saved memory.",
		"Use none for one-off tasks: schedule, reminder, calendar, mail, document, file, shopping, booking, search, report.",
		"If the user gives a new future assistant behavior preference, intent=create scope=assistant category=assistant_style is_stable_memory=true is_one_off_task=false.",
		"If the user asks for a reminder or scheduled news, intent=none category=schedule is_stable_memory=false is_one_off_task=true.",
		`Example user_message=앞으로 답은 결론부터 짧게 해줘 -> {"intent":"create","scope":"assistant","category":"assistant_style","is_stable_memory":true,"is_one_off_task":false,"memory_text":"답변은 결론부터 짧게 한다","confidence":0.92}`,
		`Example user_message=내가 매운 음식을 못 먹는다고 기억해 -> {"intent":"create","scope":"user","category":"user_fact","is_stable_memory":true,"is_one_off_task":false,"memory_text":"사용자는 매운 음식을 못 먹는다","confidence":0.92}`,
		`Example user_message=답변 길이 관련 기억을 더 짧게 수정해 -> {"intent":"update","scope":"assistant","category":"assistant_style","target":"답변 길이 관련 기억","is_stable_memory":true,"is_one_off_task":false,"memory_text":"답변은 더 짧게 한다","confidence":0.88}`,
		`Example user_message=내일 아침 8시에 뉴스 알려줘 -> {"intent":"none","category":"schedule","is_stable_memory":false,"is_one_off_task":true,"confidence":0.93}`,
		`Example user_message=회의 메모 작성해줘 -> {"intent":"none","category":"document","is_stable_memory":false,"is_one_off_task":true,"confidence":0.93}`,
		`Example user_message=메일 확인해줘 -> {"intent":"none","category":"mail","is_stable_memory":false,"is_one_off_task":true,"confidence":0.93}`,
	}, "\n")
	if cfg.MaxTokens <= 0 || cfg.MaxTokens > 256 {
		cfg.MaxTokens = 160
	}
	cfg.Temperature = 0
	payload := map[string]interface{}{
		"pending_memory_approval": hasPendingApproval,
		"user_message":            text,
	}
	data, _ := json.Marshal(payload)
	ctx, cancel := context.WithTimeout(context.Background(), assistantMemoryIntentModelTimeout())
	defer cancel()
	reply, err := aichat.NewClient(cfg).Chat(ctx, nil, string(data))
	if err != nil {
		return assistantMemoryIntent{}, false
	}
	intent, ok := parseAssistantMemoryIntentJSON(reply)
	if !ok || intent.Confidence > 0 && intent.Confidence < 0.55 {
		return assistantMemoryIntent{}, false
	}
	intent.Source = "model"
	return normalizeAssistantMemoryIntent(intent, text, hasPendingApproval)
}

func assistantMemoryIntentModelConfig(cfg aichat.Config) aichat.Config {
	if value := strings.TrimSpace(os.Getenv("MESHCLAW_ASSISTANT_MEMORY_INTENT_BASE_URL")); value != "" {
		cfg.BaseURL = strings.TrimRight(value, "/")
	}
	if value := strings.TrimSpace(os.Getenv("MESHCLAW_ASSISTANT_MEMORY_INTENT_API_KEY")); value != "" {
		cfg.APIKey = value
	}
	if value := strings.TrimSpace(os.Getenv("MESHCLAW_ASSISTANT_MEMORY_INTENT_MODEL_NAME")); value != "" {
		cfg.Model = value
	}
	return cfg
}

func assistantMemoryIntentModelEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("MESHCLAW_ASSISTANT_MEMORY_INTENT_MODEL")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func assistantMemoryIntentModelTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("MESHCLAW_ASSISTANT_MEMORY_INTENT_TIMEOUT_MS"))
	if raw == "" {
		return 1600 * time.Millisecond
	}
	if ms, err := time.ParseDuration(raw + "ms"); err == nil && ms > 0 {
		return ms
	}
	return 1600 * time.Millisecond
}

func parseAssistantMemoryIntentJSON(reply string) (assistantMemoryIntent, bool) {
	reply = strings.TrimSpace(reply)
	if reply == "" {
		return assistantMemoryIntent{}, false
	}
	if before, after, ok := strings.Cut(reply, "```json"); ok {
		_ = before
		if body, _, found := strings.Cut(after, "```"); found {
			reply = strings.TrimSpace(body)
		}
	} else if before, after, ok := strings.Cut(reply, "```"); ok {
		_ = before
		if body, _, found := strings.Cut(after, "```"); found {
			reply = strings.TrimSpace(body)
		}
	}
	start := strings.Index(reply, "{")
	end := strings.LastIndex(reply, "}")
	if start >= 0 && end > start {
		reply = reply[start : end+1]
	}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(reply), &raw); err != nil {
		return assistantMemoryIntent{}, false
	}
	return assistantMemoryIntentFromMap(raw), true
}

func normalizeAssistantMemoryIntent(intent assistantMemoryIntent, original string, hasPendingApproval bool) (assistantMemoryIntent, bool) {
	intent.Kind = normalizeAssistantMemoryIntentKind(intent.Kind)
	if intent.Kind == assistantMemoryIntentUpdate && strings.TrimSpace(intent.Target) == "" && strings.TrimSpace(intent.MemoryText) != "" && !assistantMemoryIntentLooksTaskLike(intent) {
		intent.Kind = assistantMemoryIntentCreate
	}
	switch intent.Kind {
	case assistantMemoryIntentCreate, assistantMemoryIntentRecall, assistantMemoryIntentSnapshot, assistantMemoryIntentApprove, assistantMemoryIntentUpdate, assistantMemoryIntentDelete:
	default:
		return assistantMemoryIntent{}, false
	}
	if intent.Kind == assistantMemoryIntentApprove && !hasPendingApproval {
		return assistantMemoryIntent{}, false
	}
	intent.Scope = strings.ToLower(strings.TrimSpace(intent.Scope))
	if intent.Scope != "user" && intent.Scope != "assistant" {
		intent.Scope = "user"
	}
	intent.Category = strings.ToLower(strings.TrimSpace(intent.Category))
	if intent.Kind == assistantMemoryIntentCreate {
		if assistantMemoryIntentLooksTaskLike(intent) {
			return assistantMemoryIntent{}, false
		}
		if intent.StableMemory != nil && !*intent.StableMemory {
			return assistantMemoryIntent{}, false
		}
		intent.MemoryText = strings.Trim(strings.Join(strings.Fields(intent.MemoryText), " "), " .。")
		if intent.MemoryText == "" {
			intent.MemoryText = strings.Trim(strings.Join(strings.Fields(original), " "), " .。")
		}
		if len([]rune(intent.MemoryText)) < 4 {
			return assistantMemoryIntent{}, false
		}
	}
	return intent, true
}

func assistantMemoryIntentFromMap(raw map[string]interface{}) assistantMemoryIntent {
	return assistantMemoryIntent{
		Kind:         assistantMemoryIntentStringField(raw, "intent"),
		Scope:        assistantMemoryIntentStringField(raw, "scope"),
		MemoryText:   assistantMemoryIntentStringField(raw, "memory_text"),
		Target:       assistantMemoryIntentStringField(raw, "target"),
		Category:     assistantMemoryIntentStringField(raw, "category"),
		StableMemory: assistantMemoryIntentBoolField(raw, "is_stable_memory"),
		OneOffTask:   assistantMemoryIntentBoolField(raw, "is_one_off_task"),
		Confidence:   assistantMemoryIntentFloatField(raw, "confidence"),
		Reason:       assistantMemoryIntentStringField(raw, "reason"),
	}
}

func assistantMemoryIntentStringField(raw map[string]interface{}, key string) string {
	value, ok := raw[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func assistantMemoryIntentFloatField(raw map[string]interface{}, key string) float64 {
	value, ok := raw[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case json.Number:
		f, _ := typed.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return f
	default:
		return 0
	}
}

func assistantMemoryIntentBoolField(raw map[string]interface{}, key string) *bool {
	value, ok := raw[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case bool:
		return &typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "y", "stable", "memory":
			v := true
			return &v
		case "0", "false", "no", "n", "task", "one-off", "one_off":
			v := false
			return &v
		}
	}
	return nil
}

func normalizeAssistantMemoryIntentKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case assistantMemoryIntentCreate, "remember", "save", "store", "add":
		return assistantMemoryIntentCreate
	case assistantMemoryIntentRecall, "show", "list", "read":
		return assistantMemoryIntentRecall
	case assistantMemoryIntentSnapshot, "architecture", "status":
		return assistantMemoryIntentSnapshot
	case assistantMemoryIntentApprove, "confirm", "yes":
		return assistantMemoryIntentApprove
	case assistantMemoryIntentUpdate, "edit", "modify", "change":
		return assistantMemoryIntentUpdate
	case assistantMemoryIntentDelete, "remove", "forget":
		return assistantMemoryIntentDelete
	case assistantMemoryIntentNone, "no", "false":
		return assistantMemoryIntentNone
	default:
		return ""
	}
}

func assistantMemoryIntentLooksTaskLike(intent assistantMemoryIntent) bool {
	if intent.OneOffTask != nil && *intent.OneOffTask {
		return true
	}
	category := strings.ToLower(strings.TrimSpace(intent.Category))
	switch category {
	case "workflow_task", "task", "schedule", "reminder", "calendar", "mail", "email", "document", "file", "shopping", "purchase", "booking", "travel", "research", "report":
		return true
	}
	reason := strings.ToLower(strings.TrimSpace(intent.Reason))
	return containsAny(reason, "not long-term memory", "one-off", "one off", "task", "workflow", "schedule", "reminder", "calendar", "mail", "email", "document", "file", "shopping", "purchase", "booking")
}

func classifyAssistantMemoryIntentFallback(text string, hasPendingApproval bool) (assistantMemoryIntent, bool) {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return assistantMemoryIntent{}, false
	}
	if hasPendingApproval && assistantMemoryFallbackConfirmsPending(lower) {
		return assistantMemoryIntent{Kind: assistantMemoryIntentApprove, Source: "fallback", Confidence: 0.7}, true
	}
	if assistantMemoryFallbackSnapshot(lower) {
		return assistantMemoryIntent{Kind: assistantMemoryIntentSnapshot, Source: "fallback", Confidence: 0.8}, true
	}
	if assistantMemoryFallbackRecall(lower) {
		return assistantMemoryIntent{Kind: assistantMemoryIntentRecall, Source: "fallback", Confidence: 0.8}, true
	}
	if scope, memoryText, ok := parseAssistantMemoryCreateFallback(text); ok {
		return assistantMemoryIntent{Kind: assistantMemoryIntentCreate, Scope: scope, MemoryText: memoryText, Source: "fallback", Confidence: 0.75}, true
	}
	return assistantMemoryIntent{}, false
}

func assistantMemoryFallbackConfirmsPending(lower string) bool {
	switch strings.TrimSpace(lower) {
	case "저장", "저장해", "저장해줘", "기억해", "기억해줘", "응 저장", "ㅇㅇ", "ok", "yes", "approve", "remember it", "save it":
		return true
	default:
		return false
	}
}

func assistantMemoryFallbackRecall(lower string) bool {
	compact := strings.NewReplacer(" ", "", "\t", "", "\n", "", "-", "", ".", "", "!", "", "?", "", "？", "").Replace(lower)
	switch compact {
	case "기억확인", "메모리확인", "장기기억", "장기기억확인", "내메모리", "내메모리보여줘", "메모리테스트", "기억테스트", "너뭐기억해", "너나에대해뭐기억해", "나에대해뭐기억해", "내정보기억":
		return true
	default:
		return containsAny(lower, "what do you remember", "show memory", "memory status", "long-term memory", "long term memory")
	}
}

func assistantMemoryFallbackSnapshot(lower string) bool {
	compact := strings.NewReplacer(" ", "", "\t", "", "\n", "", "-", "", ".", "", "!", "", "?", "", "？", "").Replace(lower)
	switch compact {
	case "메모리구조", "메모리아키텍처", "메모리스냅샷", "memorysnapshot", "memoryarchitecture":
		return true
	default:
		return containsAny(lower, "memory snapshot", "memory architecture", "메모리 구조", "메모리 스냅샷", "메모리 아키텍처")
	}
}

func parseAssistantMemoryCreateFallback(text string) (scope, memoryText string, ok bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", "", false
	}
	lower := strings.ToLower(trimmed)
	hasMemoryVerb := containsAny(lower, "기억해", "기억해줘", "기억해 둬", "기억해둬", "기억해놔", "메모리에 저장", "memory에 저장", "remember this", "remember that", "save to memory")
	if !hasMemoryVerb {
		return "", "", false
	}
	if containsAny(lower, "메모로 저장", "notes에", "note에", "리마인더", "reminder", "캘린더", "calendar", "파일로 저장", "문서로 저장") {
		return "", "", false
	}
	scope = "user"
	if containsAny(lower, "너는", "너의", "argos", "아르고스", "답변은", "브리핑은", "앞으로") {
		scope = "assistant"
	}
	memoryText = trimmed
	replacements := []string{
		"앞으로", "이걸", "이거", "이 내용", "기억해줘", "기억해 둬", "기억해둬", "기억해놔", "기억해",
		"메모리에 저장해줘", "메모리에 저장해", "memory에 저장해줘", "memory에 저장해",
		"remember this", "remember that", "save to memory",
	}
	for _, marker := range replacements {
		memoryText = strings.ReplaceAll(memoryText, marker, " ")
	}
	memoryText = strings.Trim(strings.Join(strings.Fields(memoryText), " "), " .。")
	if len([]rune(memoryText)) < 4 {
		return "", "", false
	}
	return scope, memoryText, true
}
