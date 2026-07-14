# MeshClaw Example Workflows

These examples are intentionally generic. They are meant to show MeshClaw as an
AI-native operations runtime rather than an email-only demo.

Create a new workflow scaffold:

```sh
meshclaw workflows scaffold my-first-ops
meshclaw workflows validate ~/.meshclaw/workflows/my-first-ops.json
meshclaw run ~/.meshclaw/workflows/my-first-ops.json --dry-run
```

Validate before running:

```sh
meshclaw workflows validate examples/workflows/fleet-health.json
meshclaw workflows validate examples/workflows/service-triage-autoheal.json
meshclaw workflows validate examples/workflows/model-worker-orchestration.json
```

Run directly by file path:

```sh
meshclaw run examples/workflows/fleet-health.json --dry-run
```

Or load them by name:

```sh
export MESHCLAW_WORKFLOW_DIR="$PWD/examples/workflows"
meshclaw workflows
meshclaw run fleet-health --dry-run
```

The examples use placeholders such as `TARGET_HOST` and `TARGET_SERVICE`.
Replace those with real node/service names before execute mode.
