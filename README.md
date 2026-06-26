# NotiCLI

NotiCLI is a non-interactive command-line application for sending notifications from other applications.

The MVP targets direct CLI invocation on Linux, with a portable Go codebase that can later support Windows builds and a local service/API mode. Initial notification channels are email, Telegram and Slack.

## Status

Project discovery, specification and implementation planning are available under `docs/`.

## Development

This project uses Go. The planned implementation keeps CLI parsing separate from the notification core so the same behavior can be reused by future entrypoints.

```sh
go test ./...
```
