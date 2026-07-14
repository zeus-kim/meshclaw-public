package matrixops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Config struct {
	Homeserver     string   `json:"homeserver"`
	UserID         string   `json:"user_id"`
	AccessToken    string   `json:"access_token"`
	RoomID         string   `json:"room_id"`
	CommandPrefix  string   `json:"command_prefix,omitempty"`
	AmbientChat    bool     `json:"ambient_chat,omitempty"`
	IgnoreCommands bool     `json:"ignore_commands,omitempty"`
	IgnoredUsers   []string `json:"ignored_users,omitempty"`
}

type Client struct {
	config     Config
	httpClient *http.Client
}

type DispatchFunc func(source, message string) (map[string]interface{}, error)
type ReplyFunc func(message string) (string, error)

type SyncResponse struct {
	NextBatch string `json:"next_batch"`
	Rooms     struct {
		Join map[string]JoinedRoom `json:"join"`
	} `json:"rooms"`
}

type JoinedRoom struct {
	Timeline struct {
		Events []Event `json:"events"`
	} `json:"timeline"`
}

type Event struct {
	Type    string  `json:"type"`
	Sender  string  `json:"sender"`
	EventID string  `json:"event_id"`
	Content Content `json:"content"`
}

type Content struct {
	MsgType string `json:"msgtype"`
	Body    string `json:"body"`
}

type SyncResult struct {
	NextBatch string   `json:"next_batch"`
	Handled   []string `json:"handled"`
	Ignored   int      `json:"ignored"`
}

func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".meshclaw", "matrix.json"), nil
}

func LoadConfig() (Config, string, error) {
	if path := strings.TrimSpace(os.Getenv("MESHCLAW_MATRIX_CONFIG")); path != "" {
		cfg, err := readConfig(path)
		return cfg, path, err
	}
	path, err := DefaultConfigPath()
	if err != nil {
		return Config{}, "", err
	}
	if _, err := os.Stat(path); err == nil {
		cfg, err := readConfig(path)
		return cfg, path, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, "", err
	}
	fallback := filepath.Join(home, ".argos", "matrix.json")
	cfg, err := readConfig(fallback)
	return cfg, fallback, err
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
	cfg.Homeserver = strings.TrimRight(strings.TrimSpace(cfg.Homeserver), "/")
	cfg.UserID = strings.TrimSpace(cfg.UserID)
	cfg.AccessToken = strings.TrimSpace(cfg.AccessToken)
	cfg.RoomID = strings.TrimSpace(cfg.RoomID)
	if cfg.CommandPrefix == "" {
		cfg.CommandPrefix = "!"
	}
	if cfg.Homeserver == "" || cfg.AccessToken == "" || cfg.RoomID == "" {
		return Config{}, fmt.Errorf("matrix config requires homeserver, access_token, and room_id")
	}
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
	if cfg.CommandPrefix == "" {
		cfg.CommandPrefix = "!"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func NewClient(cfg Config) *Client {
	return &Client{config: cfg, httpClient: &http.Client{Timeout: 35 * time.Second}}
}

func (c *Client) SendText(ctx context.Context, body string) error {
	txnID := fmt.Sprintf("meshclaw-%d", time.Now().UnixNano())
	endpoint := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s",
		c.config.Homeserver,
		url.PathEscape(c.config.RoomID),
		url.PathEscape(txnID),
	)
	payload := Content{MsgType: "m.text", Body: body}
	return c.doJSON(ctx, http.MethodPut, endpoint, payload, nil)
}

func (c *Client) Sync(ctx context.Context, since string, timeout time.Duration) (SyncResponse, error) {
	endpoint, err := url.Parse(c.config.Homeserver + "/_matrix/client/v3/sync")
	if err != nil {
		return SyncResponse{}, err
	}
	query := endpoint.Query()
	if since != "" {
		query.Set("since", since)
	}
	query.Set("timeout", fmt.Sprintf("%d", timeout.Milliseconds()))
	endpoint.RawQuery = query.Encode()

	var response SyncResponse
	if err := c.doJSON(ctx, http.MethodGet, endpoint.String(), nil, &response); err != nil {
		return SyncResponse{}, err
	}
	return response, nil
}

func (c *Client) SyncOnce(ctx context.Context, since string, dispatch DispatchFunc) (SyncResult, error) {
	response, err := c.Sync(ctx, since, 2*time.Second)
	if err != nil {
		return SyncResult{}, err
	}
	result := SyncResult{NextBatch: response.NextBatch}
	room, ok := response.Rooms.Join[c.config.RoomID]
	if !ok {
		return result, nil
	}
	for _, event := range room.Timeline.Events {
		if !shouldHandle(c.config, event) {
			result.Ignored++
			continue
		}
		reply, err := HandleMessage(c.config, event.Content.Body, dispatch)
		if err != nil {
			reply = "MeshClaw error: " + err.Error()
		}
		reply = ClientSafeMessage(reply)
		for _, chunk := range SplitMessage(reply, matrixChunkLimit()) {
			if err := c.SendText(ctx, chunk); err != nil {
				return result, err
			}
		}
		result.Handled = append(result.Handled, event.EventID)
	}
	return result, nil
}

func (c *Client) SyncOnceReply(ctx context.Context, since string, reply ReplyFunc) (SyncResult, error) {
	response, err := c.Sync(ctx, since, 2*time.Second)
	if err != nil {
		return SyncResult{}, err
	}
	result := SyncResult{NextBatch: response.NextBatch}
	room, ok := response.Rooms.Join[c.config.RoomID]
	if !ok {
		return result, nil
	}
	for _, event := range room.Timeline.Events {
		if !shouldHandle(c.config, event) {
			result.Ignored++
			continue
		}
		text, err := reply(event.Content.Body)
		if err != nil {
			text = "AI bridge error: " + err.Error()
		}
		text = ClientSafeMessage(text)
		for _, chunk := range SplitMessage(text, matrixChunkLimit()) {
			if err := c.SendText(ctx, chunk); err != nil {
				return result, err
			}
		}
		result.Handled = append(result.Handled, event.EventID)
	}
	return result, nil
}

func (c *Client) Bridge(ctx context.Context, dispatch DispatchFunc) error {
	prime, err := c.Sync(ctx, "", 1*time.Second)
	if err != nil {
		return err
	}
	since := prime.NextBatch
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		result, err := c.SyncOnce(ctx, since, dispatch)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		if result.NextBatch != "" {
			since = result.NextBatch
		}
	}
}

func (c *Client) BridgeReply(ctx context.Context, reply ReplyFunc) error {
	prime, err := c.Sync(ctx, "", 1*time.Second)
	if err != nil {
		return err
	}
	since := prime.NextBatch
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		result, err := c.SyncOnceReply(ctx, since, reply)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		if result.NextBatch != "" {
			since = result.NextBatch
		}
	}
}

func HandleMessage(cfg Config, message string, dispatch DispatchFunc) (string, error) {
	message = normalizeCommand(cfg, message)
	result, err := dispatch("matrix", message)
	if err != nil {
		return "", err
	}
	return FormatResult(result), nil
}

func shouldHandle(cfg Config, event Event) bool {
	if event.Type != "m.room.message" || event.Content.MsgType != "m.text" {
		return false
	}
	if cfg.UserID != "" && event.Sender == cfg.UserID {
		return false
	}
	for _, ignored := range cfg.IgnoredUsers {
		if strings.TrimSpace(ignored) == event.Sender {
			return false
		}
	}
	body := strings.TrimSpace(event.Content.Body)
	if body == "" {
		return false
	}
	prefix := cfg.CommandPrefix
	if prefix == "" {
		prefix = "!"
	}
	lower := strings.ToLower(body)
	if cfg.AmbientChat {
		if cfg.IgnoreCommands && strings.HasPrefix(body, prefix) {
			return false
		}
		return true
	}
	return strings.HasPrefix(body, prefix) ||
		strings.HasPrefix(lower, "meshclaw ") ||
		strings.Contains(lower, "meshclaw") ||
		isIntroBody(lower)
}

func SplitMessage(text string, max int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return []string{"(empty response)"}
	}
	if max <= 0 || len(text) <= max {
		return []string{text}
	}
	var chunks []string
	for len(text) > max {
		cut := strings.LastIndex(text[:max], "\n")
		if cut < max/2 {
			cut = strings.LastIndex(text[:max], " ")
		}
		if cut < max/2 {
			cut = max
		}
		chunks = append(chunks, strings.TrimSpace(text[:cut]))
		text = strings.TrimSpace(text[cut:])
	}
	if text != "" {
		chunks = append(chunks, text)
	}
	return chunks
}

func ClientSafeMessage(text string) string {
	text = strings.TrimSpace(text)
	limit := matrixMessageLimit()
	if limit <= 0 || len(text) <= limit {
		return text
	}
	cut := strings.LastIndex(text[:limit], "\n")
	if cut < limit/2 {
		cut = strings.LastIndex(text[:limit], " ")
	}
	if cut < limit/2 {
		cut = limit
	}
	head := strings.TrimSpace(text[:cut])
	return head + "\n\n...Trimmed for Matrix client sync. Read the full payload from MeshClaw evidence or the CLI. Matrix 클라이언트 로딩 보호를 위해 줄였습니다."
}

func matrixMessageLimit() int {
	return envInt("MESHCLAW_MATRIX_MESSAGE_LIMIT", 2800)
}

func matrixChunkLimit() int {
	limit := envInt("MESHCLAW_MATRIX_CHUNK_LIMIT", 2800)
	if limit <= 0 {
		return 2800
	}
	return limit
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	var out int
	if _, err := fmt.Sscanf(value, "%d", &out); err != nil || out <= 0 {
		return fallback
	}
	return out
}

func normalizeCommand(cfg Config, message string) string {
	message = strings.TrimSpace(message)
	prefix := cfg.CommandPrefix
	if prefix == "" {
		prefix = "!"
	}
	if strings.HasPrefix(message, prefix) {
		return message
	}
	lower := strings.ToLower(message)
	if strings.HasPrefix(lower, "meshclaw ") {
		return strings.TrimSpace(message[len("meshclaw "):])
	}
	return message
}

func isIntroBody(lower string) bool {
	lower = strings.TrimSpace(lower)
	switch lower {
	case "hi", "hello", "hey", "안녕", "안녕하세요", "넌 누구지?", "너 누구야?", "누구야?", "who are you?", "대화는 불가능한거야?", "대화 가능해?", "뭐 할 수 있어?", "사용법":
		return true
	default:
		return strings.Contains(lower, "대화 가능") ||
			strings.Contains(lower, "대화는 불가능") ||
			strings.Contains(lower, "뭐 할 수") ||
			strings.Contains(lower, "무엇을 할 수") ||
			strings.Contains(lower, "사용법") ||
			strings.Contains(lower, "도움말")
	}
}

func FormatResult(result map[string]interface{}) string {
	route, _ := result["route"].(string)
	switch route {
	case "intro":
		payload := asMap(result["result"])
		commands := asSlice(payload["commands"])
		lines := []string{
			fmt.Sprint(payload["name"]),
			fmt.Sprint(payload["role"]),
			fmt.Sprint(payload["conversation"]),
			"",
			"Commands:",
		}
		for _, command := range commands {
			lines = append(lines, "- "+fmt.Sprint(command))
		}
		return strings.Join(lines, "\n")
	case "chat":
		payload := asMap(result["result"])
		lines := []string{fmt.Sprint(payload["reply"])}
		try := asSlice(payload["try"])
		if len(try) > 0 {
			lines = append(lines, "", "Try / 이렇게 말해도 돼:")
			for _, item := range try {
				lines = append(lines, "- "+fmt.Sprint(item))
			}
		}
		return strings.Join(lines, "\n")
	case "workers":
		items := asMapSlice(result["result"])
		if len(items) == 0 {
			return formatJSONBlock(result, 6000)
		}
		var lines []string
		lines = append(lines, "MeshClaw workers")
		for _, m := range items {
			id, _ := m["id"].(string)
			status, _ := m["status"].(string)
			surface, _ := m["surface"].(string)
			lines = append(lines, fmt.Sprintf("- %s: %s (%s)", id, status, surface))
		}
		return strings.Join(lines, "\n")
	case "workspace_list":
		return formatWorkspaceList(result)
	case "workspace_add":
		return "Workspace registered.\n" + formatJSONBlock(result["result"], 2500)
	case "monitor_check":
		return formatMonitor(result)
	case "policy_summary":
		return "MeshClaw policy\n" + formatJSONBlock(result["result"], 2500)
	case "matrix_doctor":
		return explainMatrixDoctor(result)
	case "evidence_list":
		return "Recent evidence\n" + formatJSONBlock(result["result"], 4500)
	default:
		return formatJSONBlock(result, 6000)
	}
}

func FormatExplainedResult(result map[string]interface{}) string {
	return FormatExplainedResultForQuery(result, "")
}

func FormatExplainedResultForQuery(result map[string]interface{}, query string) string {
	route, _ := result["route"].(string)
	switch route {
	case "workspace_list":
		return explainWorkspaceList(result)
	case "ops_brief":
		return explainOpsBrief(result)
	case "ops_control":
		return explainOpsControl(result)
	case "node_inventory":
		return explainNodeInventory(result)
	case "fleet_inventory":
		return explainFleetInventory(result)
	case "placement_plan":
		return explainPlacementPlan(result)
	case "monitor_check":
		return explainMonitor(result, query)
	case "process_top":
		return explainWorkflowReport(result, "MeshClaw 실제 프로세스 상위 목록 조회 결과입니다.")
	case "service_check":
		return explainWorkflowReport(result, "MeshClaw 실제 서비스 상태 조회 결과입니다.")
	case "service_audit":
		return explainWorkflowReport(result, "MeshClaw 실제 서비스 장애 감사 결과입니다.")
	case "fleet_service_audit":
		return explainFleetServiceAudit(result)
	case "service_triage":
		return explainServiceTriage(result)
	case "analyze_logs":
		return explainWorkflowReport(result, "MeshClaw 실제 로그 분석 결과입니다.")
	case "security_check":
		return explainWorkflowReport(result, "MeshClaw 실제 보안 점검 결과입니다.")
	case "hygiene_scan_host":
		return explainHygieneReport(result)
	case "matrix_doctor":
		return explainMatrixDoctor(result)
	case "policy_summary":
		return "MeshClaw 실제 정책 요약입니다.\n\n" + FormatResult(result) + "\n\n해석:\n- 이 값은 현재 정책 파일을 요약한 것입니다.\n- 실제 허용/승인/차단 판단은 개별 action/resource 기준으로 policy-check를 해야 정확합니다.\n\n다음 행동:\n- 특정 작업 권한을 확인하려면 `policy-check <subject> <action> <resource>` 형태로 확인하는 것이 맞습니다."
	case "evidence_list":
		return "MeshClaw 실제 evidence 목록입니다.\n\n" + FormatResult(result) + "\n\n해석:\n- 최근 실행, 조회, 스캔 결과가 evidence로 남습니다.\n- 운영 판단은 말보다 evidence 기준으로 추적하는 것이 안전합니다."
	default:
		return "MeshClaw 실제 조회 결과입니다.\n\n" + FormatResult(result)
	}
}

func explainWorkflowReport(result map[string]interface{}, title string) string {
	payload := asMap(result["result"])
	report := asMap(payload["report"])
	findings := asMapSlice(report["findings"])
	status := strings.TrimSpace(fmt.Sprint(report["status"]))
	host := strings.TrimSpace(fmt.Sprint(report["host"]))
	lines := []string{
		naturalTitle(title, host, status),
		"",
		"상태:",
		"- " + naturalWorkflowStatus(report),
	}
	if len(findings) > 0 {
		lines = append(lines, "", "판단:")
		for i, finding := range findings {
			if i >= 3 {
				lines = append(lines, fmt.Sprintf("- 나머지 %d개 항목은 evidence에 저장했습니다.", len(findings)-i))
				break
			}
			lines = append(lines, "- "+naturalFinding(finding))
			if evidence := strings.TrimSpace(fmt.Sprint(finding["evidence"])); evidence != "" && evidence != "<nil>" {
				lines = append(lines, "  근거:\n"+indentBlock(truncateText(evidence, 1400), "  "))
			}
			if next := strings.TrimSpace(fmt.Sprint(finding["next"])); next != "" && next != "<nil>" {
				lines = append(lines, "  다음: "+next)
			}
		}
	}
	lines = append(lines,
		"",
		"해야 할 일:",
		"- 먼저 위의 다음 명령으로 범위를 좁힙니다.",
		"- 서비스 재시작, 삭제, quarantine은 원인이 확인된 뒤에만 진행합니다.",
	)
	return strings.Join(lines, "\n")
}

func explainOpsBrief(result map[string]interface{}) string {
	payload := asMap(result["result"])
	report := asMap(payload["report"])
	monitorReport := asMap(report["monitor"])
	states := asMap(monitorReport["states"])
	alerts := asSlice(monitorReport["alerts"])
	serviceAudit := asMap(report["service_audit"])
	topRisks := stringSlice(report["top_risks"])
	nextActions := stringSlice(report["next_actions"])
	serviceFindings, _ := numberValue(serviceAudit["findings"])

	lines := []string{
		"현재 운영 상태를 요약하면, 서버들은 대부분 살아 있지만 정리해야 할 서비스 장애 후보가 남아 있습니다.",
		"",
		"상태:",
		fmt.Sprintf("- 서버: %d개 확인", len(states)),
		fmt.Sprintf("- 리소스 알림: %d개", len(alerts)),
		fmt.Sprintf("- 서비스 장애 후보: %.0f개", serviceFindings),
	}
	if len(topRisks) == 0 {
		lines = append(lines, "- 지금 즉시 보이는 큰 위험은 없습니다.")
	} else {
		lines = append(lines, "", "우선순위:")
		for _, risk := range topRisks {
			lines = append(lines, "- "+naturalRisk(risk))
		}
		if int(serviceFindings) > len(topRisks) {
			lines = append(lines, fmt.Sprintf("- 그 밖의 후보 %.0f개는 evidence에 저장했습니다. 먼저 위 항목부터 보면 됩니다.", serviceFindings-float64(len(topRisks))))
		}
	}
	if len(nextActions) > 0 {
		lines = append(lines, "", "해야 할 일:")
		for i, action := range nextActions {
			lines = append(lines, fmt.Sprintf("%d. %s", i+1, naturalAction(action)))
		}
	}
	lines = append(lines,
		"",
		"판단:",
		"- 지금은 대규모 장애 대응보다 실패 서비스 후보를 하나씩 확인해서 오래된 잔재와 실제 장애를 분리하는 단계입니다.",
		"- 자동 조치는 ExecStart 대상이 사라진 서비스처럼 원인이 명확한 경우에만 적용하는 것이 맞습니다.",
	)
	return strings.Join(lines, "\n")
}

func explainOpsControl(result map[string]interface{}) string {
	payload := asMap(result["result"])
	report := asMap(payload["report"])
	summary := asMap(report["management_summary"])
	topRisks := stringSlice(report["top_risks"])
	nextActions := stringSlice(report["next_actions"])
	plan := asMapSlice(report["autoheal_plan"])
	triage := asMap(report["service_triage"])
	triageItems := asMapSlice(triage["items"])
	policyPosture := asMapSlice(report["policy_posture"])
	applied := asMapSlice(report["applied_safe"])
	lines := []string{
		"서버 운영판단 레이어 기준으로 전체를 확인했습니다.",
		"",
		"상태:",
		fmt.Sprintf("- 노드: 전체 %.0f / 온라인 %.0f / 오프라인 %.0f", numberOrZero(summary["nodes_total"]), numberOrZero(summary["nodes_online"]), numberOrZero(summary["nodes_offline"])),
		fmt.Sprintf("- 리소스 알림: %.0f개", numberOrZero(summary["resource_alerts"])),
		fmt.Sprintf("- 서비스 확인 후보: %.0f개", numberOrZero(summary["service_findings"])),
		fmt.Sprintf("- 서비스 triage: %.0f개 / 실제 장애 후보 %.0f개", numberOrZero(summary["service_triage_items"]), numberOrZero(summary["real_incidents"])),
		fmt.Sprintf("- 자동 안전조치 후보: %.0f개", numberOrZero(summary["auto_safe_candidates"])),
		fmt.Sprintf("- 승인 필요 후보: %.0f개", numberOrZero(summary["approval_required"])),
		fmt.Sprintf("- 정책 판단: allow %.0f / approval %.0f / deny %.0f", numberOrZero(summary["policy_allows"]), numberOrZero(summary["policy_approval"]), numberOrZero(summary["policy_denies"])),
	}
	if len(topRisks) > 0 {
		lines = append(lines, "", "우선순위:")
		for _, risk := range topRisks {
			lines = append(lines, "- "+naturalRisk(risk))
		}
	} else {
		lines = append(lines, "", "우선순위:", "- 지금 즉시 보이는 큰 위험은 없습니다.")
	}
	if len(plan) > 0 {
		lines = append(lines, "", "조치 후보:")
		for _, action := range firstNMaps(plan, 5) {
			lines = append(lines, fmt.Sprintf("- %v %v: %v %.1f%%, mode=%v", action["node"], action["type"], action["metric"], numberOrZero(action["value"]), action["mode"]))
			if command := strings.TrimSpace(fmt.Sprint(action["command"])); command != "" && command != "<nil>" {
				lines = append(lines, "  명령: `"+command+"`")
			}
			if reason := strings.TrimSpace(fmt.Sprint(action["reason"])); reason != "" && reason != "<nil>" {
				lines = append(lines, "  이유: "+naturalReason(reason))
			}
		}
		if len(plan) > 5 {
			lines = append(lines, fmt.Sprintf("- 나머지 %d개 후보는 evidence에 저장했습니다.", len(plan)-5))
		}
	}
	if len(triageItems) > 0 {
		lines = append(lines, "", "서비스 판단:")
		for _, item := range firstNMaps(triageItems, 5) {
			lines = append(lines, fmt.Sprintf("- %v/%v: %v (%v)", item["host"], item["service"], item["class"], item["mode"]))
			if judgement := strings.TrimSpace(fmt.Sprint(item["judgement"])); judgement != "" && judgement != "<nil>" {
				lines = append(lines, "  판단: "+judgement)
			}
		}
	}
	if len(policyPosture) > 0 {
		lines = append(lines, "", "권한/보안 정책:")
		for _, item := range firstNMaps(policyPosture, 6) {
			lines = append(lines, fmt.Sprintf("- %v %v/%v: %v", item["subject"], item["action"], item["resource"], item["decision"]))
		}
	}
	if len(applied) > 0 {
		lines = append(lines, "", "실행한 안전조치:")
		for _, action := range firstNMaps(applied, 5) {
			lines = append(lines, fmt.Sprintf("- %v %v success=%v result=%v", action["node"], action["type"], action["success"], action["result"]))
		}
	}
	if len(nextActions) > 0 {
		lines = append(lines, "", "다음 행동:")
		for i, action := range nextActions {
			lines = append(lines, fmt.Sprintf("%d. %s", i+1, naturalAction(action)))
		}
	}
	lines = append(lines,
		"",
		"판단:",
		"- 이 명령은 상태관리, 권한관리, 보안관리, 서버관리, 정책관리를 묶은 서버 운영판단 레이어입니다.",
		"- 기본 모드는 read-only입니다. 실제 자동조치는 `meshclaw ops-control --apply-safe` 또는 `meshclaw autoheal-apply-safe`처럼 명시했을 때만 수행합니다.",
		"- 위험 조치, secret 접근, 비용 발생, 서비스 변경은 정책 판단과 evidence를 거쳐야 합니다.",
	)
	return strings.Join(lines, "\n")
}

func explainNodeInventory(result map[string]interface{}) string {
	payload := asMap(result["result"])
	report := asMap(payload["report"])
	host := strings.TrimSpace(fmt.Sprint(report["host"]))
	tools := asStringMap(report["tools"])
	services := stringSlice(report["services"])
	containers := stringSlice(report["containers"])
	gpus := stringSlice(report["gpu"])
	lines := []string{
		fmt.Sprintf("%s에 설치된 주요 구성은 이렇습니다.", host),
		"",
		"상태:",
		fmt.Sprintf("- OS: %v", report["os"]),
		fmt.Sprintf("- Kernel/Arch: %v / %v", report["kernel"], report["arch"]),
		fmt.Sprintf("- 확인된 도구: %d개", len(tools)),
		fmt.Sprintf("- 실행 중 서비스: %d개", len(services)),
		fmt.Sprintf("- 실행 중 컨테이너: %d개", len(containers)),
	}
	if len(gpus) > 0 {
		lines = append(lines, fmt.Sprintf("- GPU: %s", strings.Join(firstNStrings(gpus, 3), ", ")))
	}
	lines = append(lines, "", "주요 도구:")
	for _, name := range preferredToolOrder(tools) {
		lines = append(lines, fmt.Sprintf("- %s: %s", name, shortenVersion(tools[name])))
	}
	if len(containers) > 0 {
		lines = append(lines, "", "컨테이너:")
		for _, item := range firstNStrings(containers, 8) {
			lines = append(lines, "- "+item)
		}
	}
	lines = append(lines,
		"",
		"판단:",
		"- 이 정보는 노드의 역할을 판단하기 위한 read-only software facts입니다.",
		"- 서비스 전체 목록과 원문은 evidence에 저장했고, 여기에는 운영 판단에 필요한 핵심만 줄였습니다.",
	)
	return strings.Join(lines, "\n")
}

func explainFleetInventory(result map[string]interface{}) string {
	payload := asMap(result["result"])
	report := asMap(payload["report"])
	hosts := asMapSlice(report["hosts"])
	failures, _ := numberValue(report["failures"])
	lines := []string{
		"노드별 설치 상태를 훑어봤습니다.",
		"",
		"상태:",
		fmt.Sprintf("- 확인한 노드: %d개", len(hosts)),
		fmt.Sprintf("- 조회 실패: %.0f개", failures),
		"",
		"노드별 요약:",
	}
	for _, host := range hosts {
		hostName := strings.TrimSpace(fmt.Sprint(host["host"]))
		status := strings.TrimSpace(fmt.Sprint(host["status"]))
		osName := strings.TrimSpace(fmt.Sprint(host["os"]))
		tools := asStringMap(host["tools"])
		containers := stringSlice(host["containers"])
		gpus := stringSlice(host["gpu"])
		parts := []string{fmt.Sprintf("%s: %s", hostName, status)}
		if osName != "" && osName != "<nil>" {
			parts = append(parts, osName)
		}
		if toolSummary := shortToolSummary(tools); toolSummary != "" {
			parts = append(parts, toolSummary)
		}
		if len(containers) > 0 {
			parts = append(parts, fmt.Sprintf("containers=%d", len(containers)))
		}
		if len(gpus) > 0 {
			parts = append(parts, fmt.Sprintf("gpu=%d", len(gpus)))
		}
		lines = append(lines, "- "+strings.Join(parts, " | "))
	}
	lines = append(lines,
		"",
		"판단:",
		"- 이제 각 노드가 어떤 역할을 할 수 있는지 모델이 facts로 판단할 수 있습니다.",
		"- 다음 개선은 이 facts를 capability/vault/policy와 연결해서 '어느 노드에서 어떤 작업을 맡길지' 자동 추천하는 것입니다.",
	)
	return strings.Join(lines, "\n")
}

func explainPlacementPlan(result map[string]interface{}) string {
	payload := asMap(result["result"])
	report := asMap(payload["report"])
	workload := strings.TrimSpace(fmt.Sprint(report["workload"]))
	className := strings.TrimSpace(fmt.Sprint(report["class"]))
	candidates := asMapSlice(report["candidates"])
	rejected := asMapSlice(report["rejected"])
	lines := []string{
		fmt.Sprintf("이 작업은 `%s` 유형으로 보고 배치 후보를 골랐습니다.", className),
		"",
		"요청:",
		"- " + workload,
	}
	if len(candidates) == 0 {
		lines = append(lines,
			"",
			"판단:",
			"- 지금 facts 기준으로 바로 추천할 노드가 없습니다.",
			"- 제외 사유를 보고 도구 설치, 리소스 정리, 노드 추가 중 하나를 먼저 해야 합니다.",
		)
	} else {
		lines = append(lines, "", "추천:")
		for i, candidate := range candidates {
			host := strings.TrimSpace(fmt.Sprint(candidate["host"]))
			score := strings.TrimSpace(fmt.Sprint(candidate["score"]))
			lines = append(lines, fmt.Sprintf("%d. %s (score %s)", i+1, host, score))
			for _, reason := range stringSlice(candidate["reasons"]) {
				lines = append(lines, "   - "+naturalPlacementReason(reason))
			}
			if actions := stringSlice(candidate["actions"]); len(actions) > 0 {
				lines = append(lines, "   - 확인: "+strings.Join(wrapCommands(actions), ", "))
			}
		}
		lines = append(lines,
			"",
			"판단:",
			"- 1순위 노드에서 먼저 read-only 확인을 하고, 문제가 없으면 그 노드를 작업 위치로 쓰는 것이 맞습니다.",
			"- 배포나 장기 실행 전에 service-audit 결과를 확인해야 합니다.",
		)
	}
	if len(rejected) > 0 {
		lines = append(lines, "", "제외된 주요 사유:")
		for _, item := range firstNMaps(rejected, 5) {
			lines = append(lines, fmt.Sprintf("- %v: %v", item["host"], item["reason"]))
		}
	}
	return strings.Join(lines, "\n")
}

func explainFleetServiceAudit(result map[string]interface{}) string {
	payload := asMap(result["result"])
	report := asMap(payload["report"])
	hosts := asMapSlice(report["hosts"])
	findings, _ := numberValue(report["findings"])
	lines := []string{
		"전체 서버의 서비스 장애 후보를 확인했습니다.",
		"",
		"상태:",
		fmt.Sprintf("- 확인한 서버: %d개", len(hosts)),
		fmt.Sprintf("- 조치 후보: %.0f개", findings),
	}
	if findings == 0 {
		lines = append(lines, "- 실패하거나 재시작 중인 서비스 후보는 없습니다.")
	}
	issueLines := []string{}
	for _, host := range hosts {
		status := strings.TrimSpace(fmt.Sprint(host["status"]))
		if status == "ok" {
			continue
		}
		hostName := strings.TrimSpace(fmt.Sprint(host["host"]))
		findings := asMapSlice(host["findings"])
		if len(findings) > 0 {
			finding := findings[0]
			line := fmt.Sprintf("- %s: %s", hostName, naturalFinding(finding))
			if next := strings.TrimSpace(fmt.Sprint(finding["next"])); next != "" && next != "<nil>" {
				line += "\n  다음: " + next
			}
			issueLines = append(issueLines, line)
		}
	}
	if len(issueLines) > 0 {
		lines = append(lines, "", "우선순위:")
		lines = append(lines, issueLines...)
	}
	lines = append(lines,
		"",
		"판단:",
		"- 이 결과는 Linux 노드 전체에서 실패/재시작 서비스 후보를 read-only로 훑은 것입니다.",
		"- 오래된 잔재 서비스와 실제 장애가 섞여 있을 수 있으므로, 바로 재시작하지 말고 서비스별 확인이 먼저입니다.",
		"",
		"해야 할 일:",
		"- 위에 나온 `service-check` 명령으로 원인을 확인합니다.",
		"- ExecStart 대상이 없을 때만 `service-quarantine <host> <service>`를 적용합니다.",
	)
	return strings.Join(lines, "\n")
}

func explainServiceTriage(result map[string]interface{}) string {
	payload := asMap(result["result"])
	report := asMap(payload["report"])
	items := asMapSlice(report["items"])
	counts := asMap(report["counts"])
	lines := []string{
		"서비스 장애 후보를 triage 했습니다.",
		"",
		"상태:",
		fmt.Sprintf("- 점검 항목: %d개", len(items)),
		fmt.Sprintf("- 실제 장애 후보: %.0f개", numberOrZero(counts["real_incident"])),
		fmt.Sprintf("- 잔재/부팅 후보: %.0f개", numberOrZero(counts["stale_or_boot_only"])+numberOrZero(counts["stale_or_missing_target"])),
		fmt.Sprintf("- 무시 후보: %.0f개", numberOrZero(counts["ignore_candidate"])),
		fmt.Sprintf("- 승인 필요: %.0f개", numberOrZero(counts["approval_required"])),
	}
	if len(items) > 0 {
		lines = append(lines, "", "분류:")
		for _, item := range firstNMaps(items, 8) {
			host := strings.TrimSpace(fmt.Sprint(item["host"]))
			service := strings.TrimSpace(fmt.Sprint(item["service"]))
			className := strings.TrimSpace(fmt.Sprint(item["class"]))
			mode := strings.TrimSpace(fmt.Sprint(item["mode"]))
			lines = append(lines, fmt.Sprintf("- %s/%s: %s (%s)", host, service, className, mode))
			if judgement := strings.TrimSpace(fmt.Sprint(item["judgement"])); judgement != "" && judgement != "<nil>" {
				lines = append(lines, "  판단: "+judgement)
			}
			if next := strings.TrimSpace(fmt.Sprint(item["next"])); next != "" && next != "<nil>" {
				lines = append(lines, "  다음: `"+next+"`")
			}
		}
		if len(items) > 8 {
			lines = append(lines, fmt.Sprintf("- 나머지 %d개 항목은 evidence에 저장했습니다.", len(items)-8))
		}
	}
	lines = append(lines,
		"",
		"운영 기준:",
		"- `ignore_candidate`는 바로 조치하지 않고 다음 주기에서 반복 여부를 봅니다.",
		"- `stale_or_missing_target`은 quarantine 후보지만 자동 실행하지 않습니다.",
		"- `real_incident`는 재시작 전 service-check/log evidence를 먼저 봐야 합니다.",
	)
	return strings.Join(lines, "\n")
}

func explainHygieneReport(result map[string]interface{}) string {
	payload := asMap(result["result"])
	report := asMap(payload["report"])
	findings := asMapSlice(report["findings"])
	actions := asMapSlice(report["actions"])
	lines := []string{
		"MeshClaw 실제 민감정보/시크릿 위생 스캔 결과입니다.",
		"",
		fmt.Sprintf("host=%v status=%v findings=%d actions=%d", report["host"], report["status"], len(findings), len(actions)),
	}
	if len(findings) > 0 {
		lines = append(lines, "", "findings:")
		limit := minInt(len(findings), 20)
		for _, finding := range findings[:limit] {
			line := fmt.Sprintf("- [%v] %v target=%v", finding["severity"], finding["type"], finding["target"])
			if evidence := strings.TrimSpace(fmt.Sprint(finding["evidence"])); evidence != "" && evidence != "<nil>" {
				line += " evidence=" + evidence
			}
			lines = append(lines, line)
		}
		if len(findings) > limit {
			lines = append(lines, fmt.Sprintf("- ... %d more findings stored in evidence", len(findings)-limit))
		}
	}
	if len(actions) > 0 {
		lines = append(lines, "", "safe actions:")
		limit := minInt(len(actions), 20)
		for _, action := range actions[:limit] {
			lines = append(lines, fmt.Sprintf("- [%v] %v target=%v", action["mode"], action["id"], action["target"]))
		}
		if len(actions) > limit {
			lines = append(lines, fmt.Sprintf("- ... %d more actions stored in evidence", len(actions)-limit))
		}
	}
	lines = append(lines,
		"",
		"해석:",
		"- 이 스캔은 원문 값을 노출하지 않고 redacted evidence 중심으로 보여주는 운영 위생 점검입니다.",
		"- 실제 삭제/수정은 별도 승인 또는 safe action 정책을 거쳐야 합니다.",
		"",
		"다음 행동:",
		"- findings가 있으면 대상 파일과 로그 보존 정책을 확인합니다.",
		"- 자동 정리가 가능한 항목만 autoheal-safe 또는 명시 승인된 cleanup으로 넘깁니다.",
	)
	return strings.Join(lines, "\n")
}

func explainMatrixDoctor(result map[string]interface{}) string {
	report := asMap(result["result"])
	reachable := asMap(report["server_reachable"])
	ok := fmt.Sprint(reachable["ok"]) == "true"
	privateTailnet := fmt.Sprint(report["private_tailscale"]) == "true"
	endpoints := asMap(report["client_endpoints"])
	httpsOK := fmt.Sprint(asMap(endpoints["https_default"])["ok"]) == "true"
	http8008OK := fmt.Sprint(asMap(endpoints["http_8008"])["ok"]) == "true"
	configuredOK := fmt.Sprint(asMap(endpoints["configured"])["ok"]) == "true"
	status := strings.TrimSpace(fmt.Sprint(report["status"]))
	if status == "" || status == "<nil>" {
		status = "unknown"
	}
	lines := []string{
		"Matrix 클라이언트 접속 상태를 확인했습니다.",
		"",
		"상태:",
		fmt.Sprintf("- homeserver: %v", report["homeserver"]),
		fmt.Sprintf("- room: %v", report["room_id"]),
		fmt.Sprintf("- alias: %v", report["room_alias"]),
		fmt.Sprintf("- server reachable: %v", ok),
	}
	if privateTailnet {
		lines = append(lines, "- 접속 경로: Tailscale 사설망")
	} else {
		lines = append(lines, "- 접속 경로: 공개 또는 로컬 주소")
	}
	if errText := strings.TrimSpace(fmt.Sprint(reachable["error"])); errText != "" && errText != "<nil>" {
		lines = append(lines, "- error: "+truncateText(errText, 220))
	}
	if len(endpoints) > 0 {
		lines = append(lines,
			"",
			"클라이언트 접속 경로:",
			fmt.Sprintf("- configured homeserver: %v", configuredOK),
			fmt.Sprintf("- https default 443: %v", httpsOK),
			fmt.Sprintf("- http 8008: %v", http8008OK),
		)
		if errText := strings.TrimSpace(fmt.Sprint(asMap(endpoints["https_default"])["error"])); errText != "" && errText != "<nil>" {
			lines = append(lines, "- https error: "+truncateText(errText, 160))
		}
	}
	if rooms := asMapSlice(report["rooms"]); len(rooms) > 0 {
		lines = append(lines, "", "방 상태:")
		for _, room := range rooms {
			label := strings.TrimSpace(fmt.Sprint(room["label"]))
			name := strings.TrimSpace(fmt.Sprint(room["name"]))
			if name == "" || name == "<nil>" {
				name = strings.TrimSpace(fmt.Sprint(room["room_id"]))
			}
			counts := asMap(room["membership_counts"])
			lines = append(lines, fmt.Sprintf("- %s: %s join=%.0f invite=%.0f leave=%.0f", label, name, numberOrZero(counts["join"]), numberOrZero(counts["invite"]), numberOrZero(counts["leave"])))
			membership := asMap(room["membership"])
			for _, user := range stringSlice(report["expected_members"]) {
				if state := strings.TrimSpace(fmt.Sprint(membership[user])); state != "" && state != "<nil>" {
					lines = append(lines, fmt.Sprintf("  - %s: %s", user, state))
				}
			}
			if errText := strings.TrimSpace(fmt.Sprint(room["members_error"])); errText != "" && errText != "<nil>" {
				lines = append(lines, "  - members_error: "+truncateText(errText, 160))
			}
		}
	}
	lines = append(lines, "", "판단:")
	switch {
	case status == "failed":
		lines = append(lines, "- Matrix 설정 파일을 읽지 못했습니다. 먼저 `~/.meshclaw/matrix.json`을 확인해야 합니다.")
	case configuredOK && !httpsOK && http8008OK:
		lines = append(lines, "- 홈서버와 새 운영방은 살아 있습니다. 다만 `https://matrix.hunmin.ai` 443 경로가 열려 있지 않으면 클라이언트가 기본 HTTPS 주소를 쓸 때 방 목록 sync가 멈춥니다.")
		lines = append(lines, "- `MeshClaw Fresh Unencrypted`는 레거시 Brain 방 캐시입니다. 현재 운영 브리지는 `MeshClaw Ops` 방을 기준으로 봅니다.")
	case privateTailnet && ok:
		lines = append(lines, "- 서버는 살아 있습니다. 원격 클라이언트에서 멈춘다면 대개 Tailscale VPN이 꺼져 있거나 옛 방 타임라인을 열고 있는 상태입니다.")
	case privateTailnet:
		lines = append(lines, "- homeserver가 Tailscale IP라서 일반 인터넷만으로는 접속할 수 없습니다. 클라이언트의 Tailscale VPN 상태를 먼저 봐야 합니다.")
	case !ok:
		lines = append(lines, "- homeserver 자체가 현재 응답하지 않습니다. Dendrite 또는 reverse proxy 상태를 봐야 합니다.")
	default:
		lines = append(lines, "- 공개 경로가 응답합니다. 클라이언트 방 가입/암호화/타임라인 문제를 우선 봅니다.")
	}
	lines = append(lines, "", "다음 행동:")
	for _, step := range stringSlice(report["client_steps"]) {
		lines = append(lines, "- "+step)
	}
	lines = append(lines,
		"- 새 방을 검색해야 하면 `#meshclaw-ops:matrix.hunmin.ai`를 사용합니다.",
		"- 옛 방 `MeshClaw Fresh Unencrypted`는 서버 운영 봇이 더 이상 듣지 않는 레거시 방입니다.",
	)
	return strings.Join(lines, "\n")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func naturalTitle(title, host, status string) string {
	if host == "" || host == "<nil>" {
		return title
	}
	switch status {
	case "ok":
		return fmt.Sprintf("%s는 현재 큰 문제 없이 확인됐습니다.", host)
	case "findings":
		return fmt.Sprintf("%s에서 확인할 항목이 발견됐습니다.", host)
	case "failed":
		return fmt.Sprintf("%s 조회 자체가 실패했습니다.", host)
	default:
		return fmt.Sprintf("%s 상태를 확인했습니다.", host)
	}
}

func naturalWorkflowStatus(report map[string]interface{}) string {
	name := strings.TrimSpace(fmt.Sprint(report["name"]))
	host := strings.TrimSpace(fmt.Sprint(report["host"]))
	status := strings.TrimSpace(fmt.Sprint(report["status"]))
	switch status {
	case "ok":
		return fmt.Sprintf("%s의 %s 결과는 정상 범위입니다.", host, name)
	case "findings":
		return fmt.Sprintf("%s의 %s 결과에 확인할 항목이 있습니다.", host, name)
	case "failed":
		return fmt.Sprintf("%s의 %s 조회가 실패했습니다. 연결이나 권한을 먼저 봐야 합니다.", host, name)
	case "unknown_node":
		return fmt.Sprintf("%s는 inventory에 등록되지 않은 노드입니다.", host)
	default:
		return fmt.Sprintf("%s의 %s 상태는 %s입니다.", host, name, status)
	}
}

func naturalFinding(finding map[string]interface{}) string {
	severity := strings.TrimSpace(fmt.Sprint(finding["severity"]))
	title := strings.TrimSpace(fmt.Sprint(finding["title"]))
	if title == "" || title == "<nil>" {
		title = strings.TrimSpace(fmt.Sprint(finding["type"]))
	}
	switch severity {
	case "warning":
		return "주의: " + title
	case "error":
		return "문제: " + title
	case "critical":
		return "긴급: " + title
	default:
		return title
	}
}

func naturalRisk(risk string) string {
	risk = strings.TrimSpace(risk)
	risk = strings.ReplaceAll(risk, "/warning:", " 주의:")
	risk = strings.ReplaceAll(risk, "/critical:", " 긴급:")
	risk = strings.ReplaceAll(risk, "/service:", " 서비스:")
	risk = strings.ReplaceAll(risk, "needs review", "확인이 필요합니다")
	risk = strings.ReplaceAll(risk, "failed or restarting service evidence", "실패하거나 재시작 중인 서비스 흔적이 있습니다")
	return risk
}

func naturalAction(action string) string {
	action = strings.TrimSpace(action)
	if action == "" {
		return ""
	}
	switch {
	case strings.Contains(action, "service-check"):
		return "서비스 상태와 로그를 확인합니다: `" + action + "`"
	case strings.Contains(action, "process-top"):
		return "메모리/CPU 상위 프로세스를 확인합니다: `" + action + "`"
	case strings.Contains(action, "disk-investigate"):
		return "디스크 사용 원인을 확인합니다: `" + action + "`"
	case strings.Contains(action, "doctor"):
		return "노드 연결 상태를 진단합니다: `" + action + "`"
	default:
		return "`" + action + "`"
	}
}

func naturalReason(reason string) string {
	reason = strings.TrimSpace(reason)
	reason = strings.ReplaceAll(reason, "Node is offline or SSH/Tailscale path failed.", "노드가 오프라인이거나 SSH/Tailscale 경로가 실패했습니다.")
	reason = strings.ReplaceAll(reason, "Root disk usage is critical; bounded cache, journal, temp, and Docker cleanup is safe to try first.", "루트 디스크 사용량이 위험 구간입니다. 캐시, journal, 임시 파일, Docker prune 같은 제한된 정리를 먼저 시도할 수 있습니다.")
	reason = strings.ReplaceAll(reason, "Cleanup may not be enough; collect top-level disk evidence before any targeted deletion.", "정리만으로 부족할 수 있어 삭제 전에 디스크 사용 근거를 먼저 수집해야 합니다.")
	reason = strings.ReplaceAll(reason, "Disk usage is high but not critical; inspect largest directories before cleanup.", "디스크 사용량이 높습니다. 정리 전에 큰 디렉터리부터 확인해야 합니다.")
	reason = strings.ReplaceAll(reason, "Memory usage is very high; dropping Linux page cache is bounded and non-destructive.", "메모리 사용량이 매우 높습니다. Linux page cache drop은 제한적이고 비파괴적인 조치입니다.")
	return reason
}

func naturalPlacementReason(reason string) string {
	reason = strings.TrimSpace(reason)
	reason = strings.ReplaceAll(reason, "online, disk", "온라인 상태이고 디스크")
	reason = strings.ReplaceAll(reason, "memory", "메모리")
	reason = strings.ReplaceAll(reason, "GPU", "GPU")
	reason = strings.ReplaceAll(reason, "docker 있음", "Docker가 설치되어 있습니다")
	reason = strings.ReplaceAll(reason, "ollama 있음", "Ollama가 설치되어 있습니다")
	reason = strings.ReplaceAll(reason, "macmini 로컬 모델 워커", "Mac mini 로컬 모델 워커입니다")
	reason = strings.ReplaceAll(reason, "meshclaw/vssh 준비됨", "MeshClaw/vssh 실행 준비가 되어 있습니다")
	return reason
}

func wrapCommands(actions []string) []string {
	out := make([]string, 0, len(actions))
	for _, action := range actions {
		out = append(out, "`"+action+"`")
	}
	return out
}

func firstNMaps(values []map[string]interface{}, n int) []map[string]interface{} {
	if len(values) <= n {
		return values
	}
	return values[:n]
}

func asStringMap(value interface{}) map[string]string {
	out := map[string]string{}
	raw := asMap(value)
	for key, val := range raw {
		text := strings.TrimSpace(fmt.Sprint(val))
		if text != "" && text != "<nil>" {
			out[key] = text
		}
	}
	return out
}

func preferredToolOrder(tools map[string]string) []string {
	preferred := []string{"tailscale", "vssh", "meshclaw", "docker", "kubectl", "k3s", "python3", "go", "node", "ollama", "nvidia-smi"}
	out := []string{}
	seen := map[string]bool{}
	for _, name := range preferred {
		if _, ok := tools[name]; ok {
			out = append(out, name)
			seen[name] = true
		}
	}
	extra := []string{}
	for name := range tools {
		if !seen[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	out = append(out, extra...)
	return firstNStrings(out, 12)
}

func shortToolSummary(tools map[string]string) string {
	names := preferredToolOrder(tools)
	if len(names) == 0 {
		return ""
	}
	return "tools=" + strings.Join(firstNStrings(names, 6), ",")
}

func shortenVersion(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 90 {
		return value
	}
	return value[:87] + "..."
}

func firstNStrings(values []string, n int) []string {
	if len(values) <= n {
		return values
	}
	return values[:n]
}

func stringSlice(value interface{}) []string {
	if out, ok := value.([]string); ok {
		return out
	}
	raw := asSlice(value)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text := strings.TrimSpace(fmt.Sprint(item))
		if text != "" && text != "<nil>" {
			out = append(out, text)
		}
	}
	return out
}

func truncateText(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

func indentBlock(value, prefix string) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func formatWorkspaceList(result map[string]interface{}) string {
	payload := asMap(result["result"])
	items := asMapSlice(payload["workspaces"])
	if len(items) == 0 {
		return "No workspaces registered."
	}
	lines := []string{"MeshClaw workspaces"}
	for _, m := range items {
		lines = append(lines, fmt.Sprintf("- %v: %v:%v owner=%v purpose=%v", m["id"], m["host"], m["path"], m["owner"], m["purpose"]))
	}
	sort.Strings(lines[1:])
	return strings.Join(lines, "\n")
}

func explainWorkspaceList(result map[string]interface{}) string {
	payload := asMap(result["result"])
	items := asMapSlice(payload["workspaces"])
	if len(items) == 0 {
		return "MeshClaw 실제 워크스페이스 조회 결과입니다.\n\n현재 등록된 워크스페이스가 없습니다.\n\n해석:\n- Codex, Claude, Open WebUI, Matrix 작업이 어느 서버/폴더에서 진행되는지 추적하려면 workspace 등록이 먼저 필요합니다.\n\n다음 행동:\n- 현재 프로젝트를 등록하려면 `현재 워크스페이스 등록` 또는 `workspace-add`를 사용하면 됩니다."
	}
	lines := []string{
		"MeshClaw 실제 워크스페이스 조회 결과입니다.",
		"",
		fmt.Sprintf("현재 등록된 워크스페이스는 %d개입니다.", len(items)),
		"",
		"목록:",
	}
	itemLines := make([]string, 0, len(items))
	for _, m := range items {
		line := fmt.Sprintf("- %v: %v:%v", m["id"], m["host"], m["path"])
		if owner := strings.TrimSpace(fmt.Sprint(m["owner"])); owner != "" {
			line += fmt.Sprintf(" owner=%s", owner)
		}
		if purpose := strings.TrimSpace(fmt.Sprint(m["purpose"])); purpose != "" {
			line += fmt.Sprintf(" purpose=%s", purpose)
		}
		itemLines = append(itemLines, line)
	}
	sort.Strings(itemLines)
	lines = append(lines, itemLines...)
	lines = append(lines,
		"",
		"해석:",
		"- 워크스페이스는 모델/사람이 실제 작업하는 서버와 폴더의 위치 기록입니다.",
		"- 지금 기준으로 MeshClaw 본 프로젝트는 local:/Users/example/Projects/meshclaw 쪽에 등록되어 있습니다.",
		"- owner 값은 마지막 등록 주체를 나타내므로, 실제 현재 작업자와 다를 수 있습니다. 작업 시작/종료 이벤트를 더 기록하면 정확도가 올라갑니다.",
		"",
		"다음 행동:",
		"- 다른 서버의 프로젝트 폴더도 등록하면 Matrix에서 '누가 어디서 뭘 하는지'를 더 잘 추적할 수 있습니다.",
		"- 작업자가 바뀔 때 `workspace-activity`를 남기면 evidence와 연결됩니다.",
	)
	return strings.Join(lines, "\n")
}

func formatMonitor(result map[string]interface{}) string {
	payload := asMap(result["result"])
	states := asMap(payload["states"])
	alerts := asSlice(payload["alerts"])
	return fmt.Sprintf("Fleet status: nodes=%d alerts=%d\n%s", len(states), len(alerts), formatJSONBlock(payload, 5000))
}

func explainMonitor(result map[string]interface{}, query string) string {
	payload := asMap(result["result"])
	states := asMap(payload["states"])
	alerts := asSlice(payload["alerts"])
	if node := queryNode(query, states); node != "" {
		return explainMonitorNode(node, states[node], alerts, query)
	}
	offline := 0
	highDisk := []string{}
	highMem := []string{}
	for name, raw := range states {
		state := asMap(raw)
		if online, ok := state["online"].(bool); ok && !online {
			offline++
		}
		if disk, ok := numberValue(state["disk"]); ok && disk >= 80 {
			highDisk = append(highDisk, fmt.Sprintf("%s %.1f%%", name, disk))
		}
		if mem, ok := numberValue(state["memory"]); ok && mem >= 80 {
			highMem = append(highMem, fmt.Sprintf("%s %.1f%%", name, mem))
		}
	}
	sort.Strings(highDisk)
	sort.Strings(highMem)
	lines := []string{
		"MeshClaw 실제 서버 상태 조회 결과입니다.",
		"",
		fmt.Sprintf("요약: nodes=%d offline=%d alerts=%d", len(states), offline, len(alerts)),
	}
	if len(highDisk) > 0 {
		lines = append(lines, "디스크 주의: "+strings.Join(highDisk, ", "))
	}
	if len(highMem) > 0 {
		lines = append(lines, "메모리 주의: "+strings.Join(highMem, ", "))
	}
	lines = append(lines,
		"",
		"해석:",
		"- 이 결과는 현재 MeshClaw monitor/fleet 조회에서 나온 실제 상태입니다.",
		"- 디스크 80% 이상, 메모리 80% 이상, offline 노드는 우선 확인 대상입니다.",
		"- CPU 값이 0으로 보이면 해당 facts 수집 방식이 아직 load/cpu를 충분히 채우지 못했을 수 있으므로 vssh facts 개선 대상으로 봐야 합니다.",
		"",
		"다음 행동:",
		"- 디스크가 높은 노드는 `disk-investigate <host> /`로 원인을 확인합니다.",
		"- 로그 문제가 의심되면 `analyze-logs <host> system`으로 최근 오류를 봅니다.",
		"- 개인정보/시크릿 누출이 걱정되면 `hygiene-scan-host <host>`를 실행합니다.",
	)
	return strings.Join(lines, "\n")
}

func explainMonitorNode(name string, raw interface{}, alerts []interface{}, query string) string {
	state := asMap(raw)
	online, _ := state["online"].(bool)
	ip := strings.TrimSpace(fmt.Sprint(state["ip"]))
	disk, hasDisk := numberValue(state["disk"])
	mem, hasMem := numberValue(state["memory"])
	cpu, hasCPU := numberValue(state["cpu"])
	gpuMem, hasGPUMem := numberValue(state["gpu_memory"])
	focus := queryFocus(query)
	lines := []string{
		fmt.Sprintf("MeshClaw 실제 노드 상태 조회 결과입니다: %s", name),
		"",
		"상태:",
		fmt.Sprintf("- online: %t", online),
	}
	if ip != "" && ip != "<nil>" {
		lines = append(lines, fmt.Sprintf("- ip: %s", ip))
	}
	if hasMem {
		lines = append(lines, fmt.Sprintf("- memory: %.1f%%", mem))
	}
	if hasDisk {
		lines = append(lines, fmt.Sprintf("- disk: %.1f%%", disk))
	}
	if hasCPU {
		lines = append(lines, fmt.Sprintf("- cpu: %.1f%%", cpu))
	}
	if hasGPUMem {
		lines = append(lines, fmt.Sprintf("- gpu_memory: %.1f%%", gpuMem))
	}
	lines = append(lines, "", "판단:")
	if !online {
		lines = append(lines, "- 이 노드는 offline 상태입니다. 우선 Tailscale/vssh 연결성과 전원 상태를 확인해야 합니다.")
	} else {
		lines = append(lines, "- 이 노드는 online 상태입니다. 장애보다는 자원 사용량 또는 서비스 상태 확인 대상으로 봅니다.")
	}
	if focus == "memory" || focus == "" {
		switch {
		case hasMem && mem >= 90:
			lines = append(lines, fmt.Sprintf("- 메모리 %.1f%%는 위험 구간입니다. OOM 또는 서비스 지연 가능성을 우선 확인해야 합니다.", mem))
		case hasMem && mem >= 80:
			lines = append(lines, fmt.Sprintf("- 메모리 %.1f%%는 주의 구간입니다. 상위 프로세스와 최근 증가 추이를 확인하는 것이 맞습니다.", mem))
		case hasMem:
			lines = append(lines, fmt.Sprintf("- 메모리 %.1f%%는 즉시 위험 구간은 아닙니다.", mem))
		}
	}
	if focus == "disk" || focus == "" {
		switch {
		case hasDisk && disk >= 90:
			lines = append(lines, fmt.Sprintf("- 디스크 %.1f%%는 위험 구간입니다. 정리 후보와 로그 폭증 여부를 확인해야 합니다.", disk))
		case hasDisk && disk >= 80:
			lines = append(lines, fmt.Sprintf("- 디스크 %.1f%%는 주의 구간입니다. `disk-investigate %s /`가 다음 확인입니다.", disk, name))
		case hasDisk:
			lines = append(lines, fmt.Sprintf("- 디스크 %.1f%%는 즉시 위험 구간은 아닙니다.", disk))
		}
	}
	if focus == "gpu" && hasGPUMem {
		lines = append(lines, fmt.Sprintf("- GPU memory %.1f%% 기준으로는 현재 급한 포화 상태로 보이지 않습니다.", gpuMem))
	}
	lines = append(lines, "", "다음 행동:")
	if focus == "memory" || (hasMem && mem >= 80) {
		lines = append(lines,
			fmt.Sprintf("- read-only로 `%s`의 메모리 상위 프로세스를 확인합니다. 다음 개발 항목은 `process-top %s`입니다.", name, name),
			fmt.Sprintf("- 최근 에러와 재시작 흔적은 `analyze-logs %s system`으로 확인합니다.", name),
		)
	}
	if focus == "disk" || (hasDisk && disk >= 80) {
		lines = append(lines, fmt.Sprintf("- 디스크 원인은 `disk-investigate %s /`로 확인합니다.", name))
	}
	if focus == "service" {
		lines = append(lines, fmt.Sprintf("- 특정 서비스명이 있으면 `service-check %s <service>`로 read-only 상태를 봅니다.", name))
	}
	if len(lines) > 0 && lines[len(lines)-1] == "다음 행동:" {
		lines = append(lines, "- 현재는 즉시 조치보다 추세 확인이 우선입니다. 다음 질문에 서비스명이나 로그 범위를 붙이면 더 좁혀볼 수 있습니다.")
	}
	return strings.Join(lines, "\n")
}

func queryNode(query string, states map[string]interface{}) string {
	lower := strings.ToLower(strings.TrimSpace(query))
	if lower == "" {
		return ""
	}
	names := make([]string, 0, len(states))
	for name := range states {
		names = append(names, strings.ToLower(name))
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	for _, name := range names {
		if name == "" {
			continue
		}
		if lower == name ||
			strings.Contains(lower, name+" ") ||
			strings.Contains(lower, name+"에서") ||
			strings.Contains(lower, name+"에 ") ||
			strings.Contains(lower, name+"의") ||
			strings.Contains(lower, name+"가") ||
			strings.Contains(lower, name+"는") {
			return name
		}
	}
	return ""
}

func queryFocus(query string) string {
	lower := strings.ToLower(query)
	switch {
	case strings.Contains(lower, "메모리") || strings.Contains(lower, "memory") || strings.Contains(lower, "mem"):
		return "memory"
	case strings.Contains(lower, "디스크") || strings.Contains(lower, "disk"):
		return "disk"
	case strings.Contains(lower, "gpu"):
		return "gpu"
	case strings.Contains(lower, "cpu") || strings.Contains(lower, "load"):
		return "cpu"
	case strings.Contains(lower, "서비스") || strings.Contains(lower, "service"):
		return "service"
	default:
		return ""
	}
}

func numberValue(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		out, err := v.Float64()
		return out, err == nil
	default:
		return 0, false
	}
}

func numberOrZero(value interface{}) float64 {
	out, _ := numberValue(value)
	return out
}

func asMap(value interface{}) map[string]interface{} {
	if out, ok := value.(map[string]interface{}); ok {
		return out
	}
	data, err := json.Marshal(value)
	if err != nil {
		return map[string]interface{}{}
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

func asMapSlice(value interface{}) []map[string]interface{} {
	if out, ok := value.([]map[string]interface{}); ok {
		return out
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out []map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func asSlice(value interface{}) []interface{} {
	if out, ok := value.([]interface{}); ok {
		return out
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out []interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func formatJSONBlock(value interface{}, max int) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprint(value)
	}
	text := string(data)
	if max > 0 && len(text) > max {
		text = text[:max] + "\n... truncated ..."
	}
	return "```json\n" + text + "\n```"
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, payload interface{}, out interface{}) error {
	var body *bytes.Reader
	if payload == nil {
		body = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.config.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("matrix %s %s: %s %s", method, endpoint, resp.Status, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
