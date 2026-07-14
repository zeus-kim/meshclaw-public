package hygiene

import "testing"

func TestReportFromRemoteOutputClassifiesRedactedFindings(t *testing.T) {
	report := reportFromRemoteOutput("d1", `FILE /var/log/app.log
12:token=[REDACTED]
13:user=[EMAIL_REDACTED]
FILE /etc/app.conf
8:password=[REDACTED]`)

	if report.Status != "findings" {
		t.Fatalf("status = %q, want findings", report.Status)
	}
	if len(report.Findings) != 3 {
		t.Fatalf("findings = %d, want 3", len(report.Findings))
	}
	if report.Findings[0].Type != FindingSecretPattern || report.Findings[0].Severity != "high" {
		t.Fatalf("first finding = %#v, want high secret finding", report.Findings[0])
	}
	if report.Findings[1].Type != FindingPIIPattern {
		t.Fatalf("second finding type = %q, want pii_pattern", report.Findings[1].Type)
	}
	if len(report.Actions) != len(report.Findings) {
		t.Fatalf("actions = %d, want %d", len(report.Actions), len(report.Findings))
	}
}

func TestReportFromRemoteOutputClean(t *testing.T) {
	report := reportFromRemoteOutput("d1", "")
	if report.Status != "clean" {
		t.Fatalf("status = %q, want clean", report.Status)
	}
	if len(report.Findings) != 0 {
		t.Fatalf("findings = %d, want 0", len(report.Findings))
	}
}
