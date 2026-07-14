package provision

import (
	"fmt"
	"strings"
	"time"
)

type Request struct {
	Purpose       string  `json:"purpose"`
	Provider      string  `json:"provider"`
	Region        string  `json:"region"`
	BudgetUSD     float64 `json:"budget_usd"`
	TTLHours      int     `json:"ttl_hours"`
	RequiredClass string  `json:"required_class"`
}

type Step struct {
	ID       string `json:"id"`
	Mode     string `json:"mode"`
	Action   string `json:"action"`
	Reason   string `json:"reason"`
	Evidence string `json:"evidence,omitempty"`
}

type Plan struct {
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	Request      Request   `json:"request"`
	Decision     string    `json:"decision"`
	EstimatedMax string    `json:"estimated_max"`
	Steps        []Step    `json:"steps"`
}

func NewPlan(request Request) Plan {
	request.Purpose = strings.TrimSpace(request.Purpose)
	if request.Provider == "" {
		request.Provider = "provider-api"
	}
	if request.Region == "" {
		request.Region = "nearest-available"
	}
	if request.TTLHours == 0 {
		request.TTLHours = 6
	}
	if request.RequiredClass == "" {
		request.RequiredClass = "small-vps"
	}
	status := "plan_only"
	decision := "requires_approval_for_cost"
	if request.Purpose == "" {
		status = "invalid"
		decision = "missing purpose"
	}
	return Plan{
		Status:    status,
		CreatedAt: time.Now().UTC(),
		Request:   request,
		Decision:  decision,
		EstimatedMax: fmt.Sprintf("budget_cap_usd=%.2f ttl_hours=%d class=%s",
			request.BudgetUSD, request.TTLHours, request.RequiredClass),
		Steps: []Step{
			{
				ID:     "capacity_check",
				Mode:   "read_only",
				Action: "check existing fleet capacity before renting",
				Reason: "Avoid cost when current servers are enough.",
			},
			{
				ID:     "provider_quote",
				Mode:   "plan_only",
				Action: "request provider quote without creating resources",
				Reason: "Cost-incurring API calls are separated from planning.",
			},
			{
				ID:     "provision_server",
				Mode:   "requires_approval",
				Action: "create temporary server with TTL, owner, purpose, and teardown policy",
				Reason: "This changes provider state and can incur cost.",
			},
			{
				ID:     "bootstrap_server",
				Mode:   "requires_approval",
				Action: "install SSH, Tailscale, optional meshclaw-agent, and attach to inventory",
				Reason: "New servers must be auditable and reachable through the same control plane.",
			},
			{
				ID:     "deprovision_server",
				Mode:   "requires_approval",
				Action: "destroy temporary server at TTL expiry or when workload completes",
				Reason: "Temporary capacity must not leak cost.",
			},
		},
	}
}
