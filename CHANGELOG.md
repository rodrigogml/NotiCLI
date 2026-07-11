# Changelog

## v2.0.1 - 2026-07-11

- Updated email subjects and private Telegram message titles to include priority as `[sender] [HIGH|NORMAL|LOW] title`.
- Expanded configuration documentation for broadcast routing and Telegram `private`, `topics` and `thread` delivery modes.

## v2.0.0 - 2026-07-11

- Introduced broadcast routing: callers now provide sender, optional category, priority, title, message and attachments while NotiCLI resolves routes from configuration.
- Removed legacy CLI targeting flags `--recipient` and `--channel`; added `--priority` with `NORMAL` default and optional `--category`.
- Replaced legacy `recipients`/`channels` configuration with `destinations`, `delivery_accounts`, `routes`, mandatory `catch_all` and optional delivery logging.
- Added multi-route dispatch with partial failure handling, complete delivery attempts and redacted JSONL delivery logs.
- Preserved Telegram automatic topic state outside the declarative config and added fixed Telegram thread destinations with `message_thread_id`.
- Updated documentation and tests for the breaking configuration and CLI contract.

## v1.1.3 - 2026-07-11

- Hardened default configuration discovery by removing caller `PATH` lookup from executable resolution.
- Documented the recommended `setuid`/`setgid` Linux installation model for protected shared configuration.
- Documented the recommended `/opt/NotiCLI/releases/<version>` and `/opt/NotiCLI/current` production layout.
- Added regression coverage to ensure fallback config discovery does not trust caller-controlled `PATH`.
