# Contract: CLI Notifications

Contratos de interface externa para a CLI do NotiCLI.

## Command: `noticli send`

**Purpose**: Send one notification request through one configured channel.

### Arguments

| Field | Required | Validation | Description |
|-------|----------|------------|-------------|
| `--config <path>` | no | Must be readable if provided; empty explicit value is invalid | Configuration file path; when omitted, defaults to `config/noticli.json` beside the resolved executable path |
| `--sender <text>` | yes | Non-empty, max 20 characters | Calling system identifier used to compose the notification sender context |
| `--recipient <id>` | yes | Non-empty | Configured recipient identifier |
| `--channel <name>` | yes | One supported channel | MVP: `email`, `telegram`, `slack` |
| `--title <text>` | yes | Non-empty | Notification title or subject |
| `--message <text>` | yes | Non-empty | Notification body |
| `--attach <path>` | no | File must exist and be readable | May be repeated for multiple attachments |

For email delivery, the final subject is formatted as `[--sender value] --title value`.

### Attachment Behavior

| Channel | Initial MVP behavior |
|---------|----------------------|
| `email` | Supports readable file attachments using multipart MIME. |
| `telegram` | Does not support attachments in the initial MVP; requests with attachments fail with `attachment_error`. |
| `slack` | Does not support attachments through the incoming webhook flow; requests with attachments fail with `attachment_error`. |

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
| `recipients` | yes | Map of configured recipients keyed by recipient ID |
| `channels` | yes | Map of channel configurations |
| `defaults` | no | Optional defaults for command behavior |

### Recipient Fields

| Field | Required | Description |
|-------|----------|-------------|
| `id` | no | Stable identifier used by CLI callers; defaults to the recipient map key when omitted |
| `name` | no | Human-readable label |
| `email` | conditional | Required for email delivery to this recipient |
| `telegram_chat_id` | conditional | Private chat ID for Telegram private delivery; retained for backward compatibility |
| `telegram_delivery_mode` | no | `private` or `topics`; defaults to `private` when omitted |
| `telegram_topic_group_chat_id` | conditional | Required for Telegram topic-based delivery |
| `telegram_topic_group_name` | no | Human-readable label for the topic-enabled group |
| `slack_destination` | conditional | Required for Slack delivery to this recipient when not implied by channel settings |
| `enabled` | no | Defaults to true |

### Channel Fields

| Field | Required | Description |
|-------|----------|-------------|
| `type` | no | `email`, `telegram` or `slack`; defaults to the channel map key when omitted |
| `enabled` | no | Defaults to true |
| `settings` | yes | Non-empty map of non-secret settings required by the channel |
| `secrets` | yes | Non-empty map of secret-bearing values required by the channel |
| `attachments` | no | Channel attachment policy; defaults to `unsupported` when omitted |

For email channels, `settings.from_name` is optional and controls the display name in the email `From` header while `settings.from` remains the sender email address used for delivery.

For Telegram recipients, private delivery sends to `telegram_chat_id` and prefixes delivered titles as `[--sender value] --title value`. Topic delivery sends to `telegram_topic_group_chat_id`, creates or reuses one topic per sender, and sends the title without the `[--sender value]` prefix.

Telegram topic associations are runtime state, not operator-authored configuration. The state file is derived from the active config path; for `config/noticli.json` beside the resolved executable path, it is `config/noticli.telegram-topics.json` in the same directory. In production, that release-local `config/` directory is symlinked to the centralized config tree, so the state file remains under `/opt/NotiCLI/config/`. It must be writable by the NotiCLI runtime before a topic is created and must be protected and backed up with the NotiCLI installation. Telegram Bot API does not provide complete topic listing or name lookup, so topics created outside NotiCLI may require future assisted binding. Reserved future bot commands include `/noticli_bind`, `/noticli_unbind` and `/noticli_topics`; they are outside the current MVP.

### MVP Required Secret Keys

| Channel | Required secret keys |
|---------|----------------------|
| `email` | `smtp_password` |
| `telegram` | `token` |
| `slack` | `webhook_url` |

### Secret Handling Requirements

- Secret-bearing fields must be grouped or named clearly as secrets.
- Secret values must be redacted from diagnostics.
- Example configuration must use placeholder values, never real-looking credentials.

## Future Command: `noticli serve`

Service mode is out of MVP scope. The MVP design must leave room for a future local service command that reuses the same notification request validation and dispatch behavior.
