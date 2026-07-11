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
noticli send --config ./noticli.json --sender BackupJob --category backup --priority HIGH --title "Backup failed" --message "Nightly backup failed on server-01"
```

### Flags

| Flag | Required | Description |
|------|----------|-------------|
| `--config <path>` | no | JSON configuration file override. Use only when the caller needs to override the default NotiCLI configuration. |
| `--sender <text>` | yes | Calling system identifier, up to 20 characters. |
| `--category <text>` | no | Routing category matched by configured routes. |
| `--priority <HIGH\|NORMAL\|LOW>` | no | Routing priority. Defaults to `NORMAL`. |
| `--title <text>` | yes | Notification title or subject. |
| `--message <text>` | yes | Notification body. |
| `--attach <path>` | no | Readable file attachment. May be repeated. Destinations whose delivery account does not support attachments receive the message without attachments, and the omission is logged. |

## Configuration

Example `noticli.json`:

```json
{
  "destinations": {
    "ops-email": {
      "type": "email",
      "email": "ops@example.invalid"
    },
    "ops-telegram-topics": {
      "type": "telegram",
      "telegram_delivery_mode": "topics",
      "telegram_topic_group_chat_id": "TELEGRAM_SUPERGROUP_CHAT_ID",
      "telegram_topic_group_name": "Operations Notifications"
    },
    "ops-telegram-thread": {
      "type": "telegram",
      "telegram_delivery_mode": "thread",
      "telegram_topic_group_chat_id": "TELEGRAM_SUPERGROUP_CHAT_ID",
      "message_thread_id": 42
    },
    "ops-slack": {
      "type": "slack",
      "slack_destination": "#ops"
    }
  },
  "delivery_accounts": {
    "smtp-main": {
      "type": "email",
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
    "telegram-main": {
      "type": "telegram",
      "settings": {
        "parse_mode": "HTML"
      },
      "secrets": {
        "token": "TELEGRAM_BOT_TOKEN_PLACEHOLDER"
      },
      "attachments": "unsupported"
    },
    "slack-main": {
      "type": "slack",
      "settings": {
        "workspace": "example"
      },
      "secrets": {
        "webhook_url": "https://hooks.slack.com/services/PLACEHOLDER/PLACEHOLDER/PLACEHOLDER"
      },
      "attachments": "unsupported"
    }
  },
  "routes": [
    {
      "id": "backup-high",
      "match": {
        "senders": ["BackupJob"],
        "categories": ["backup"],
        "priorities": ["HIGH"]
      },
      "deliveries": [
        {"account": "smtp-main", "destination": "ops-email"},
        {"account": "telegram-main", "destination": "ops-telegram-topics"}
      ]
    }
  ],
  "catch_all": {
    "deliveries": [
      {"account": "smtp-main", "destination": "ops-email"}
    ]
  },
  "logging": {
    "path": ""
  }
}
```

Secret values are redacted from user-visible diagnostics and delivery logs. Do not put real credentials in examples, issue reports or shared logs.

## Configuration Scope

The recommended setup is to keep NotiCLI configuration centralized in the NotiCLI config file and omit `--config` on callers that do not need to override it.

Pass `--config <path>` only when a caller explicitly needs to replace the default configuration lookup for that execution.

If `--config` is not provided, NotiCLI uses `config/noticli.json` beside the resolved executable path and the shared NotiCLI config remains the source of truth.

## Privileged Linux Installation

For shared Linux hosts where different local users or service accounts must call NotiCLI without direct read access to the NotiCLI configuration, install the release binary under a dedicated `noticli` user and group. The release binary should be owned by `noticli:noticli` and use `setuid` plus `setgid` mode so executions run with the NotiCLI effective user and group.

> [!WARNING]
> `setuid`/`setgid` binaries increase the security impact of application bugs. Use a dedicated `noticli` user and dedicated `noticli` group only for NotiCLI, keep that account without an interactive shell, and grant it access only to the NotiCLI configuration and runtime state it needs. Do not reuse a human user, broad service account or privileged group for this installation model.

The recommended production layout is:

```text
/opt/NotiCLI/releases/<version>/noticli
/opt/NotiCLI/releases/<version>/config -> /opt/NotiCLI/config
/opt/NotiCLI/current -> /opt/NotiCLI/releases/<version>
/opt/NotiCLI/bin/noticli -> /opt/NotiCLI/current/noticli
/usr/local/bin/noticli -> /opt/NotiCLI/bin/noticli
```

Keep `/opt/NotiCLI/config` owned by `root:noticli` with no access for other users. The symlink permissions are not the access boundary; the release binary owner/mode and the target config directory permissions are what enforce access.

## Safe Diagnostics

Failures are written as one-line diagnostics containing a stable category and, when applicable, the affected channel. NotiCLI redacts configured secret values and common credential patterns such as tokens, passwords, bearer credentials and Slack webhook URLs.

Diagnostic output is not a secret storage boundary. Avoid putting credentials or sensitive message content in notification titles, message bodies, recipient names, file names or configuration keys, because those fields may be needed for operator troubleshooting.

## Channel Setup

Routes match when every configured criterion in `match` matches the request. Missing criteria do not filter. Multiple routes can match, and every configured delivery is attempted without automatic deduplication. If no route matches, `catch_all` is required and used.

If `logging.path` is empty or omitted, NotiCLI writes delivery events to `noticli.delivery.log` beside the active config file. Delivery failures and attachment omissions include route, destination, account, channel type, sender, category and priority.

Email requires SMTP settings under an email delivery account: `host`, `port`, `from` and optional `from_name` and `username`. It requires `secrets.smtp_password`. If `from_name` is set, it is used as the display name in the email `From` header. If `username` is omitted, NotiCLI uses `from` for SMTP authentication. Email subjects are prefixed with the calling sender as `[sender] title`.

Telegram requires a bot token in a Telegram delivery account under `secrets.token`. A Telegram destination can use one of three delivery modes:

- Private chat delivery: omit `telegram_delivery_mode` or set it to `private`, and set `telegram_chat_id`. Private Telegram titles are formatted as `[sender] title`.
- Topic group delivery: set `telegram_delivery_mode` to `topics`, set `telegram_topic_group_chat_id` to a topic-enabled supergroup ID, and optionally set `telegram_topic_group_name` for diagnostics. Topic messages use the topic as sender context, so titles are sent without the `[sender]` prefix.
- Fixed thread delivery: set `telegram_delivery_mode` to `thread`, set `telegram_topic_group_chat_id`, and set `message_thread_id`.

Topic delivery stores generated sender-topic associations in a sibling state file next to the active config. For `config/noticli.json` beside the resolved executable path, the state file is `config/noticli.telegram-topics.json` in the same directory. In production, the release `config/` directory is symlinked to the centralized config tree, so the state file remains under `/opt/NotiCLI/config/`. Back up this file with the production installation; if it is lost, NotiCLI may create replacement topics because Telegram bots cannot list or find every existing topic by name. Before creating a new topic, NotiCLI verifies that the state file can be written; write failures abort the notification with an error instead of creating an untracked topic. Future assisted commands such as `/noticli_bind`, `/noticli_unbind` and `/noticli_topics` are reserved for binding or listing manually managed topics, but they are not part of the current CLI send flow.

Slack requires an incoming webhook URL in a Slack delivery account under `secrets.webhook_url` and a Slack destination `slack_destination`. The initial MVP sends text messages through the webhook only.

## Exit Codes

| Code | Category | Meaning |
|------|----------|---------|
| `0` | `success` | All resolved deliveries accepted the notification request. |
| `1` | `internal_error` | Unexpected failure. |
| `2` | `invalid_input` | Missing, empty or unsupported CLI input. |
| `3` | `missing_config` | Configuration file, destination or delivery account is missing. |
| `4` | `invalid_config` | Configuration is malformed or incomplete. |
| `5` | `attachment_error` | Attachment is missing, unreadable or a directory. |
| `6` | `delivery_failure` | One or more providers rejected or failed delivery after all possible deliveries were attempted. |

## Portable Builds

NotiCLI is intended to build as a single binary for Linux now and Windows later. The core uses Go path and file APIs so configuration and attachment paths are interpreted by the target operating system.

```sh
GOOS=linux GOARCH=amd64 go build -o dist/noticli ./cmd/noticli
GOOS=windows GOARCH=amd64 go build -o dist/noticli.exe ./cmd/noticli
```
