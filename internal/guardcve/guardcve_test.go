package guardcve

import "testing"

func TestParseInventory(t *testing.T) {
	stdout := `
deb|openssl|3.0.2-0ubuntu1.10
PyPI|requests|2.25.1
npm|@scope/cli|1.4.0
npm|lodash|4.17.21
brew|git|2.39.0
crates.io|ripgrep|14.1.0
garbage line with no pipes
unknownEco|foo|1.0
deb||1.0
deb|emptyver|`
	pkgs := ParseInventory(stdout)
	if len(pkgs) != 6 {
		t.Fatalf("expected 6 packages, got %d: %+v", len(pkgs), pkgs)
	}
	byName := map[string]Package{}
	for _, p := range pkgs {
		byName[p.Name] = p
	}
	if p, ok := byName["@scope/cli"]; !ok || p.Ecosystem != "npm" || p.Version != "1.4.0" {
		t.Errorf("scoped npm package parsed wrong: %+v", byName["@scope/cli"])
	}
	if p := byName["openssl"]; p.Ecosystem != "deb" || p.Version != "3.0.2-0ubuntu1.10" {
		t.Errorf("deb package parsed wrong: %+v", p)
	}
	if _, ok := byName["foo"]; ok {
		t.Errorf("unknown ecosystem should be dropped")
	}
}

func TestScanOfflineAndEmpty(t *testing.T) {
	empty := Scan("h1", nil, Options{Offline: true})
	if empty.Status != "empty" {
		t.Errorf("expected empty status, got %q", empty.Status)
	}

	pkgs := []Package{{Ecosystem: "deb", Name: "openssl", Version: "3.0.0"}}
	off := Scan("h1", pkgs, Options{Offline: true})
	if off.Status != "clean" {
		t.Errorf("offline non-empty should be clean, got %q", off.Status)
	}
	if off.PackageCount != 1 || off.ByEcosystem["deb"] != 1 {
		t.Errorf("counts wrong: %+v", off)
	}
}

func TestEcosystemFilter(t *testing.T) {
	pkgs := []Package{
		{Ecosystem: "deb", Name: "a", Version: "1"},
		{Ecosystem: "PyPI", Name: "b", Version: "2"},
	}
	r := Scan("h1", pkgs, Options{Offline: true, Ecosystems: []string{"PyPI"}})
	if r.PackageCount != 1 || r.ByEcosystem["PyPI"] != 1 || r.ByEcosystem["deb"] != 0 {
		t.Errorf("ecosystem filter failed: %+v", r.ByEcosystem)
	}
}

func TestSeverityFrom(t *testing.T) {
	high := severityFrom(&osvDetail{Summary: "Remote code execution in foo"})
	if high != "high" {
		t.Errorf("expected high, got %q", high)
	}
	warn := severityFrom(&osvDetail{Severity: []struct {
		Type  string `json:"type"`
		Score string `json:"score"`
	}{{Type: "CVSS_V3", Score: "CVSS:3.1/AV:N/AC:L/PR:N"}}})
	if warn != "warn" {
		t.Errorf("expected warn, got %q", warn)
	}
	info := severityFrom(&osvDetail{Summary: "minor typo fix"})
	if info != "info" {
		t.Errorf("expected info, got %q", info)
	}
}

func TestInventoryCommandShape(t *testing.T) {
	cmd := InventoryCommand()
	for _, want := range []string{"dpkg-query", "pip3", "npm ls -g", "brew list", "cargo install --list"} {
		if !contains(cmd, want) {
			t.Errorf("inventory command missing %q", want)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
