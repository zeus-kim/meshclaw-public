# Contributing

MeshClaw is currently early-stage developer preview software.

## Local Checks

Run these before opening a PR:

```sh
go test ./...
go build -o ./bin/meshclaw ./cmd/meshclaw
./bin/meshclaw --help
```

## Guidelines

- Keep user-facing copy behind language packs when practical.
- Keep mutating actions split into plan/apply flows.
- Do not commit private inventories, local evidence, personal deployment
  scripts, or credentials.
- Prefer focused tests for runtime, policy, MCP, Guard, and assistant behavior.

## Public-Safe Fixtures

Use example domains, example IP ranges, placeholder users, and synthetic
tokens. Avoid strings that look like real provider secrets, even in tests.
