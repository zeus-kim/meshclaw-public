# MeshClaw Mail Tools

Mail automation starts read-only. Sending, deletion, moving messages, account
changes, and bulk actions require approval and are not transmitted by the first
adapter.

## Account Config

Default path:

```text
~/.meshclaw/mail-accounts.json
```

Example:

```json
{
  "kind": "meshclaw_mail_accounts",
  "accounts": [
    {
      "id": "personal",
      "backend": "imap",
      "email": "operator@example.com",
      "host": "imap.example.com",
      "port": 993,
      "username": "operator@example.com",
      "password_handle": "vault://meshclaw/mail/app-password",
      "mailbox": "INBOX",
      "tls": true
    }
  ]
}
```

Use either `password_handle` or `password_env`. Do not put raw passwords in this
file.

## Commands

```sh
meshclaw mail setup operator@gmail.com --json
meshclaw mail setup operator@naver.com --execute --json
meshclaw mail setup you@example.com --mode keychain --service 'Mox: you@example.com' --account you@example.com --execute --json
meshclaw mail setup ops@example.com --mode direct --host imap.example.com --port 993 --username ops@example.com --execute --json
meshclaw mail discover-keychain --json
meshclaw mail doctor --json
meshclaw mail accounts --json
meshclaw mail search --account personal --query "from:example" --since 24h --limit 10 --json
meshclaw mail thread <message-uid> --account personal --max-body 5000 --json
meshclaw mail read-many <message-uid,message-uid> --account personal --max-body 5000 --json
meshclaw mail attachments <message-uid> --account personal --approve --json
meshclaw mail move <message-uid,message-uid> --account personal --target Archive --approve --json
meshclaw mail delete <message-uid> --account personal --approve --json
meshclaw mail summarize --account personal --since 24h --limit 10 --json
meshclaw mail draft-reply <message-uid> --account personal --intent "polite short reply" --json
meshclaw mail send --draft <draft-id> --approve --json
meshclaw mail watch-once --account personal --since 15m --limit 10 --json
meshclaw schedule run-once mail-watch --json
```

MCP tool names:

- `meshclaw_mail_setup`
- `meshclaw_mail_doctor`
- `meshclaw_mail_discover_keychain`
- `meshclaw_mail_accounts`
- `meshclaw_mail_search`
- `meshclaw_mail_thread`
- `meshclaw_mail_read_many`
- `meshclaw_mail_attachments`
- `meshclaw_mail_move`
- `meshclaw_mail_delete`
- `meshclaw_mail_draft_reply`
- `meshclaw_mail_send`
- `meshclaw_mail_watch_once`

`search`, `thread`, and `summarize` store redacted evidence. `draft-reply`
writes a local draft JSON under `~/.meshclaw/mail-drafts`.

`mail send`, `mail move`, `mail delete`, and `mail attachments` are mutating or
sensitive actions. They return `approval_required` unless the caller passes
`--approve` or MCP `approve=true`. SMTP sending uses the account SMTP settings,
falling back to the IMAP host on port 465 with the same Guard/Keychain password
handle.

## Setup Modes

`mail setup` has three user paths:

- `provider`: normal user flow for Gmail, Naver, Daum, Outlook, and iCloud.
  MeshClaw chooses the IMAP host and username style, then stores only a Guard
  vault handle. The user still has to create an app password or future OAuth
  token.
- `keychain`: links an existing macOS Keychain item by service/account without
  copying the raw secret into MeshClaw files. This is the right path for local
  power users or imported mail accounts.
- `direct`: developer/server flow for custom IMAP hosts, ports, and usernames.
  This is useful for private domains like `fools.ai`.

`auto` chooses `provider` for known consumer domains and `direct` for unknown
custom domains. `--execute` saves the account; without it the command returns a
dry-run plan and likely Keychain candidates.

## Product UX

The product path should be:

1. Run `meshclaw mail discover-keychain --json` to find existing local mail
   credentials by metadata only.
2. Run `meshclaw mail setup <email> --json` for a provider/default dry run.
3. If a Keychain candidate exists, run keychain mode with `--execute`. If not,
   create an app password and store it through Guard.
4. Run `meshclaw mail doctor --json` to verify config, network, login, and
   mailbox readiness.
5. Allow Argos/Signal/local LLM to use only read-only mail tools by default.

For non-technical users, the UI should expose these as buttons: connect Gmail,
connect Naver, connect existing Mac Mail account, and test connection. Raw
password entry should happen only in a local Guard-controlled flow, never in a
Signal chat.

## Safety

- Raw message bodies are redacted before evidence storage.
- Attachments are downloaded only with explicit approval and are stored under a
  local path with sanitized filenames.
- Drafts are local artifacts until `mail send --approve`.
- Mail sending is policy-gated through `email_send`.
- Delete and move operations are explicit-approval operations and store evidence.

## Argos Mail Requests

Argos can route these natural-language requests to the mail tools. English is
the primary documentation language; Korean prompts are supported as product
examples.

- "Which mail accounts are connected?"
- "Summarize important mail from the last 24 hours."
- "Search mail for Cloudflare."
- "Read mail message 12345."
- "Read messages 12345,12346,12347."
- "Draft a polite short reply to message 12345."
- "Any new mail?"
- "Send an email to operator@example.com. Subject: Test. Body: It works."
- If multiple sending accounts are healthy, Argos asks which account to use and
  waits for an id/number reply.
- "approve" sends the pending SMTP draft if it is still inside the approval TTL.
- "delete mail 12345" returns an approval-only response; Signal does not
  directly delete mail.

Korean examples:

- "최근 중요한 메일 요약해줘"
- "메일에서 cloudflare 찾아줘"
- "메일 12345 읽어줘"
- "operator@example.com 에게 메일 보내줘. 제목은 테스트. 내용은 잘 됩니다."
- "승인"

This is enabled for account listing, search/summary, thread read, local draft
creation, read-only watch checks, and two-step email sending. Real delete/move
and attachment download are classified but blocked in Signal with an
approval-only response. Email sending creates a pending draft first and sends
only if the user replies `approve` or `승인` within the approval TTL. Set
`MESHCLAW_SIGNAL_MAIL_ACCOUNT` to choose the default sending account.
If it is not set and multiple healthy accounts exist, Argos asks the user to
choose a sender account before creating the pending send draft.

## Scheduler

`meshclaw schedule run-once mail-watch --json` checks all configured accounts
for recent messages and stores redacted evidence. Accounts that fail login are
reported per-account without blocking the healthy accounts. The default schedule
plan includes a `mail-watch` job.

On the macmini controller, launchd `schedule-runner` runs
`meshclaw schedule run-due --execute` every 15 minutes. The `mail-watch` job
deduplicates seen message IDs and repeated account errors. It sends Signal to
`argos-briefing` only when there are new messages or new errors; quiet repeats
write evidence but do not spam Signal.

Check the live automation state with:

```sh
meshclaw schedule status --json
meshclaw daemon status schedule-runner --json
```
