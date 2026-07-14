# MeshClaw

MeshClaw is an AI-native runtime for operating infrastructure and local
assistant workflows through policy, approval gates, structured evidence, and
MCP tools.

The repository contains three related product lines:

- **Argos Assistant**: Signal/local assistant workflows, briefings, documents,
  mail, calendar, memory, and task automation.
- **MeshClaw Ops**: server, VM, container, log, monitoring, and remediation
  workflows.
- **MeshClaw Agent**: lightweight node-local collectors and bounded execution
  helpers.

MeshClaw is designed to be called by Codex, Claude, Cursor, local models, or an
assistant frontend. It is not a replacement for those tools; it provides shared
runtime state, safer execution boundaries, and evidence records.

## Status

This branch is a public-ready source snapshot. Private fleet inventories,
Signal room IDs, credentials, local evidence, and operator handoff logs are not
included.

The project is still early and should be treated as developer preview software.
Do not point it at production systems until you have reviewed the policy and
approval model.

## Quick Start

```sh
git clone https://github.com/zeus-kim/meshclaw-public.git
cd meshclaw
go test ./...
go build -o ./bin/meshclaw ./cmd/meshclaw
./bin/meshclaw --help
./bin/meshclaw mcp
```

Optional Python wrapper (downloads the official binary for you):

```sh
pip install meshclaw-go
meshclaw --help
```

## First Things To Read

- [Architecture](docs/ARCHITECTURE_2026-05-23.md)
- [Argos and MeshClaw](docs/ARGOS_AND_MESHCLAW.md)
- [MCP setup](docs/MCP_SETUP.md)
- [Policy](docs/POLICY.md)
- [Guard](docs/GUARD.md)
- [Public install UX](docs/PUBLIC_INSTALL_UX.md)

## Safety Model

MeshClaw separates planning from execution.

- Read-only inspection and evidence generation are the default.
- Mutating actions should be represented as plan/apply flows.
- Secrets should be referenced through vault handles, not copied into prompts,
  logs, memory, or evidence.
- Sending mail, deleting data, changing accounts, payments, bookings, and other
  irreversible actions require explicit approval in the runtime layer.

## Runtime Configuration

The source tree builds without private configuration. Live fleet execution
requires your own inventory, policy, vssh/SSH access, and optional model/API
configuration under `~/.meshclaw`.

Useful local paths are examples only:

```sh
mkdir -p ~/.meshclaw
./bin/meshclaw init
./bin/meshclaw doctor --json
./bin/meshclaw workflows inspect fleet-health-demo --json
./bin/meshclaw run fleet-health-demo --dry-run --json
```

## Development

```sh
go test ./...
go test ./cmd/meshclaw ./internal/monitor ./internal/guard ./internal/messenger
go build -o ./bin/meshclaw ./cmd/meshclaw
```

## Repository Hygiene

This public snapshot intentionally omits private handoff documents, local
evidence bundles, built binaries, and deployment scripts that were specific to a
single operator environment.

See [PUBLICATION.md](PUBLICATION.md) for the public-release checklist.
