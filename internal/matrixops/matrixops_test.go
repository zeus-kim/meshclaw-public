package matrixops

import (
	"strings"
	"testing"
)

func TestShouldHandleAmbientAndIgnoredUsers(t *testing.T) {
	cfg := Config{
		UserID:         "@ops-ai:example",
		CommandPrefix:  "@ops-ai",
		AmbientChat:    true,
		IgnoreCommands: true,
		IgnoredUsers:   []string{"@dev:example"},
	}

	tests := []struct {
		name  string
		event Event
		want  bool
	}{
		{
			name:  "ambient human chat",
			event: Event{Type: "m.room.message", Sender: "@operator:example", Content: Content{MsgType: "m.text", Body: "운영 안정화"}},
			want:  true,
		},
		{
			name:  "ignore own message",
			event: Event{Type: "m.room.message", Sender: "@ops-ai:example", Content: Content{MsgType: "m.text", Body: "hello"}},
			want:  false,
		},
		{
			name:  "ignore tool bot",
			event: Event{Type: "m.room.message", Sender: "@dev:example", Content: Content{MsgType: "m.text", Body: "MeshClaw workers"}},
			want:  false,
		},
		{
			name:  "ignore command intended for mention prefix",
			event: Event{Type: "m.room.message", Sender: "@operator:example", Content: Content{MsgType: "m.text", Body: "@ops-ai help"}},
			want:  false,
		},
		{
			name:  "ignore non text",
			event: Event{Type: "m.room.message", Sender: "@operator:example", Content: Content{MsgType: "m.image", Body: "image"}},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldHandle(cfg, tt.event); got != tt.want {
				t.Fatalf("shouldHandle()=%v want %v", got, tt.want)
			}
		})
	}
}

func TestSplitMessage(t *testing.T) {
	chunks := SplitMessage("alpha beta gamma delta", 10)
	if len(chunks) < 2 {
		t.Fatalf("expected split chunks, got %v", chunks)
	}
	for _, chunk := range chunks {
		if len(chunk) > 10 {
			t.Fatalf("chunk too long: %q", chunk)
		}
	}
}

func TestClientSafeMessageTruncatesLongOutput(t *testing.T) {
	t.Setenv("MESHCLAW_MATRIX_MESSAGE_LIMIT", "40")
	text := ClientSafeMessage(strings.Repeat("alpha ", 20))
	if len(text) > 220 {
		t.Fatalf("client-safe output too long: %d", len(text))
	}
	if !strings.Contains(text, "Matrix 클라이언트 로딩 보호") {
		t.Fatalf("expected client-safe suffix: %s", text)
	}
}

func TestFormatExplainedWorkspaceList(t *testing.T) {
	text := FormatExplainedResult(map[string]interface{}{
		"route": "workspace_list",
		"result": map[string]interface{}{
			"workspaces": []map[string]interface{}{
				{"id": "meshclaw-local", "host": "local", "path": "/Users/example/Projects/meshclaw", "owner": "codex", "purpose": "serverops"},
			},
		},
	})
	for _, want := range []string{"실제 워크스페이스", "해석", "다음 행동", "meshclaw-local"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in explained workspace output: %s", want, text)
		}
	}
}

func TestFormatExplainedMonitor(t *testing.T) {
	text := FormatExplainedResult(map[string]interface{}{
		"route": "monitor_check",
		"result": map[string]interface{}{
			"states": map[string]interface{}{
				"d1": map[string]interface{}{"online": true, "disk": 87.2, "memory": 12.5},
				"g1": map[string]interface{}{"online": false, "disk": 10.0, "memory": 8.0},
			},
			"alerts": []interface{}{"disk"},
		},
	})
	for _, want := range []string{"실제 서버 상태", "offline=1", "디스크 주의", "다음 행동"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in explained monitor output: %s", want, text)
		}
	}
}

func TestFormatExplainedMonitorNodeFocus(t *testing.T) {
	text := FormatExplainedResultForQuery(map[string]interface{}{
		"route": "monitor_check",
		"result": map[string]interface{}{
			"states": map[string]interface{}{
				"v2": map[string]interface{}{"online": true, "ip": "100.64.0.2", "disk": 67.0, "memory": 82.2, "cpu": 3.0},
				"g1": map[string]interface{}{"online": true, "disk": 40.0, "memory": 9.0},
			},
			"alerts": []interface{}{},
		},
	}, "v2 메모리 왜 높아?")
	for _, want := range []string{"실제 노드 상태", "v2", "memory: 82.2%", "메모리 82.2%는 주의 구간", "process-top v2", "analyze-logs v2 system"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in node-focused monitor output: %s", want, text)
		}
	}
}

func TestFormatExplainedProcessTop(t *testing.T) {
	text := FormatExplainedResult(map[string]interface{}{
		"route": "process_top",
		"result": map[string]interface{}{
			"report": map[string]interface{}{
				"name":   "process-top",
				"host":   "v2",
				"status": "ok",
				"findings": []map[string]interface{}{
					{"severity": "info", "title": "Top process snapshot collected", "evidence": "---memory---\nPID COMMAND %MEM\n1 ollama 18.0", "next": "service-check v2 ollama"},
				},
			},
		},
	})
	for _, want := range []string{"v2는 현재 큰 문제 없이 확인", "상태", "ollama", "해야 할 일"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in process-top explained output: %s", want, text)
		}
	}
}

func TestFormatExplainedHygiene(t *testing.T) {
	text := FormatExplainedResult(map[string]interface{}{
		"route": "hygiene_scan_host",
		"result": map[string]interface{}{
			"report": map[string]interface{}{
				"host":   "v2",
				"status": "findings",
				"findings": []map[string]interface{}{
					{"severity": "warning", "type": "secret", "target": "/var/log/app.log", "evidence": "REDACTED"},
				},
				"actions": []map[string]interface{}{
					{"mode": "safe", "id": "redact-log", "target": "/var/log/app.log"},
				},
			},
		},
	})
	for _, want := range []string{"민감정보", "host=v2 status=findings", "REDACTED", "safe actions"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in hygiene explained output: %s", want, text)
		}
	}
}

func TestFormatExplainedMatrixDoctor(t *testing.T) {
	text := FormatExplainedResult(map[string]interface{}{
		"route": "matrix_doctor",
		"result": map[string]interface{}{
			"status":            "ok",
			"homeserver":        "http://100.64.0.1:8008",
			"room_id":           "!room:matrix.example",
			"room_alias":        "#meshclaw-ops:matrix.example",
			"private_tailscale": true,
			"server_reachable": map[string]interface{}{
				"ok":     true,
				"status": "200 OK",
			},
			"client_steps": []interface{}{
				"클라이언트에서 Tailscale VPN을 켠다.",
				"Matrix 클라이언트를 완전히 종료한 뒤 다시 연다.",
			},
			"expected_members": []interface{}{"@operator:matrix.example", "@dev:matrix.example"},
			"rooms": []map[string]interface{}{
				{
					"label":             "configured",
					"name":              "MeshClaw Ops",
					"membership_counts": map[string]interface{}{"join": 1, "invite": 1, "leave": 0},
					"membership":        map[string]interface{}{"@operator:matrix.example": "invite", "@dev:matrix.example": "join"},
				},
			},
		},
	})
	for _, want := range []string{"Matrix 클라이언트 접속 상태", "Tailscale", "server reachable: true", "#meshclaw-ops", "방 상태", "@operator:matrix.example: invite", "옛 방"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in matrix doctor output: %s", want, text)
		}
	}
}

func TestFormatExplainedFleetServiceAudit(t *testing.T) {
	text := FormatExplainedResult(map[string]interface{}{
		"route": "fleet_service_audit",
		"result": map[string]interface{}{
			"report": map[string]interface{}{
				"findings":     1,
				"max_parallel": 4,
				"hosts": []map[string]interface{}{
					{
						"name":   "service-audit",
						"host":   "v2",
						"status": "findings",
						"findings": []map[string]interface{}{
							{"severity": "warning", "title": "Failed service", "next": "service-check v2 x"},
						},
					},
					{"name": "service-audit", "host": "v3", "status": "ok"},
				},
			},
		},
	})
	for _, want := range []string{"전체 서버의 서비스 장애", "확인한 서버: 2개", "v2", "service-check v2 x"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in fleet service audit output: %s", want, text)
		}
	}
}

func TestFormatExplainedServiceTriage(t *testing.T) {
	text := FormatExplainedResult(map[string]interface{}{
		"route": "service_triage",
		"result": map[string]interface{}{
			"report": map[string]interface{}{
				"counts": map[string]interface{}{
					"real_incident":           1,
					"stale_or_boot_only":      1,
					"stale_or_missing_target": 0,
					"ignore_candidate":        1,
					"approval_required":       0,
				},
				"items": []map[string]interface{}{
					{"host": "d1", "service": "open-webui", "class": "real_incident", "mode": "inspect", "judgement": "로그 확인 필요", "next": "meshclaw service-check d1 open-webui"},
					{"host": "c1", "service": "systemd-networkd-wait-online", "class": "stale_or_boot_only", "mode": "ignore_candidate", "judgement": "부팅 흔적", "next": "meshclaw service-check c1 systemd-networkd-wait-online"},
				},
			},
		},
	})
	for _, want := range []string{"서비스 장애 후보를 triage", "실제 장애 후보: 1개", "d1/open-webui", "stale_or_boot_only", "운영 기준"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in service triage output: %s", want, text)
		}
	}
}

func TestFormatExplainedOpsBrief(t *testing.T) {
	text := FormatExplainedResult(map[string]interface{}{
		"route": "ops_brief",
		"result": map[string]interface{}{
			"report": map[string]interface{}{
				"monitor": map[string]interface{}{
					"states": map[string]interface{}{"v2": map[string]interface{}{"online": true}},
					"alerts": []interface{}{},
				},
				"service_audit": map[string]interface{}{"findings": 1},
				"top_risks":     []string{"v2/service: systemd-resolved needs review"},
				"next_actions":  []string{"meshclaw service-check v2 systemd-resolved"},
			},
		},
	})
	for _, want := range []string{"현재 운영 상태", "해야 할 일", "서비스 상태와 로그", "판단"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in ops brief output: %s", want, text)
		}
	}
}

func TestFormatExplainedOpsControl(t *testing.T) {
	text := FormatExplainedResult(map[string]interface{}{
		"route": "ops_control",
		"result": map[string]interface{}{
			"report": map[string]interface{}{
				"management_summary": map[string]interface{}{
					"nodes_total":          2,
					"nodes_online":         1,
					"nodes_offline":        1,
					"resource_alerts":      1,
					"service_findings":     1,
					"auto_safe_candidates": 1,
					"approval_required":    1,
				},
				"top_risks":    []string{"v2/warning: disk high"},
				"next_actions": []string{"meshclaw disk-investigate v2 /", "meshclaw autoheal-apply-safe"},
				"autoheal_plan": []map[string]interface{}{
					{"node": "v2", "type": "disk_cleanup", "metric": "disk_percent", "value": 91.2, "mode": "auto_safe", "command": "meshclaw autoheal-apply-safe", "reason": "Root disk usage is critical; bounded cache, journal, temp, and Docker cleanup is safe to try first."},
				},
			},
		},
	})
	for _, want := range []string{"서버 운영판단", "자동 안전조치 후보: 1개", "조치 후보", "meshclaw autoheal-apply-safe", "기본 모드는 read-only"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in ops control output: %s", want, text)
		}
	}
}

func TestFormatExplainedFleetInventory(t *testing.T) {
	text := FormatExplainedResult(map[string]interface{}{
		"route": "fleet_inventory",
		"result": map[string]interface{}{
			"report": map[string]interface{}{
				"failures": 0,
				"hosts": []map[string]interface{}{
					{"host": "v2", "status": "ok", "os": "Ubuntu", "tools": map[string]interface{}{"docker": "Docker version 1", "tailscale": "1.2"}, "containers": []interface{}{"radio image"}, "gpu": []interface{}{}},
					{"host": "g1", "status": "ok", "os": "Ubuntu", "tools": map[string]interface{}{"nvidia-smi": "NVIDIA-SMI"}, "gpu": []interface{}{"RTX 4090, 24564 MiB"}},
				},
			},
		},
	})
	for _, want := range []string{"노드별 설치 상태", "v2", "tools=tailscale,docker", "g1"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in fleet inventory output: %s", want, text)
		}
	}
}

func TestFormatExplainedNodeInventory(t *testing.T) {
	text := FormatExplainedResult(map[string]interface{}{
		"route": "node_inventory",
		"result": map[string]interface{}{
			"report": map[string]interface{}{
				"host":       "v2",
				"status":     "ok",
				"os":         "Ubuntu",
				"kernel":     "6.8",
				"arch":       "x86_64",
				"tools":      map[string]interface{}{"meshclaw": "meshclaw 1.2", "vssh": "vssh 4.2"},
				"services":   []interface{}{"tailscaled.service"},
				"containers": []interface{}{},
				"gpu":        []interface{}{},
			},
		},
	})
	for _, want := range []string{"v2에 설치된", "OS: Ubuntu", "meshclaw", "vssh"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in node inventory output: %s", want, text)
		}
	}
}

func TestFormatExplainedPlacementPlan(t *testing.T) {
	text := FormatExplainedResult(map[string]interface{}{
		"route": "placement_plan",
		"result": map[string]interface{}{
			"report": map[string]interface{}{
				"workload": "GPU 추론",
				"class":    "gpu",
				"candidates": []map[string]interface{}{
					{"host": "g1", "score": 120, "reasons": []interface{}{"online, disk 40.0%, memory 9.0%", "GPU 1개 확인", "docker 있음"}, "actions": []interface{}{"meshclaw node-inventory g1"}},
				},
				"rejected": []map[string]interface{}{{"host": "v2", "reason": "GPU 작업인데 GPU가 없음"}},
			},
		},
	})
	for _, want := range []string{"gpu", "추천", "g1", "GPU", "제외된"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in placement output: %s", want, text)
		}
	}
}
