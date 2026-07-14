# Policy

MeshClaw policy answers what an operator may do. Codex, Claude, local LLMs,
and automations can share the same MCP server while receiving different
decisions.

## File

Default path:

```text
~/.meshclaw/policy.json
```

Override path:

```sh
MESHCLAW_POLICY_FILE=/path/to/policy.json meshclaw policy-show
```

Create the default DevOps operator template:

```sh
meshclaw policy-init --preset devops
```

List presets:

```sh
meshclaw policy-presets
```

## Rule Format

Rules are evaluated top-to-bottom before built-in defaults.

```json
{
  "version": "1",
  "rules": [
    {
      "id": "codex-read-only",
      "subject": "codex",
      "action": "fleet_scan",
      "resource": "server",
      "decision": "allow",
      "reason": "Codex may inspect fleet state and evidence"
    },
    {
      "id": "local-llm-no-shell",
      "subject": "local-llm",
      "action": "run_command",
      "resource": "server",
      "decision": "require_approval",
      "reason": "Local model shell execution needs operator approval"
    },
    {
      "id": "deny-shadow-read",
      "action": "run_command",
      "resource": "server",
      "context": "/etc/shadow",
      "decision": "deny",
      "reason": "Credential material must not be read"
    }
  ]
}
```

Supported decisions:

- `allow`
- `require_approval`
- `deny`

`subject`, `action`, and `resource` support exact values or `*`. `context`
matches a case-insensitive substring.

## Presets

`devops` is the default. It allows Codex, Claude, ChatGPT, local LLMs, and
Open WebUI to inspect fleet state, logs, security, hygiene findings, policy,
and evidence. It gates mutating or unknown local-model actions behind
`require_approval`.

`strict` keeps local LLM actions approval-gated unless the operator adds narrower
allow rules.
