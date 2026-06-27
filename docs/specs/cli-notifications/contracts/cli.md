# Contract: CLI Notifications

Contratos de interface externa para a CLI do NotiCLI.

## Command: `noticli send`

**Purpose**: Send one notification request through one configured channel.

### Arguments

| Field | Required | Validation | Description |
|-------|----------|------------|-------------|
| `--config <path>` | no | Must be readable if provided; empty explicit value is invalid | Configuration file path; when omitted, defaults to `noticli.json` in the executable directory |
| `--sender <text>` | yes | Non-empty, max 20 characters | Calling system identifier used to compose the notification sender context |
| `--recipient <id>` | yes | Non-empty | Configured recipient identifier |
| `--channel <name>` | yes | One supported channel | MVP: `email`, `telegram`, `slack` |
| `--title <text>` | yes | Non-empty | Notification title or subject |
| `--message <text>` | yes | Non-empty | Notification body |
| `--attach <path>` | no | File must exist and be readable | May be repeated for multiple attachments |

### Result Semantics

| Exit Code | Category | Description |
|-----------|----------|-------------|
| 0 | success | Selected channel accepted the notification request |
| 2 | invalid_input | Required argument is missing, empty, duplicated incorrectly or unsupported |
| 3 | missing_config | Configuration file or referenced recipient/channel is missing |
| 4 | invalid_config | Configuration exists but is malformed or incomplete |
| 5 | attachment_error | Attachment is missing, unreadable or unsupported for the selected channel |
| 6 | delivery_failure | Selected channel rejected or failed the notification request |
| 1 | internal_error | Unexpected failure not covered by another category |

### Diagnostic Output

Default diagnostic output is a single line.

| Field | Requirement |
|-------|-------------|
| Success output | May be empty or a concise success message |
| Failure output | Must include category and human-readable reason |
| Channel failures | Must include affected channel |
| Secrets | Must never include tokens, passwords, webhook URLs or equivalent credentials |

### Examples

```text
noticli send --config ./noticli.json --sender BackupJob --recipient ops --channel email --title "Backup failed" --message "Nightly backup failed on server-01"
```

```text
noticli send --sender DeployBot --recipient ops --channel slack --title "Deploy complete" --message "Release 2026.06.26 completed" --attach ./release-notes.txt
```

## Configuration Contract

**Format**: JSON for MVP.

### Top-Level Shape

| Field | Required | Description |
|-------|----------|-------------|
| `recipients` | yes | Map or list of configured recipients |
| `channels` | yes | Map of channel configurations |
| `defaults` | no | Optional defaults for command behavior |

### Recipient Fields

| Field | Required | Description |
|-------|----------|-------------|
| `id` | yes | Stable identifier used by CLI callers |
| `name` | no | Human-readable label |
| `email` | conditional | Required for email delivery to this recipient |
| `telegram_chat_id` | conditional | Required for Telegram delivery to this recipient |
| `slack_destination` | conditional | Required for Slack delivery to this recipient when not implied by channel settings |
| `enabled` | no | Defaults to true |

### Channel Fields

| Field | Required | Description |
|-------|----------|-------------|
| `type` | yes | `email`, `telegram` or `slack` |
| `enabled` | no | Defaults to true |
| `settings` | yes | Non-secret settings required by the channel |
| `secrets` | yes | Secret-bearing values required by the channel |
| `attachments` | yes | Channel attachment policy |

### Secret Handling Requirements

- Secret-bearing fields must be grouped or named clearly as secrets.
- Secret values must be redacted from diagnostics.
- Example configuration must use placeholder values, never real-looking credentials.

## Future Command: `noticli serve`

Service mode is out of MVP scope. The MVP design must leave room for a future local service command that reuses the same notification request validation and dispatch behavior.
