# MeshClaw Guard

MeshClaw Guard is the local safety layer for AI-assisted operations. It is not
a chatbot and it does not need a model for its core work. It gives Codex,
Claude, Cursor, Open WebUI, and local models a safer way to handle secret-
adjacent work.

Guard owns three modes:

- `credential`: detect and redact pasted secrets, then turn raw values into
  capability-handle workflow.
- `posture`: check whether the local operator environment is safe for sensitive
  AI work, including local model memory and chat-history stores.
- `vuln`: find vulnerability and hygiene risks without returning raw values.
  This covers three things: scanning local files for leaked credentials, scanning
  a fleet node's installed packages against the OSV.dev CVE database, and running
  deterministic SAST (semgrep + bandit) against a repo on a fleet node. The two
  fleet scans were moved here from pwagent so the credential vault stays focused on
  secrets and the DevOps/fleet security surface lives in Guard.

The first rule is simple: models may receive redacted context and handles, never
raw secret values.

## Why Guard Exists

AI clients are good at reasoning, planning, and coding, but they should not be
trusted as a raw credential store. A user can still accidentally paste a
password, API key, DKIM key, or provider token into a conversation. Guard gives
the runtime a deterministic layer that can:

- detect sensitive values before they enter evidence or model context
- redact text while preserving useful structure
- mark rotation and cleanup as approval-gated actions
- write audit evidence without leaking the value
- check local model memory, RAG, and chat-history risk

This logic is intentionally model-less. Regex, entropy checks, policy, and
evidence are deterministic and testable.

## Optional Local Guard Chat

Most operations should stay with Codex, Claude, Cursor, or another strong AI
operator. They can explain plans, inspect evidence, run MCP tools, and continue
orchestration. Passwords, private keys, recovery codes, raw API tokens, and
vault work are the exception.

For those moments MeshClaw should provide a local-only Guard Chat: a small local
model and UI that helps the user talk through credential work without sending
raw values to a cloud model. This local chat is not the main MeshClaw brain. It
is the private credential conversation window.

Signal is the preferred future mobile ingress channel for raw secrets. Matrix
is intentionally not the secret room: Matrix rooms, bridges, search, and
history are too easy to connect to other models. Signal may be used for
short-lived secret entry, approvals, and redacted reports, but raw secret
messages must not be forwarded into Claude, Codex, GPT, MCP evidence, or RAG.

Codex and Claude can still automate reports through a messenger. They should
read MeshClaw evidence and post redacted status summaries, failed steps, next
actions, approval requests, and evidence paths. They must not read or quote the
raw Signal Guard ingress messages.

The local model may explain findings, guide the user through cleanup, or ask
natural follow-up questions. It must not become the authority for whether a
value is safe.

Recommended division:

- Guard core: detection, redaction, posture, policy, evidence
- Optional local model: Korean/English explanation, operator coaching, cleanup
  checklist narration, vault conversation
- MeshClaw policy: approval requirements and deny rules
- Vault/provider adapters: actual secret storage and short-lived secret use

Sensitive-mode conversations should start by disabling persistent memory,
history sync, and RAG ingestion in the chosen local UI. Guard can check for
known local stores, but it cannot guarantee every third-party UI is clean.

Guard Chat should eventually support local vault operations through handles:

- create or import a secret into a local vault through local-only input
- show metadata, not raw values, to cloud AI operators
- produce `vault://...` handles for Codex/Claude workflows
- approve use-only injection into adapters without revealing the value
- plan rotation when a secret may have leaked
- delete sensitive local chat history after the session

This keeps the product boundary clear:

- Codex/Claude/Cursor: general reasoning and orchestration
- MeshClaw Runtime: state, policy, execution, evidence, workflows
- MeshClaw Guard: secret detection, redaction, posture, vault handles
- Local Guard Chat: password/vault conversation only

## CLI

```sh
meshclaw guard-modes --json
meshclaw guard-model --json
meshclaw guard-clients --json
meshclaw guard-session policy --json
meshclaw guard-session start --surface signal --json
meshclaw guard-session end guard-... --json
meshclaw guard-session purge --json
meshclaw guard-signal-policy --json
meshclaw messenger target-add owner-signal --channel signal --recipient +8210... --json
meshclaw messenger target-add my-guard-room --channel signal --group-id '<user-created-signal-group-id>' --label 'My Guard Room' --mode guard --json
meshclaw messenger report latest --channel signal --json
meshclaw messenger approval-request latest send-approval --channel signal --json
meshclaw messenger send-report owner-signal latest --dry-run --json
meshclaw guard-vault --json
meshclaw guard-vault-backends --json
meshclaw guard-vault-init --json
printf '%s' "$TOKEN" | meshclaw guard-vault-put cloudflare dns-token api-token "Cloudflare DNS token" --backend keychain --json
meshclaw guard-vault-list --json
meshclaw guard-vault-metadata vault://meshclaw/cloudflare/dns-token --json
meshclaw guard-vault-use vault://meshclaw/cloudflare/dns-token CLOUDFLARE_TOKEN --approve --actor operator --reason "read DNS zones" -- provider-cli zones list --json
meshclaw guard-detect chat "token=..." --json
meshclaw guard-redact chat "password=..." --json
meshclaw guard-posture --json
meshclaw guard-clean-plan --json
meshclaw guard-vuln /path/to/project --json
meshclaw guard-intent "store my Cloudflare token" --json
meshclaw guard-local-server 127.0.0.1:18765
```

Every command returns structured JSON with status, findings, recommended
actions, and evidence paths where applicable.

## MCP Tools

AI clients should call these tools instead of parsing prose:

- `meshclaw_guard_modes`
- `meshclaw_guard_model`
- `meshclaw_guard_clients`
- `meshclaw_guard_session_policy`
- `meshclaw_guard_signal_policy`
- `meshclaw_guard_vault`
- `meshclaw_guard_vault_backends`
- `meshclaw_guard_vault_list`
- `meshclaw_guard_vault_metadata`
- `meshclaw_guard_detect`
- `meshclaw_guard_redact`
- `meshclaw_guard_posture`
- `meshclaw_guard_clean_plan`
- `meshclaw_guard_vuln`
- `meshclaw_guard_vuln_scan`
- `meshclaw_guard_code_review`
- `meshclaw_guard_intent`

The tools use underscore-only names for Codex, Claude, and Cursor MCP
compatibility.

`meshclaw_guard_vuln_scan` and `meshclaw_guard_code_review` are the fleet-facing
vuln-mode scanners (MCP-only; no CLI subcommand). They run over the same
policy-checked `run_evidence` transport the rest of MeshClaw uses, and store
evidence like every other Guard action.

`meshclaw_guard_vuln_scan` collects a node's installed-package inventory
(deb/PyPI/npm/brew/cargo) and queries OSV.dev, returning CVE counts and
high-severity matches with fixed versions. Only package names and versions leave
the host; it is read-only and patching stays approval-gated. This replaces
pwagent's old `vuln_scan`.

`meshclaw_guard_code_review` runs deterministic SAST (semgrep + bandit) against a
repository on a node and aggregates findings by tool and severity. Source
snippets are deliberately not returned, so scan output never carries code off the
host. It is the deterministic core of pwagent's old `ai_edits`; optional LLM
triage of the findings can be layered on top via MeshClaw's model gateway. Secret
or credential leak detection is not duplicated here — that stays in
`meshclaw_guard_vuln` / hygiene.

`meshclaw_guard_clients` is the productized local-client safety guide. It tells
AI operators how to treat Ollama CLI, Open WebUI, LM Studio, Enchanted,
Cursor/Continue/VS Code extensions, and cloud AI clients. The output is
structured, not prose-only: each client has `risk`, `required_settings`,
`local_stores`, and recommended use. `meshclaw_guard_posture` uses the same
known local stores to flag chat-history, memory, and RAG locations that need
cleanup after credential work.

`guard-vault-put` can store secrets in MeshClaw's local AES-GCM fallback, Apple
Keychain, or Ubuntu `pass`:

- `--backend local`: encrypted fallback under `~/.meshclaw/guard-vault`
- `--backend keychain`: macOS Apple Keychain via `security`
- `--backend pass`: Linux/Ubuntu `pass` store under `meshclaw/<scope>/<name>`

1Password and Bitwarden are exposed in `guard-vault-backends` as adapter status
targets first. They should be integrated as metadata/CLI-backed managers without
ever returning raw values through MCP.

`meshclaw_guard_intent` is for local Guard Chat. It classifies English/Korean
messages such as "store this token", "show vault metadata", "delete this
handle", "check posture", "scan this repo", "토큰 저장해줘", or "이 repo
스캔해줘" into structured actions. It does not execute the action by itself;
MeshClaw policy, local-only boundaries, and approval gates still decide what
can run.

`meshclaw_guard_clean_plan` is deliberately read-only. It lists local chat,
memory, RAG, and IDE-context stores that may need cleanup after secret work.
Deletion is destructive and should stay local-only with strong approval.

## Signal Guard Policy

Signal is treated as a secret ingress and approval channel, not as the general
MeshClaw assistant. The intended policy is exposed by:

```sh
meshclaw guard-signal-policy --json
```

Allowed:

- owner user sends a raw secret into a short-lived Signal Guard chat
- local MeshClaw Guard stores it into Keychain/pass/vault immediately
- Guard replies with a `vault://meshclaw/...` handle
- Codex/Claude post redacted operational reports generated from evidence
- Codex/Claude ask for approval to use a handle

Denied:

- Claude, Codex, GPT, or remote LLMs reading the raw Signal Guard chat

## Signal Guard Vault Import

Signal Guard is an advanced convenience path for secret ingress. The safer
default remains local vault input through CLI, Keychain, `pass`, or a dedicated
local Guard UI. When Signal is used, raw values are not sent to Codex, Claude,
MCP, evidence, or the optional local chat model.

Preferred explicit capture flow:

```text
User -> Signal Guard room:
  secret cloudflare dns-token keychain

Argos/MeshClaw:
  Guard secret capture armed.
  Send only the raw secret within 60 seconds.
  The next message is stored directly in the vault and is not sent to a model,
  MCP evidence, or chat history.

User -> Signal Guard room:
  cf_...

Argos/MeshClaw:
  Stored.
  handle: vault://meshclaw/cloudflare/dns-token
```

Compatibility auto-detect flow:

```text
User -> Signal Guard room:
  api_key=...

Argos/MeshClaw:
  Secret-like content detected.
  The raw value is kept only in this local Guard process for 60 seconds and is
  not forwarded to a model.
  To store it, reply:
  `store operator-example api-key keychain`

User -> Signal Guard room:
  store operator-example api-key keychain

Argos/MeshClaw:
  Stored.
  handle: vault://meshclaw/operator-example/api-key
```

Supported store command forms:

```text
secret <scope> <name> [local|keychain|pass]
capture <scope> <name> [local|keychain|pass]
store <scope> <name> [local|keychain|pass]
save <scope> <name> [local|keychain|pass]
비밀 입력 <scope> <name> [local|keychain|pass]
저장 <scope> <name> [local|keychain|pass]
```

`MESHCLAW_SIGNAL_GUARD_SECRET_TTL_SECONDS` controls the short in-memory TTL. The
default is 60 seconds and values above 600 seconds are capped. Signal itself may
still retain the raw message until disappearing-message deletion or manual
deletion. This is why Signal Guard is useful for personal convenience, but it is
not the strongest possible secret-entry surface.

Default backend order:

1. `MESHCLAW_SIGNAL_GUARD_BACKEND` when set
2. Apple Keychain when `security` is available
3. Ubuntu `pass` when `pass` is available
4. MeshClaw local AES-GCM fallback

The pending raw value is kept in process memory only and expires after 10
minutes. If the daemon restarts before the store command, the user must paste the
secret again. Evidence records only the handle metadata and fingerprint.
- Matrix or other bridges copying raw messages out of Signal
- RAG/search/indexers processing raw Signal messages
- raw Signal messages entering MCP evidence or logs

The normal conversation loop stays with Codex/Claude/Cursor/Open WebUI. Signal
is only the private gate for raw credential entry, approval prompts, and short
redacted reports.

## Ephemeral Guard Sessions

Guard sessions are metadata-only. They record a session id, surface, and
policy state; they do not store the raw transcript. The default policy is:

- transcript disabled
- memory disabled
- RAG disabled
- raw secrets forbidden in evidence
- purge after credential work

Commands:

```sh
meshclaw guard-session policy --json
meshclaw guard-session start --surface signal --json
meshclaw guard-session list --json
meshclaw guard-session end guard-... --json
meshclaw guard-session purge --json
```

## Local Guard Chat Bridge

`meshclaw guard-local-server` exposes a localhost-only HTTP bridge for local
models and local chat UIs. This is the missing piece between "talk to a local
model" and "store the secret in Keychain/pass":

```sh
meshclaw guard-local-server 127.0.0.1:18765
```

Endpoints:

- `GET /health`
- `POST /api/guard/intent`
- `POST /api/guard/vault/import`
- `GET /api/guard/vault/list`
- `GET /api/guard/vault/backends`
- `GET /api/guard/posture`
- `GET /api/guard/clean-plan`

Example local-only import:

```sh
curl -sS http://127.0.0.1:18765/api/guard/vault/import \
  -H 'Content-Type: application/json' \
  -d '{"scope":"cloudflare","name":"dns-token","kind":"api-token","backend":"keychain","value":"..."}'
```

The response returns a `vault://meshclaw/...` handle and metadata. It does not
return the raw value. This server refuses non-local bind addresses and non-local
clients. Do not expose it through tunnels, reverse proxies, Tailscale serve, or
public ports.

MCP intentionally does not expose raw secret reveal. Cloud AI operators can list
handles and read metadata, but they cannot retrieve password/token values.
Secret import and deletion stay local-only until a dedicated Guard Chat UI is
implemented.

`guard-vault-use` is also local-only. It requires explicit `--approve` before
execution, injects the secret into one child process environment variable,
redacts the value from stdout/stderr, records approval evidence, and updates
`last_used_at`. It is not exposed through MCP. MeshClaw options must appear
before the `--` separator; options after `--` belong to the child command.

## Workflow Integration

MeshClaw workflows reference credentials by `vault_handles`, not by raw values.
For example, `email-orchestration-demo` declares:

- `vault://meshclaw/cloudflare/dns-token` for Cloudflare DNS/token steps
- `vault://meshclaw/mail/codex-app-password` for mail client setup
- `vault://meshclaw/mail/macmini-sender-password` for test-mail sending

These handles appear in workflow inspection, approval gates, `steps.jsonl`,
`plan.md`, and `meshclaw-actions.md`. They tell Codex/Claude what local secret
capability is required without revealing the value.

Workflows can also declare adapter-local environment injection with
`secret_env`. This is generic runtime behavior, not an email-specific feature:

```json
{
  "id": "provider-dry-check",
  "title": "Read provider state with a local token",
  "transport": "local",
  "command": "provider-cli zones list --json",
  "action": "read_state",
  "resource": "dns",
  "secret_env": {
    "PROVIDER_TOKEN": "vault://meshclaw/provider/api-token"
  }
}
```

The workflow result records only `PROVIDER_TOKEN` and the vault handle. During
execute mode the local adapter resolves the handle, injects the raw value into
the child process environment, redacts stdout/stderr, and writes evidence with
the handle rather than the secret. Remote adapters should use provider-native
secret stores or explicit future adapters; raw values are not copied into vssh
commands by this schema.

Execute-mode workflow steps run a vault preflight before action execution. If a
required handle is missing, the step fails with `vault handle preflight failed`
and records the missing handle in structured evidence. Missing-handle checks
include an `import_cli` hint so the operator can add the value locally without
pasting raw secrets into Codex, Claude, or workflow evidence.

Every workflow step result also carries a typed continuation classification:
`status`, optional `failure_kind`, and `next_action`. This keeps AI operators
from parsing prose to decide what to do next. For example, a missing secret
handle records `status: vault_missing`, `failure_kind: vault_missing`, and a
next action telling the operator to import the handle locally before rerunning
the targeted step.

Workflow steps can use common orchestration fields across domains:

- `depends_on`: block a step in execute mode unless prior steps completed with
  `status: ok`.
- `fallback_for`: execute a fallback step only when one of the referenced
  source steps failed.
- `timeout_seconds`: override the adapter default timeout for a specific local
  or vssh step.
- `retry`: retry transient local/vssh execution failures with structured
  attempt evidence.

Dependency blocks are recorded as typed outcomes (`dependency_missing` or
`dependency_blocked`) so Codex, Claude, and Cursor can rerun only the correct
upstream repair step instead of replaying the whole workflow blindly.

Retry evidence is deterministic: each attempt records `attempt`, `success`,
`exit_code`, `duration_ms`, optional error text, and whether another retry was
allowed. Retry policy can match exit codes or error substrings, and defaults to
retrying non-zero exits when `max_attempts` is greater than one.

Fallback evidence never hides the original failure. A failed source step remains
failed in `execution.json`; the fallback step records `fallback_for` and gets
`status: fallback_ok` if it succeeds. If the source step succeeded, the fallback
step is skipped with `status: fallback_not_needed`.

`meshclaw workflows resume` classifies these failures as `vault_missing` and
keeps the same `vault_checks` repair hints, so an AI operator can guide the user
to local import without ever asking for the raw secret.

## Product Boundary

Guard is part of MeshClaw Runtime. It is not a personal assistant and it is not
a replacement for password managers, provider secret stores, or operating system
keychains. It wraps those systems with AI-friendly policy, handles, evidence,
and approval semantics.
