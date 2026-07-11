# Changelog

## v1.1.3 - 2026-07-11

- Hardened default configuration discovery by removing caller `PATH` lookup from executable resolution.
- Documented the recommended `setuid`/`setgid` Linux installation model for protected shared configuration.
- Documented the recommended `/opt/NotiCLI/releases/<version>` and `/opt/NotiCLI/current` production layout.
- Added regression coverage to ensure fallback config discovery does not trust caller-controlled `PATH`.

