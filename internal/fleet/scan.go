package fleet

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/meshclaw/meshclaw/internal/hygiene"
	"github.com/meshclaw/meshclaw/internal/inventory"
	"github.com/meshclaw/meshclaw/internal/monitor"
	"github.com/meshclaw/meshclaw/internal/workflow"
)

type Options struct {
	Hosts       []string `json:"hosts,omitempty"`
	Security    bool     `json:"security"`
	Hygiene     bool     `json:"hygiene"`
	Logs        bool     `json:"logs"`
	MaxParallel int      `json:"max_parallel"`
}

type Report struct {
	Time    time.Time                     `json:"time"`
	Options Options                       `json:"options"`
	States  map[string]*monitor.NodeState `json:"states"`
	Alerts  []monitor.Alert               `json:"alerts"`
	Hosts   []HostReport                  `json:"hosts"`
}

type HostReport struct {
	Host     string           `json:"host"`
	Online   bool             `json:"online"`
	Error    string           `json:"error,omitempty"`
	Security *workflow.Report `json:"security,omitempty"`
	Logs     *workflow.Report `json:"logs,omitempty"`
	Hygiene  *hygiene.Report  `json:"hygiene,omitempty"`
}

func Scan(opts Options) (Report, error) {
	opts = normalizeOptions(opts)
	selected, err := selectedHosts(opts.Hosts)
	if err != nil {
		return Report{}, err
	}

	m, err := monitor.New(monitor.DefaultConfig())
	if err != nil {
		return Report{}, err
	}
	states := m.CheckAll()
	alerts := m.DetectAlerts()

	report := Report{
		Time:    time.Now().UTC(),
		Options: opts,
		States:  states,
		Alerts:  alerts,
		Hosts:   make([]HostReport, len(selected)),
	}

	sem := make(chan struct{}, opts.MaxParallel)
	var wg sync.WaitGroup
	for i, host := range selected {
		i, host := i, host
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			report.Hosts[i] = scanHost(host, states[host], opts)
		}()
	}
	wg.Wait()
	return report, nil
}

func normalizeOptions(opts Options) Options {
	if !opts.Security && !opts.Hygiene && !opts.Logs {
		opts.Security = true
		opts.Hygiene = true
		opts.Logs = true
	}
	if opts.MaxParallel <= 0 {
		opts.MaxParallel = envInt("MESHCLAW_FLEET_SCAN_PARALLEL", 3)
	}
	if opts.MaxParallel <= 0 {
		opts.MaxParallel = 3
	}
	return opts
}

func selectedHosts(hosts []string) ([]string, error) {
	if len(hosts) == 0 {
		nodes := inventory.DefaultNodes()
		selected := make([]string, 0, len(nodes))
		for _, node := range nodes {
			selected = append(selected, node.Name)
		}
		return selected, nil
	}
	selected := make([]string, 0, len(hosts))
	seen := map[string]bool{}
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" || seen[host] {
			continue
		}
		if _, ok := inventory.Find(host); !ok {
			return nil, fmt.Errorf("unknown node: %s", host)
		}
		seen[host] = true
		selected = append(selected, host)
	}
	return selected, nil
}

func scanHost(host string, state *monitor.NodeState, opts Options) HostReport {
	result := HostReport{Host: host}
	if state == nil {
		result.Error = "node has no monitor state"
		return result
	}
	result.Online = state.Online
	if !state.Online {
		result.Error = state.Error
		if result.Error == "" {
			result.Error = "node is offline"
		}
		return result
	}
	if opts.Security {
		security := workflow.SecurityCheck(host)
		result.Security = &security
	}
	if opts.Logs {
		logs := workflow.AnalyzeLogs(host, "system")
		result.Logs = &logs
	}
	if opts.Hygiene {
		hygieneReport := hygiene.ScanHost(host)
		result.Hygiene = &hygieneReport
	}
	return result
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
