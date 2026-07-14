package fileorg

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type DownloadsPlan struct {
	Kind           string               `json:"kind"`
	Generated      time.Time            `json:"generated"`
	Status         string               `json:"status"`
	Path           string               `json:"path"`
	MinAgeDays     int                  `json:"min_age_days"`
	LargeMB        int                  `json:"large_mb"`
	AccessError    string               `json:"access_error,omitempty"`
	UserMessage    string               `json:"user_message"`
	AccessGuidance []string             `json:"access_guidance,omitempty"`
	ReportTitle    string               `json:"report_title"`
	ScannedFiles   int                  `json:"scanned_files"`
	ScannedBytes   int64                `json:"scanned_bytes"`
	CandidateFiles int                  `json:"candidate_files"`
	CandidateBytes int64                `json:"candidate_bytes"`
	Candidates     []DownloadsCandidate `json:"candidates"`
	ApprovalNote   string               `json:"approval_note"`
	Next           []string             `json:"next"`
}

type DownloadsCandidate struct {
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	AgeDays  int       `json:"age_days"`
	Category string    `json:"category"`
	Reason   string    `json:"reason"`
}

type DownloadsApplyPlan struct {
	Kind             string                 `json:"kind"`
	Generated        time.Time              `json:"generated"`
	Status           string                 `json:"status"`
	Operation        string                 `json:"operation"`
	Destination      string                 `json:"destination"`
	RequestedPaths   []string               `json:"requested_paths"`
	Approved         bool                   `json:"approved"`
	Execute          bool                   `json:"execute"`
	UserMessage      string                 `json:"user_message"`
	ApprovalNote     string                 `json:"approval_note"`
	StopBefore       []string               `json:"stop_before"`
	PlannedMoves     []DownloadsMovePreview `json:"planned_moves"`
	Moved            []DownloadsMoveResult  `json:"moved,omitempty"`
	Skipped          []DownloadsMoveResult  `json:"skipped,omitempty"`
	CandidateFiles   int                    `json:"candidate_files"`
	CandidateBytes   int64                  `json:"candidate_bytes"`
	CompletedFiles   int                    `json:"completed_files,omitempty"`
	CompletedBytes   int64                  `json:"completed_bytes,omitempty"`
	ApprovalRequired bool                   `json:"approval_required"`
	ApprovalMissing  bool                   `json:"approval_missing,omitempty"`
}

type DownloadsMovePreview struct {
	Path        string `json:"path"`
	Destination string `json:"destination"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	Reason      string `json:"reason,omitempty"`
}

type DownloadsMoveResult struct {
	Path        string `json:"path"`
	Destination string `json:"destination,omitempty"`
	Name        string `json:"name,omitempty"`
	Size        int64  `json:"size,omitempty"`
	OK          bool   `json:"ok"`
	Error       string `json:"error,omitempty"`
}

func DownloadsCleanupPlan(now time.Time, dir string, minAgeDays, largeMB, limit int) (DownloadsPlan, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if minAgeDays <= 0 {
		minAgeDays = 30
	}
	if largeMB <= 0 {
		largeMB = 500
	}
	if limit <= 0 {
		limit = 50
	}
	if strings.TrimSpace(dir) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return DownloadsPlan{}, err
		}
		dir = filepath.Join(home, "Downloads")
	}
	plan := DownloadsPlan{
		Kind:         "meshclaw_downloads_cleanup_plan",
		Generated:    now.UTC(),
		Status:       "scanning",
		Path:         dir,
		MinAgeDays:   minAgeDays,
		LargeMB:      largeMB,
		UserMessage:  "다운로드 폴더 정리 후보를 검토용으로만 확인합니다.",
		ReportTitle:  "Downloads cleanup review",
		ApprovalNote: "Plan-only. This tool does not move, archive, rename, or delete files.",
		Next: []string{
			"Review candidates before moving or deleting anything.",
			"Create an explicit approved archive/apply step for any mutation.",
		},
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			plan.Status = "path_missing"
			plan.UserMessage = "지정한 폴더가 없어 정리 후보를 스캔하지 못했습니다. 다른 폴더 경로를 지정해 다시 실행하세요."
			return plan, nil
		}
		if os.IsPermission(err) {
			plan.Status = "needs_access"
			plan.AccessError = err.Error()
			plan.UserMessage = "다운로드 폴더 접근 권한이 없어 정리 후보를 스캔하지 못했습니다. 시스템 설정에서 파일 접근 권한을 허용하거나 읽을 수 있는 폴더 경로를 지정해야 합니다."
			plan.AccessGuidance = []string{
				"Open System Settings > Privacy & Security > Full Disk Access.",
				"Allow the terminal, app, or helper process that runs MeshClaw.",
				"Alternatively pass a readable folder path to this tool.",
			}
			plan.Next = append([]string{
				"Grant file access for this folder or choose a readable folder path, then run the plan again.",
			}, plan.Next...)
			return plan, nil
		}
		return plan, err
	}
	largeBytes := int64(largeMB) * 1024 * 1024
	candidates := []DownloadsCandidate{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		plan.ScannedFiles++
		plan.ScannedBytes += info.Size()
		ageDays := int(now.Sub(info.ModTime()).Hours() / 24)
		category, reason, ok := downloadsCandidateReason(entry.Name(), info.Size(), ageDays, minAgeDays, largeBytes)
		if !ok {
			continue
		}
		candidates = append(candidates, DownloadsCandidate{
			Path:     path,
			Name:     entry.Name(),
			Size:     info.Size(),
			Modified: info.ModTime(),
			AgeDays:  ageDays,
			Category: category,
			Reason:   reason,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Category != candidates[j].Category {
			return candidatePriority(candidates[i].Category) < candidatePriority(candidates[j].Category)
		}
		if candidates[i].Size != candidates[j].Size {
			return candidates[i].Size > candidates[j].Size
		}
		return candidates[i].Modified.Before(candidates[j].Modified)
	})
	for _, candidate := range candidates {
		plan.CandidateFiles++
		plan.CandidateBytes += candidate.Size
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	plan.Candidates = candidates
	if plan.CandidateFiles == 0 {
		plan.Status = "no_candidates"
		plan.UserMessage = "현재 기준으로 정리 후보가 없습니다."
	} else {
		plan.Status = "review_ready"
		plan.UserMessage = "정리 후보를 찾았습니다. 삭제나 이동은 하지 않았으니 먼저 후보 목록을 검토하세요."
	}
	return plan, nil
}

func DownloadsCleanupApply(now time.Time, paths []string, destination string, execute, approve bool) (DownloadsApplyPlan, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	cleanPaths := normalizeApplyPaths(paths)
	if strings.TrimSpace(destination) == "" {
		destination = defaultCleanupDestination(now, cleanPaths)
	}
	plan := DownloadsApplyPlan{
		Kind:             "meshclaw_downloads_cleanup_apply",
		Generated:        now.UTC(),
		Status:           "review_required",
		Operation:        "move_to_review_folder",
		Destination:      destination,
		RequestedPaths:   cleanPaths,
		Approved:         approve,
		Execute:          execute,
		UserMessage:      "승인된 파일만 리뷰 폴더로 이동할 수 있습니다. 삭제, 압축, 덮어쓰기, 폴더 이동은 하지 않습니다.",
		ApprovalNote:     "Requires execute=true and approve=true. This tool only moves explicit regular-file paths into a review folder; it never deletes files.",
		StopBefore:       []string{"delete files", "empty trash", "overwrite existing files", "move folders", "archive/compress files"},
		ApprovalRequired: true,
	}
	for _, path := range cleanPaths {
		preview := DownloadsMovePreview{Path: path, Name: filepath.Base(path)}
		info, err := safeMoveCandidateInfo(path)
		if err != nil {
			preview.Reason = err.Error()
			plan.PlannedMoves = append(plan.PlannedMoves, preview)
			continue
		}
		preview.Size = info.Size()
		preview.Destination = uniqueDestinationPath(destination, filepath.Base(path))
		plan.CandidateFiles++
		plan.CandidateBytes += info.Size()
		plan.PlannedMoves = append(plan.PlannedMoves, preview)
	}
	if len(cleanPaths) == 0 {
		plan.Status = "no_paths"
		plan.UserMessage = "이동할 파일 경로가 없습니다. 먼저 정리 후보를 선택하세요."
		return plan, nil
	}
	if !execute || !approve {
		if execute && !approve {
			plan.ApprovalMissing = true
			plan.UserMessage = "파일 이동은 승인 없이는 실행하지 않습니다. 후보와 대상 폴더를 확인한 뒤 approve=true가 필요합니다."
		}
		return plan, nil
	}
	if err := os.MkdirAll(destination, 0700); err != nil {
		plan.Status = "failed"
		plan.UserMessage = "리뷰 폴더를 만들지 못해 파일을 이동하지 않았습니다."
		return plan, err
	}
	plan.Status = "completed"
	plan.UserMessage = "승인된 파일을 리뷰 폴더로 이동했습니다. 삭제는 하지 않았습니다."
	for _, preview := range plan.PlannedMoves {
		result := DownloadsMoveResult{
			Path:        preview.Path,
			Destination: preview.Destination,
			Name:        preview.Name,
			Size:        preview.Size,
		}
		if preview.Destination == "" || preview.Size <= 0 {
			result.OK = false
			result.Error = firstNonEmpty(preview.Reason, "not a movable regular file")
			plan.Skipped = append(plan.Skipped, result)
			continue
		}
		info, err := safeMoveCandidateInfo(preview.Path)
		if err != nil {
			result.OK = false
			result.Error = err.Error()
			plan.Skipped = append(plan.Skipped, result)
			continue
		}
		finalDestination := uniqueDestinationPath(destination, filepath.Base(preview.Path))
		result.Destination = finalDestination
		if err := os.Rename(preview.Path, finalDestination); err != nil {
			result.OK = false
			result.Error = err.Error()
			plan.Skipped = append(plan.Skipped, result)
			continue
		}
		result.OK = true
		result.Size = info.Size()
		plan.Moved = append(plan.Moved, result)
		plan.CompletedFiles++
		plan.CompletedBytes += info.Size()
	}
	if plan.CompletedFiles == 0 {
		plan.Status = "no_files_moved"
		plan.UserMessage = "승인된 이동을 실행했지만 이동 가능한 파일이 없었습니다."
	}
	return plan, nil
}

func normalizeApplyPaths(paths []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}

func defaultCleanupDestination(now time.Time, paths []string) string {
	base := ""
	for _, path := range paths {
		if strings.TrimSpace(path) != "" {
			base = filepath.Dir(path)
			break
		}
	}
	if base == "" {
		home, err := os.UserHomeDir()
		if err == nil && strings.TrimSpace(home) != "" {
			base = filepath.Join(home, "Downloads")
		}
	}
	if base == "" {
		base = "."
	}
	return filepath.Join(base, "Argos Cleanup Review "+now.UTC().Format("20060102T150405Z"))
}

func safeMoveCandidateInfo(path string) (os.FileInfo, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("empty path")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("folders are not moved by this tool")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("symlinks are not moved by this tool")
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("only regular files can be moved")
	}
	return info, nil
}

func uniqueDestinationPath(dir, name string) string {
	dest := filepath.Join(dir, name)
	if _, err := os.Lstat(dest); os.IsNotExist(err) {
		return dest
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; i < 1000; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, i, ext))
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, time.Now().UnixNano(), ext))
}

func downloadsCandidateReason(name string, size int64, ageDays, minAgeDays int, largeBytes int64) (string, string, bool) {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".dmg", ".pkg", ".mpkg":
		return "installer", "installer or disk image usually safe to review after installation", true
	case ".zip", ".tar", ".gz", ".tgz", ".bz2", ".xz", ".7z", ".rar":
		return "archive", "downloaded archive candidate for review", true
	}
	if size >= largeBytes {
		return "large", "large file exceeds configured size threshold", true
	}
	if ageDays >= minAgeDays {
		return "old", "file is older than configured age threshold", true
	}
	return "", "", false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func candidatePriority(category string) int {
	switch category {
	case "installer":
		return 0
	case "archive":
		return 1
	case "large":
		return 2
	case "old":
		return 3
	default:
		return 9
	}
}
