# Contract: Telegram Topics por Recipient

Contratos externos para configuracao e comportamento observavel da feature Telegram Topics.

## Command: `noticli send`

**Purpose**: Send one Telegram notification using the recipient's configured Telegram delivery mode.

The command shape remains unchanged:

```text
noticli send --sender <system> --recipient <id> --channel telegram --title <text> --message <text>
```

### Behavior by Recipient Mode

| Recipient Telegram mode | Destination | Title formatting | Sender grouping |
|-------------------------|-------------|------------------|-----------------|
| `private` | Private bot chat | `[sender] title` | Message title prefix |
| `topics` | Topic-enabled supergroup | `title` | Topic named/identified by sender |

### Result Semantics

| Exit Code | Category | Description |
|-----------|----------|-------------|
| 0 | success | Telegram accepted the notification request |
| 2 | invalid_input | Existing CLI input validation failed |
| 3 | missing_config | Recipient or required Telegram destination is missing |
| 4 | invalid_config | Recipient Telegram mode or destination is malformed/incomplete |
| 5 | attachment_error | Telegram attachment request remains unsupported |
| 6 | delivery_failure | Telegram rejected topic creation, topic send, private send, or bot permission is insufficient |
| 1 | internal_error | Unexpected local failure, including unrecoverable topic state failure |

### Failure Diagnostics

| Failure | Required Diagnostic Behavior |
|---------|------------------------------|
| Missing private chat ID | Identify recipient Telegram destination as missing |
| Missing topic group chat ID | Identify topic group destination as missing |
| Group is not usable for topics | Identify Telegram delivery failure or invalid config based on provider response |
| Bot cannot create topic | Identify Telegram delivery failure without exposing token |
| Known topic no longer works | Either recover with one replacement topic or report Telegram delivery failure |
| Topic state cannot be read/written | Report local failure without exposing topic state file contents or token |

## Configuration Contract

Existing recipient fields continue to work for private chat delivery.

### Recipient Telegram Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `telegram_chat_id` | string | conditional | Private chat ID for private mode; retained for backward compatibility |
| `telegram_delivery_mode` | string | no | `private` or `topics`; defaults to `private` when omitted |
| `telegram_topic_group_chat_id` | string | conditional | Topic-enabled supergroup chat ID for `topics` mode |
| `telegram_topic_group_name` | string | no | Human-readable label for operator diagnostics |

### Examples

Private chat recipient:

```json
{
  "recipients": {
    "rodrigogml": {
      "name": "Rodrigo GML",
      "telegram_chat_id": "TELEGRAM_PRIVATE_CHAT_ID",
      "telegram_delivery_mode": "private",
      "enabled": true
    }
  }
}
```

Topic-based group recipient:

```json
{
  "recipients": {
    "rodrigogml-topics": {
      "name": "Rodrigo GML Topics",
      "telegram_delivery_mode": "topics",
      "telegram_topic_group_chat_id": "-1001234567890",
      "telegram_topic_group_name": "NotiCLI",
      "enabled": true
    }
  }
}
```

## Topic State Contract

Topic associations are stored outside main configuration.

Production path is derived from the active config path:

```text
/opt/NotiCLI/releases/v1.1.2/config/noticli.json -> /opt/NotiCLI/config/noticli.telegram-topics.json
```

### Shape

```json
{
  "version": 1,
  "updated_at": "2026-06-28T12:00:00Z",
  "associations": [
    {
      "recipient_id": "rodrigogml-topics",
      "chat_id": "-1001234567890",
      "sender": "ProdSmoke",
      "topic_name": "ProdSmoke",
      "message_thread_id": 4,
      "created_by_noticli": true,
      "created_at": "2026-06-28T12:00:00Z",
      "last_used_at": "2026-06-28T12:00:01Z",
      "last_verified_at": "2026-06-28T12:00:01Z",
      "status": "active"
    }
  ]
}
```

### State Rules

| Rule | Requirement |
|------|-------------|
| State ownership | File is owned by the NotiCLI installation and writable only by the execution identity/effective group that runs NotiCLI |
| Missing file | Initialize empty state on first topic-based send when parent directory is writable |
| Malformed file | Fail safely; do not overwrite automatically without backup/recovery policy |
| Concurrent writes | Serialized locally |
| Duplicate key | Unique by `recipient_id + chat_id + sender` |
| Secrets | No bot token or provider credential is stored in topic state |
| State backup | Last known valid state is preserved before replacement |
| Lost state | New topics may be created for senders until assisted bind commands exist |

### Topic Name Rules

| Rule | Requirement |
|------|-------------|
| State identity | Use the validated `--sender` value scoped by recipient and group chat |
| Display name | Derive from sender by trimming/collapsing whitespace and removing unsafe control characters |
| Provider constraints | Display name must satisfy Telegram topic-name constraints before creation |
| Collision | If two senders produce the same display name in the same recipient/group, add a deterministic disambiguator |

## Future Bot Commands

The following commands are out of MVP scope for this spec but reserved for future assisted administration:

| Command | Purpose |
|---------|---------|
| `/noticli_bind <sender>` | Bind the current Telegram topic to a sender |
| `/noticli_unbind <sender>` | Remove a sender-topic association |
| `/noticli_topics` | List known sender-topic associations |
