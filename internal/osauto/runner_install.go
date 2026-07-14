package osauto

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	argosUIRunnerAppName  = "Argos UI Runner.app"
	argosUIRunnerBundleID = "ai.meshclaw.argosrunner"
)

type uiRunnerHealthPayload struct {
	Executable string `json:"executable"`
	AppPath    string `json:"app_path"`
	BundleID   string `json:"bundle_id"`
	CodeSigned bool   `json:"code_signed"`
}

type UIRunnerUpdateReport struct {
	Kind          string                `json:"kind"`
	OK            bool                  `json:"ok"`
	Applied       bool                  `json:"applied"`
	StagedPath    string                `json:"staged_path,omitempty"`
	InstalledPath string                `json:"installed_path,omitempty"`
	BackupPath    string                `json:"backup_path,omitempty"`
	Before        UIRunnerInstallReport `json:"before"`
	After         UIRunnerInstallReport `json:"after"`
	QuitRunner    Result                `json:"quit_runner,omitempty"`
	Apply         Result                `json:"apply,omitempty"`
	OpenRunner    Result                `json:"open_runner,omitempty"`
	Setup         ArgosMacSetupReport   `json:"setup"`
	Doctor        ArgosMacDoctorReport  `json:"doctor"`
	Problems      []string              `json:"problems,omitempty"`
	NextActions   []string              `json:"next_actions,omitempty"`
	CreatedAt     time.Time             `json:"created_at"`
}

func ArgosUIRunnerInstallDoctor(ctx context.Context, health *Result) UIRunnerInstallReport {
	report := UIRunnerInstallReport{
		Kind:             "meshclaw_argos_ui_runner_install",
		ExpectedBundleID: argosUIRunnerBundleID,
		RecommendedPath:  recommendedUIRunnerPath(),
		CreatedAt:        time.Now().UTC(),
	}
	if runtime.GOOS != "darwin" {
		report.Problems = append(report.Problems, "Argos UI Runner install checks require macOS.")
		report.NextActions = append(report.NextActions, "Run this doctor on the Mac that owns the Argos UI session.")
		report.OK = false
		return report
	}
	report.InstalledPath = findInstalledUIRunner()
	report.Installed = report.InstalledPath != ""
	report.StagedPath = findStagedUIRunner()
	if report.StagedPath != "" {
		report.StagedBundleID = plistBundleIdentifier(filepath.Join(report.StagedPath, "Contents", "Info.plist"))
		report.StagedCodeSigned, _ = verifyCodeSignature(report.StagedPath)
		report.StagedBinarySHA256 = fileSHA256(filepath.Join(report.StagedPath, "Contents", "MacOS", "argos-ui-runner"))
		report.StagedAppSHA256 = directorySHA256(report.StagedPath)
	}
	if !report.Installed {
		report.Problems = append(report.Problems, "Stable Argos UI Runner.app is not installed in ~/Applications or /Applications.")
		report.NextActions = append(report.NextActions, "Run `INSTALL=1 scripts/build-argos-ui-runner-app.sh` once, open the installed app, then grant macOS permissions to that app.")
	} else {
		report.BundleID = plistBundleIdentifier(filepath.Join(report.InstalledPath, "Contents", "Info.plist"))
		report.Executable = filepath.Join(report.InstalledPath, "Contents", "MacOS", "argos-ui-runner")
		report.CodeSigned, report.CodeSignature = verifyCodeSignature(report.InstalledPath)
		report.BinarySHA256 = fileSHA256(report.Executable)
		report.AppSHA256 = directorySHA256(report.InstalledPath)
		if report.BundleID != argosUIRunnerBundleID {
			report.Problems = append(report.Problems, "Installed Argos UI Runner bundle id is "+firstNonEmpty(report.BundleID, "missing")+", expected "+argosUIRunnerBundleID+".")
			report.NextActions = append(report.NextActions, "Rebuild the Runner with the fixed bundle id before granting macOS permissions.")
		}
		if !report.CodeSigned {
			report.Problems = append(report.Problems, "Installed Argos UI Runner is not code signed.")
			report.NextActions = append(report.NextActions, "Install a signed Runner app and keep that app stable after granting permissions.")
		}
	}
	if report.StagedPath != "" {
		report.StagedNeedsApply = report.StagedAppSHA256 != "" && report.AppSHA256 != "" && report.StagedAppSHA256 != report.AppSHA256
		if !report.StagedNeedsApply && (report.StagedAppSHA256 == "" || report.AppSHA256 == "") {
			report.StagedNeedsApply = report.StagedBinarySHA256 != "" && report.BinarySHA256 != "" && report.StagedBinarySHA256 != report.BinarySHA256
		}
		if report.StagedBundleID != "" && report.StagedBundleID != argosUIRunnerBundleID {
			report.Problems = append(report.Problems, "Staged Argos UI Runner bundle id is "+report.StagedBundleID+", expected "+argosUIRunnerBundleID+".")
		}
		if !report.StagedCodeSigned {
			report.Problems = append(report.Problems, "Staged Argos UI Runner is not code signed.")
		}
		if report.StagedNeedsApply {
			report.NextActions = append(report.NextActions, "A newer Argos UI Runner is staged at "+report.StagedPath+". Apply it only while the user is present, because macOS may ask to re-enable Accessibility or Screen Recording.")
		}
	}
	payload := uiRunnerHealthPayload{}
	if health != nil {
		_ = json.Unmarshal([]byte(health.Stdout), &payload)
	} else {
		current := UIRunnerHealth(ctx, defaultUIRunnerURL())
		_ = json.Unmarshal([]byte(current.Stdout), &payload)
	}
	report.RunningAppPath = strings.TrimSpace(payload.AppPath)
	report.RunningExecutable = strings.TrimSpace(payload.Executable)
	report.RunningBundleID = strings.TrimSpace(payload.BundleID)
	report.RunningCodeSigned = payload.CodeSigned
	if report.RunningAppPath != "" {
		report.StableInstallInUse = samePath(report.RunningAppPath, report.InstalledPath)
		if !report.StableInstallInUse {
			report.Problems = append(report.Problems, "Running Argos UI Runner is not the stable installed app: "+report.RunningAppPath)
			report.NextActions = append(report.NextActions, "Quit the temporary Runner, then open "+firstNonEmpty(report.InstalledPath, report.RecommendedPath)+".")
		}
	}
	if report.RunningBundleID != "" && report.RunningBundleID != argosUIRunnerBundleID {
		report.Problems = append(report.Problems, "Running Argos UI Runner bundle id is "+report.RunningBundleID+", expected "+argosUIRunnerBundleID+".")
	}
	if report.RunningAppPath != "" && !report.RunningCodeSigned {
		report.Problems = append(report.Problems, "Running Argos UI Runner is not code signed; macOS may invalidate permissions after updates.")
	}
	report.NextActions = uniqueLocalStrings(report.NextActions)
	report.Problems = uniqueLocalStrings(report.Problems)
	report.OK = len(report.Problems) == 0
	return report
}

func ApplyStagedUIRunnerUpdate(ctx context.Context) UIRunnerUpdateReport {
	report := UIRunnerUpdateReport{
		Kind:      "meshclaw_argos_ui_runner_update",
		CreatedAt: time.Now().UTC(),
	}
	if runtime.GOOS != "darwin" {
		report.Problems = append(report.Problems, "Argos UI Runner updates require macOS.")
		report.NextActions = append(report.NextActions, "Run this on the Mac that owns the Argos UI session.")
		report.OK = false
		return report
	}

	report.Before = ArgosUIRunnerInstallDoctor(ctx, nil)
	report.StagedPath = report.Before.StagedPath
	report.InstalledPath = firstNonEmpty(report.Before.InstalledPath, recommendedUIRunnerPath())
	if report.StagedPath == "" {
		report.Problems = append(report.Problems, "No staged Argos UI Runner app found at "+recommendedUIRunnerPath()+".next.")
		report.NextActions = append(report.NextActions, "Upload or build the new Runner as "+recommendedUIRunnerPath()+".next, then rerun this command.")
		report.OK = false
		return report
	}
	if report.Before.StagedBundleID != "" && report.Before.StagedBundleID != argosUIRunnerBundleID {
		report.Problems = append(report.Problems, "Staged Argos UI Runner bundle id is "+report.Before.StagedBundleID+", expected "+argosUIRunnerBundleID+".")
	}
	if !report.Before.StagedCodeSigned {
		report.Problems = append(report.Problems, "Staged Argos UI Runner is not code signed.")
	}
	if len(report.Problems) > 0 {
		report.OK = false
		return report
	}
	if !report.Before.StagedNeedsApply && report.Before.Installed {
		report.Applied = false
		report.After = report.Before
		report.Doctor = ArgosMacDoctor(ctx, true)
		report.OK = report.Doctor.OK
		report.Problems = append(report.Problems, report.Doctor.Problems...)
		report.NextActions = append(report.NextActions, report.Doctor.NextActions...)
		return report
	}

	report.QuitRunner = run(ctx, "meshclaw_argos_ui_runner_quit", "pkill", "-f", "Argos UI Runner.app/Contents/MacOS/argos-ui-runner")
	if !report.QuitRunner.OK && !strings.Contains(report.QuitRunner.Error, "exit status 1") {
		report.NextActions = append(report.NextActions, "Could not stop the current Runner cleanly; continuing because it may not have been running.")
	}
	time.Sleep(700 * time.Millisecond)

	report.BackupPath = report.InstalledPath + ".prev." + time.Now().Format("20060102150405")
	if st, err := os.Stat(report.InstalledPath); err == nil && st.IsDir() {
		if err := os.Rename(report.InstalledPath, report.BackupPath); err != nil {
			report.Apply = failed("meshclaw_argos_ui_runner_update_apply", err.Error())
			report.Problems = append(report.Problems, "Could not move installed Runner to backup: "+err.Error())
			report.OK = false
			return report
		}
	} else {
		report.BackupPath = ""
		if err := os.MkdirAll(filepath.Dir(report.InstalledPath), 0755); err != nil {
			report.Apply = failed("meshclaw_argos_ui_runner_update_apply", err.Error())
			report.Problems = append(report.Problems, "Could not create Runner install directory: "+err.Error())
			report.OK = false
			return report
		}
	}
	if err := os.Rename(report.StagedPath, report.InstalledPath); err != nil {
		if report.BackupPath != "" {
			_ = os.Rename(report.BackupPath, report.InstalledPath)
		}
		report.Apply = failed("meshclaw_argos_ui_runner_update_apply", err.Error())
		report.Problems = append(report.Problems, "Could not apply staged Runner: "+err.Error())
		report.OK = false
		return report
	}
	report.Applied = true
	report.Apply = Result{
		Kind:      "meshclaw_argos_ui_runner_update_apply",
		Action:    "ui_runner_update_apply",
		OK:        true,
		Command:   []string{"rename", report.StagedPath, report.InstalledPath},
		Stdout:    fmt.Sprintf("applied staged Runner to %s", report.InstalledPath),
		CreatedAt: time.Now().UTC(),
	}
	_ = exec.Command("xattr", "-dr", "com.apple.quarantine", report.InstalledPath).Run()
	if ok, signature := verifyCodeSignature(report.InstalledPath); !ok {
		report.Problems = append(report.Problems, "Applied Runner failed code signature verification: "+signature)
		report.OK = false
		return report
	}

	report.OpenRunner = OpenUIRunnerApp(ctx)
	time.Sleep(1500 * time.Millisecond)
	report.Setup = ArgosMacSetup(ctx)
	report.Doctor = ArgosMacDoctor(ctx, true)
	report.After = ArgosUIRunnerInstallDoctor(ctx, nil)
	report.Problems = append(report.Problems, report.Setup.Problems...)
	report.Problems = append(report.Problems, report.Doctor.Problems...)
	report.NextActions = append(report.NextActions, report.Setup.NextActions...)
	report.NextActions = append(report.NextActions, report.Doctor.NextActions...)
	if report.Doctor.OK {
		report.Problems = nil
		report.NextActions = nil
	}
	report.Problems = uniqueLocalStrings(report.Problems)
	report.NextActions = uniqueLocalStrings(report.NextActions)
	report.OK = report.Doctor.OK
	return report
}

func OpenUIRunnerApp(ctx context.Context) Result {
	if runtime.GOOS != "darwin" {
		return failed("meshclaw_automation_open_ui_runner", "Argos UI Runner is only available on macOS")
	}
	if path := findInstalledUIRunner(); path != "" {
		result := run(ctx, "meshclaw_automation_open_ui_runner", "open", path)
		result.Action = "open_ui_runner"
		result.App = path
		return result
	}
	result := OpenApp(ctx, strings.TrimSuffix(argosUIRunnerAppName, ".app"))
	result.Kind = "meshclaw_automation_open_ui_runner"
	result.Action = "open_ui_runner"
	return result
}

func recommendedUIRunnerPath() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Join("/Applications", argosUIRunnerAppName)
	}
	return filepath.Join(home, "Applications", argosUIRunnerAppName)
}

func findInstalledUIRunner() string {
	candidates := []string{recommendedUIRunnerPath(), filepath.Join("/Applications", argosUIRunnerAppName)}
	for _, candidate := range candidates {
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate
		}
	}
	return ""
}

func findStagedUIRunner() string {
	candidates := []string{recommendedUIRunnerPath() + ".next"}
	for _, candidate := range candidates {
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate
		}
	}
	return ""
}

func plistBundleIdentifier(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	re := regexp.MustCompile(`(?s)<key>CFBundleIdentifier</key>\s*<string>([^<]+)</string>`)
	match := re.FindSubmatch(data)
	if len(match) != 2 {
		return ""
	}
	return strings.TrimSpace(string(match[1]))
}

func verifyCodeSignature(path string) (bool, string) {
	cmd := exec.Command("codesign", "--verify", "--strict", "--verbose=2", path)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()
	msg := strings.TrimSpace(stderr.String())
	if err != nil {
		if msg == "" {
			msg = err.Error()
		}
		return false, msg
	}
	return true, firstNonEmpty(msg, "codesign verification passed")
}

func fileSHA256(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func directorySHA256(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	entries := []string{}
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entries = append(entries, filepath.ToSlash(rel))
		return nil
	}); err != nil {
		return ""
	}
	sort.Strings(entries)
	hash := sha256.New()
	for _, rel := range entries {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return ""
		}
		hash.Write([]byte(rel))
		hash.Write([]byte{0})
		hash.Write(data)
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func samePath(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	aa, errA := filepath.EvalSymlinks(a)
	bb, errB := filepath.EvalSymlinks(b)
	if errA == nil {
		a = aa
	}
	if errB == nil {
		b = bb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func uniqueLocalStrings(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
