<!--
Sync Impact Report
- Version: none -> 1.0.0
- Principios modificados: criacao inicial da constituicao
- Secoes adicionadas: Core Principles, Technical Standards, Delivery Standards, Governance
- Secoes removidas: nenhuma
- Artefatos que precisam atualizacao: AGENTS.md ausente; specs ausentes; plans ausentes; tasks ausentes
- TODOs pendentes: decidir formato final do arquivo de configuracao; definir convencao exata da CLI; definir politica detalhada de protecao de segredos; definir estrategia de anexos, logs e erros por canal
-->

# NotiCLI Constitution

## Core Principles

### I. Non-Interactive Contract

NotiCLI MUST operate without prompts, confirmations, menus or interactive input during notification execution. Every execution path MUST be fully controlled by command-line arguments, configuration files, environment variables or documented defaults.

Rationale: the primary caller is another application, so blocking on user interaction breaks automation and makes failures hard to diagnose.

### II. Deterministic CLI Results

Every command MUST return a deterministic exit code and a concise machine-readable or script-friendly error message on failure. Success and failure semantics MUST be documented before a command is considered complete.

Rationale: callers need reliable branching behavior when using NotiCLI in scripts, schedulers, CI jobs or server-side processes.

### III. Secure Configuration and Secret Handling

Configuration handling MUST avoid leaking tokens, passwords, SMTP credentials, webhook URLs or message-sensitive content through logs, error messages or examples. Any configuration schema MUST make secret-bearing fields explicit and MUST support safe local operation without paid services.

Rationale: notification integrations depend on credentials, and the project has a cost-zero constraint while still requiring robust security hygiene.

### IV. Channel Isolation

Notification channels MUST be implemented behind explicit channel boundaries so that email, Telegram, Slack and future channels can be added or changed without rewriting CLI parsing, configuration loading or core dispatch flow.

Rationale: future growth includes adding new notification channels, and the MVP already includes multiple integrations.

### V. Portable Core

The core implementation MUST remain portable across Linux and Windows unless a feature is explicitly documented as platform-specific. OS-specific behavior MUST be isolated behind small interfaces or adapter functions.

Rationale: Linux server execution is required now, and Windows portable distribution is a known future direction.

## Technical Standards

- Go is the project language for the initial implementation.
- The primary distribution target SHOULD be a single binary.
- The architecture MUST keep CLI command handling separate from notification dispatch logic so a future local HTTP API can reuse the same core behavior.
- Configuration MUST be file-based for the MVP.
- The final configuration format is not fixed yet; JSON is acceptable by user familiarity, but the implementation plan MAY choose another format if it provides a concrete advantage for Go and CLI ergonomics.
- The MVP MUST support email, Telegram and Slack as notification channels.
- The project MUST avoid paid infrastructure requirements.

## Delivery Standards

- Tests MUST cover CLI parsing, configuration loading, validation, dispatch routing and error mapping.
- Security-sensitive failures MUST be tested to avoid accidental credential disclosure.
- Logs and diagnostics SHOULD help identify failed channel, invalid configuration and rejected input without exposing secrets.
- Documentation MUST include command examples, configuration examples, exit code semantics and channel setup notes.
- Performance work SHOULD focus on low startup overhead and predictable execution time for direct CLI invocation.

## Governance

- This constitution governs architecture, quality and delivery decisions for NotiCLI.
- Specifications, implementation plans and task lists MUST be checked against these principles before execution.
- Exceptions to MUST-level principles require an explicit written rationale in the affected spec, plan or ADR.
- Changes to this constitution require updating the Sync Impact Report and version metadata.
- Versioning follows SemVer:
  - MAJOR for removing or redefining existing principles incompatibly.
  - MINOR for adding principles or materially expanding governance sections.
  - PATCH for clarifications that do not change project governance.
- The initial ratified version is 1.0.0 because this is the first formal constitution.

**Version**: 1.0.0 | **Ratified**: 2026-06-26 | **Last Amended**: 2026-06-26
