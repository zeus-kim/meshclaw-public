package runtimeflow

type AdapterSpec struct {
	Name                string `json:"name"`
	Kind                string `json:"kind"`
	Executable          bool   `json:"executable"`
	RequiresCommand     bool   `json:"requires_command"`
	ApprovalCanExecute  bool   `json:"approval_can_execute"`
	Description         string `json:"description"`
	RecommendedForAI    string `json:"recommended_for_ai"`
	UnsupportedBehavior string `json:"unsupported_behavior,omitempty"`
}

func AdapterRegistry() []AdapterSpec {
	return []AdapterSpec{
		{
			Name:               "vssh",
			Kind:               "remote-exec",
			Executable:         true,
			RequiresCommand:    true,
			ApprovalCanExecute: true,
			Description:        "Run a structured command on a managed remote node through the vssh-backed runtime adapter.",
			RecommendedForAI:   "Use for read-only server inspection and approved remote operations that need evidence and policy checks.",
		},
		{
			Name:               "local",
			Kind:               "local-exec",
			Executable:         true,
			RequiresCommand:    true,
			ApprovalCanExecute: true,
			Description:        "Run a structured command on the local controller node.",
			RecommendedForAI:   "Use for local artifact generation, report rendering, and controller-side verification.",
		},
		{
			Name:                "manual",
			Kind:                "human-or-external",
			Executable:          false,
			RequiresCommand:     false,
			ApprovalCanExecute:  false,
			Description:         "Record a human, browser, app, or external-system step without pretending MeshClaw executed it.",
			RecommendedForAI:    "Use for checklist, UI, or provider actions until a concrete adapter exists.",
			UnsupportedBehavior: "Skipped with structured evidence in execute mode.",
		},
		{
			Name:                "policy",
			Kind:                "approval-gate",
			Executable:          false,
			RequiresCommand:     false,
			ApprovalCanExecute:  false,
			Description:         "Represent an approval gate or policy-only workflow boundary.",
			RecommendedForAI:    "Use to separate human approval from actual execution adapters.",
			UnsupportedBehavior: "Approval is recorded, but no external action is performed.",
		},
		{
			Name:                "mail",
			Kind:                "domain-adapter-placeholder",
			Executable:          false,
			RequiresCommand:     false,
			ApprovalCanExecute:  false,
			Description:         "Placeholder for future mail send/receive adapters.",
			RecommendedForAI:    "Use only to model mail workflow intent until a concrete mail adapter is configured.",
			UnsupportedBehavior: "Skipped with approval/evidence metadata.",
		},
		{
			Name:                "dns",
			Kind:                "domain-adapter-placeholder",
			Executable:          false,
			RequiresCommand:     false,
			ApprovalCanExecute:  false,
			Description:         "Placeholder for future DNS provider adapters such as Cloudflare.",
			RecommendedForAI:    "Use to model DNS changes with approval before a provider adapter is installed.",
			UnsupportedBehavior: "Skipped with approval/evidence metadata.",
		},
		{
			Name:                "browser",
			Kind:                "domain-adapter-placeholder",
			Executable:          false,
			RequiresCommand:     false,
			ApprovalCanExecute:  false,
			Description:         "Placeholder for browser automation adapters.",
			RecommendedForAI:    "Use for browser-required workflows until a browser adapter is wired into runtime execution.",
			UnsupportedBehavior: "Skipped with structured evidence.",
		},
		{
			Name:                "cloud",
			Kind:                "domain-adapter-placeholder",
			Executable:          false,
			RequiresCommand:     false,
			ApprovalCanExecute:  false,
			Description:         "Placeholder for cloud and VPS provider adapters.",
			RecommendedForAI:    "Use for provisioning plans and approval-gated provider operations.",
			UnsupportedBehavior: "Skipped until a provider adapter is configured.",
		},
	}
}

func adapterByName(name string) (AdapterSpec, bool) {
	for _, adapter := range AdapterRegistry() {
		if adapter.Name == name {
			return adapter, true
		}
	}
	return AdapterSpec{
		Name:                name,
		Kind:                "unknown",
		Executable:          false,
		RequiresCommand:     false,
		ApprovalCanExecute:  false,
		Description:         "Unknown adapter.",
		UnsupportedBehavior: "Skipped until the adapter is registered.",
	}, false
}

func stepExecutable(step StepSpec) bool {
	adapter, ok := adapterByName(step.Transport)
	return ok && adapter.Executable && (!adapter.RequiresCommand || step.Command != "")
}

func approvalCanExecute(step StepSpec) bool {
	adapter, ok := adapterByName(step.Transport)
	return ok && adapter.ApprovalCanExecute && (!adapter.RequiresCommand || step.Command != "")
}

func adapterReason(adapter AdapterSpec, step StepSpec) string {
	if adapter.Executable && adapter.RequiresCommand && step.Command == "" {
		return "adapter requires a command, but this step has no command"
	}
	if adapter.Executable {
		return adapter.RecommendedForAI
	}
	if adapter.UnsupportedBehavior != "" {
		return adapter.UnsupportedBehavior
	}
	return adapter.Description
}
