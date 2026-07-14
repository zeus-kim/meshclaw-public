# Security Policy

MeshClaw controls infrastructure, local automation, and assistant workflows.
Treat it as high-trust software.

## Supported Versions

This repository is currently a developer preview. Security fixes target the
default development branch and the latest public-ready branch.

## Reporting A Vulnerability

Open a private security advisory on GitHub if available. If advisories are not
enabled, contact the repository owner directly and do not publish exploit
details until a fix is available.

## Secret Handling

- Do not commit credentials, API keys, private keys, Signal group IDs, vault
  material, local evidence bundles, or private fleet inventories.
- Use environment variables or local files under `~/.meshclaw` for private
  runtime configuration.
- Use Guard/vault handles for secret use. Models and MCP clients should receive
  handles and redacted metadata, not raw secret values.

## Safe Defaults

- Prefer dry-run workflows before execute mode.
- Keep destructive operations behind explicit approval.
- Review generated evidence before using it as an audit record.
