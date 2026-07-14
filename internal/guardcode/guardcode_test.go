package guardcode

import "testing"

const semgrepSample = `{"results":[
  {"check_id":"python.lang.security.ssrf","path":"app/net.py","start":{"line":42},"extra":{"severity":"ERROR","message":"possible SSRF","lines":"requests.get(url)"}},
  {"check_id":"generic.style.todo","path":"app/x.py","start":{"line":3},"extra":{"severity":"INFO","message":"todo found"}}
]}`

const banditSample = `{"results":[
  {"test_id":"B602","filename":"app/run.py","line_number":10,"issue_severity":"HIGH","issue_text":"subprocess with shell=True","code":"subprocess.call(cmd, shell=True)"},
  {"test_id":"B101","filename":"app/run.py","line_number":20,"issue_severity":"LOW","issue_text":"assert used"}
]}`

func TestParseSemgrep(t *testing.T) {
	f := ParseSemgrep(semgrepSample)
	if len(f) != 2 {
		t.Fatalf("expected 2 semgrep findings, got %d", len(f))
	}
	if f[0].Severity != "high" || f[0].RuleID != "python.lang.security.ssrf" || f[0].Line != 42 {
		t.Errorf("semgrep ERROR not mapped to high: %+v", f[0])
	}
	if f[1].Severity != "info" {
		t.Errorf("semgrep INFO not mapped to info: %+v", f[1])
	}
}

func TestParseBandit(t *testing.T) {
	f := ParseBandit(banditSample)
	if len(f) != 2 {
		t.Fatalf("expected 2 bandit findings, got %d", len(f))
	}
	if f[0].Severity != "high" || f[0].Tool != "bandit" {
		t.Errorf("bandit HIGH not mapped to high: %+v", f[0])
	}
	if f[1].Severity != "info" {
		t.Errorf("bandit LOW not mapped to info: %+v", f[1])
	}
}

func TestBuildReportFindings(t *testing.T) {
	stdout := "###SEMGREP###\n" + semgrepSample + "\n###SEMGREP_END###\n###BANDIT###\n" + banditSample + "\n###BANDIT_END###\n"
	r := BuildReport("d1", "/srv/app", stdout, true)
	if r.Status != "findings" {
		t.Fatalf("expected findings status, got %q", r.Status)
	}
	if r.FindingCount != 4 {
		t.Errorf("expected 4 findings, got %d", r.FindingCount)
	}
	if r.BySeverity["high"] != 2 {
		t.Errorf("expected 2 high, got %d", r.BySeverity["high"])
	}
	if r.ByTool["semgrep"] != 2 || r.ByTool["bandit"] != 2 {
		t.Errorf("by_tool wrong: %+v", r.ByTool)
	}
	if len(r.HighSeverity) != 2 {
		t.Errorf("expected 2 high-severity entries, got %d", len(r.HighSeverity))
	}
	// findings sorted high-first
	if r.Findings[0].Severity != "high" {
		t.Errorf("findings not severity-sorted: %+v", r.Findings[0])
	}
}

func TestBuildReportCleanAndFailed(t *testing.T) {
	clean := BuildReport("d1", "/srv/app", "###SEMGREP###\n{}\n###SEMGREP_END###\n###BANDIT###\n{}\n###BANDIT_END###\n", true)
	if clean.Status != "clean" {
		t.Errorf("expected clean, got %q", clean.Status)
	}
	failed := BuildReport("d1", "/srv/app", "", false)
	if failed.Status != "failed" {
		t.Errorf("expected failed, got %q", failed.Status)
	}
}

func TestScanCommandShape(t *testing.T) {
	cmd := ScanCommand("/srv/my repo")
	for _, want := range []string{"semgrep", "bandit -r -f json", markSemgrep, markBanditEnd, "'/srv/my repo'"} {
		if !contains(cmd, want) {
			t.Errorf("scan command missing %q", want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
