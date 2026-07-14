# MeshClaw Inventory

MeshClaw inventory is file-backed. The control plane should not ship with a private fleet hardcoded into the binary.

## Source Of Truth

Default path:

```bash
~/.meshclaw/inventory.json
```

Override path:

```bash
MESHCLAW_INVENTORY_FILE=/path/to/inventory.json
```

Role/tag override path:

```bash
~/.meshclaw/inventory_overrides.json
MESHCLAW_INVENTORY_OVERRIDES_FILE=/path/to/inventory_overrides.json
```

Override config directory:

```bash
MESHCLAW_CONFIG_DIR=/path/to/config
```

## Commands

Initialize or refresh the saved inventory:

```bash
meshclaw inventory-init --force
```

Discover nodes without writing:

```bash
meshclaw inventory-discover
```

Compare saved inventory with current discovery:

```bash
meshclaw inventory-diff
```

List the active inventory:

```bash
meshclaw list
```

Manage role/tag overrides without editing JSON by hand:

```bash
meshclaw inventory-override list
meshclaw inventory-override set c1 --role mail-server --tag mail --tag mox
meshclaw inventory-override remove c1
```

## Discovery Inputs

MeshClaw merges these sources:

- `tailscale status --json` for MagicDNS names, Tailscale IPs, LAN hints, OS, and online state.
- `vssh list` for Wire/vssh mesh IPs.
- `~/.wire/users.json` for SSH fallback users.
- Legacy `~/.meshclaw/nodes.json` only as a migration source.

The merge keeps operator-managed user fields while refreshing network facts from live discovery.

## Role And Tag Overrides

Discovery intentionally starts conservative: a host named `c1` may only be
known as a generic Linux VPS until MeshClaw has operator-owned context. Put
human-maintained role and tag refinements in `inventory_overrides.json` instead
of hardcoding private fleet meaning into the binary.

Example:

```json
{
  "version": 1,
  "nodes": [
    {
      "name": "c1",
      "role": "mail-server",
      "tags": ["linux", "vps", "mail", "mox"]
    },
    {
      "name": "g4",
      "role": "automation-worker",
      "tags": ["linux", "gpu", "automation", "n8n"]
    }
  ]
}
```

Overrides are applied after saved inventory and live discovery, so they can
refine roles and add tags while keeping fresh Tailscale, vssh, LAN, user, OS,
and online state facts.

## MCP Tools

AI clients can inspect inventory through:

- `meshclaw_server_list`
- `meshclaw_inventory_discover`
- `meshclaw_inventory_diff`
- `meshclaw_node_repair_plan`

Use `meshclaw_inventory_diff` before routing fleet work when the saved state may be stale.

## Repair Planning

For AI operators, `meshclaw_node_repair_plan` is the canonical first call when a node cannot run vssh-native commands.

It combines:

- daemon reachability
- daemon/client protocol version
- run-many and facts auth paths
- inventory endpoint and SSH fallback target
- target OS artifact selection for vssh daemon upgrades
- safe next actions with `read_only`, `safe_apply`, or `requires_root` modes

Example:

```bash
meshclaw node-repair-plan --hosts s1,s2
```

The tool returns a plan even when one node is down. One unreachable host should not block the whole fleet diagnosis.

When MeshClaw has a fresh Tailscale endpoint in inventory, repair planning uses that endpoint directly instead of trusting an older vssh route cache.
