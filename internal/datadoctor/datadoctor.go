package datadoctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Report struct {
	Kind       string        `json:"kind"`
	Home       string        `json:"home"`
	Generated  time.Time     `json:"generated"`
	OK         bool          `json:"ok"`
	Locations  []Location    `json:"locations"`
	Evidence   EvidenceState `json:"evidence"`
	StateFiles []StateFile   `json:"state_files,omitempty"`
	Warnings   []string      `json:"warnings,omitempty"`
	Next       []string      `json:"next,omitempty"`
	PolicyNote string        `json:"policy_note"`
}

type Location struct {
	ID          string    `json:"id"`
	Path        string    `json:"path"`
	Exists      bool      `json:"exists"`
	Files       int       `json:"files"`
	Bytes       int64     `json:"bytes"`
	Oldest      time.Time `json:"oldest,omitempty"`
	Newest      time.Time `json:"newest,omitempty"`
	Retention   string    `json:"retention"`
	AutoCleaned bool      `json:"auto_cleaned"`
}

type StateFile struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	Bytes  int64  `json:"bytes"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}

type EvidenceState struct {
	Files       int               `json:"files"`
	Bytes       int64             `json:"bytes"`
	Latest      []EvidenceSummary `json:"latest,omitempty"`
	ByKind      map[string]int    `json:"by_kind,omitempty"`
	ArchiveOnly bool              `json:"archive_only"`
}

type EvidenceSummary struct {
	ID      string    `json:"id"`
	Kind    string    `json:"kind"`
	Time    time.Time `json:"time"`
	Path    string    `json:"path"`
	Summary string    `json:"summary,omitempty"`
}

type ArchivePlan struct {
	Kind           string             `json:"kind"`
	Home           string             `json:"home"`
	Generated      time.Time          `json:"generated"`
	EvidenceRoot   string             `json:"evidence_root"`
	KeepNewest     int                `json:"keep_newest"`
	TotalFiles     int                `json:"total_files"`
	TotalBytes     int64              `json:"total_bytes"`
	CandidateFiles int                `json:"candidate_files"`
	CandidateBytes int64              `json:"candidate_bytes"`
	Candidates     []ArchiveCandidate `json:"candidates"`
	CommandHint    string             `json:"command_hint"`
	ApprovalNote   string             `json:"approval_note"`
}

type ArchiveCandidate struct {
	Date   string    `json:"date"`
	Path   string    `json:"path"`
	Files  int       `json:"files"`
	Bytes  int64     `json:"bytes"`
	Oldest time.Time `json:"oldest,omitempty"`
	Newest time.Time `json:"newest,omitempty"`
}

func Check(now time.Time) (Report, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Report{}, err
	}
	report := Report{
		Kind:       "meshclaw_data_doctor",
		Home:       home,
		Generated:  now.UTC(),
		OK:         true,
		PolicyNote: "ephemeral public and doctor files are auto-cleaned; evidence and logs are counted and warned but not deleted automatically",
	}
	root := filepath.Join(home, ".meshclaw")
	locations := []Location{
		scanLocation("public_argos", filepath.Join(root, "public", "argos"), "delete files older than 48h except index.html; keep newest 100 files", true),
		scanLocation("doctor", filepath.Join(root, "doctor"), "delete files older than 7d; keep newest 20 files", true),
		scanLocation("logs", filepath.Join(root, "logs"), "count and warn only; configure rotation if large", false),
		scanStateLocation(root),
	}
	report.Locations = locations
	for _, loc := range locations {
		switch loc.ID {
		case "public_argos":
			if loc.Files > 120 {
				report.Warnings = append(report.Warnings, "public Argos files exceed expected cap; run local-hygiene")
			}
		case "doctor":
			if loc.Files > 30 {
				report.Warnings = append(report.Warnings, "doctor/recording files exceed expected cap; run local-hygiene")
			}
		case "logs":
			if loc.Bytes > 1024*1024*1024 {
				report.Warnings = append(report.Warnings, "logs exceed 1GB; review log rotation")
			}
		}
	}
	report.Evidence = scanEvidence(filepath.Join(root, "evidence"))
	report.StateFiles = scanStateFiles(root)
	for _, file := range report.StateFiles {
		if file.Exists && !file.OK {
			report.Warnings = append(report.Warnings, "state file is not valid JSON: "+file.ID)
			report.Next = append(report.Next, "repair or restore "+file.Path+" before relying on unattended automation")
		}
	}
	if report.Evidence.Files > 5000 {
		report.Warnings = append(report.Warnings, "evidence files exceed 5000; keep for audit, but add archival policy")
		report.Next = append(report.Next, "archive old evidence to a dated bundle instead of deleting it")
	}
	if len(report.Warnings) > 0 {
		report.OK = false
	}
	if len(report.Next) == 0 {
		report.Next = append(report.Next, "keep scheduled local-hygiene running")
	}
	return report, nil
}

func EvidenceArchivePlan(now time.Time, keepNewest int) (ArchivePlan, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if keepNewest <= 0 {
		keepNewest = 14
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ArchivePlan{}, err
	}
	root := filepath.Join(home, ".meshclaw", "evidence")
	plan := ArchivePlan{
		Kind:         "meshclaw_evidence_archive_plan",
		Home:         home,
		Generated:    now.UTC(),
		EvidenceRoot: root,
		KeepNewest:   keepNewest,
		CommandHint:  "Review candidates, then archive date directories into a compressed bundle. Do not delete evidence without a separate approved archive/apply step.",
		ApprovalNote: "Plan-only. This tool does not write, compress, move, or delete evidence files.",
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return plan, nil
		}
		return plan, err
	}
	dirs := []ArchiveCandidate{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		date := entry.Name()
		if len(date) != len("2006-01-02") {
			continue
		}
		path := filepath.Join(root, date)
		candidate := scanArchiveCandidate(date, path)
		if candidate.Files == 0 {
			continue
		}
		plan.TotalFiles += candidate.Files
		plan.TotalBytes += candidate.Bytes
		dirs = append(dirs, candidate)
	}
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].Date > dirs[j].Date
	})
	if len(dirs) <= keepNewest {
		return plan, nil
	}
	plan.Candidates = append(plan.Candidates, dirs[keepNewest:]...)
	for _, candidate := range plan.Candidates {
		plan.CandidateFiles += candidate.Files
		plan.CandidateBytes += candidate.Bytes
	}
	return plan, nil
}

func scanArchiveCandidate(date, path string) ArchiveCandidate {
	item := ArchiveCandidate{Date: date, Path: path}
	_ = filepath.WalkDir(path, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry == nil || entry.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		item.Files++
		item.Bytes += info.Size()
		if item.Oldest.IsZero() || info.ModTime().Before(item.Oldest) {
			item.Oldest = info.ModTime()
		}
		if item.Newest.IsZero() || info.ModTime().After(item.Newest) {
			item.Newest = info.ModTime()
		}
		return nil
	})
	return item
}

func scanLocation(id, path, retention string, autoCleaned bool) Location {
	loc := Location{ID: id, Path: path, Retention: retention, AutoCleaned: autoCleaned}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return loc
	}
	loc.Exists = true
	_ = filepath.WalkDir(path, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry == nil || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		loc.Files++
		loc.Bytes += info.Size()
		if loc.Oldest.IsZero() || info.ModTime().Before(loc.Oldest) {
			loc.Oldest = info.ModTime()
		}
		if loc.Newest.IsZero() || info.ModTime().After(loc.Newest) {
			loc.Newest = info.ModTime()
		}
		return nil
	})
	return loc
}

func scanStateLocation(root string) Location {
	loc := Location{
		ID:          "state",
		Path:        root,
		Retention:   "top-level configuration and runtime state; never auto-delete blindly",
		AutoCleaned: false,
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return loc
	}
	loc.Exists = true
	entries, err := os.ReadDir(root)
	if err != nil {
		return loc
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		loc.Files++
		loc.Bytes += info.Size()
		if loc.Oldest.IsZero() || info.ModTime().Before(loc.Oldest) {
			loc.Oldest = info.ModTime()
		}
		if loc.Newest.IsZero() || info.ModTime().After(loc.Newest) {
			loc.Newest = info.ModTime()
		}
	}
	return loc
}

func scanEvidence(root string) EvidenceState {
	state := EvidenceState{ByKind: map[string]int{}, ArchiveOnly: true}
	var latest []EvidenceSummary
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry == nil || entry.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		state.Files++
		state.Bytes += info.Size()
		id := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		kind := evidenceKindFromID(id)
		state.ByKind[kind]++
		latest = append(latest, EvidenceSummary{ID: id, Kind: kind, Time: info.ModTime(), Path: path})
		return nil
	})
	sort.Slice(latest, func(i, j int) bool {
		return latest[i].Time.After(latest[j].Time)
	})
	if len(latest) > 10 {
		latest = latest[:10]
	}
	state.Latest = latest
	if len(state.ByKind) == 0 {
		state.ByKind = nil
	}
	return state
}

func scanStateFiles(root string) []StateFile {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	files := []StateFile{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(root, entry.Name())
		item := StateFile{ID: strings.TrimSuffix(entry.Name(), ".json"), Path: path}
		info, err := entry.Info()
		if err != nil {
			item.Error = err.Error()
			files = append(files, item)
			continue
		}
		item.Exists = true
		item.Bytes = info.Size()
		data, err := os.ReadFile(path)
		if err != nil {
			item.Error = err.Error()
		} else if !json.Valid(data) {
			item.Error = "invalid json"
		} else {
			item.OK = true
		}
		files = append(files, item)
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].ID < files[j].ID
	})
	return files
}

func evidenceKindFromID(id string) string {
	parts := strings.Split(id, "-")
	if len(parts) < 3 {
		return "unknown"
	}
	kind := parts[2:]
	if len(kind) > 1 && isEvidenceHostOrMode(kind[len(kind)-1]) {
		kind = kind[:len(kind)-1]
	}
	if len(kind) == 0 {
		return "unknown"
	}
	return strings.Join(kind, "-")
}

func isEvidenceHostOrMode(value string) bool {
	switch value {
	case "ops", "assistant", "briefing", "fleet", "mac":
		return true
	default:
		return false
	}
}
