# Research: CLI Notifications

Documento produzido no Phase 0 do plan. Resolve as decisoes tecnicas abertas antes do design.

## Decision 1: Runtime and Distribution

**Decision**: Use Go 1.26.x as the target language/runtime and distribute the MVP primarily as a single executable binary for Linux, with Windows portable builds planned from the same codebase.

**Rationale**: The project needs a non-interactive CLI that can be called by other applications with low startup overhead, predictable exit codes and simple deployment. Go supports single-binary distribution and cross-platform builds, which aligns with Linux now and Windows portable later.

**Alternatives considered**: Python was considered for rapid implementation, but it adds runtime and packaging concerns for automated server usage and Windows portability. Shell scripts were rejected because multi-channel notification, config parsing, attachments and future local service/API support need stronger structure.

## Decision 2: Configuration Format

**Decision**: Use JSON for the MVP configuration file.

**Rationale**: The user is already familiar with JSON, Go supports JSON in the standard library, and JSON keeps the MVP dependency footprint small. The configuration must remain explicit about secret-bearing fields and channel-specific settings.

**Alternatives considered**: YAML is more ergonomic for humans but requires an external parser and can introduce surprising parsing behavior. TOML is readable and structured, but the user has less familiarity with it and it still adds format-specific decisions.

## Decision 3: CLI Shape

**Decision**: Define one primary command flow: `noticli send`, with flags for config file, sender system, recipient, channel, title, message content and attachments.

**Rationale**: A single explicit send command keeps the MVP small while leaving room for future commands such as `validate-config`, `channels` or `serve`. The command must be fully non-interactive and script-friendly.

**Alternatives considered**: A root command that sends directly was rejected because it leaves less room for future service and validation commands. Multiple channel-specific commands were rejected because they duplicate behavior and weaken channel isolation.

## Decision 4: Dependencies

**Decision**: Start with the Go standard library for argument parsing, JSON configuration, file validation and HTTP-based channel calls. Evaluate a focused SMTP/mail dependency during implementation only if standard-library mail support is insufficient for attachments and authentication requirements.

**Rationale**: The constitution prioritizes cost-zero operation, portability, small distribution footprint and robustness. Standard-library-first reduces dependency risk and keeps the MVP easier to audit.

**Alternatives considered**: Full channel SDKs were rejected for the MVP because Telegram and Slack can be reached through HTTP contracts, and SDKs can add unnecessary dependency surface. A CLI framework was deferred because the first command set is small.

## Decision 5: Result and Error Semantics

**Decision**: Use stable exit codes and single-line diagnostics by default. Diagnostics must identify category and channel when applicable, while redacting all secrets.

**Rationale**: Calling applications need deterministic branching behavior. Single-line diagnostics are easier to parse in scripts and logs. Secret redaction is required by the constitution.

**Alternatives considered**: Provider-native errors were rejected as direct output because they can leak secrets or vary unpredictably. JSON-only output was deferred; the MVP can support script-friendly text first and add structured output later if needed.

## Decision 6: Future Local Service

**Decision**: Keep notification dispatch independent from the CLI entrypoint so a future local service/API can reuse the same validation, configuration and channel dispatch behavior.

**Rationale**: The briefing identifies local service mode as a likely 6-month evolution. Separating command handling from dispatch avoids a rewrite when adding `serve`.

**Alternatives considered**: Building service mode now was rejected because the MVP scope is CLI-only and urgency is high. Coupling all behavior to CLI flag handling was rejected because it would violate the constitution's portability and future-service direction.
