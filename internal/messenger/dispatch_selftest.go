package messenger

import (
	"sort"
	"strings"
	"time"
)

type DispatchSelfTestResult struct {
	Kind        string                 `json:"kind"`
	Generated   time.Time              `json:"generated"`
	OK          bool                   `json:"ok"`
	GlobalMode  string                 `json:"global_mode,omitempty"`
	Targets     []DispatchSelfTestItem `json:"targets"`
	Warnings    []string               `json:"warnings,omitempty"`
	Problems    []string               `json:"problems,omitempty"`
	NextActions []string               `json:"next_actions,omitempty"`
}

type DispatchSelfTestItem struct {
	TargetID     string `json:"target_id"`
	Mode         string `json:"mode,omitempty"`
	Label        string `json:"label,omitempty"`
	GroupID      string `json:"group_id,omitempty"`
	OneWay       bool   `json:"one_way"`
	ReplyAllowed bool   `json:"reply_allowed"`
	ReplyPreview string `json:"reply_preview,omitempty"`
	Status       string `json:"status"`
	Problem      string `json:"problem,omitempty"`
}

func DispatchSelfTest(globalMode string) (DispatchSelfTestResult, error) {
	store, err := ListTargets()
	result := DispatchSelfTestResult{
		Kind:       "meshclaw_signal_dispatch_self_test",
		Generated:  time.Now().UTC(),
		OK:         true,
		GlobalMode: strings.TrimSpace(globalMode),
	}
	if err != nil {
		result.OK = false
		result.Problems = append(result.Problems, "target registry could not be read: "+err.Error())
		result.NextActions = append(result.NextActions, "Run `meshclaw messenger targets --json` and repair the target registry.")
		return result, err
	}
	result.Warnings = append(result.Warnings, mixedModeGroupWarnings(store.Targets)...)
	result.Warnings = append(result.Warnings, argosDirectRecipientWarnings(store.Targets)...)
	for _, target := range store.Targets {
		item := dispatchSelfTestTarget(target, result.GlobalMode)
		if item.Problem != "" {
			result.Problems = append(result.Problems, item.Problem)
		}
		result.Targets = append(result.Targets, item)
	}
	if len(result.Targets) == 0 {
		result.Warnings = append(result.Warnings, "no Signal targets are registered")
		result.NextActions = append(result.NextActions, "Bind at least one assistant target and one briefing/report target before enabling dispatcher automation.")
	}
	if len(result.Problems) > 0 {
		result.OK = false
		result.NextActions = append(result.NextActions, "Fix the target mode or group binding before running the Signal dispatcher in execute mode.")
	}
	return result, nil
}

func dispatchSelfTestTarget(target Target, globalMode string) DispatchSelfTestItem {
	mode := strings.ToLower(strings.TrimSpace(firstNonEmpty(globalMode, target.Mode)))
	opts := ListenOptions{TargetID: target.ID, Mode: mode}
	event := IncomingMessage{Source: "+10000000000", GroupID: target.GroupID, Redacted: "뭘 할 수 있어?"}
	oneWay := isOneWayReportTarget(opts, target, mode)
	reply := guardReply(opts, target, event)
	item := DispatchSelfTestItem{
		TargetID:     target.ID,
		Mode:         target.Mode,
		Label:        target.Label,
		GroupID:      target.GroupID,
		OneWay:       oneWay,
		ReplyAllowed: !oneWay,
		ReplyPreview: compactReplyPreview(reply),
		Status:       "ok",
	}
	if oneWay && reply != "" {
		item.Status = "failed"
		item.Problem = "one-way report target " + target.ID + " produced a reply"
		return item
	}
	if !oneWay && isInteractiveTargetMode(target.Mode) && reply == "" {
		item.Status = "failed"
		item.Problem = "interactive target " + target.ID + " did not produce a basic capability reply"
		return item
	}
	if oneWay {
		item.Status = "one-way"
	} else if reply != "" {
		item.Status = "reply-ready"
	} else {
		item.Status = "ignored"
	}
	return item
}

func mixedModeGroupWarnings(targets []Target) []string {
	type groupState struct {
		modes   map[string]bool
		targets []string
	}
	groups := map[string]*groupState{}
	for _, target := range targets {
		groupID := strings.TrimSpace(target.GroupID)
		if groupID == "" {
			continue
		}
		state := groups[groupID]
		if state == nil {
			state = &groupState{modes: map[string]bool{}}
			groups[groupID] = state
		}
		state.modes[strings.ToLower(strings.TrimSpace(target.Mode))] = true
		state.targets = append(state.targets, target.ID)
	}
	warnings := []string{}
	for groupID, state := range groups {
		if hasOneWayMode(state.modes) && hasInteractiveMode(state.modes) {
			sort.Strings(state.targets)
			warnings = append(warnings, "same Signal group_id is bound to both report and interactive targets: "+groupID+" targets="+strings.Join(state.targets, ","))
		}
	}
	sort.Strings(warnings)
	return warnings
}

func argosDirectRecipientWarnings(targets []Target) []string {
	warnings := []string{}
	for _, target := range targets {
		if !isArgosUserFacingSignalTarget(target) {
			continue
		}
		if strings.TrimSpace(target.GroupID) != "" {
			continue
		}
		recipient := strings.TrimSpace(target.Recipient)
		if recipient == "" {
			continue
		}
		warnings = append(warnings, "Argos user-facing Signal target "+target.ID+" is bound to a direct recipient; use group_id for shared runtime messaging")
	}
	sort.Strings(warnings)
	return warnings
}

func hasOneWayMode(modes map[string]bool) bool {
	return modes["ops"] || modes["briefing"]
}

func hasInteractiveMode(modes map[string]bool) bool {
	return modes["assistant"] || modes["chat"] || modes["guard"]
}

func isInteractiveTargetMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "assistant", "chat", "guard":
		return true
	default:
		return false
	}
}

func compactReplyPreview(reply string) string {
	reply = strings.TrimSpace(signalReplyVisibleText(reply))
	if reply == "" {
		return ""
	}
	reply = strings.Join(strings.Fields(reply), " ")
	if len([]rune(reply)) <= 140 {
		return reply
	}
	runes := []rune(reply)
	return string(runes[:140]) + "..."
}
