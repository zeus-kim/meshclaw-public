// Package guardcve implements MeshClaw Guard's vuln-mode package CVE scanner.
//
// It collects a fleet node's installed-package inventory (deb / PyPI / npm /
// Homebrew / cargo) and matches it against the OSV.dev vulnerability database.
// This capability previously lived in pwagent as `vuln_scan`; it belongs in the
// MeshClaw DevOps/Guard layer because it operates on fleet hosts, not on the
// credential vault. pwagent keeps credential hygiene (security_audit); the
// host package-CVE scan is here.
//
// The package is transport-agnostic: the caller is responsible for running
// InventoryCommand() on the target host (via the MeshClaw runtime runner) and
// passing the captured stdout to ParseInventory(). Scan() then performs the
// OSV.dev lookups. This keeps guardcve free of any vssh/ssh dependency and
// easy to unit-test.
package guardcve

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Package is one installed package discovered on a host.
type Package struct {
	Ecosystem string `json:"ecosystem"` // deb | PyPI | npm | brew | crates.io
	Name      string `json:"name"`
	Version   string `json:"version"`
}

// osvEcosystem maps our inventory labels to OSV.dev ecosystem names.
var osvEcosystem = map[string]string{
	"deb":       "Debian",
	"PyPI":      "PyPI",
	"npm":       "npm",
	"brew":      "Homebrew",
	"crates.io": "crates.io",
}

// Finding is a single package/CVE match.
type Finding struct {
	Package      Package  `json:"package"`
	ID           string   `json:"id"`
	Severity     string   `json:"severity"` // high | warn | info
	Summary      string   `json:"summary,omitempty"`
	FixedVersion string   `json:"fixed_version,omitempty"`
	References   []string `json:"references,omitempty"`
}

// Report is the structured result of a host scan.
type Report struct {
	Mode         string         `json:"mode"`
	Host         string         `json:"host"`
	Status       string         `json:"status"` // clean | findings | empty | failed
	PackageCount int            `json:"package_count"`
	ByEcosystem  map[string]int `json:"by_ecosystem"`
	CVECount     int            `json:"cve_count"`
	BySeverity   map[string]int `json:"by_severity"`
	HighSeverity []Finding      `json:"high_severity"`
	Findings     []Finding      `json:"findings"`
	Errors       []string       `json:"errors,omitempty"`
	Principle    string         `json:"principle"`
}

// Options controls a scan.
type Options struct {
	Offline    bool     // skip network; report inventory only
	MaxDetail  int      // cap OSV detail fetches (severity/summary); default 40
	Ecosystems []string // optional filter; empty = all supported
	Timeout    time.Duration
}

const principle = "read-only fleet scan; package versions only, never secrets; rotation/patching is approval-gated"

// InventoryCommand returns a POSIX shell script that emits one
// "ecosystem|name|version" line per installed package. Each collector is
// independently guarded by command -v so a missing manager is simply skipped.
// Designed to run unprivileged and fail safe (no error exit on empty managers).
func InventoryCommand() string {
	return strings.TrimSpace(`
set +e
# Debian/Ubuntu packages
if command -v dpkg-query >/dev/null 2>&1; then
  dpkg-query -W -f='deb|${Package}|${Version}\n' 2>/dev/null
fi
# Python (pip) packages
if command -v pip3 >/dev/null 2>&1; then
  pip3 list --format=freeze --disable-pip-version-check 2>/dev/null \
    | grep '==' | sed 's/==/|/' | sed 's/^/PyPI|/'
fi
# Global npm packages (handles @scope/name@version)
if command -v npm >/dev/null 2>&1; then
  npm ls -g --depth=0 2>/dev/null \
    | grep -oE '@?[A-Za-z0-9._/-]+@[0-9][^ ]*' \
    | awk -F@ '{ if (NF==3) print "npm|@"$2"|"$3; else if (NF==2) print "npm|"$1"|"$2 }'
fi
# Homebrew formulae
if command -v brew >/dev/null 2>&1; then
  brew list --versions 2>/dev/null | awk 'NF>=2 { print "brew|"$1"|"$NF }'
fi
# Cargo-installed crates
if command -v cargo >/dev/null 2>&1; then
  cargo install --list 2>/dev/null | grep ':$' \
    | sed -E 's/ v([^:]+):/|\1/' | sed 's/^/crates.io|/'
fi
`)
}

// ParseInventory turns InventoryCommand stdout into a package list.
func ParseInventory(stdout string) []Package {
	var pkgs []Package
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) != 3 {
			continue
		}
		eco, name, ver := parts[0], strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
		if name == "" || ver == "" {
			continue
		}
		if _, ok := osvEcosystem[eco]; !ok {
			continue
		}
		pkgs = append(pkgs, Package{Ecosystem: eco, Name: name, Version: ver})
	}
	return pkgs
}

// --- OSV.dev wire types ---

type osvQuery struct {
	Package osvPkgRef `json:"package"`
	Version string    `json:"version"`
}

type osvPkgRef struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

type osvBatchResp struct {
	Results []struct {
		Vulns []struct {
			ID string `json:"id"`
		} `json:"vulns"`
	} `json:"results"`
}

type osvDetail struct {
	ID       string `json:"id"`
	Summary  string `json:"summary"`
	Details  string `json:"details"`
	Severity []struct {
		Type  string `json:"type"`
		Score string `json:"score"`
	} `json:"severity"`
	Affected []struct {
		Package osvPkgRef `json:"package"`
		Ranges  []struct {
			Events []map[string]string `json:"events"`
		} `json:"ranges"`
	} `json:"affected"`
	References []struct {
		URL string `json:"url"`
	} `json:"references"`
}

// queryBatch maps each package index to its list of OSV vuln IDs.
func queryBatch(client *http.Client, pkgs []Package) (map[int][]string, error) {
	queries := make([]osvQuery, 0, len(pkgs))
	idx := make([]int, 0, len(pkgs))
	for i, p := range pkgs {
		eco := osvEcosystem[p.Ecosystem]
		if eco == "" {
			continue
		}
		queries = append(queries, osvQuery{
			Package: osvPkgRef{Name: p.Name, Ecosystem: eco},
			Version: p.Version,
		})
		idx = append(idx, i)
	}
	out := map[int][]string{}
	if len(queries) == 0 {
		return out, nil
	}

	body, err := json.Marshal(map[string]interface{}{"queries": queries})
	if err != nil {
		return out, err
	}
	resp, err := client.Post("https://api.osv.dev/v1/querybatch",
		"application/json", bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	var parsed osvBatchResp
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return out, err
	}
	for i, res := range parsed.Results {
		if i >= len(idx) {
			break
		}
		var ids []string
		for _, v := range res.Vulns {
			if v.ID != "" {
				ids = append(ids, v.ID)
			}
		}
		if len(ids) > 0 {
			out[idx[i]] = ids
		}
	}
	return out, nil
}

func fetchDetail(client *http.Client, id string) *osvDetail {
	resp, err := client.Get("https://api.osv.dev/v1/vulns/" + id)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var d osvDetail
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil
	}
	return &d
}

// severityFrom classifies a vuln. OSV severity is usually a CVSS vector string
// (not a bare number), so we treat keyword-level signals as high, the presence
// of a CVSS_V3 vector as warn, and everything else as info.
func severityFrom(d *osvDetail) string {
	text := strings.ToLower(d.Summary + " " + d.Details)
	for _, kw := range []string{"remote code", "rce", "arbitrary code", "auth bypass", "authentication bypass", "privilege escalation"} {
		if strings.Contains(text, kw) {
			return "high"
		}
	}
	for _, s := range d.Severity {
		if strings.Contains(strings.ToUpper(s.Score), "CVSS") {
			return "warn"
		}
	}
	return "info"
}

func fixedVersionFrom(d *osvDetail, ecosystem, name string) string {
	for _, aff := range d.Affected {
		if aff.Package.Ecosystem != ecosystem || aff.Package.Name != name {
			continue
		}
		for _, rng := range aff.Ranges {
			for _, evt := range rng.Events {
				if fixed, ok := evt["fixed"]; ok && fixed != "" {
					return fixed
				}
			}
		}
	}
	return ""
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Scan runs the full pipeline against an already-collected package list.
func Scan(host string, pkgs []Package, opts Options) Report {
	if opts.MaxDetail == 0 {
		opts.MaxDetail = 40
	}
	if opts.Timeout == 0 {
		opts.Timeout = 20 * time.Second
	}

	// optional ecosystem filter
	if len(opts.Ecosystems) > 0 {
		allow := map[string]bool{}
		for _, e := range opts.Ecosystems {
			allow[strings.TrimSpace(e)] = true
		}
		filtered := pkgs[:0:0]
		for _, p := range pkgs {
			if allow[p.Ecosystem] {
				filtered = append(filtered, p)
			}
		}
		pkgs = filtered
	}

	report := Report{
		Mode:         "vuln",
		Host:         host,
		PackageCount: len(pkgs),
		ByEcosystem:  map[string]int{},
		BySeverity:   map[string]int{},
		Findings:     []Finding{},
		HighSeverity: []Finding{},
		Principle:    principle,
	}
	for _, p := range pkgs {
		report.ByEcosystem[p.Ecosystem]++
	}

	if len(pkgs) == 0 {
		report.Status = "empty"
		report.Errors = append(report.Errors, "no installed packages discovered (no supported package manager, or host unreachable)")
		return report
	}

	if opts.Offline {
		report.Status = "clean"
		report.Errors = append(report.Errors, "offline mode: inventory only, OSV.dev not queried")
		return report
	}

	client := &http.Client{Timeout: opts.Timeout}
	batch, err := queryBatch(client, pkgs)
	if err != nil {
		report.Status = "failed"
		report.Errors = append(report.Errors, "OSV.dev query failed: "+err.Error())
		return report
	}

	detailBudget := opts.MaxDetail
	for i, p := range pkgs {
		ids := batch[i]
		for _, id := range ids {
			f := Finding{Package: p, ID: id, Severity: "warn"}
			if detailBudget > 0 {
				if d := fetchDetail(client, id); d != nil {
					f.Severity = severityFrom(d)
					f.Summary = truncate(d.Summary, 200)
					if f.Summary == "" {
						f.Summary = truncate(d.Details, 200)
					}
					f.FixedVersion = fixedVersionFrom(d, osvEcosystem[p.Ecosystem], p.Name)
					for _, ref := range d.References {
						if ref.URL != "" {
							f.References = append(f.References, ref.URL)
						}
						if len(f.References) >= 3 {
							break
						}
					}
				}
				detailBudget--
			}
			report.Findings = append(report.Findings, f)
			report.BySeverity[f.Severity]++
			if f.Severity == "high" {
				report.HighSeverity = append(report.HighSeverity, f)
			}
		}
	}

	report.CVECount = len(report.Findings)
	sort.SliceStable(report.Findings, func(a, b int) bool {
		return severityRank(report.Findings[a].Severity) < severityRank(report.Findings[b].Severity)
	})
	if report.CVECount == 0 {
		report.Status = "clean"
	} else {
		report.Status = "findings"
	}
	return report
}

func severityRank(s string) int {
	switch s {
	case "high":
		return 0
	case "warn":
		return 1
	default:
		return 2
	}
}
