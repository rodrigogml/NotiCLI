# Implementation Plan: Telegram Topics por Recipient

**Feature**: `telegram-topics` | **Date**: 2026-06-28 | **Spec**: [spec.md](./spec.md)

## Summary

Implement recipient-level Telegram delivery modes so each recipient can receive Telegram notifications either in a private bot chat or in a topic-enabled supergroup. Private mode keeps sender context in the message title as `[sender] title`; topic mode uses the sender as the topic identity, automatically creates missing sender topics, stores topic associations in a separate local state file, and reuses known topics for later sends.

## Technical Context

**Language/Version**: Go 1.26
**Primary Dependencies**: Go standard library; existing Telegram Bot API HTTP integration
**Storage**: Main configuration remains JSON file; generated topic associations use a separate local JSON state file
**Testing**: `go test ./...` with package-level fakes/HTTP test servers and integration-style CLI tests
**Target Platform**: Linux server now; portable Go core for future Windows builds
**Project Type**: CLI application with internal channel adapters
**Performance Goals**: Private Telegram and known-topic reuse perform one message-send provider request. Unknown-sender topic delivery performs at most one topic-creation request plus one message-send request. Stale-topic recovery performs at most one failed send, one replacement-topic creation, and one retry send.
**Constraints**: Non-interactive execution; no paid infrastructure; no secret leakage; topic creation requires bot admin permission in a topic-enabled supergroup
**Scale/Scope**: Single-host production setup; multiple local NotiCLI invocations may happen concurrently

## Constitution Check

*GATE: Deve passar antes do Phase 0. Rechecar após Phase 1.*

| Princípio | Status | Notas |
|-----------|--------|-------|
| Non-Interactive Contract | PASS | Topic creation/reuse/recovery happens during send without prompts |
| Deterministic CLI Results | PASS | Existing exit categories remain; Telegram topic failures map to config/delivery/internal categories |
| Secure Configuration and Secret Handling | PASS | Bot token remains in secrets; topic state stores no credentials |
| Channel Isolation | PASS | Changes stay within Telegram channel/config/state boundaries |
| Portable Core | PASS | State and locking must use portable abstractions or isolated OS-specific adapters |

## Phase 0 Research

Research decisions are documented in [research.md](./research.md):

1. Telegram forum topics depend on `message_thread_id`.
2. Telegram topics cannot be fully discovered by name.
3. Runtime topic associations live in a separate local state file.
4. Local file locking protects topic state mutation.
5. Recovery tries known topic first, then controlled replacement.
6. Topic identity uses sender, display names are sanitized and disambiguated.
7. Topic state backup is required for operational recovery.
8. Private and topic modes use different title formatting.
9. Administrative bot commands are outside MVP implementation.

## Phase 1 Design

Design artifacts:

- [data-model.md](./data-model.md)
- [contracts/telegram-topics.md](./contracts/telegram-topics.md)
- [quickstart.md](./quickstart.md)

### Architecture Decision

Extend the existing Telegram channel flow with a routing decision after recipient/config resolution:

1. Resolve request, recipient, and Telegram channel config using existing core flow.
2. Determine recipient Telegram delivery mode.
3. Private mode:
   - require private chat destination;
   - format text with `[sender] title`;
   - send one Telegram message without topic fields.
4. Topic mode:
   - require topic-enabled group destination;
   - load topic state under local lock;
   - find association by recipient, group chat, and sender;
   - if missing, create topic and persist association;
   - send message with `message_thread_id` and title without `[sender]`;
   - on stale-topic delivery failure, apply one controlled recovery attempt.

### State Decision

Production state path is derived from the active config path:

```text
/opt/NotiCLI/releases/v1.1.2/config/noticli.json -> /opt/NotiCLI/config/noticli.telegram-topics.json
```

State is not part of the main recipient/channel config. It is generated operational data and should be protected with permissions equivalent to the NotiCLI runtime state owner. The config directory must allow the NotiCLI runtime group to create/replace the sibling state file. It must not contain Telegram bot tokens.

### Recovery Decision

Recovery is bounded:

1. Try known topic association first.
2. If provider response indicates the known topic cannot be used, mark association stale.
3. Create replacement topic once.
4. Persist replacement association.
5. Retry message once.
6. If replacement fails, return Telegram delivery failure.

No unbounded retries or prompts are allowed.

### Topic Naming Decision

Topic state identity uses the validated sender value, scoped by recipient and group chat. Topic display names are derived from the sender by trimming/collapsing whitespace, removing control characters, and enforcing Telegram topic-name constraints. If two sender values would produce the same display name for the same recipient/group, NotiCLI must add a deterministic disambiguator to the display name while keeping each sender's state association separate.

### Backup/Restore Decision

Topic state is operational data. Before replacing an existing valid state file, NotiCLI should preserve the last valid version in the same state directory so an operator can recover from accidental corruption or deletion. Production operations should include `/opt/NotiCLI/config/noticli.telegram-topics.json` in host backups. If the state file is lost and no backup is restored, the MVP may create new topics rather than discovering manual topics by name.

## Project Structure

### Documentation (this feature)

```text
docs/specs/telegram-topics/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
└── contracts/
    └── telegram-topics.md
```

### Source Code (repository root)

```text
cmd/noticli/
└── main.go
internal/
├── app/
│   ├── app.go
│   └── app_test.go
├── channels/
│   └── telegram/
│       ├── telegram.go
│       └── telegram_test.go
├── cli/
│   ├── parser.go
│   ├── parser_test.go
│   └── integration_test.go
├── config/
│   ├── loader.go
│   └── loader_test.go
├── diagnostics/
│   ├── diagnostics.go
│   └── redactor.go
└── notify/
    ├── types.go
    └── types_test.go
```

**Structure Decision**: Keep the Telegram provider HTTP integration inside `internal/channels/telegram`. Add recipient configuration fields through `internal/config` and `internal/notify`. Add a small internal state abstraction for Telegram topic associations rather than embedding state inside the channel sender, so state handling and locking can be tested independently.

## Convenções de Borda

| Camada | Case style | Validação | Fonte da verdade |
|--------|------------|-----------|------------------|
| CLI flags | kebab-case | existing parser validation | `internal/cli/parser.go` and contracts |
| Main config JSON | snake_case | config loader + domain validation | `docs/specs/telegram-topics/contracts/telegram-topics.md` |
| Topic state JSON | snake_case | state loader + schema version check | `docs/specs/telegram-topics/contracts/telegram-topics.md` |
| Telegram provider payload | Telegram Bot API field names | HTTP test server + provider responses | Telegram Bot API docs and channel adapter tests |
| Diagnostics | stable category strings | diagnostics tests | `internal/diagnostics` |

**Mapper layer (config -> domain)**: `internal/config/loader.go` maps JSON recipient fields into `internal/notify` domain types.

**Mapper layer (domain -> Telegram payload)**: `internal/channels/telegram` maps resolved request/recipient/channel state into provider payloads.

**Validação de schema**: Configuration validation happens before delivery; topic state validation happens before mutation/send; provider payload shape is validated by Telegram channel tests.

## Complexity Tracking

No constitution violations. Added complexity is limited to local topic state because Telegram requires `message_thread_id` for topic delivery and does not provide topic discovery by name.

## Post-Design Constitution Re-check

| Princípio | Status | Notas |
|-----------|--------|-------|
| Non-Interactive Contract | PASS | Recovery remains bounded and prompt-free |
| Deterministic CLI Results | PASS | All new failures map to existing categories |
| Secure Configuration and Secret Handling | PASS | Topic state excludes secrets and diagnostics stay redacted |
| Channel Isolation | PASS | Telegram-specific behavior remains isolated from email/Slack |
| Portable Core | PASS | Any OS-specific locking must be isolated behind a small state adapter |
