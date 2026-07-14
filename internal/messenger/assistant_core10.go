package messenger

type assistantCoreCommand struct {
	ID                     string
	Category               string
	Examples               []string
	ExpectedVisibleMarkers []string
	ExpectedAnyMarkers     []string
	ForbiddenMarkers       []string
}

func assistantCore10Commands() []assistantCoreCommand {
	commands := []assistantCoreCommand{
		{
			ID:       "capabilities",
			Category: "identity",
			Examples: []string{
				"뭐 할 수 있어?",
				"what can you do?",
			},
			ExpectedAnyMarkers:     []string{"저는 Argos", "I am Argos"},
			ExpectedVisibleMarkers: []string{"바로 해볼 예시", "승인 필요"},
			ForbiddenMarkers:       []string{"쿠팡", "구매 실행", "결제 자동화", "purchase execution"},
		},
		{
			ID:       "mail_priority",
			Category: "mail",
			Examples: []string{
				"최근 메일 요약해줘. 답장 필요한 것만 5개 뽑아줘",
			},
			ExpectedAnyMarkers:     []string{"메일 처리 우선순위", "메일 조회에 실패했습니다", "최근 메일", "확인할 메일 계정을 찾지 못했습니다"},
			ExpectedVisibleMarkers: []string{"메일"},
			ForbiddenMarkers:       []string{"발송했습니다.", "보냈습니다.", "메일을 보냈습니다"},
		},
		{
			ID:       "daily_plan",
			Category: "daily_planning",
			Examples: []string{
				"오늘 일정, 할 일, 메일, 날씨를 하루 계획으로 묶어줘",
			},
			ExpectedAnyMarkers:     []string{"오늘 하루 계획 요약", "오늘 챙길 것 요약"},
			ExpectedVisibleMarkers: []string{"일정", "할 일", "메일", "날씨", "읽기 전용"},
		},
		{
			ID:       "industry_news",
			Category: "research",
			Examples: []string{
				"제약회사 최신 뉴스 찾아서 정리해줘",
			},
			ExpectedVisibleMarkers: []string{"최신 뉴스 브리프", "공식 공시 소스", "DART", "SEC", "공개 웹 공시는 브라우저에서 바로 확인"},
		},
		{
			ID:       "industry_meeting_materials",
			Category: "documents",
			Examples: []string{
				"제약회사 최신 뉴스 DOCX/PPTX 회의자료로 만들어줘",
			},
			ExpectedVisibleMarkers: []string{"회의 자료 패키지를 준비했습니다.", "회의 브리프는 Word/Pages 문서로 만들었습니다.", "발표자료는 PPTX로 만들었습니다.", "제약/바이오 최신 뉴스 회의자료"},
		},
		{
			ID:       "scheduled_industry_brief",
			Category: "schedule",
			Examples: []string{
				"매일 오전 8시에 보고방에 제약/바이오 최신 뉴스 브리프 보내줘",
			},
			ExpectedVisibleMarkers: []string{"예약 발송 계획을 만들었습니다.", "대상: 보고방", "매일 오전 8시", "내용: 제약/바이오 최신 뉴스 브리프", "아직 예약 등록이나 발송은 하지 않았습니다.", "예약 등록 승인"},
			ForbiddenMarkers:       []string{"예약 발송을 등록했습니다.", "발송했습니다.", "보냈습니다."},
		},
		{
			ID:       "memory_create",
			Category: "memory",
			Examples: []string{
				"답변은 결론부터 짧게 한다고 기억해",
			},
			ExpectedVisibleMarkers: []string{"Argos memory에 저장할까요", "범위: assistant", "답변은 결론부터 짧게", "저장해"},
			ForbiddenMarkers:       []string{"기억했습니다."},
		},
		{
			ID:       "memory_recall",
			Category: "memory",
			Examples: []string{
				"기억 확인",
			},
			ExpectedAnyMarkers: []string{"현재 승인된 Argos memory", "아직 승인된 assistant memory가 없습니다", "Current approved Argos memory"},
			ForbiddenMarkers:   []string{"/Users/", "비밀번호 원문", "token=ghp_"},
		},
		{
			ID:       "industry_skill_draft",
			Category: "skill",
			Examples: []string{
				"제약/바이오 최신 뉴스 브리프를 스킬로 만들어줘",
			},
			ExpectedVisibleMarkers: []string{"스킬 설치 초안", "Pharma Biotech News Brief", "pharma-biotech-news-brief", "제약/바이오 최신 뉴스 브리프", "스킬 저장해"},
			ForbiddenMarkers:       []string{"스킬을 저장했습니다.", "/Users/"},
		},
		{
			ID:       "automation_list",
			Category: "automation",
			Examples: []string{
				"자동화 목록",
			},
			ExpectedAnyMarkers:     []string{"Argos 자동화 목록", "Argos 자동화 요약"},
			ExpectedVisibleMarkers: []string{"읽기 전용", "자동화"},
			ForbiddenMarkers:       []string{"중지했습니다.", "비활성화했습니다.", "삭제했습니다."},
		},
	}
	out := make([]assistantCoreCommand, len(commands))
	copy(out, commands)
	return out
}

func assistantCommercialForbiddenVisibleMarkers() []string {
	return []string{
		"/Users/",
		"/private/var/",
		"/var/",
		"/tmp/",
		".meshclaw/evidence",
		"meshclaw-attachment:",
		"진행 증거:",
		"evidence id",
		"exit status",
		"router",
		"raw log",
		"구매 완료",
		"주문 완료",
		"결제 완료",
		"purchase completed",
		"order completed",
		"payment completed",
		"최종 주문 클릭했습니다",
	}
}
