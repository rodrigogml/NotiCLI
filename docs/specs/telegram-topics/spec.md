# Feature Specification: Telegram Topics por Recipient

**Feature**: `telegram-topics`
**Created**: 2026-06-27
**Status**: Draft

## Clarifications

### Session 2026-06-28

- Q: How should topic state recovery behave when local state is missing or a manually created topic exists but is unknown to NotiCLI? -> A: The MVP treats local topic state as the operational source of truth; if state is missing, NotiCLI may create a new topic and operators should restore state from backup to avoid duplicates until assisted bind commands exist.
- Q: How should sender values become topic names? -> A: The validated sender remains the state identity; the topic display name is a sanitized display form of sender, and NotiCLI-created display-name collisions must be disambiguated deterministically.
- Q: How should stale known topics be handled? -> A: Try the known topic first, then create one replacement topic and retry once when the provider indicates the known topic is unusable.

## User Scenarios & Testing

### User Story 1 - Choose Telegram Delivery Mode Per Recipient (Priority: P1)

An operator configures each recipient to receive Telegram notifications either in a private chat or in a topic-enabled group, without changing how external applications invoke NotiCLI.

**Why this priority**: The feature only creates value if different recipients can choose the Telegram organization model that fits their workflow while callers keep using the same non-interactive command.

**Independent Test**: Configure one recipient for private Telegram delivery and another recipient for topic-based group delivery, send the same notification request shape to both recipients, and verify that each recipient receives the message in the configured mode.

**Acceptance Scenarios**:

1. **Given** a recipient configured for private Telegram delivery, **When** a caller sends a Telegram notification to that recipient, **Then** the notification is delivered to the recipient's private bot conversation.
2. **Given** a recipient configured for topic-based group delivery, **When** a caller sends a Telegram notification to that recipient, **Then** the notification is delivered to the configured group topic for the sender.
3. **Given** two recipients with different Telegram delivery modes, **When** the same caller sends Telegram notifications to both, **Then** each recipient's configured mode is honored independently.

---

### User Story 2 - Identify Private Chat Notifications By Sender (Priority: P2)

A recipient using private Telegram delivery sees the sender context directly in the notification title because all messages arrive in one conversation.

**Why this priority**: Private chat delivery has no topic separation, so the recipient needs the sender visible in the message itself to understand where the notification came from.

**Independent Test**: Send a Telegram notification to a private-chat recipient with a sender and title, then verify that the delivered message title includes the sender prefix.

**Acceptance Scenarios**:

1. **Given** a recipient configured for private Telegram delivery, **When** a notification is sent with sender `BackupJob` and title `Backup failed`, **Then** the delivered Telegram message title starts with `[BackupJob] Backup failed`.
2. **Given** a private-chat recipient receives multiple Telegram notifications from different senders, **When** the recipient views the conversation, **Then** each notification title identifies its sender.

---

### User Story 3 - Organize Group Notifications Into Sender Topics (Priority: P3)

A recipient using topic-based Telegram delivery receives notifications in a topic named after the notification sender, so messages from each calling system stay grouped without repeating the sender in every title.

**Why this priority**: The main purpose of the feature is to reduce clutter in Telegram by grouping operational notifications by source.

**Independent Test**: Configure a recipient for topic-based group delivery, send notifications from two different senders, and verify that each sender's messages appear in a distinct topic with message titles that do not repeat the sender prefix.

**Acceptance Scenarios**:

1. **Given** a topic-based recipient and no known topic for sender `DeployBot`, **When** a notification is sent from `DeployBot`, **Then** a topic for `DeployBot` is created or selected and the notification is delivered there.
2. **Given** a topic-based recipient and a known topic for sender `DeployBot`, **When** another notification is sent from `DeployBot`, **Then** the notification is delivered to the same topic.
3. **Given** a notification is delivered inside a sender topic, **When** the recipient reads the message, **Then** the title uses the caller-provided title without an additional `[sender]` prefix.

---

### User Story 4 - Recover When Topic State Is Missing Or Stale (Priority: P4)

An operator can rely on NotiCLI to continue sending Telegram group notifications even when local topic knowledge is missing or a previously known topic is no longer usable.

**Why this priority**: Topic IDs are required for reliable delivery, but Telegram does not provide a complete topic inventory. The feature must handle normal operational drift without requiring immediate manual JSON edits.

**Independent Test**: Send a topic-based notification when no sender topic is known, then send again using the stored topic knowledge; separately simulate an unusable known topic and verify that the user receives a clear failure or recovery behavior according to documented rules.

**Acceptance Scenarios**:

1. **Given** a topic-based recipient and no known topic for the sender, **When** a notification is sent, **Then** NotiCLI creates a new topic and records enough information to reuse it later.
2. **Given** a topic-based recipient has a known topic for the sender, **When** delivery to that topic succeeds, **Then** the known topic remains associated with that sender.
3. **Given** delivery to a known topic fails because the topic is unavailable, **When** NotiCLI handles the failure, **Then** it either creates a replacement topic or reports a clear delivery failure without exposing secrets.

---

### User Story 5 - Keep Configuration And Runtime Topic State Separate (Priority: P5)

An operator can edit recipient and channel configuration separately from topic state generated during Telegram delivery.

**Why this priority**: Recipient configuration is intentional operator input, while topic IDs are runtime state. Mixing them would make configuration harder to review, back up, and protect.

**Independent Test**: Configure a topic-based recipient, send notifications that create topic associations, and verify that recipient configuration remains stable while topic associations are stored separately.

**Acceptance Scenarios**:

1. **Given** a recipient is configured for topic-based Telegram delivery, **When** new sender topics are created, **Then** the recipient's declared configuration is not rewritten with runtime topic mappings.
2. **Given** an operator reviews the configuration, **When** runtime topic mappings exist, **Then** the operator can distinguish declared recipients from generated topic state.
3. **Given** runtime topic state is missing, **When** a sender notification is sent, **Then** NotiCLI can recreate or report the missing state according to documented rules.

---

### Edge Cases

- Recipient is configured for topic-based delivery but the target group is not topic-enabled.
- Bot is present in the group but lacks permission to create topics.
- Bot can create topics but cannot send messages in the group or topic.
- Sender name contains characters unsuitable for a topic name.
- Two senders normalize to the same topic name.
- Two concurrent invocations try to create the same sender topic for the same recipient.
- Known topic was deleted, closed, hidden, or otherwise cannot receive messages.
- Local topic state is missing, unreadable, malformed, or not writable.
- Topic exists manually with the desired name but NotiCLI does not know its ID.
- Telegram provider accepts a topic creation request but rejects the follow-up message.
- Private-chat recipient is missing a private chat ID.
- Topic-based recipient is missing a group chat ID.
- Attachments are requested for Telegram, which remains unsupported by this feature.

## Requirements

### Functional Requirements

- **FR-001**: The system MUST allow each recipient to define whether Telegram notifications are delivered to a private chat or to a topic-enabled group.
- **FR-002**: The system MUST preserve the existing Telegram private-chat delivery behavior for recipients that do not opt into topic-based group delivery.
- **FR-003**: For private-chat Telegram delivery, the system MUST include the sender in the delivered notification title using the format `[sender] title`.
- **FR-004**: For topic-based Telegram delivery, the system MUST use the sender as the topic identity for grouping notifications.
- **FR-005**: For topic-based Telegram delivery, the system MUST NOT add a `[sender]` prefix to the delivered title because the topic already represents the sender.
- **FR-006**: The system MUST create a sender topic automatically when a topic-based recipient receives a notification for a sender with no known topic association.
- **FR-007**: The system MUST reuse a known sender topic for subsequent topic-based notifications to the same recipient and sender.
- **FR-008**: The system MUST store generated topic associations separately from the recipient/channel configuration.
- **FR-009**: The system MUST include enough topic association data to distinguish recipient, target group, sender, topic identity, and last known usability.
- **FR-010**: The system MUST report a clear configuration failure when a recipient requests topic-based delivery but lacks the required group destination information.
- **FR-011**: The system MUST report a clear delivery failure when the bot lacks permission to create topics or send messages to the selected group/topic.
- **FR-012**: The system MUST handle stale topic associations by trying the known topic first, creating one replacement topic when the provider indicates the known topic is unusable, and retrying delivery once before returning a Telegram delivery failure.
- **FR-013**: The system MUST redact Telegram bot tokens and equivalent secrets from all diagnostics related to topic creation, topic reuse, and message delivery.
- **FR-014**: The system MUST keep Telegram attachment behavior unchanged; Telegram requests with attachments continue to fail with the documented attachment failure category.
- **FR-015**: The system MUST preserve non-interactive CLI execution; topic creation, reuse, recovery, and failures must not prompt for input.
- **FR-016**: The system MUST provide documentation showing how to configure a recipient for private chat delivery and topic-based group delivery.
- **FR-017**: The system MUST document that Telegram does not provide a complete topic listing through the bot integration, so topics created outside NotiCLI may require future assisted binding.
- **FR-018-INFRA-STATE**: Topic associations MUST be persisted in local state separate from main configuration, with an explicit policy for missing, unreadable, malformed, or unwritable state.
- **FR-019-INFRA-IDEMP**: Automatic topic creation MUST be idempotent per recipient, target group, and sender so repeated or concurrent sends do not intentionally create duplicate topics for the same sender.
- **FR-020-INFRA-LOCK**: The system MUST serialize updates to topic association state for the same local installation to avoid corrupting state or racing duplicate topic creation.
- **FR-021-INFRA-BACKUP**: The system MUST preserve the last known valid topic state before replacing it and documentation MUST describe operator backup/restore expectations for topic state.
- **FR-022**: The system MUST use the validated sender value as the unique state identity for topic routing.
- **FR-023**: The system MUST derive Telegram topic display names from sender values using a documented sanitization rule that removes unsafe control characters and respects provider topic-name constraints.
- **FR-024**: When two sender values would produce the same NotiCLI-created topic display name for the same recipient and group, the system MUST disambiguate the display name deterministically while preserving each sender's separate state identity.
- **FR-025**: If topic state is missing, the system MAY create new topics for senders even when similarly named manual topics already exist, and documentation MUST explain that assisted binding is future work.

> Decisoes de infraestrutura: esta feature persiste estado local e precisa de politica explicita de idempotencia, locking local, backup/restore operacional e recuperacao quando o estado estiver ausente ou inconsistente. Nao introduz scheduler obrigatorio no MVP; comandos por webhook/polling ficam fora do MVP desta spec.

### Key Entities

- **Telegram Delivery Preference**: Recipient-level choice that determines whether Telegram messages go to a private chat or to a topic-enabled group.
- **Telegram Private Destination**: The recipient's private Telegram conversation used when no topic separation is desired.
- **Telegram Topic Group Destination**: A recipient's topic-enabled group destination used to organize Telegram messages by sender.
- **Sender Topic Association**: Runtime association between a recipient, group destination, sender, and the known topic used for delivery.
- **Topic State**: Locally persisted set of sender topic associations generated or updated during Telegram delivery.
- **Telegram Delivery Result**: Outcome of a Telegram notification attempt, including success, invalid configuration, attachment failure, or delivery failure.

## Success Criteria

### Measurable Outcomes

- **SC-001**: A private-chat Telegram recipient receives a valid notification with `[sender] title` formatting in 100% of covered private delivery tests.
- **SC-002**: A topic-based Telegram recipient receives a valid first notification for an unknown sender in a sender-specific topic in 100% of covered happy-path tests.
- **SC-003**: A second notification for the same topic-based recipient and sender reuses the same known topic in 100% of covered reuse tests.
- **SC-004**: Topic-based Telegram messages omit the `[sender]` title prefix in 100% of covered topic delivery tests.
- **SC-005**: Invalid topic-based recipient configuration is reported before message delivery is attempted in 100% of covered configuration tests.
- **SC-006**: Telegram bot tokens are absent from user-visible diagnostics in 100% of covered failure tests.
- **SC-007**: Concurrent local sends for the same recipient and sender do not corrupt topic state in 100% of covered concurrency tests.
- **SC-008**: A new operator can configure one private-chat recipient and one topic-based recipient using project documentation in under 20 minutes.
- **SC-009**: Topic reuse for a known active topic performs no topic-creation request in 100% of covered reuse tests.
- **SC-010**: First send for an unknown sender performs at most one topic-creation request before the message send in 100% of covered happy-path tests.
- **SC-011**: Stale known-topic recovery performs at most one replacement-topic creation and one retry in 100% of covered stale-topic tests.
