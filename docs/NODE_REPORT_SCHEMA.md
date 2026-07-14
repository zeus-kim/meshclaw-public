# MeshClaw Node Report Schema

MeshClaw node agents are read-only sensors. They do not run an LLM on every
server. Each node emits a structured `meshclaw_node_report`; MeshClaw stores the
reports, compares them over time, and lets Codex, Claude, Open WebUI, Argos, or a
local model explain the state in natural language.

Current schema version: `3`.

## Top-Level Contract

```json
{
  "kind": "meshclaw_node_report",
  "version": "3",
  "hostname": "g4",
  "node_name": "g4",
  "collected_at": "2026-05-26T00:00:00Z",
  "agent": {},
  "identity": {},
  "system": {},
  "gpus": [],
  "docker": {},
  "network": {},
  "remote_access": {},
  "firewall": {},
  "fail2ban": {},
  "schedules": {},
  "logins": {},
  "health": {},
  "workloads": [],
  "processes": [],
  "services": [],
  "logs": {},
  "security": [],
  "inventory": {},
  "capacity": {},
  "ai_view": {},
  "errors": []
}
```

## What Each Section Means

| Section | Purpose |
| --- | --- |
| `agent` | Collector metadata: schema date, binary path, user, timeout. This helps detect stale agents. |
| `identity` | Stable but redacted identity signals: timezone, boot fingerprint, machine fingerprint, virtualization. Raw IDs are never emitted. |
| `system` | OS, architecture, CPU count, load, memory, root disk, uptime. |
| `gpus` | GPU name, utilization, memory, and temperature when `nvidia-smi` exists. |
| `docker` | Docker availability, container counts, container summaries, health details, restart/OOM/exit signals, and public port mappings. |
| `network` | Interfaces, listeners, default routes, and DNS. Listeners are marked `public` when bound to external addresses. |
| `remote_access` | Tailscale availability/running/IPs, vssh listener state, SSH listener state. |
| `firewall` | UFW/firewalld/pf status and warnings. |
| `fail2ban` | fail2ban availability, running state, jails, and errors. |
| `schedules` | cron samples, systemd timers, launchd agents/daemons. |
| `logins` | current logged-in users and bounded recent login samples. |
| `health` | reboot-required, time sync, package updates, swap, mounts, failed units, auth/kernel signals, and container health warnings. |
| `processes` | Top resource-consuming processes, redacted and classified by purpose. |
| `workloads` | LLM-friendly rollups that explain what the node is being used for right now. |
| `services` | Selected important service states such as ssh, docker, tailscaled, vsshd, ollama, open-webui. |
| `logs` | Bounded recent error-log samples. |
| `security` | Structured findings with category/severity/title/detail. |
| `inventory` | Inferred role/tags for inventory management, with confidence and observed worker count. |
| `capacity` | Schedulability and headroom judgment: CPU, memory, disk, GPU, blockers, suggested/avoid uses. |
| `ai_view` | A plain-language interpretation layer for general LLMs. Prefer this for the first answer to a user. |
| `errors` | Collector errors and timeouts. Collection is best-effort and should not fail the whole report. |

## Docker Health

Schema v3 extends each `docker.containers[]` item with optional health and
restart diagnostics. Older consumers can ignore these fields because they are
omitted when the value is empty or zero:

```json
{
  "name": "open-webui",
  "image": "ghcr.io/open-webui/open-webui:main",
  "state": "running",
  "status": "Up 2 hours (healthy)",
  "ports": "0.0.0.0:8080->8080/tcp",
  "health_status": "healthy",
  "restart_count": 1,
  "oom_killed": false,
  "exit_code": 0,
  "started_at": "2026-06-23T00:00:00.000000000Z"
}
```

`health_status` is one of `healthy`, `unhealthy`, `starting`, or `none`.
Crash-loop style signals are rolled up into `health.warnings`, for example:

```json
{
  "warnings": [
    "docker container worker is unhealthy",
    "docker container api has high restart count 12"
  ]
}
```

## Log Findings

Schema v3 may include optional structured log findings in `logs.log_findings`.
Samples are bounded and redacted before they are stored:

```json
{
  "logs": {
    "recent_error_count": 2,
    "samples": ["kernel: Out of memory: Killed process ..."],
    "log_findings": [
      {
        "severity": "critical",
        "source": {"type": "host_journal", "name": "journal"},
        "pattern": "oom",
        "count": 1,
        "sample": "kernel: Out of memory: Killed process ..."
      }
    ]
  }
}
```

Known patterns include `oom`, `crash_loop`, `auth_failure`, and `http_5xx`.
Older consumers can ignore `log_findings`.

## AI View

`ai_view` exists because generic models should not need to reverse-engineer
operational meaning from raw `ps`, `ss`, `docker`, and `journalctl` fields.
It uses plain field names and short human-readable strings:

```json
{
  "plain_summary": "g4 is classified as llm-worker; overall pressure is medium; it is currently schedulable for LLM inference and GPU jobs.",
  "what_this_node_is": "This node appears to be useful for local AI model inference.",
  "what_is_running": [
    "llm inference using 1 process(es) at about 42.0% CPU and 18.0% memory",
    "containerized app using 3 process(es); containers: open-webui, n8n"
  ],
  "can_use_for": ["LLM inference", "GPU jobs"],
  "should_avoid": ["large Docker builds"],
  "immediate_concerns": ["medium: public listening ports present (8 public listeners)"],
  "recommended_actions": ["Confirm public listening ports are intentional."],
  "interpretation_tips": [
    "Use ai_view for a quick natural-language answer.",
    "Use inventory for role/tag decisions.",
    "Use capacity before scheduling new work."
  ]
}
```

Rules for AI clients:

- Start with `ai_view.plain_summary`.
- Use `ai_view.immediate_concerns` for the first risk paragraph.
- Use `ai_view.recommended_actions` for next steps.
- Use `inventory` only when choosing or updating node roles.
- Use `capacity` before placing new jobs.
- Use raw sections only when the user asks for evidence or diagnosis detail.

## Inventory Management

`inventory` is the bridge between raw monitoring and fleet planning. It answers:

- What is this node probably for?
- Which tags should be assigned?
- How confident is MeshClaw?
- How many active workers or runnable lanes are visible?

Example:

```json
{
  "primary_role": "llm-worker",
  "secondary_roles": ["execution-node", "app-host"],
  "tags": ["linux", "amd64", "gpu", "llm", "docker", "tailscale", "vssh"],
  "reason": "GPU detected: NVIDIA RTX A6000; observed workloads: llm_inference, containerized_app",
  "confidence": "high",
  "observed_workers": 3
}
```

## Capacity Management

`capacity` is what an AI operator should use before scheduling work. It answers:

- Is this node currently schedulable?
- Is CPU, memory, disk, or GPU tight?
- What should be scheduled here?
- What should be avoided?

Example:

```json
{
  "cpu_headroom": "high",
  "memory_headroom": "medium",
  "disk_headroom": "low",
  "gpu_headroom": "high",
  "schedulable": false,
  "blockers": ["disk above 90 percent"],
  "suggested_use": ["read-only monitoring"],
  "avoid_use": ["model training", "large Docker builds"],
  "overall_pressure": "high"
}
```

## Change Tracking

The report is stored under:

```text
~/.meshclaw/state/nodes/<node>.json
~/.meshclaw/state/history/<node>.jsonl
```

MeshClaw should compare snapshots over day/week windows and report:

- role/tag changes
- new or removed workloads
- new public ports
- failed service changes
- package/update drift
- disk/memory/load pressure changes
- security finding changes

This is the basis for questions like:

```text
What changed on g4 since yesterday?
Which nodes became risky this week?
Which server has room for another local model?
Why did MeshClaw classify d1 as not schedulable?
```
