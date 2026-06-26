# Implementation Plan: CLI Notifications

**Feature**: `cli-notifications` | **Date**: 2026-06-26 | **Spec**: [spec.md](./spec.md)

## Summary

Implement the MVP as a non-interactive notification CLI with one primary `send` command. The technical approach is standard-library-first, JSON configuration, explicit channel boundaries for email, Telegram and Slack, deterministic exit codes, secret-safe diagnostics and a reusable notification dispatch core that can later support a local service/API entrypoint.

## Technical Context

**Language/Version**: Go 1.26.x
**Primary Dependencies**: Go standard library for CLI parsing, JSON configuration, file checks and HTTP calls; focused mail dependency only if required for robust SMTP attachments/authentication
**Storage**: Local JSON configuration file; no database
**Testing**: `go test`, table-driven unit tests, CLI integration tests via process execution
**Target Platform**: Linux server execution for MVP; Windows portable build compatibility preserved
**Project Type**: CLI tool with reusable core package
**Performance Goals**: Validation-only paths complete in under 1 second locally; startup overhead remains suitable for direct process invocation
**Constraints**: Non-interactive execution; cost-zero operation; secret-safe diagnostics; deterministic exit codes; no paid infrastructure requirement
**Scale/Scope**: Single notification attempt per invocation; MVP channels are email, Telegram and Slack; future service mode is planned but out of MVP runtime scope

## Constitution Check

*GATE: Deve passar antes do Phase 0. Rechecar apos Phase 1.*

| Principio | Status | Notas |
|-----------|--------|-------|
| Non-Interactive Contract | PASS | The CLI contract forbids prompts and requires all input through args/config/defaults. |
| Deterministic CLI Results | PASS | Exit codes and failure categories are defined in `contracts/cli.md`. |
| Secure Configuration and Secret Handling | PASS | Research and contracts require secret grouping and redacted diagnostics. |
| Channel Isolation | PASS | Design separates notification request, channel configuration and delivery behavior. |
| Portable Core | PASS | Plan targets Linux now and keeps Windows path/build compatibility as a design constraint. |

## Project Structure

### Documentation (this feature)

```text
docs/specs/cli-notifications/
|--- spec.md
|--- plan.md
|--- research.md
|--- data-model.md
|--- quickstart.md
`--- contracts/
    `--- cli.md
```

### Source Code (repository root)

Current repository source structure:

```text
.
`--- docs/
    |--- briefing/
    |--- constitution.md
    `--- specs/
```

Planned implementation structure:

```text
.
|--- cmd/
|   `--- noticli/              # CLI entrypoint
|--- internal/
|   |--- app/                  # use-case orchestration for one notification attempt
|   |--- channels/
|   |   |--- email/            # email delivery adapter
|   |   |--- slack/            # Slack delivery adapter
|   |   `--- telegram/         # Telegram delivery adapter
|   |--- config/               # config loading, validation and secret redaction metadata
|   |--- diagnostics/          # exit code and safe error mapping
|   `--- notify/               # core request, validation and dispatch interfaces
|--- testdata/                 # sample configs and fixtures without real secrets
|--- go.mod
`--- README.md
```

**Structure Decision**: The repository currently contains documentation only. Implementation should create a small Go CLI layout with one executable under `cmd/noticli` and internal packages separated by responsibility. CLI parsing must depend on the core notification use case, not the other way around, so future service mode can reuse the same core behavior.

## Convencoes de Borda

| Camada | Case style | Validacao | Fonte da verdade |
|--------|------------|-----------|------------------|
| CLI flags | kebab-case | command parser + contract tests | `contracts/cli.md` |
| Config fields | snake_case | config schema validation tests | `contracts/cli.md` and config package |
| Diagnostic category | snake_case | exit-code mapping tests | `contracts/cli.md` |
| Channel identifiers | lower-case words | channel registry validation | `contracts/cli.md` |
| File paths | OS-native path strings | attachment/config file validation tests | data model and quickstart |

**Mapper layer (CLI <-> core)**: CLI argument parsing maps raw flags to a Notification Request in `internal/app` or equivalent orchestration layer.

**Validacao de schema**: Request validation occurs before dispatch; configuration validation occurs before channel use; diagnostic mapping is validated for both expected user errors and unexpected internal errors.

## Complexity Tracking

No constitution violations identified. No added complexity requires exception tracking.

## Phase 0: Research Summary

Research decisions are documented in [research.md](./research.md):

1. Go 1.26.x and single-binary distribution.
2. JSON configuration for MVP.
3. Primary command flow: `noticli send`.
4. Standard-library-first dependency posture.
5. Stable exit codes and secret-safe diagnostics.
6. Dispatch core kept independent for future local service mode.

## Phase 1: Design Summary

Design artifacts:

- [data-model.md](./data-model.md): Notification Request, Recipient, Channel Configuration, Attachment Reference and Delivery Result.
- [contracts/cli.md](./contracts/cli.md): CLI command, arguments, exit codes, diagnostic output and configuration contract.
- [quickstart.md](./quickstart.md): happy paths and failure scenarios for implementation validation.

## Post-Design Constitution Re-check

| Principio | Status | Notas |
|-----------|--------|-------|
| Non-Interactive Contract | PASS | Quickstart and contract validate non-interactive operation. |
| Deterministic CLI Results | PASS | Exit-code categories are part of the external contract. |
| Secure Configuration and Secret Handling | PASS | Secret redaction is required in contracts, data model and quickstart error cases. |
| Channel Isolation | PASS | Source structure and data model keep channel adapters isolated. |
| Portable Core | PASS | Plan avoids OS-specific assumptions except documented path validation. |
