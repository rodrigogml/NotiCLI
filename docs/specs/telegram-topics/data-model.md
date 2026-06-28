# Data Model: Telegram Topics por Recipient

## Entity: Telegram Delivery Preference

| Field | Type | Constraints | Notes |
|-------|------|-------------|-------|
| mode | enum | `private`, `topics` | Selects private chat or topic-based group delivery for a recipient |
| private_destination | Telegram Private Destination | Required when `mode=private` | Existing private chat behavior |
| topic_group_destination | Telegram Topic Group Destination | Required when `mode=topics` | Topic-enabled group target |

### Relationships

- Recipient 1:1 Telegram Delivery Preference for the Telegram channel.

### State Transitions

```text
private -> topics
topics -> private
```

Switching modes changes routing for future notifications only; it does not delete existing topic state unless an operator chooses to clean it later.

## Entity: Telegram Private Destination

| Field | Type | Constraints | Notes |
|-------|------|-------------|-------|
| chat_id | string | Required | Telegram private chat ID previously discovered by bot interaction |

### Relationships

- Telegram Delivery Preference references Telegram Private Destination when private mode is selected.

## Entity: Telegram Topic Group Destination

| Field | Type | Constraints | Notes |
|-------|------|-------------|-------|
| chat_id | string | Required | Telegram supergroup chat ID |
| title | string | Optional | Human-readable group label for operator diagnostics |
| requires_topics | boolean | Defaults true | Destination must be a topic-enabled group |

### Relationships

- Telegram Delivery Preference references Telegram Topic Group Destination when topic mode is selected.
- Telegram Topic Group Destination 1:N Sender Topic Association.

## Entity: Sender Topic Association

| Field | Type | Constraints | Notes |
|-------|------|-------------|-------|
| recipient_id | string | Required | Recipient key used by callers |
| chat_id | string | Required | Topic-enabled Telegram group ID |
| sender | string | Required, same sender validation as notification request | Calling system identifier |
| topic_name | string | Required | Sanitized display name used for the Telegram topic; derived from sender |
| topic_name_disambiguator | string | Optional | Deterministic suffix or marker when sender display names collide |
| message_thread_id | integer | Required, positive | Telegram topic ID used for delivery |
| created_by_noticli | boolean | Required | True when NotiCLI created the topic automatically |
| created_at | timestamp | Required | When association was first stored |
| last_used_at | timestamp | Optional | Last successful send using the association |
| last_verified_at | timestamp | Optional | Last time the association was known usable |
| status | enum | `active`, `stale`, `replaced` | Operational state of the association |

### Relationships

- Topic State 1:N Sender Topic Association.
- Sender Topic Association belongs to one recipient and one Telegram group destination.

### State Transitions

```text
active -> stale -> replaced
active -> replaced
stale -> active
```

## Entity: Topic State

| Field | Type | Constraints | Notes |
|-------|------|-------------|-------|
| version | integer | Required | State schema version |
| associations | array | Required | Known Sender Topic Associations |
| updated_at | timestamp | Required | Last successful state write |
| previous_backup_at | timestamp | Optional | Last time a known valid state backup was written |

### Relationships

- Topic State contains all generated Telegram topic associations for the local installation.

### State Transitions

```text
missing -> initialized -> updated
initialized -> invalid
invalid -> recovered
```

Missing state is initialized on first topic-based send. Malformed state is treated as an operational failure unless a safe recovery policy is explicitly defined.

## Entity: Telegram Delivery Result

| Field | Type | Constraints | Notes |
|-------|------|-------------|-------|
| success | boolean | Required | Final outcome of one send attempt |
| category | enum | Existing diagnostic categories | `success`, `invalid_config`, `attachment_error`, `delivery_failure`, `internal_error` |
| channel | string | `telegram` | Affected channel |
| delivery_mode | enum | `private`, `topics` | Route used for the attempt |
| message | string | Secret-safe | Human-readable result context |

### Relationships

- One Notification Request produces one Telegram Delivery Result.
