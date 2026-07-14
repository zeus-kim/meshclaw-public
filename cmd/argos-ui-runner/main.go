package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const expectedBundleID = "ai.meshclaw.argosrunner"

type healthResponse struct {
	Kind          string    `json:"kind"`
	OK            bool      `json:"ok"`
	PID           int       `json:"pid"`
	Host          string    `json:"host,omitempty"`
	User          string    `json:"user,omitempty"`
	Address       string    `json:"address"`
	Executable    string    `json:"executable,omitempty"`
	AppPath       string    `json:"app_path,omitempty"`
	BundleID      string    `json:"bundle_id,omitempty"`
	CodeSigned    bool      `json:"code_signed"`
	CodeSignature string    `json:"code_signature,omitempty"`
	AXTrusted     bool      `json:"accessibility_trusted"`
	SignalRunning bool      `json:"signal_running"`
	Actions       actions   `json:"actions"`
	CreatedAt     time.Time `json:"created_at"`
	Problems      []string  `json:"problems,omitempty"`
	NextActions   []string  `json:"next_actions,omitempty"`
}

type clickRequest struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type keyRequest struct {
	Key       string   `json:"key"`
	Modifiers []string `json:"modifiers"`
}

type textRequest struct {
	Text string `json:"text"`
}

type commandResponse struct {
	Kind      string    `json:"kind"`
	OK        bool      `json:"ok"`
	Path      string    `json:"path,omitempty"`
	Stdout    string    `json:"stdout,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type screenRecordRequest struct {
	Seconds int    `json:"seconds"`
	Output  string `json:"output"`
}

type reminderRequest struct {
	Title string `json:"title"`
	Notes string `json:"notes"`
	Due   string `json:"due"`
}

type reminderListRequest struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Query string `json:"query"`
}

type reminderMutationRequest struct {
	ID    string `json:"id"`
	Query string `json:"query"`
	Title string `json:"title"`
}

type calendarEventRequest struct {
	Title string `json:"title"`
	Notes string `json:"notes"`
	Start string `json:"start"`
	End   string `json:"end"`
}

type calendarListRequest struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Query string `json:"query"`
}

type calendarMutationRequest struct {
	ID    string `json:"id"`
	Query string `json:"query"`
	Title string `json:"title"`
	Start string `json:"start"`
	End   string `json:"end"`
}

type contactsSearchRequest struct {
	Query string `json:"query"`
}

type runnerConfig struct {
	SignalStartClick  string `json:"signal_start_click"`
	SignalHangupClick string `json:"signal_hangup_click"`
}

type actions struct {
	SignalStartCall  bool   `json:"signal_start_call"`
	SignalHangupCall bool   `json:"signal_hangup_call"`
	ReminderCreate   bool   `json:"reminder_create"`
	ReminderList     bool   `json:"reminder_list"`
	ReminderComplete bool   `json:"reminder_complete"`
	ReminderDelete   bool   `json:"reminder_delete"`
	CalendarCreate   bool   `json:"calendar_create"`
	CalendarList     bool   `json:"calendar_list"`
	CalendarDelete   bool   `json:"calendar_delete"`
	ContactsSearch   bool   `json:"contacts_search"`
	ConfigPath       string `json:"config_path,omitempty"`
}

func main() {
	addr := flag.String("listen", "127.0.0.1:48292", "listen address")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, health(*addr))
	})
	mux.HandleFunc("/activate-signal", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		err := exec.Command("open", "-a", "Signal").Run()
		writeCommand(w, "argos_activate_signal", err)
	})
	mux.HandleFunc("/request-accessibility", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		trusted := accessibilityTrusted(true)
		res := commandResponse{Kind: "argos_request_accessibility", OK: trusted, CreatedAt: time.Now().UTC()}
		if !trusted {
			res.Error = "macOS Accessibility permission is still not granted; enable Argos UI Runner in System Settings"
		}
		writeJSON(w, res)
	})
	mux.HandleFunc("/request-reminders", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, reminderHelperPath(), "--request-access")
		out, err := cmd.CombinedOutput()
		res := commandResponse{Kind: "argos_request_reminders", OK: err == nil, Stdout: strings.TrimSpace(string(out)), CreatedAt: time.Now().UTC()}
		if err != nil {
			res.Error = strings.TrimSpace(string(out) + "\n" + err.Error())
			if ctx.Err() != nil {
				res.Error = "Reminders permission prompt timed out. Approve the Argos UI Runner Reminders prompt on this Mac, then retry."
			}
		}
		writeJSON(w, res)
	})
	mux.HandleFunc("/request-calendar", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, calendarHelperPath(), "--request-access")
		out, err := cmd.CombinedOutput()
		res := commandResponse{Kind: "argos_request_calendar", OK: err == nil, Stdout: strings.TrimSpace(string(out)), CreatedAt: time.Now().UTC()}
		if err != nil {
			res.Error = strings.TrimSpace(string(out) + "\n" + err.Error())
			if ctx.Err() != nil {
				res.Error = "Calendar permission prompt timed out. Approve the Argos UI Runner Calendar prompt on this Mac, then retry."
			}
		}
		writeJSON(w, res)
	})
	mux.HandleFunc("/request-contacts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, contactsHelperPath(), "--request-access")
		out, err := cmd.CombinedOutput()
		res := commandResponse{Kind: "argos_request_contacts", OK: err == nil, Stdout: strings.TrimSpace(string(out)), CreatedAt: time.Now().UTC()}
		if err != nil {
			res.Error = strings.TrimSpace(string(out) + "\n" + err.Error())
			if ctx.Err() != nil {
				res.Error = "Contacts permission prompt timed out. Approve the Argos UI Runner Contacts prompt on this Mac, then retry."
			}
		}
		writeJSON(w, res)
	})
	mux.HandleFunc("/click", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		if !accessibilityTrusted(false) {
			writeJSONStatus(w, http.StatusForbidden, commandResponse{
				Kind:      "argos_click",
				OK:        false,
				Error:     "accessibility permission is required for Argos UI Runner",
				CreatedAt: time.Now().UTC(),
			})
			return
		}
		var req clickRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, commandResponse{Kind: "argos_click", OK: false, Error: err.Error(), CreatedAt: time.Now().UTC()})
			return
		}
		err := clickPoint(req.X, req.Y)
		writeCommand(w, "argos_click", err)
	})
	mux.HandleFunc("/key", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		if !accessibilityTrusted(false) {
			writeJSONStatus(w, http.StatusForbidden, commandResponse{
				Kind:      "argos_key",
				OK:        false,
				Error:     "accessibility permission is required for Argos UI Runner",
				CreatedAt: time.Now().UTC(),
			})
			return
		}
		var req keyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, commandResponse{Kind: "argos_key", OK: false, Error: err.Error(), CreatedAt: time.Now().UTC()})
			return
		}
		err := pressKey(strings.TrimSpace(req.Key), req.Modifiers)
		writeCommand(w, "argos_key", err)
	})
	mux.HandleFunc("/type-text", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		if !accessibilityTrusted(false) {
			writeJSONStatus(w, http.StatusForbidden, commandResponse{
				Kind:      "argos_type_text",
				OK:        false,
				Error:     "accessibility permission is required for Argos UI Runner",
				CreatedAt: time.Now().UTC(),
			})
			return
		}
		var req textRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, commandResponse{Kind: "argos_type_text", OK: false, Error: err.Error(), CreatedAt: time.Now().UTC()})
			return
		}
		err := typeText(req.Text)
		writeCommand(w, "argos_type_text", err)
	})
	mux.HandleFunc("/signal/start-call", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		runNamedClick(w, "argos_signal_start_call", "ARGOS_SIGNAL_START_CLICK")
	})
	mux.HandleFunc("/signal/hangup-call", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		runNamedClick(w, "argos_signal_hangup_call", "ARGOS_SIGNAL_HANGUP_CLICK")
	})
	mux.HandleFunc("/screen-record", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		var req screenRecordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, commandResponse{Kind: "argos_screen_record", OK: false, Error: err.Error(), CreatedAt: time.Now().UTC()})
			return
		}
		if req.Seconds <= 0 {
			writeJSONStatus(w, http.StatusBadRequest, commandResponse{Kind: "argos_screen_record", OK: false, Error: "seconds must be positive", CreatedAt: time.Now().UTC()})
			return
		}
		if strings.TrimSpace(req.Output) == "" {
			writeJSONStatus(w, http.StatusBadRequest, commandResponse{Kind: "argos_screen_record", OK: false, Error: "output path is required", CreatedAt: time.Now().UTC()})
			return
		}
		if err := os.MkdirAll(filepath.Dir(req.Output), 0700); err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, commandResponse{Kind: "argos_screen_record", OK: false, Error: err.Error(), CreatedAt: time.Now().UTC()})
			return
		}
		cmd := exec.Command(screenRecorderPath(), "--seconds", fmt.Sprintf("%d", req.Seconds), "--output", req.Output)
		out, err := cmd.CombinedOutput()
		res := commandResponse{Kind: "argos_screen_record", OK: err == nil, Path: req.Output, CreatedAt: time.Now().UTC()}
		if err != nil {
			res.Error = strings.TrimSpace(string(out) + "\n" + err.Error())
		} else if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
			res.Path = req.Output
		}
		writeJSON(w, res)
	})
	mux.HandleFunc("/reminder/create", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		var req reminderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, commandResponse{Kind: "argos_reminder_create", OK: false, Error: err.Error(), CreatedAt: time.Now().UTC()})
			return
		}
		req.Title = strings.TrimSpace(req.Title)
		if req.Title == "" {
			writeJSONStatus(w, http.StatusBadRequest, commandResponse{Kind: "argos_reminder_create", OK: false, Error: "title is required", CreatedAt: time.Now().UTC()})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, reminderHelperPath(), "--title", req.Title, "--notes", req.Notes, "--due", strings.TrimSpace(req.Due))
		out, err := cmd.CombinedOutput()
		res := commandResponse{Kind: "argos_reminder_create", OK: err == nil, CreatedAt: time.Now().UTC()}
		if err != nil {
			res.Error = strings.TrimSpace(string(out) + "\n" + err.Error())
			if ctx.Err() != nil {
				res.Error = "Reminders permission prompt timed out. Open Argos UI Runner once on this Mac and allow Reminders access, then retry."
			}
		} else {
			res.Stdout = strings.TrimSpace(string(out))
		}
		writeJSON(w, res)
	})
	mux.HandleFunc("/reminder/list", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		var req reminderListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, commandResponse{Kind: "argos_reminder_list", OK: false, Error: err.Error(), CreatedAt: time.Now().UTC()})
			return
		}
		if strings.TrimSpace(req.Start) == "" || strings.TrimSpace(req.End) == "" {
			writeJSONStatus(w, http.StatusBadRequest, commandResponse{Kind: "argos_reminder_list", OK: false, Error: "start and end are required", CreatedAt: time.Now().UTC()})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, reminderHelperPath(), "--list", "--start", strings.TrimSpace(req.Start), "--end", strings.TrimSpace(req.End), "--query", strings.TrimSpace(req.Query))
		out, err := cmd.CombinedOutput()
		res := commandResponse{Kind: "argos_reminder_list", OK: err == nil, CreatedAt: time.Now().UTC()}
		if err != nil {
			res.Error = strings.TrimSpace(string(out) + "\n" + err.Error())
			if ctx.Err() != nil {
				res.Error = "Reminders permission prompt timed out. Open Argos UI Runner once on this Mac and allow Reminders access, then retry."
			}
		} else {
			res.Stdout = strings.TrimSpace(string(out))
		}
		writeJSON(w, res)
	})
	mux.HandleFunc("/reminder/complete", func(w http.ResponseWriter, r *http.Request) {
		handleReminderMutation(w, r, "argos_reminder_complete", "--complete")
	})
	mux.HandleFunc("/reminder/delete", func(w http.ResponseWriter, r *http.Request) {
		handleReminderMutation(w, r, "argos_reminder_delete", "--delete")
	})
	mux.HandleFunc("/calendar/create", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		var req calendarEventRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, commandResponse{Kind: "argos_calendar_create", OK: false, Error: err.Error(), CreatedAt: time.Now().UTC()})
			return
		}
		req.Title = strings.TrimSpace(req.Title)
		if req.Title == "" {
			writeJSONStatus(w, http.StatusBadRequest, commandResponse{Kind: "argos_calendar_create", OK: false, Error: "title is required", CreatedAt: time.Now().UTC()})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, calendarHelperPath(), "--title", req.Title, "--notes", req.Notes, "--start", strings.TrimSpace(req.Start), "--end", strings.TrimSpace(req.End))
		out, err := cmd.CombinedOutput()
		res := commandResponse{Kind: "argos_calendar_create", OK: err == nil, CreatedAt: time.Now().UTC()}
		if err != nil {
			res.Error = strings.TrimSpace(string(out) + "\n" + err.Error())
			if ctx.Err() != nil {
				res.Error = "Calendar permission prompt timed out. Open Argos UI Runner once on this Mac and allow Calendar access, then retry."
			}
		} else {
			res.Stdout = strings.TrimSpace(string(out))
		}
		writeJSON(w, res)
	})
	mux.HandleFunc("/calendar/list", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		var req calendarListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, commandResponse{Kind: "argos_calendar_list", OK: false, Error: err.Error(), CreatedAt: time.Now().UTC()})
			return
		}
		if strings.TrimSpace(req.Start) == "" || strings.TrimSpace(req.End) == "" {
			writeJSONStatus(w, http.StatusBadRequest, commandResponse{Kind: "argos_calendar_list", OK: false, Error: "start and end are required", CreatedAt: time.Now().UTC()})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, calendarHelperPath(), "--list", "--start", strings.TrimSpace(req.Start), "--end", strings.TrimSpace(req.End), "--query", strings.TrimSpace(req.Query))
		out, err := cmd.CombinedOutput()
		res := commandResponse{Kind: "argos_calendar_list", OK: err == nil, CreatedAt: time.Now().UTC()}
		if err != nil {
			res.Error = strings.TrimSpace(string(out) + "\n" + err.Error())
			if ctx.Err() != nil {
				res.Error = "Calendar permission prompt timed out. Open Argos UI Runner once on this Mac and allow Calendar access, then retry."
			}
		} else {
			res.Stdout = strings.TrimSpace(string(out))
		}
		writeJSON(w, res)
	})
	mux.HandleFunc("/calendar/delete", func(w http.ResponseWriter, r *http.Request) {
		handleCalendarMutation(w, r, "argos_calendar_delete", "--delete")
	})
	mux.HandleFunc("/contacts/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		var req contactsSearchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, commandResponse{Kind: "argos_contacts_search", OK: false, Error: err.Error(), CreatedAt: time.Now().UTC()})
			return
		}
		if strings.TrimSpace(req.Query) == "" {
			writeJSONStatus(w, http.StatusBadRequest, commandResponse{Kind: "argos_contacts_search", OK: false, Error: "query is required", CreatedAt: time.Now().UTC()})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, contactsHelperPath(), "--search", "--query", strings.TrimSpace(req.Query))
		out, err := cmd.CombinedOutput()
		res := commandResponse{Kind: "argos_contacts_search", OK: err == nil, CreatedAt: time.Now().UTC()}
		if err != nil {
			res.Error = strings.TrimSpace(string(out) + "\n" + err.Error())
			if ctx.Err() != nil {
				res.Error = "Contacts permission prompt timed out. Open Argos UI Runner once on this Mac and allow Contacts access, then retry."
			}
		} else {
			res.Stdout = strings.TrimSpace(string(out))
		}
		writeJSON(w, res)
	})

	log.Printf("Argos UI Runner listening on http://%s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

func screenRecorderPath() string {
	executable, err := os.Executable()
	if err != nil {
		return "argos-screen-recorder"
	}
	path := filepath.Join(filepath.Dir(executable), "argos-screen-recorder")
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		return path
	}
	return "argos-screen-recorder"
}

func reminderHelperPath() string {
	executable, err := os.Executable()
	if err != nil {
		return "argos-reminder-helper"
	}
	path := filepath.Join(filepath.Dir(executable), "argos-reminder-helper")
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		return path
	}
	return "argos-reminder-helper"
}

func calendarHelperPath() string {
	executable, err := os.Executable()
	if err != nil {
		return "argos-calendar-helper"
	}
	path := filepath.Join(filepath.Dir(executable), "argos-calendar-helper")
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		return path
	}
	return "argos-calendar-helper"
}

func contactsHelperPath() string {
	executable, err := os.Executable()
	if err != nil {
		return "argos-contacts-helper"
	}
	path := filepath.Join(filepath.Dir(executable), "argos-contacts-helper")
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		return path
	}
	return "argos-contacts-helper"
}

func health(addr string) healthResponse {
	host, _ := os.Hostname()
	executable, _ := os.Executable()
	executable, _ = filepath.EvalSymlinks(executable)
	appPath := appBundlePath(executable)
	bundleID := bundleIdentifier(appPath)
	codeSigned, codeSignature := codeSignatureStatus(appPath, executable)
	res := healthResponse{
		Kind:          "argos_ui_runner_health",
		PID:           os.Getpid(),
		Host:          host,
		User:          os.Getenv("USER"),
		Address:       addr,
		Executable:    executable,
		AppPath:       appPath,
		BundleID:      bundleID,
		CodeSigned:    codeSigned,
		CodeSignature: codeSignature,
		AXTrusted:     accessibilityTrusted(false),
		SignalRunning: commandSucceeds("pgrep", "-x", "Signal"),
		Actions:       configuredActions(),
		CreatedAt:     time.Now().UTC(),
	}
	if !res.AXTrusted {
		res.Problems = append(res.Problems, "macOS Accessibility permission is not granted to Argos UI Runner.")
		res.NextActions = append(res.NextActions, "Open System Settings -> Privacy & Security -> Accessibility and enable Argos UI Runner.")
	}
	if res.AppPath == "" {
		res.Problems = append(res.Problems, "Argos UI Runner is not running from an app bundle.")
		res.NextActions = append(res.NextActions, "Install and run the stable Argos UI Runner.app instead of a raw development binary.")
	}
	if res.BundleID != "" && res.BundleID != expectedBundleID {
		res.Problems = append(res.Problems, "Argos UI Runner bundle id changed: "+res.BundleID)
		res.NextActions = append(res.NextActions, "Rebuild with bundle id "+expectedBundleID+" and avoid replacing the app after granting macOS permissions.")
	}
	if !res.CodeSigned {
		res.Problems = append(res.Problems, "Argos UI Runner is not code signed.")
		res.NextActions = append(res.NextActions, "Sign the stable Runner app before granting Accessibility or Screen Recording permissions.")
	}
	if !res.SignalRunning {
		res.Problems = append(res.Problems, "Signal Desktop is not running.")
		res.NextActions = append(res.NextActions, "Open Signal Desktop in the Argos macOS user session.")
	}
	res.OK = res.AXTrusted && res.SignalRunning && res.AppPath != "" && res.CodeSigned && (res.BundleID == "" || res.BundleID == expectedBundleID)
	return res
}

func appBundlePath(executable string) string {
	executable = filepath.Clean(strings.TrimSpace(executable))
	marker := ".app" + string(os.PathSeparator)
	idx := strings.Index(executable, marker)
	if idx < 0 {
		if strings.HasSuffix(executable, ".app") {
			return executable
		}
		return ""
	}
	return executable[:idx+len(".app")]
}

func bundleIdentifier(appPath string) string {
	if appPath == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(appPath, "Contents", "Info.plist"))
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

func codeSignatureStatus(appPath, executable string) (bool, string) {
	target := appPath
	if target == "" {
		target = executable
	}
	if strings.TrimSpace(target) == "" {
		return false, "missing executable path"
	}
	cmd := exec.Command("codesign", "--verify", "--strict", "--verbose=2", target)
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
	if msg == "" {
		msg = "codesign verification passed"
	}
	return true, msg
}

func writeCommand(w http.ResponseWriter, kind string, err error) {
	res := commandResponse{Kind: kind, OK: err == nil, CreatedAt: time.Now().UTC()}
	if err != nil {
		res.Error = err.Error()
	}
	writeJSON(w, res)
}

func handleReminderMutation(w http.ResponseWriter, r *http.Request, kind, flag string) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req reminderMutationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, commandResponse{Kind: kind, OK: false, Error: err.Error(), CreatedAt: time.Now().UTC()})
		return
	}
	query := strings.TrimSpace(firstNonEmptyString(req.Query, req.Title))
	if strings.TrimSpace(req.ID) == "" && query == "" {
		writeJSONStatus(w, http.StatusBadRequest, commandResponse{Kind: kind, OK: false, Error: "id or query is required", CreatedAt: time.Now().UTC()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
	defer cancel()
	args := []string{flag}
	if id := strings.TrimSpace(req.ID); id != "" {
		args = append(args, "--id", id)
	}
	if query != "" {
		args = append(args, "--query", query)
	}
	cmd := exec.CommandContext(ctx, reminderHelperPath(), args...)
	out, err := cmd.CombinedOutput()
	res := commandResponse{Kind: kind, OK: err == nil, CreatedAt: time.Now().UTC()}
	if err != nil {
		res.Error = strings.TrimSpace(string(out) + "\n" + err.Error())
		if ctx.Err() != nil {
			res.Error = "Reminders permission prompt timed out. Open Argos UI Runner once on this Mac and allow Reminders access, then retry."
		}
	} else {
		res.Stdout = strings.TrimSpace(string(out))
	}
	writeJSON(w, res)
}

func handleCalendarMutation(w http.ResponseWriter, r *http.Request, kind, flag string) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req calendarMutationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, commandResponse{Kind: kind, OK: false, Error: err.Error(), CreatedAt: time.Now().UTC()})
		return
	}
	query := strings.TrimSpace(firstNonEmptyString(req.Query, req.Title))
	if strings.TrimSpace(req.ID) == "" && query == "" {
		writeJSONStatus(w, http.StatusBadRequest, commandResponse{Kind: kind, OK: false, Error: "id or query is required", CreatedAt: time.Now().UTC()})
		return
	}
	if strings.TrimSpace(req.Start) == "" || strings.TrimSpace(req.End) == "" {
		writeJSONStatus(w, http.StatusBadRequest, commandResponse{Kind: kind, OK: false, Error: "start and end are required", CreatedAt: time.Now().UTC()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
	defer cancel()
	args := []string{flag, "--start", strings.TrimSpace(req.Start), "--end", strings.TrimSpace(req.End)}
	if id := strings.TrimSpace(req.ID); id != "" {
		args = append(args, "--id", id)
	}
	if query != "" {
		args = append(args, "--query", query)
	}
	cmd := exec.CommandContext(ctx, calendarHelperPath(), args...)
	out, err := cmd.CombinedOutput()
	res := commandResponse{Kind: kind, OK: err == nil, CreatedAt: time.Now().UTC()}
	if err != nil {
		res.Error = strings.TrimSpace(string(out) + "\n" + err.Error())
		if ctx.Err() != nil {
			res.Error = "Calendar permission prompt timed out. Open Argos UI Runner once on this Mac and allow Calendar access, then retry."
		}
	} else {
		res.Stdout = strings.TrimSpace(string(out))
	}
	writeJSON(w, res)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func runNamedClick(w http.ResponseWriter, kind, envName string) {
	if !accessibilityTrusted(false) {
		writeJSONStatus(w, http.StatusForbidden, commandResponse{
			Kind:      kind,
			OK:        false,
			Error:     "accessibility permission is required for Argos UI Runner",
			CreatedAt: time.Now().UTC(),
		})
		return
	}
	click, err := clickFromEnv(envName)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, commandResponse{Kind: kind, OK: false, Error: err.Error(), CreatedAt: time.Now().UTC()})
		return
	}
	writeCommand(w, kind, clickPoint(click.X, click.Y))
}

func clickFromEnv(name string) (clickRequest, error) {
	value, source := clickValue(name)
	parts := strings.Split(value, ",")
	if len(parts) != 2 {
		return clickRequest{}, fmt.Errorf("%s must be set to x,y in environment or %s", name, source)
	}
	x, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return clickRequest{}, fmt.Errorf("%s has invalid x coordinate: %w", source, err)
	}
	y, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return clickRequest{}, fmt.Errorf("%s has invalid y coordinate: %w", source, err)
	}
	return clickRequest{X: x, Y: y}, nil
}

func clickValue(name string) (string, string) {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value, name
	}
	cfg, path := loadRunnerConfig()
	switch name {
	case "ARGOS_SIGNAL_START_CLICK":
		return strings.TrimSpace(cfg.SignalStartClick), path
	case "ARGOS_SIGNAL_HANGUP_CLICK":
		return strings.TrimSpace(cfg.SignalHangupClick), path
	default:
		return "", path
	}
}

func configuredActions() actions {
	_, path := loadRunnerConfig()
	start, _ := clickValue("ARGOS_SIGNAL_START_CLICK")
	hangup, _ := clickValue("ARGOS_SIGNAL_HANGUP_CLICK")
	return actions{
		SignalStartCall:  strings.TrimSpace(start) != "",
		SignalHangupCall: strings.TrimSpace(hangup) != "",
		ReminderCreate:   helperAvailable(reminderHelperPath()),
		ReminderList:     helperAvailable(reminderHelperPath()),
		ReminderComplete: helperAvailable(reminderHelperPath()),
		ReminderDelete:   helperAvailable(reminderHelperPath()),
		CalendarCreate:   helperAvailable(calendarHelperPath()),
		CalendarList:     helperAvailable(calendarHelperPath()),
		CalendarDelete:   helperAvailable(calendarHelperPath()),
		ContactsSearch:   helperAvailable(contactsHelperPath()),
		ConfigPath:       path,
	}
}

func helperAvailable(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func loadRunnerConfig() (runnerConfig, string) {
	path := runnerConfigPath()
	var cfg runnerConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, path
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg, path
}

func runnerConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".meshclaw/argos-ui-runner.json"
	}
	return home + "/.meshclaw/argos-ui-runner.json"
}

func writeJSON(w http.ResponseWriter, value interface{}) {
	writeJSONStatus(w, http.StatusOK, value)
}

func writeJSONStatus(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "json encode failed: %v\n", err)
	}
}

func commandSucceeds(name string, args ...string) bool {
	return exec.Command(name, args...).Run() == nil
}
