package opsdb

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type PowerEventOptions struct {
	Window    time.Duration
	UptimeMax time.Duration
	MinNodes  int
	Limit     int
}

type PowerEventReport struct {
	Kind             string          `json:"kind"`
	Generated        time.Time       `json:"generated"`
	WindowSeconds    int64           `json:"window_seconds"`
	UptimeMaxSeconds int64           `json:"uptime_max_seconds"`
	MinNodes         int             `json:"min_nodes"`
	Incidents        []PowerIncident `json:"incidents"`
	RecordedEvents   []Event         `json:"recorded_events,omitempty"`
}

type PowerIncident struct {
	Time            time.Time          `json:"time"`
	Severity        string             `json:"severity"`
	Confidence      string             `json:"confidence"`
	Summary         string             `json:"summary"`
	Nodes           []PowerNodeRestart `json:"nodes"`
	RecommendedNext []string           `json:"recommended_next"`
}

type PowerNodeRestart struct {
	Node                    string    `json:"node"`
	Hostname                string    `json:"hostname,omitempty"`
	PreviousCollectedAt     time.Time `json:"previous_collected_at"`
	CurrentCollectedAt      time.Time `json:"current_collected_at"`
	PreviousBootFingerprint string    `json:"previous_boot_fingerprint,omitempty"`
	CurrentBootFingerprint  string    `json:"current_boot_fingerprint,omitempty"`
	CurrentUptimeSeconds    int64     `json:"current_uptime_seconds,omitempty"`
}

type storedNodeReport struct {
	StoredAt time.Time `json:"stored_at"`
	Report   struct {
		NodeName    string    `json:"node_name"`
		Hostname    string    `json:"hostname"`
		CollectedAt time.Time `json:"collected_at"`
		Identity    struct {
			BootIDFingerprint string `json:"boot_id_fingerprint"`
		} `json:"identity"`
		System struct {
			UptimeSeconds int64 `json:"uptime_seconds"`
		} `json:"system"`
	} `json:"report"`
}

func (db DB) DetectPowerEvents(opts PowerEventOptions) (PowerEventReport, error) {
	if opts.Window <= 0 {
		opts.Window = 15 * time.Minute
	}
	if opts.UptimeMax <= 0 {
		opts.UptimeMax = 2 * time.Hour
	}
	if opts.MinNodes <= 0 {
		opts.MinNodes = 2
	}
	if opts.Limit <= 0 {
		opts.Limit = 10
	}

	restarts, err := db.nodeRestarts(opts.UptimeMax)
	if err != nil {
		return PowerEventReport{}, err
	}
	sort.SliceStable(restarts, func(i, j int) bool {
		return restarts[i].CurrentCollectedAt.Before(restarts[j].CurrentCollectedAt)
	})

	var incidents []PowerIncident
	used := make([]bool, len(restarts))
	for i, restart := range restarts {
		if used[i] {
			continue
		}
		group := []PowerNodeRestart{restart}
		used[i] = true
		for j := i + 1; j < len(restarts); j++ {
			if used[j] {
				continue
			}
			if absDuration(restarts[j].CurrentCollectedAt.Sub(restart.CurrentCollectedAt)) <= opts.Window {
				group = append(group, restarts[j])
				used[j] = true
			}
		}
		if len(group) < opts.MinNodes {
			continue
		}
		sort.SliceStable(group, func(a, b int) bool {
			return group[a].CurrentCollectedAt.Before(group[b].CurrentCollectedAt)
		})
		incident := PowerIncident{
			Time:       group[0].CurrentCollectedAt,
			Severity:   "warning",
			Confidence: "medium",
			Nodes:      group,
			RecommendedNext: []string{
				"Treat as a physical power-quality candidate unless application logs show an intentional shutdown.",
				"Check UPS/AVR logs, wall outlets, wet/humid contact points, and whether affected nodes share a building circuit.",
				"Keep expensive GPU/workstation nodes on UPS or voltage-regulated power before scheduling heavy work.",
			},
		}
		if len(group) >= 3 {
			incident.Severity = "high"
			incident.Confidence = "high"
		}
		incident.Summary = powerIncidentSummary(group)
		incidents = append(incidents, incident)
	}

	sort.SliceStable(incidents, func(i, j int) bool {
		return incidents[i].Time.After(incidents[j].Time)
	})
	if len(incidents) > opts.Limit {
		incidents = incidents[:opts.Limit]
	}
	recorded, _ := db.Recent(RecentOptions{Node: "fleet", Kind: "power_event", Limit: 5})
	return PowerEventReport{
		Kind:             "meshclaw_opsdb_power_events",
		Generated:        time.Now().UTC(),
		WindowSeconds:    int64(opts.Window.Seconds()),
		UptimeMaxSeconds: int64(opts.UptimeMax.Seconds()),
		MinNodes:         opts.MinNodes,
		Incidents:        incidents,
		RecordedEvents:   recorded.Events,
	}, nil
}

func (db DB) nodeRestarts(uptimeMax time.Duration) ([]PowerNodeRestart, error) {
	dir := db.Paths().HistoryDir
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []PowerNodeRestart
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		reports, err := readStoredReports(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		for i := 1; i < len(reports); i++ {
			prev, curr := reports[i-1], reports[i]
			prevBoot := strings.TrimSpace(prev.Report.Identity.BootIDFingerprint)
			currBoot := strings.TrimSpace(curr.Report.Identity.BootIDFingerprint)
			if prevBoot == "" || currBoot == "" || prevBoot == currBoot {
				continue
			}
			if curr.Report.System.UptimeSeconds <= 0 || time.Duration(curr.Report.System.UptimeSeconds)*time.Second > uptimeMax {
				continue
			}
			node := strings.TrimSpace(curr.Report.NodeName)
			if node == "" {
				node = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			}
			out = append(out, PowerNodeRestart{
				Node:                    node,
				Hostname:                strings.TrimSpace(curr.Report.Hostname),
				PreviousCollectedAt:     firstTime(prev.Report.CollectedAt, prev.StoredAt),
				CurrentCollectedAt:      firstTime(curr.Report.CollectedAt, curr.StoredAt),
				PreviousBootFingerprint: prevBoot,
				CurrentBootFingerprint:  currBoot,
				CurrentUptimeSeconds:    curr.Report.System.UptimeSeconds,
			})
		}
	}
	return out, nil
}

func readStoredReports(path string) ([]storedNodeReport, error) {
	items, err := readJSONL[json.RawMessage](path)
	if err != nil {
		return nil, err
	}
	var reports []storedNodeReport
	for _, raw := range items {
		var report storedNodeReport
		if err := json.Unmarshal(raw, &report); err == nil && !firstTime(report.Report.CollectedAt, report.StoredAt).IsZero() {
			reports = append(reports, report)
		}
	}
	sort.SliceStable(reports, func(i, j int) bool {
		return firstTime(reports[i].Report.CollectedAt, reports[i].StoredAt).Before(firstTime(reports[j].Report.CollectedAt, reports[j].StoredAt))
	})
	return reports, nil
}

func powerIncidentSummary(nodes []PowerNodeRestart) string {
	names := make([]string, 0, len(nodes))
	for _, node := range nodes {
		names = append(names, node.Node)
	}
	sort.Strings(names)
	return "Multiple nodes changed boot identity within the configured window: " + strings.Join(names, ", ")
}

func firstTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}
