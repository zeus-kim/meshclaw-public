package argosreport

import (
	"strings"
	"testing"
)

func TestRenderMobileHTMLUsesPhoneReadableSizing(t *testing.T) {
	got := RenderMobileHTML(MobileReport{
		Title:    "회의 보고",
		Subtitle: "iPhone에서 바로 읽는 문서",
		Findings: []string{"첫 번째 결과를 충분히 큰 글씨로 보여줍니다."},
	})

	for _, want := range []string{
		`name="viewport" content="width=device-width,initial-scale=1,maximum-scale=5,viewport-fit=cover"`,
		"-webkit-text-size-adjust:100%",
		".query{font-size:18px",
		".findings{margin:0;padding-left:22px;font-size:18px",
		".flowchart{display:grid;grid-template-columns:1fr;gap:14px;counter-reset:step}",
		".flow-node{position:relative;border:1px solid #dce6f2;border-radius:8px;padding:13px 14px 13px 46px;font-size:17px",
		".flow-node:not(:last-child):after{content:\"↓\"",
		`aria-label="작업 흐름도"`,
		"overflow-wrap:anywhere",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("mobile HTML missing %q:\n%s", want, got)
		}
	}
	for _, bad := range []string{
		".findings{font-size:13px",
		".step{font-size:12px",
		".query{font-size:15px",
	} {
		if strings.Contains(got, bad) {
			t.Fatalf("mobile HTML kept too-small sizing %q:\n%s", bad, got)
		}
	}
}
