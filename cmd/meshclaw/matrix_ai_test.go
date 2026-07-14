package main

import (
	"strings"
	"testing"
)

func TestShouldRouteMatrixAIToTool(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{name: "workspace lookup ko", message: "워크스페이스 보여줘", want: true},
		{name: "ops brief ko", message: "지금 전체 요약하고 뭐 해야 하는지 알려줘", want: true},
		{name: "ops control ko", message: "서버 상태 관리해줘", want: true},
		{name: "fleet inventory ko", message: "노드별로 뭐뭐 깔려 있어?", want: true},
		{name: "node inventory ko", message: "v2에 뭐 깔려 있어?", want: true},
		{name: "placement ko", message: "GPU 추론 작업 어디서 돌리면 좋아?", want: true},
		{name: "server status ko", message: "서버 상태 보여줘", want: true},
		{name: "policy lookup ko", message: "정책 보여줘", want: true},
		{name: "evidence lookup ko", message: "최근 기록 보여줘", want: true},
		{name: "workers lookup ko", message: "작업자 목록 확인", want: true},
		{name: "process lookup ko", message: "process-top v2 확인", want: true},
		{name: "service lookup ko", message: "service-check v2 liquidsoap 확인", want: true},
		{name: "service audit ko", message: "service-audit v2 확인", want: true},
		{name: "service triage ko", message: "서비스 장애 분류해줘", want: true},
		{name: "fleet service audit ko", message: "전체 서비스 장애 확인", want: true},
		{name: "logs lookup ko", message: "analyze-logs v2 system 확인", want: true},
		{name: "security lookup ko", message: "security-check v2 확인", want: true},
		{name: "hygiene lookup ko", message: "v2 민감정보 확인", want: true},
		{name: "matrix client loading ko", message: "Matrix 클라이언트에서 로딩이 안돼", want: true},
		{name: "fleet lookup en", message: "show fleet status", want: true},
		{name: "workspace lookup en", message: "list workspaces", want: true},
		{name: "server problem ko", message: "v2 메모리 왜 높아?", want: true},
		{name: "conceptual policy question", message: "운영정책이 뭔데?", want: false},
		{name: "conceptual workspace question", message: "워크스페이스라는 개념이 뭐야?", want: false},
		{name: "casual chat", message: "음 지금 방향이 좀 애매한데", want: false},
		{name: "dangerous action is not read lookup", message: "서버들에 rm -rf 실행해", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRouteMatrixAIToTool(tt.message); got != tt.want {
				t.Fatalf("shouldRouteMatrixAIToTool(%q)=%v want %v", tt.message, got, tt.want)
			}
		})
	}
}

func TestDangerousMatrixAIAction(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{name: "rm rf", message: "서버들에 rm -rf 실행해", want: true},
		{name: "restart service", message: "g2에서 nginx 재시작해줘", want: true},
		{name: "provision vps", message: "새 vps 하나 임대해", want: true},
		{name: "delete folder", message: "이 폴더 삭제해", want: true},
		{name: "read server status", message: "서버 상태 보여줘", want: false},
		{name: "concept chat", message: "운영 안정화 방향이 뭐야?", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDangerousMatrixAIAction(tt.message); got != tt.want {
				t.Fatalf("isDangerousMatrixAIAction(%q)=%v want %v", tt.message, got, tt.want)
			}
		})
	}
}

func TestDangerousMatrixAIActionResponse(t *testing.T) {
	got := dangerousMatrixAIActionResponse("서버들에 rm -rf 실행해")
	for _, want := range []string{"판단", "policy-check", "evidence", "다음 행동"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in response: %s", want, got)
		}
	}
}

func TestMatrixAIReplyGuardsDangerousAction(t *testing.T) {
	got, err := matrixAIReply(nil, nil, nil, "서버들에 rm -rf 실행해")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "직접 수행하면 안 됩니다") {
		t.Fatalf("expected guarded response, got %s", got)
	}
}

func TestDispatchNodeMemoryQuestionRoutesToMonitor(t *testing.T) {
	t.Setenv("MESHCLAW_DISPATCH_ROUTE_ONLY", "1")
	result, err := dispatchOpsMessage("matrix-ai", "v2 메모리 왜 높아?")
	if err != nil {
		t.Fatal(err)
	}
	if result["route"] != "monitor_check" {
		t.Fatalf("route=%v want monitor_check", result["route"])
	}
}

func TestDispatchProcessTopRoutesToWorkflow(t *testing.T) {
	t.Setenv("MESHCLAW_DISPATCH_ROUTE_ONLY", "1")
	result, err := dispatchOpsMessage("matrix-ai", "process-top not-a-node 확인")
	if err != nil {
		t.Fatal(err)
	}
	if result["route"] != "process_top" {
		t.Fatalf("route=%v want process_top", result["route"])
	}
}

func TestDispatchServiceCheckRoutesToWorkflow(t *testing.T) {
	t.Setenv("MESHCLAW_DISPATCH_ROUTE_ONLY", "1")
	result, err := dispatchOpsMessage("matrix-ai", "service-check not-a-node liquidsoap 확인")
	if err != nil {
		t.Fatal(err)
	}
	if result["route"] != "service_check" {
		t.Fatalf("route=%v want service_check", result["route"])
	}
}

func TestDispatchOpsReadOnlyWorkflows(t *testing.T) {
	t.Setenv("MESHCLAW_DISPATCH_ROUTE_ONLY", "1")
	tests := []struct {
		message string
		route   string
	}{
		{message: "analyze-logs not-a-node system 확인", route: "analyze_logs"},
		{message: "security-check not-a-node 확인", route: "security_check"},
		{message: "not-a-node 민감정보 확인", route: "hygiene_scan_host"},
		{message: "Matrix 클라이언트에서 로딩이 안돼", route: "matrix_doctor"},
		{message: "서버 상태 관리해줘", route: "ops_control"},
		{message: "서비스 장애 분류해줘", route: "service_triage"},
	}
	for _, tt := range tests {
		t.Run(tt.route, func(t *testing.T) {
			result, err := dispatchOpsMessage("matrix-ai", tt.message)
			if err != nil {
				t.Fatal(err)
			}
			if result["route"] != tt.route {
				t.Fatalf("route=%v want %s", result["route"], tt.route)
			}
		})
	}
}
