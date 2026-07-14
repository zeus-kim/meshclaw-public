package router

import (
	"strings"

	"github.com/meshclaw/meshclaw/internal/inventory"
	"github.com/meshclaw/meshclaw/internal/policy"
)

type Plan struct {
	Kind             string        `json:"kind"`
	Source           string        `json:"source"`
	Subject          string        `json:"subject"`
	Message          string        `json:"message"`
	Intent           string        `json:"intent"`
	Target           string        `json:"target,omitempty"`
	Action           string        `json:"action"`
	Resource         string        `json:"resource"`
	Route            string        `json:"route"`
	Lane             string        `json:"lane"`
	Confidence       float64       `json:"confidence"`
	Risk             string        `json:"risk"`
	Decision         policy.Result `json:"policy"`
	Execute          bool          `json:"execute"`
	ApprovalRequired bool          `json:"approval_required"`
	SendToModel      bool          `json:"send_to_model"`
	Reason           string        `json:"reason"`
	Next             []string      `json:"next"`
}

type Options struct {
	Source  string
	Subject string
	Message string
}

type phraseRule struct {
	Intent     string
	Action     string
	Resource   string
	Route      string
	Confidence float64
	Risk       string
	Reason     string
	Phrases    []string
}

var phraseRules = []phraseRule{
	{
		Intent:     "opsdb_power_events",
		Action:     "read_state",
		Resource:   "opsdb",
		Route:      "opsdb_power_events",
		Confidence: 0.96,
		Risk:       "read",
		Reason:     "Power, voltage, and simultaneous reboot questions should start from OpsDB boot-history correlation before blaming applications or services.",
		Phrases: []string{
			"power outage", "power event", "power dip", "power quality", "ups", "voltage", "brownout", "blackout", "simultaneous reboot", "correlated reboot", "boot identity",
			"정전", "전원 나갔", "전원 이벤트", "전압", "순간 전압", "전압강하", "순간 정전", "동시 재부팅", "동시에 재부팅", "같이 꺼", "같이 떨어", "여러 서버가 동시에", "여러 서버가 같이", "동시에 꺼",
		},
	},
	{
		Intent:     "meshclaw_help",
		Action:     "read_state",
		Resource:   "meshclaw",
		Route:      "meshclaw_help",
		Confidence: 0.94,
		Risk:       "read",
		Reason:     "MeshClaw product/function questions should be answered from MeshClaw's own architecture report, not free-form model guesses.",
		Phrases: []string{
			"meshclaw 기능", "meshclaw 뭐", "meshclaw 사용법", "meshclaw 설명", "what is meshclaw", "meshclaw help", "meshclaw features", "what can meshclaw do", "what can i do here",
			"메시클로 기능", "메시클로 뭐", "메시클로 사용법", "메시클로 설명", "메시클로가 뭐", "메시클로는 뭐", "메시클로로 뭘", "메시클로로 뭐", "여기서 뭐 할", "여기서 뭘 할", "여기서 무엇을", "뭘 할 수", "뭐 할 수", "무엇을 할 수",
			"매시클로 기능", "매시클로 뭐", "매시클로 사용법", "매시클로 설명",
		},
	},
	{
		Intent:     "openwebui_status",
		Action:     "read_state",
		Resource:   "openwebui",
		Route:      "openwebui_scan",
		Confidence: 0.98,
		Risk:       "read",
		Reason:     "Open WebUI status is a MeshClaw read-only runtime query.",
		Phrases: []string{
			"openwebui", "open-webui", "open webui", "open web ui", "오픈웹유아이", "웹유아이",
			"オープンwebui", "开放webui", "เปิด open webui", "open webui trạng thái",
		},
	},
	{
		Intent:     "evidence_list",
		Action:     "evidence_list",
		Resource:   "evidence",
		Route:      "evidence_list",
		Confidence: 0.95,
		Risk:       "read",
		Reason:     "Evidence lookup is a MeshClaw read-only query.",
		Phrases: []string{
			"evidence list", "latest evidence", "recent evidence", "show evidence", "evidence id",
			"최근 evidence", "최근 증거", "최근 기록", "증거 목록", "근거 목록", "evidence 보여", "증거 보여", "기록 보여",
			"証拠一覧", "最新証拠", "最近の記録", "记录列表", "最新记录", "รายการหลักฐาน", "bằng chứng gần đây", "nhật ký gần đây",
		},
	},
	{
		Intent:     "data_doctor",
		Action:     "read_state",
		Resource:   "meshclaw_data",
		Route:      "data_doctor",
		Confidence: 0.93,
		Risk:       "read",
		Reason:     "MeshClaw data growth and retention checks are read-only local runtime diagnostics.",
		Phrases: []string{
			"data doctor", "data status", "storage status", "meshclaw data", "argos data",
			"데이터 상태", "저장소 상태", "저장 상태", "쌓이는 데이터", "데이터 쌓", "데이터 꼬", "꼬이지", "자동 정리 상태",
			"메시클로 데이터", "아르고스 데이터", "리포트 파일 상태", "evidence 상태", "로그 용량",
		},
	},
	{
		Intent:     "schedule_status",
		Action:     "read_state",
		Resource:   "scheduler",
		Route:      "schedule_status",
		Confidence: 0.94,
		Risk:       "read",
		Reason:     "Scheduler health and automation backlog checks are read-only MeshClaw runtime diagnostics.",
		Phrases: []string{
			"schedule status", "scheduler status", "schedule-runner", "schedule runner", "due_count", "due count", "due jobs", "next due", "automation backlog",
			"자동화 상태", "자동화 밀", "밀렸", "밀린", "스케줄 상태", "스케줄러 상태", "스케줄러 데몬", "자동화 데몬", "다음 자동화", "다음 실행", "due 작업",
		},
	},
	{
		Intent:     "policy_show",
		Action:     "read_state",
		Resource:   "policy",
		Route:      "policy_summary",
		Confidence: 0.9,
		Risk:       "read",
		Reason:     "Policy lookup is a MeshClaw read-only query.",
		Phrases: []string{
			"policy", "정책", "권한", "승인 정책",
			"ポリシー", "権限", "策略", "权限", "นโยบาย", "สิทธิ์", "chính sách", "quyền",
		},
	},
	{
		Intent:     "linux_worker_nodes",
		Action:     "read_state",
		Resource:   "workers",
		Route:      "linux_worker_nodes",
		Confidence: 0.95,
		Risk:       "read",
		Reason:     "Linux worker registry status is a read-only MeshClaw query.",
		Phrases: []string{
			"workers nodes", "worker nodes", "linux workers", "linux worker", "g-series status", "g series status",
			"g 계열 상태", "g계열 상태", "g 계열 워커 상태", "g계열 워커 상태", "리눅스 워커 상태", "워커 노드 상태",
		},
	},
	{
		Intent:     "worker_plan",
		Action:     "read_state",
		Resource:   "workers",
		Route:      "worker_plan",
		Confidence: 0.94,
		Risk:       "read",
		Reason:     "Worker placement is a read-only MeshClaw inventory decision.",
		Phrases: []string{
			"worker-plan", "worker plan", "placement worker", "g-series", "g series", "g 계열", "g계열",
			"어디에 맡", "어디서 돌", "어느 워커", "워커 골라", "백그라운드 워커", "헤드리스 워커",
		},
	},
	{
		Intent:     "workers",
		Action:     "read_state",
		Resource:   "workers",
		Route:      "workers",
		Confidence: 0.9,
		Risk:       "read",
		Reason:     "Worker lookup is a MeshClaw read-only query.",
		Phrases: []string{
			"worker", "workers", "워커", "작업자",
			"ワーカー", "工作者", "worker", "người làm việc",
		},
	},
	{
		Intent:     "restart_service",
		Action:     "restart_service",
		Resource:   "server",
		Route:      "approval_request",
		Confidence: 0.92,
		Risk:       "write",
		Reason:     "Service restart changes live server state.",
		Phrases: []string{
			"restart", "재시작", "systemctl restart", "서비스 재시작",
			"再起動", "重启", "รีสตาร์ท", "khởi động lại",
		},
	},
	{
		Intent:     "data_clean",
		Action:     "delete_data",
		Resource:   "server",
		Route:      "approval_request",
		Confidence: 0.86,
		Risk:       "write",
		Reason:     "Data cleanup or deletion requires an explicit approval gate.",
		Phrases: []string{
			"삭제", "delete", "remove", "rm ", "rm -", "지워", "cleanup", "clean up",
			"체크포인트 정리", "캐시 정리", "디스크 정리", "로그 삭제", "데이터 정리",
			"削除", "删除", "ลบ", "xóa", "xoá",
		},
	},
	{
		Intent:     "guard_secret",
		Action:     "guard_vault_use",
		Resource:   "secret",
		Route:      "guard",
		Confidence: 0.9,
		Risk:       "secret",
		Reason:     "Secret handling must stay in Guard/Vault policy flow.",
		Phrases: []string{
			"비밀번호", "패스워드", "password", "token", "토큰", "secret", "시크릿",
			"パスワード", "トークン", "密码", "令牌", "รหัสผ่าน", "โทเคน", "mật khẩu", "mã thông báo",
		},
	},
	{
		Intent:     "email_send",
		Action:     "email_send",
		Resource:   "mail",
		Route:      "approval_request",
		Confidence: 0.88,
		Risk:       "write",
		Reason:     "Sending real email requires approval and evidence.",
		Phrases: []string{
			"메일 보내", "이메일 보내", "send mail", "send email",
			"メールを送", "发送邮件", "ส่งอีเมล", "gửi email",
		},
	},
	{
		Intent:     "signal_call",
		Action:     "signal_call",
		Resource:   "messenger",
		Route:      "approval_request",
		Confidence: 0.88,
		Risk:       "write",
		Reason:     "Placing a Signal call requires an approved target and explicit approval.",
		Phrases: []string{
			"전화", "통화", "signal call", "시그널 전화",
			"電話", "通話", "打电话", "โทร", "gọi điện",
		},
	},
}

var fleetStatusPhrases = []string{
	"fleet", "fleet status", "fleet health", "cluster status", "cluster health", "monitor", "monitoring",
	"server status", "server health", "node status", "node health", "host status", "host health",
	"system status", "system health", "machine status", "machine health",
	"disk usage", "memory usage", "gpu status", "cpu status", "alert", "alerts", "warning", "warnings", "incident", "incidents",
	"fail2ban", "open port", "open ports", "public port", "public ports", "listening port", "listener", "listeners",
	"firewall", "ufw", "iptables", "cron", "crontab", "systemd timer", "docker port", "docker ports",
	"intrusion", "bruteforce", "brute force", "ssh attack", "ssh attacks", "suspicious login",
	"노드", "서버 전체", "전체 서버", "서버 상태", "상태 점검", "상태 확인", "서버 점검", "노드 상태", "모니터", "종합 보고", "전체 보고", "서버 보고", "운영 보고", "운영자 관점", "알림", "경고", "장애", "문제",
	"gpu", "cpu", "디스크", "메모리", "보안", "열린 포트", "공개 포트", "포트", "방화벽", "크론", "크론잡", "침입", "침입 징후", "무차별", "ssh 공격", "도커", "도커 포트",
	"어제", "비교", "변화", "달라", "지난", "최근 변화", "히스토리",
	"サーバー状態", "状態確認", "ノード", "服务器状态", "节点", "สถานะเซิร์ฟเวอร์", "โหนด", "trạng thái máy chủ", "nút", "bộ nhớ", "đĩa",
}

func Classify(opts Options) Plan {
	source := strings.TrimSpace(opts.Source)
	if source == "" {
		source = "openwebui"
	}
	subject := strings.TrimSpace(opts.Subject)
	if subject == "" {
		subject = subjectForSource(source)
	}
	message := strings.TrimSpace(opts.Message)
	lower := strings.ToLower(message)
	target := extractKnownNode(lower)

	plan := Plan{
		Kind:        "meshclaw_route_plan",
		Source:      source,
		Subject:     subject,
		Message:     message,
		Intent:      "general_chat",
		Action:      "chat",
		Resource:    "conversation",
		Route:       "model",
		Lane:        "model:local_chat",
		Confidence:  0.2,
		Risk:        "none",
		SendToModel: true,
		Reason:      "No MeshClaw operations intent was detected.",
		Next:        []string{"Send the message to the selected chat model."},
	}

	switch {
	case message == "":
		plan.Intent = "empty"
		plan.Action = "none"
		plan.Resource = "message"
		plan.Route = "none"
		plan.Lane = "none"
		plan.Confidence = 1
		plan.SendToModel = false
		plan.Reason = "Message is empty."
	case isDataCleanupRequest(lower):
		plan.Intent = "data_clean"
		plan.Action = "delete_data"
		plan.Resource = "server"
		plan.Route = "approval_request"
		plan.Target = target
		plan.Confidence = 0.9
		plan.Risk = "write"
		plan.SendToModel = false
		plan.Reason = "Cleanup or deletion changes server state and requires an explicit approval gate."
	case isRestartRequest(lower):
		plan.Intent = "restart_service"
		plan.Action = "restart_service"
		plan.Resource = "server"
		plan.Route = "approval_request"
		plan.Target = target
		plan.Confidence = 0.94
		plan.Risk = "write"
		plan.SendToModel = false
		plan.Reason = "Service or application restart changes live server state."
	case target != "" && containsAny(lower, "process", "process-top", "top process", "프로세스", "상위 프로세스", "プロセス", "进程", "โปรเซส", "tiến trình"):
		plan.Intent = "process_top"
		plan.Action = "read_state"
		plan.Resource = "server"
		plan.Route = "process_top"
		plan.Target = target
		plan.Confidence = 0.9
		plan.Risk = "read"
		plan.SendToModel = false
		plan.Reason = "Process inspection is a MeshClaw read-only server query."
	case target != "" && containsAny(lower, "disk", "df", "du ", "디스크", "용량", "저장공간", "ディスク", "磁盘", "ดิสก์", "ổ đĩa"):
		plan.Intent = "disk_investigate"
		plan.Action = "read_state"
		plan.Resource = "server"
		plan.Route = "disk_investigate"
		plan.Target = target
		plan.Confidence = 0.93
		plan.Risk = "read"
		plan.SendToModel = false
		plan.Reason = "Disk inspection is a MeshClaw read-only server query."
	case isOpenWebUITroubleRequest(lower):
		plan.Intent = "openwebui_status"
		plan.Action = "read_state"
		plan.Resource = "openwebui"
		plan.Route = "openwebui_scan"
		plan.Target = target
		plan.Confidence = 0.94
		plan.Risk = "read"
		plan.SendToModel = false
		plan.Reason = "Open WebUI response trouble should be inspected through the Open WebUI runtime scanner."
	case target != "" && isSlowHostRequest(lower):
		plan.Intent = "process_top"
		plan.Action = "read_state"
		plan.Resource = "server"
		plan.Route = "process_top"
		plan.Target = target
		plan.Confidence = 0.88
		plan.Risk = "read"
		plan.SendToModel = false
		plan.Reason = "Host slowness is usually diagnosed by checking top CPU and memory consumers first."
	case target != "" && !containsAny(lower, "삭제", "delete", "remove", "rm ", "rm -", "지워", "로그 삭제", "削除", "删除", "ลบ", "xóa", "xoá") && containsAny(lower, "log", "logs", "journal", "error", "errors", "로그", "에러", "오류", "ログ", "错误", "บันทึก", "lỗi"):
		plan.Intent = "analyze_logs"
		plan.Action = "read_state"
		plan.Resource = "server"
		plan.Route = "analyze_logs"
		plan.Target = target
		plan.Confidence = 0.9
		plan.Risk = "read"
		plan.SendToModel = false
		plan.Reason = "Log analysis is a MeshClaw read-only server query."
	case target != "" && containsAny(lower, "security", "security-check", "보안", "취약", "침입", "포트", "리스너", "listener", "sudoers", "セキュリティ", "安全", "ความปลอดภัย", "bảo mật"):
		plan.Intent = "security_check"
		plan.Action = "read_state"
		plan.Resource = "server"
		plan.Route = "security_check"
		plan.Target = target
		plan.Confidence = 0.9
		plan.Risk = "read"
		plan.SendToModel = false
		plan.Reason = "Security posture inspection is a MeshClaw read-only server query."
	case target != "" && containsAny(lower, "installed", "install", "inventory", "capability", "tools", "깔려", "설치", "인벤토리", "도구", "ツール", "安装", "เครื่องมือ", "cài đặt"):
		plan.Intent = "node_inventory"
		plan.Action = "read_state"
		plan.Resource = "server"
		plan.Route = "node_inventory"
		plan.Target = target
		plan.Confidence = 0.88
		plan.Risk = "read"
		plan.SendToModel = false
		plan.Reason = "Node software inventory is a MeshClaw read-only server query."
	case target != "" && containsAny(lower, "service-check", "서비스 확인", "서비스 체크", "service status", "service check", "systemd 상태", "サービス確認", "服务状态", "ตรวจสอบบริการ", "dịch vụ"):
		plan.Intent = "service_check"
		plan.Action = "read_state"
		plan.Resource = "server"
		plan.Route = "service_check"
		plan.Target = target
		plan.Confidence = 0.9
		plan.Risk = "read"
		plan.SendToModel = false
		plan.Reason = "Service inspection is a MeshClaw read-only server query."
	case target != "" && containsAny(lower, "service-audit", "service audit", "failed service", "failing service", "restarting service", "restart loop", "restart failed", "서비스 장애", "실패 서비스", "죽은 서비스", "재시작 중인 서비스", "재시작 반복", "재시작 실패", "서비스가 죽", "サービス障害", "服务故障"):
		plan.Intent = "service_audit"
		plan.Action = "read_state"
		plan.Resource = "server"
		plan.Route = "service_audit"
		plan.Target = target
		plan.Confidence = 0.9
		plan.Risk = "read"
		plan.SendToModel = false
		plan.Reason = "Service audit is a MeshClaw read-only server query."
	case isReadOnlyServiceAuditRequest(lower):
		plan.Intent = "service_audit"
		plan.Action = "read_state"
		plan.Resource = "server"
		plan.Route = "service_audit"
		plan.Target = target
		plan.Confidence = 0.86
		plan.Risk = "read"
		plan.SendToModel = false
		plan.Reason = "Service audit is a MeshClaw read-only server query."
	case isWorkerRunRequest(lower):
		plan.Intent = "worker_run"
		plan.Action = "read_state"
		plan.Resource = "workers"
		plan.Route = "worker_run"
		plan.Target = target
		plan.Confidence = 0.9
		plan.Risk = "read"
		plan.SendToModel = false
		plan.Reason = "Worker task execution uses a registered MeshClaw worker and stores evidence."
	case isDirectWeatherRequest(lower):
		plan.Intent = "assistant_weather"
		plan.Action = "read_state"
		plan.Resource = "assistant"
		plan.Route = "assistant_weather"
		plan.Confidence = 0.9
		plan.Risk = "read"
		plan.SendToModel = false
		plan.Reason = "Current weather is a read-only Automation Mode query."
	case isNewsSourceRequest(lower):
		plan.Intent = "assistant_news_sources"
		plan.Action = "read_state"
		plan.Resource = "assistant"
		plan.Route = "assistant_news_sources"
		plan.Confidence = 0.88
		plan.Risk = "read"
		plan.SendToModel = false
		plan.Reason = "News source lookup uses the latest stored news evidence."
	case isNewsDetailRequest(lower):
		plan.Intent = "assistant_news_detail"
		plan.Action = "read_state"
		plan.Resource = "assistant"
		plan.Route = "assistant_news_detail"
		plan.Confidence = 0.86
		plan.Risk = "read"
		plan.SendToModel = false
		plan.Reason = "News follow-up uses the latest stored news evidence."
	case isDirectNewsRequest(lower):
		plan.Intent = "assistant_news"
		plan.Action = "read_state"
		plan.Resource = "assistant"
		plan.Route = "assistant_news"
		plan.Confidence = 0.9
		plan.Risk = "read"
		plan.SendToModel = false
		plan.Reason = "News briefing is a read-only Automation Mode query."
	case isDirectBriefingRequest(lower):
		plan.Intent = "assistant_morning"
		plan.Action = "read_state"
		plan.Resource = "assistant"
		plan.Route = "assistant_morning"
		plan.Confidence = 0.86
		plan.Risk = "read"
		plan.SendToModel = false
		plan.Reason = "Briefing generation is a read-only Automation Mode query."
	case applyPhraseRule(&plan, target, lower):
	case containsAny(lower, fleetStatusPhrases...) || isFleetWellnessRequest(lower) || target != "":
		plan.Intent = "fleet_status"
		plan.Action = "fleet_status"
		plan.Resource = "server"
		plan.Route = "monitor_check"
		plan.Target = target
		plan.Confidence = 0.82
		plan.Risk = "read"
		plan.SendToModel = false
		plan.Reason = "Server or node status is a MeshClaw read-only fleet query."
	}

	if plan.Action != "chat" && plan.Action != "none" {
		plan.Decision = policy.Evaluate(policy.Request{
			Subject:  plan.Subject,
			Action:   plan.Action,
			Resource: plan.Resource,
			Context:  message,
		})
	} else {
		plan.Decision = policy.Result{Decision: policy.Allow, Reason: "general chat is delegated to the selected model", Source: "router"}
	}
	plan.ApprovalRequired = plan.Decision.Decision == policy.RequireApproval
	plan.Execute = plan.Decision.Decision == policy.Allow && !plan.SendToModel && plan.Route != "none"
	if plan.Decision.Decision == policy.Deny {
		plan.Lane = "meshclaw:deny"
		plan.Next = []string{"Do not execute. Return the policy denial to the user."}
	} else if plan.ApprovalRequired {
		plan.Lane = "meshclaw:approval"
		plan.Next = []string{"Create an approval request.", "Wait for the human operator to approve before execution."}
	} else if plan.Execute {
		plan.Lane = "meshclaw:direct"
		plan.Next = []string{"Execute the MeshClaw read-only route.", "Store evidence when the route produces operational facts."}
	} else if plan.SendToModel {
		plan.Lane = "model:local_chat"
	}
	return plan
}

func isDataCleanupRequest(lower string) bool {
	if isNegatedDataCleanupRequest(lower) {
		return false
	}
	return containsAny(lower,
		"삭제", "delete", "remove", "rm ", "rm -", "지워", "cleanup", "clean up",
		"체크포인트 정리", "캐시 정리", "디스크 정리", "로그 삭제", "데이터 정리",
		"削除", "删除", "ลบ", "xóa", "xoá",
	)
}

func isNegatedDataCleanupRequest(lower string) bool {
	return containsAny(lower,
		"삭제하지", "삭제 말고", "삭제는 하지", "지우지", "제거하지", "정리하지 말고",
		"do not delete", "don't delete", "dont delete", "no delete", "no deletion",
		"do not remove", "don't remove", "dont remove", "no cleanup", "without deleting",
	)
}

func applyPhraseRule(plan *Plan, target, lower string) bool {
	for _, rule := range phraseRules {
		if !containsAny(lower, rule.Phrases...) {
			continue
		}
		plan.Intent = rule.Intent
		plan.Action = rule.Action
		plan.Resource = rule.Resource
		plan.Route = rule.Route
		plan.Target = target
		plan.Confidence = rule.Confidence
		plan.Risk = rule.Risk
		plan.SendToModel = false
		plan.Reason = rule.Reason
		return true
	}
	return false
}

func isDirectWeatherRequest(lower string) bool {
	if containsAny(lower, "모르", "왜", "어디서", "방식", "api", "브라우저", "weather api", "not know", "don't know", "how do you") {
		return false
	}
	if containsAny(lower, "날씨", "기온", "비 와", "비오", "비 올", "weather", "forecast", "天気", "天气", "อากาศ", "thời tiết") {
		return containsAny(lower, "알려", "조회", "확인", "어때", "현재", "오늘", "지금", "와?", "오나", "tell", "show", "check", "what", "今日", "现在", "วันนี้", "hôm nay")
	}
	return false
}

func isOpenWebUITroubleRequest(lower string) bool {
	if !containsAny(lower, "openwebui", "open-webui", "open webui", "오픈웹유아이", "웹유아이", "model", "models", "모델", "답변", "응답") {
		return false
	}
	return containsAny(lower, "안돼", "안되", "안 됨", "답이 없어", "답없", "응답 없어", "응답이 없어", "먹통", "500", "error", "failed", "no answer", "not responding", "timeout", "타임아웃")
}

func isRestartRequest(lower string) bool {
	if containsAny(lower, "왜", "방법", "어떻게", "how", "why", "restart policy") {
		return false
	}
	if containsAny(lower, "재시작 중", "재시작중", "재시작 반복", "restarting", "restart loop") && containsAny(lower, "확인", "봐", "점검", "check", "show") {
		return false
	}
	return containsAny(lower,
		"재시작", "다시 시작", "restart", "reboot service", "systemctl restart",
		"再起動", "重启", "รีสตาร์ท", "khởi động lại",
	)
}

func isFleetWellnessRequest(lower string) bool {
	if !containsAny(lower, "서버", "서버들", "노드", "전체 서버", "fleet", "nodes", "servers") {
		return false
	}
	return containsAny(lower, "괜찮", "이상", "문제", "상태", "건강", "오늘", "점검", "ok", "okay", "healthy", "health", "problem")
}

func isSlowHostRequest(lower string) bool {
	return containsAny(lower, "느려", "느림", "느린", "느릴", "느리", "버벅", "버벅거", "무거", "slow", "sluggish", "lag", "laggy", "hang", "stuck", "멈춰", "멈춤")
}

func isReadOnlyServiceAuditRequest(lower string) bool {
	if isRestartRequest(lower) {
		return false
	}
	if containsAny(lower, "서버 전체", "전체 상태", "종합 보고", "전체 보고", "운영 보고", "운영자처럼", "fleet status", "full report", "comprehensive") {
		return false
	}
	if !containsAny(lower, "service-audit", "service audit", "failed service", "failing service", "서비스", "systemd", "unit", "유닛", "서비스 목록", "서비스 상태") {
		return false
	}
	if containsAny(lower, "읽기 전용", "확인", "점검", "목록", "상태", "감사", "audit", "check", "show", "list", "failed", "failing", "비정상", "실패", "장애") {
		return true
	}
	return false
}

func isWorkerRunRequest(lower string) bool {
	if strings.TrimSpace(lower) == "" {
		return false
	}
	if containsAny(lower, "상태", "보고", "점검", "확인", "가능", "괜찮", "맡겨도", "맡겨도 돼", "무슨 역할", "어떤 작업", "용도", "위험", "설명", "status", "report", "check", "can i", "can we", "safe to", "role", "risk") {
		return false
	}
	if containsAny(lower, "worker run", "workers run", "워커 실행", "워커에게", "워커에", "worker에게", "worker에") {
		return true
	}
	if !containsAny(lower, "맡겨", "돌려줘", "실행해", "처리해", "시켜", "run it", "run this") {
		return false
	}
	return containsAny(lower,
		"g1", "g2", "g3", "g4", "워커", "worker", "리서치", "research", "뉴스", "요약", "브리핑", "작업", "job", "task",
	)
}

func isVagueHostTroubleRequest(lower string) bool {
	return containsAny(lower, "이상", "문제", "괜찮", "먹통", "안돼", "안되", "weird", "strange", "wrong", "acting up", "acting weird", "broken", "not right")
}

func isDirectNewsRequest(lower string) bool {
	if isNewsSourceRequest(lower) || containsAny(lower, "자세히", "detail") {
		return false
	}
	if containsAny(lower,
		"오늘의 주요뉴스", "오늘 주요뉴스", "주요 뉴스", "주요뉴스", "뉴스 정리", "뉴스 브리핑",
		"주여뉴스", "주여 뉴스",
		"news brief", "top news", "headlines", "ニュース", "新闻", "ข่าว", "tin tức",
	) {
		return true
	}
	if containsAny(lower, "뉴스", "소식", "기사", "헤드라인", "news", "headline") {
		return containsAny(lower,
			"알려", "정리", "요약", "브리핑", "뭐 있", "뭐있", "볼만", "주요", "오늘", "지금", "최근",
			"show", "tell", "brief", "summarize", "summary", "what", "latest", "today", "top",
		)
	}
	return false
}

func isNewsSourceRequest(lower string) bool {
	lower = strings.TrimSpace(lower)
	if lower == "" {
		return false
	}
	if containsAny(lower, "출처", "링크", "원문") {
		return true
	}
	for _, exact := range []string{"source", "sources", "link", "links", "url", "urls"} {
		if lower == exact {
			return true
		}
	}
	return containsAny(lower,
		"show sources", "show source", "give sources", "give source", "send sources", "send links",
		"source links", "source link", "news sources", "original links", "original source",
		"links for", "urls for", "evidence for",
	)
}

func isNewsDetailRequest(lower string) bool {
	if !containsAny(lower, "자세히", "더 자세", "더 알려", "더 설명", "깊게", "detail", "explain", "tell me more", "more about") {
		return false
	}
	return containsAny(lower, "뉴스", "기사", "헤드라인", "headline", "source", "sources", "원문", "출처") || hasExplicitNewsItemReference(lower)
}

func isDirectBriefingRequest(lower string) bool {
	if containsAny(lower, "브리핑") && containsAny(lower, "해줘", "보내", "생성", "만들", "morning", "briefing", "สรุป", "tóm tắt") {
		return true
	}
	return containsAny(lower, "모닝 브리핑", "아침 브리핑", "morning briefing")
}

func hasExplicitNewsItemReference(lower string) bool {
	return containsAny(lower,
		"1번", "2번", "3번", "4번", "5번", "6번", "7번", "8번", "9번",
		"첫 번째", "첫번째", "두 번째", "두번째", "세 번째", "세번째", "네 번째", "네번째",
		"first item", "second item", "third item", "fourth item", "fifth item",
		"item 1", "item 2", "item 3", "item 4", "item 5",
		"article 1", "article 2", "article 3", "article 4", "article 5",
		"about 1", "about 2", "about 3", "about 4", "about 5",
	)
}

func subjectForSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "openwebui", "signal", "argos", "local", "local-llm":
		return "local-llm"
	case "codex", "claude", "cursor":
		return strings.ToLower(strings.TrimSpace(source))
	default:
		return "local-llm"
	}
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func extractKnownNode(lower string) string {
	lower = strings.ToLower(strings.TrimSpace(lower))
	nodes := inventory.DefaultNodes()
	names := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if name := strings.ToLower(strings.TrimSpace(node.Name)); name != "" {
			names = append(names, name)
		}
	}
	for _, name := range names {
		if lower == name ||
			strings.Contains(lower, name+" ") ||
			strings.HasSuffix(lower, " "+name) ||
			strings.HasSuffix(lower, ":"+name) ||
			strings.HasSuffix(lower, "/"+name) ||
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
