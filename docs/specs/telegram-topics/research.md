# Research: Telegram Topics por Recipient

Documento produzido no Phase 0 do plan. Resolve decisoes tecnicas antes do design.

## Decision 1: Telegram forum topics dependem de `message_thread_id`

**Decision**: Topic-based Telegram delivery will send messages to a configured supergroup chat ID plus a sender-specific `message_thread_id`.

**Rationale**: Telegram Bot API sends topic messages by including `message_thread_id` in `sendMessage`. Topic creation is done with `createForumTopic`, which returns a `ForumTopic` containing `message_thread_id`. This matches the validated production experiment in the `NotiCLI` supergroup where topic `ProdSmoke` was created and used successfully.

**Alternatives considered**: Sending all group messages to the main group was rejected because it does not provide sender separation. Encoding sender in message titles only was rejected because it duplicates the private-chat behavior and does not use Telegram topics.

**Source**: Telegram Bot API, `sendMessage` and `createForumTopic`: https://core.telegram.org/bots/api

## Decision 2: Telegram topics cannot be fully discovered by name

**Decision**: NotiCLI will not rely on runtime listing or lookup of all existing Telegram topics. Existing topics unknown to NotiCLI require future assisted binding or a newly created topic.

**Rationale**: Telegram Bot API does not provide a complete topic inventory endpoint or a "find topic by name" operation. Creating a topic with an existing visible name should be treated as possible and not assumed to return the previous topic ID.

**Alternatives considered**: Querying all topics before every send was rejected because the API does not support it. Treating topic names as unique was rejected because the API contract does not guarantee uniqueness or recovery by name.

**Source**: Telegram Bot API forum topic methods: https://core.telegram.org/bots/api

## Decision 3: Runtime topic associations live in a separate local state file

**Decision**: Store generated sender topic associations outside the main configuration, in a sibling file derived from the active config path. For `/opt/NotiCLI/releases/v1.1.2/config/noticli.json`, the state path is `/opt/NotiCLI/config/noticli.telegram-topics.json` through the release-local `config/` symlink.

**Rationale**: Recipient and channel configuration is operator-authored declarative data, while `message_thread_id` mappings are generated runtime state. Separating them keeps config reviewable and avoids noisy rewrites of the config file. Deriving the state file from the active `--config` path keeps alternate config files isolated instead of sharing one global topic cache.

**Alternatives considered**: Storing mappings inside `noticli.json` was rejected because it mixes runtime data with declared configuration. A fixed global state path was rejected because it ignores `--config` and can cross-contaminate profiles. An external database was rejected for the MVP because it violates the zero-infrastructure preference and is unnecessary for a single-host CLI.

## Decision 4: Use local file locking around topic state mutation

**Decision**: Serialize topic state reads/writes and auto-topic creation for the local installation using an OS file lock or equivalent local lock file.

**Rationale**: NotiCLI can be invoked concurrently by multiple applications. Without a local lock, two executions for the same recipient/sender could both miss the cache and create duplicate topics. A local lock preserves the single-binary/file-based model and is sufficient for the current single-server production setup.

**Alternatives considered**: No locking was rejected because it risks duplicated topics and corrupted state. Distributed locking was rejected for MVP because the deployment is single-host and the constitution favors simple local operation.

## Decision 5: Recovery tries known topic first, then controlled replacement

**Decision**: When a known topic association exists, NotiCLI first sends to that known topic. If Telegram rejects delivery in a way consistent with stale/unusable topic state, NotiCLI creates a replacement topic, updates state, and retries once.

**Rationale**: The send operation is the only reliable practical validation of topic usability. A retry-once recovery handles deleted or stale topics without prompting the caller, while avoiding unbounded duplicate topic creation.

**Alternatives considered**: Proactively validating every association was rejected because there is no authoritative topic lookup and it adds unnecessary provider calls. Failing immediately on stale state was rejected because automatic recovery is part of the feature value.

## Decision 6: Topic identity uses sender, display names are sanitized and disambiguated

**Decision**: Use the validated sender value as the state identity. Derive the Telegram topic display name from sender by trimming/collapsing whitespace, removing unsafe control characters, and applying a deterministic disambiguator if another sender would produce the same display name for the same recipient/group.

**Rationale**: The sender is already the caller-facing routing key. Keeping it as state identity avoids lossy matching, while sanitized display names keep Telegram topic creation valid and readable.

**Alternatives considered**: Using sanitized topic name as the sole key was rejected because two senders could collide. Rejecting all senders with imperfect display names was rejected because existing CLI sender validation should remain simple.

## Decision 7: Topic state backup is required for operational recovery

**Decision**: Preserve the last known valid topic state before replacing it and document that operators should back up the sibling `*.telegram-topics.json` state file next to each production config.

**Rationale**: Telegram cannot list topics by name, so topic state is operationally important. Backup reduces duplicate-topic risk after accidental deletion or corruption. Before creating a new Telegram topic, NotiCLI verifies that the active state file can be written; if not, the notification aborts instead of creating a topic whose `message_thread_id` cannot be persisted.

**Alternatives considered**: Treating state as disposable was rejected because state loss can create duplicate topics. Requiring a database was rejected for MVP because the current deployment is single-host and file-based.

## Decision 8: Private and topic modes use different title formatting

**Decision**: Private Telegram delivery prefixes the delivered title with `[sender]`; topic-based delivery does not prefix the title because the topic itself is the sender context.

**Rationale**: Private chat has no structural grouping, so sender context must appear in the message. Topic-based delivery already groups by sender, so repeating the sender in every title adds noise.

**Alternatives considered**: Always prefixing titles was rejected because it creates redundant topic messages. Never prefixing titles was rejected because private chat would lose sender context.

## Decision 9: Administrative bot commands are outside MVP implementation

**Decision**: The MVP of this feature will support automatic topic creation and local state persistence only. Telegram commands such as bind/list/unbind are documented as future work.

**Rationale**: Command handling requires webhook or polling lifecycle decisions that are larger than the direct-send CLI path. Keeping commands out of MVP preserves non-interactive send behavior while still supporting the primary topic organization flow.

**Alternatives considered**: Implementing commands immediately was rejected because it introduces listener/runtime concerns before the direct send path is stable. Manual JSON editing for each topic was rejected because it conflicts with the desired operator experience.
