# NotiCLI

NotiCLI is a non-interactive command-line application for sending notifications from other applications.

The MVP targets direct CLI invocation on Linux, with a portable Go codebase that can later support Windows builds and a local service/API mode. Initial notification channels are email, Telegram and Slack.

## Status

Project discovery, specification and implementation planning are available under `docs/`.

## Development

This project uses Go. The planned implementation keeps CLI parsing separate from the notification core so the same behavior can be reused by future entrypoints.

```sh
go test ./...
```

## Usage

```sh
noticli send --config ./noticli.json --sender BackupJob --recipient ops --channel email --title "Backup failed" --message "Nightly backup failed on server-01"
```

### Flags

| Flag | Required | Description |
|------|----------|-------------|
| `--config <path>` | no | JSON configuration file. Defaults to `noticli.json` beside the executable. |
| `--sender <text>` | yes | Calling system identifier, up to 20 characters. |
| `--recipient <id>` | yes | Recipient key from the configuration file. |
| `--channel <name>` | yes | One of `email`, `telegram` or `slack`. |
| `--title <text>` | yes | Notification title or subject. |
| `--message <text>` | yes | Notification body. |
| `--attach <path>` | no | Readable file attachment. May be repeated. Email supports attachments; Telegram and Slack return `attachment_error` for attachment requests. |

## Configuration

Example `noticli.json`:

```json
{
  "recipients": {
    "ops": {
      "name": "Operations",
      "email": "ops@example.invalid",
      "telegram_chat_id": "TELEGRAM_CHAT_ID",
      "slack_destination": "#ops"
    },
    "ops-telegram-topics": {
      "name": "Operations Telegram Topics",
      "telegram_delivery_mode": "topics",
      "telegram_topic_group_chat_id": "TELEGRAM_SUPERGROUP_CHAT_ID",
      "telegram_topic_group_name": "Operations Notifications"
    }
  },
  "channels": {
    "email": {
      "settings": {
        "host": "smtp.example.invalid",
        "port": "587",
        "from": "noticli@example.invalid",
        "from_name": "NotiCLI",
        "username": "smtp-user"
      },
      "secrets": {
        "smtp_password": "SMTP_PASSWORD_PLACEHOLDER"
      },
      "attachments": "supported"
    },
    "telegram": {
      "settings": {
        "parse_mode": "HTML"
      },
      "secrets": {
        "token": "TELEGRAM_BOT_TOKEN_PLACEHOLDER"
      },
      "attachments": "unsupported"
    },
    "slack": {
      "settings": {
        "workspace": "example"
      },
      "secrets": {
        "webhook_url": "https://hooks.slack.com/services/PLACEHOLDER/PLACEHOLDER/PLACEHOLDER"
      },
      "attachments": "unsupported"
    }
  },
  "defaults": {
    "channel": "email"
  }
}
```

Secret values are redacted from user-visible diagnostics. Do not put real credentials in examples, issue reports or shared logs.

## Safe Diagnostics

Failures are written as one-line diagnostics containing a stable category and, when applicable, the affected channel. NotiCLI redacts configured secret values and common credential patterns such as tokens, passwords, bearer credentials and Slack webhook URLs.

Diagnostic output is not a secret storage boundary. Avoid putting credentials or sensitive message content in notification titles, message bodies, recipient names, file names or configuration keys, because those fields may be needed for operator troubleshooting.

## Channel Setup

Email requires SMTP settings under `channels.email.settings`: `host`, `port`, `from` and optional `from_name` and `username`. It requires `channels.email.secrets.smtp_password`. If `from_name` is set, it is used as the display name in the email `From` header. If `username` is omitted, NotiCLI uses `from` for SMTP authentication. Email subjects are prefixed with the calling sender as `[sender] title`.

Telegram requires a bot token in `channels.telegram.secrets.token`. A recipient can use one of two delivery modes:

- Private chat delivery: omit `telegram_delivery_mode` or set it to `private`, and set `telegram_chat_id`. Private Telegram titles are formatted as `[sender] title`.
- Topic group delivery: set `telegram_delivery_mode` to `topics`, set `telegram_topic_group_chat_id` to a topic-enabled supergroup ID, and optionally set `telegram_topic_group_name` for diagnostics. Topic messages use the topic as sender context, so titles are sent without the `[sender]` prefix.

Topic delivery stores generated sender-topic associations in a sibling state file next to the active config. For `/opt/NotiCLI/config/noticli.json`, the state file is `/opt/NotiCLI/config/noticli.telegram-topics.json`. Back up this file with the production installation; if it is lost, NotiCLI may create replacement topics because Telegram bots cannot list or find every existing topic by name. Before creating a new topic, NotiCLI verifies that the state file can be written; write failures abort the notification with an error instead of creating an untracked topic. Future assisted commands such as `/noticli_bind`, `/noticli_unbind` and `/noticli_topics` are reserved for binding or listing manually managed topics, but they are not part of the current CLI send flow.

Slack requires an incoming webhook URL in `channels.slack.secrets.webhook_url` and a recipient `slack_destination`. The initial MVP sends text messages through the webhook only.

## Exit Codes

| Code | Category | Meaning |
|------|----------|---------|
| `0` | `success` | The selected channel accepted the notification request. |
| `1` | `internal_error` | Unexpected failure. |
| `2` | `invalid_input` | Missing, empty or unsupported CLI input. |
| `3` | `missing_config` | Configuration file, recipient or channel is missing. |
| `4` | `invalid_config` | Configuration is malformed or incomplete. |
| `5` | `attachment_error` | Attachment is missing, unreadable, a directory or unsupported for the channel. |
| `6` | `delivery_failure` | Provider rejected or failed the delivery request. |

## Portable Builds

NotiCLI is intended to build as a single binary for Linux now and Windows later. The core uses Go path and file APIs so configuration and attachment paths are interpreted by the target operating system.

```sh
GOOS=linux GOARCH=amd64 go build -o dist/noticli ./cmd/noticli
GOOS=windows GOARCH=amd64 go build -o dist/noticli.exe ./cmd/noticli
```
