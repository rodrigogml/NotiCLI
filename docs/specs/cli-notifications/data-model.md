# Data Model: CLI Notifications

## Entity: Notification Request

| Field | Type | Constraints | Notes |
|-------|------|-------------|-------|
| recipient_id | string | Required, non-empty | References a configured recipient |
| channel | string | Required, supported channel | MVP values: email, telegram, slack |
| title | string | Required, non-empty | May be rendered differently depending on channel |
| content | string | Required, non-empty | Main notification body |
| attachments | list of Attachment Reference | Optional | Each item must be readable before delivery |
| config_path | path | Optional with documented default | Path to local configuration file |

### Relationships

- Notification Request references one Recipient by `recipient_id`.
- Notification Request selects one Channel Configuration by `channel`.
- Notification Request may include zero or more Attachment References.
- Notification Request produces one Delivery Result.

### State Transitions

```text
received -> validated -> dispatching -> accepted
received -> rejected
received -> validated -> dispatching -> failed
```

## Entity: Recipient

| Field | Type | Constraints | Notes |
|-------|------|-------------|-------|
| id | string | Required, unique | Stable identifier used by callers |
| name | string | Optional | Human-readable label |
| email | string | Required when recipient uses email | Destination for email channel |
| telegram_chat_id | string | Required when recipient uses Telegram | Destination conversation |
| slack_destination | string | Required when recipient uses Slack | Channel, user or webhook-bound destination |
| enabled | boolean | Optional, default true | Disabled recipients must not receive notifications |

### Relationships

- Recipient can have destinations for multiple channels.
- Recipient is referenced by Notification Request.

### State Transitions

```text
enabled -> disabled
disabled -> enabled
```

## Entity: Channel Configuration

| Field | Type | Constraints | Notes |
|-------|------|-------------|-------|
| type | string | Required, supported channel | email, telegram or slack |
| enabled | boolean | Optional, default true | Disabled channels reject delivery |
| secrets | map | Required per channel | Tokens, passwords, webhook URLs or equivalent credentials |
| settings | map | Required per channel | Non-secret channel settings |
| attachment_policy | enum | Required per channel | supported, limited or unsupported |

### Relationships

- Channel Configuration is selected by Notification Request.
- Channel Configuration defines how Delivery Result is produced for one provider.

### State Transitions

```text
enabled -> disabled
disabled -> enabled
```

## Entity: Attachment Reference

| Field | Type | Constraints | Notes |
|-------|------|-------------|-------|
| path | path | Required | Must exist and be readable |
| filename | string | Optional | Defaults to file basename |
| size | integer | Derived | Used for channel compatibility checks |
| content_type | string | Optional or derived | Used when a channel needs media type |

### Relationships

- Attachment Reference belongs to one Notification Request.
- Attachment Reference is evaluated against Channel Configuration attachment policy.

### State Transitions

```text
declared -> readable -> accepted
declared -> unreadable
declared -> unsupported
```

## Entity: Delivery Result

| Field | Type | Constraints | Notes |
|-------|------|-------------|-------|
| success | boolean | Required | Final result of one invocation |
| exit_code | integer | Required | Stable command result for automation |
| category | enum | Required | success, invalid_input, missing_config, invalid_config, attachment_error, delivery_failure, internal_error |
| channel | string | Optional | Present for channel-specific attempts |
| message | string | Required | Single-line diagnostic by default |
| redacted | boolean | Required for failures | Indicates secret redaction was applied |

### Relationships

- Delivery Result belongs to exactly one Notification Request.

### State Transitions

```text
pending -> success
pending -> failure
```
