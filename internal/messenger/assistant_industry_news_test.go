package messenger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssistantIndustryLatestNewsBriefPharma(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	t.Setenv("MESHCLAW_ASSISTANT_RESEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_DART_API_KEY", "")
	t.Setenv("OPENDART_API_KEY", "")
	t.Setenv("DART_API_KEY", "")
	t.Setenv("MESHCLAW_SEC_USER_AGENT", "")
	t.Setenv("SEC_USER_AGENT", "")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "제약회사 최신 뉴스 찾아서 정리해줘")
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"제약/바이오 최신 뉴스 브리프",
		"최신 이슈 TOP 5",
		"임상/허가",
		"약가/급여",
		"기술수출/M&A",
		"특허/소송/바이오시밀러",
		"다음 브리프에서 추적할 지표",
		"PDUFA",
		"upfront",
		"처방량",
		"공식 공시 소스",
		"DART",
		"SEC",
		"공개 웹 공시는 브라우저에서 바로 확인",
		"https://dart.fss.or.kr/dsab007/main.do",
		"https://www.sec.gov/edgar/search/",
		"회의에서 바로 확인할 질문",
		"매일 오전 8시에 보고방에 제약/바이오 최신 뉴스 브리프 보내줘",
		"예약 등록 승인",
		"제약/바이오 최신 뉴스 브리프를 스킬로 만들어줘",
		"제약/바이오 최신 뉴스 브리프 작업을 재사용해줘",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("pharma brief missing %q:\n%s", want, visible)
		}
	}
	for _, unexpected := range []string{"검색 기반 리서치 노트", "meshclaw-attachment", "미설정", home, os.Getenv("HOME") + "/"} {
		if unexpected != "" && strings.Contains(reply, unexpected) {
			t.Fatalf("pharma brief exposed unexpected %q:\n%s", unexpected, reply)
		}
	}
}

func TestAssistantIndustryLatestNewsBriefSemiconductor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	t.Setenv("MESHCLAW_ASSISTANT_RESEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_DART_API_KEY", "")
	t.Setenv("MESHCLAW_SEC_USER_AGENT", "")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "반도체 장비 최신 뉴스 정리해줘")
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"반도체/장비 최신 뉴스 브리프",
		"HBM",
		"수출통제",
		"CAPEX",
		"다음 브리프에서 추적할 지표",
		"DRAM/NAND ASP",
		"장비 리드타임",
		"공식 공시 소스",
		"DART",
		"SEC",
		"공개 웹 공시는 브라우저에서 바로 확인",
		"https://dart.fss.or.kr/dsab007/main.do",
		"https://www.sec.gov/edgar/search/",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("semiconductor brief missing %q:\n%s", want, visible)
		}
	}
}

func TestAssistantIndustryNewsMeetingMaterialsPackage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	t.Setenv("MESHCLAW_ASSISTANT_RESEARCH_DISABLE", "1")
	t.Setenv("MESHCLAW_SKIP_PREVIEW_IMAGE", "1")

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "제약회사 최신 뉴스 DOCX/PPTX 회의자료로 만들어줘")
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"회의 자료 패키지를 준비했습니다.",
		"회의 브리프는 Word/Pages 문서로 만들었습니다.",
		"발표자료는 PPTX로 만들었습니다.",
		"제약/바이오 최신 뉴스 회의자료",
		"패키지에 포함: 업종 이슈, 추적 지표",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("industry meeting materials missing %q:\n%s", want, visible)
		}
	}
	if len(signalReplyAttachments(reply)) == 0 {
		t.Fatalf("industry meeting materials should include attachments:\n%s", reply)
	}
}

func TestAssistantIndustryNewsNaturalSchedulingPlan(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	t.Setenv("MESHCLAW_SCHEDULED_DELIVERIES", filepath.Join(home, ".meshclaw", "scheduled-deliveries.json"))
	if _, _, err := UpsertTarget(Target{
		ID:      "argos-briefing",
		Channel: "signal",
		GroupID: "group.argos-briefing",
		Label:   "보고방",
		Mode:    "briefing",
	}); err != nil {
		t.Fatal(err)
	}

	opts := ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}
	reply := assistantReply(opts, "매일 오전 8시에 보고방에 제약/바이오 최신 뉴스 브리프 보내줘")
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"예약 발송 계획을 만들었습니다.",
		"대상: 보고방",
		"매일 오전 8시",
		"내용: 제약/바이오 최신 뉴스 브리프",
		"아직 예약 등록이나 발송은 하지 않았습니다.",
		"예약 등록 승인",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("industry natural scheduling missing %q:\n%s", want, visible)
		}
	}
	for _, bad := range []string{"증거:", "/Users/", "/tmp/", "evidence"} {
		if strings.Contains(visible, bad) {
			t.Fatalf("industry natural scheduling exposed noisy detail %q:\n%s", bad, visible)
		}
	}
	registered := signalReplyVisibleText(assistantReply(opts, "예약 등록 승인"))
	for _, want := range []string{
		"예약 발송을 등록했습니다.",
		"대상: 보고방",
		"주기: 매일 오전 8시에",
		"내용: 제약/바이오 최신 뉴스 브리프",
		"지금 즉시 발송하지 않았습니다.",
	} {
		if !strings.Contains(registered, want) {
			t.Fatalf("industry natural scheduling registration missing %q:\n%s", want, registered)
		}
	}
}

func TestAssistantIndustryNewsSkillSuggestionCreatesDraft(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "제약/바이오 최신 뉴스 브리프를 스킬로 만들어줘")
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"스킬 설치 초안",
		"Pharma Biotech News Brief",
		"pharma-biotech-news-brief",
		"제약/바이오 최신 뉴스 브리프",
		"DART/SEC 공개 공시 확인",
		"지금은 파일을 쓰지 않았습니다",
		"스킬 저장해",
		"스킬 테스트",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("industry skill suggestion missing %q:\n%s", want, visible)
		}
	}
}

func TestAssistantIndustryNewsReuseSuggestionShowsCard(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	reply := assistantReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "제약/바이오 최신 뉴스 브리프 작업을 재사용해줘")
	visible := signalReplyVisibleText(reply)
	for _, want := range []string{
		"작업 학습 및 재사용 카드",
		"요청: 제약/바이오 최신 뉴스 브리프",
		"개인 스킬 상태:",
		"스킬 초안 만들기:",
		"기존 스킬 찾기:",
		"이 카드는 읽기 전용입니다",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("industry reuse suggestion missing %q:\n%s", want, visible)
		}
	}
}

func TestAssistantIndustryLatestNewsDoesNotHijackGenericDailyNews(t *testing.T) {
	if _, handled := assistantIndustryLatestNewsReply(ListenOptions{TargetID: "argos-assistant", Mode: "assistant"}, "오늘 주요뉴스 정리해줘"); handled {
		t.Fatal("industry latest-news router should not handle generic daily news")
	}
}
