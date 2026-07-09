# Release Notes

## v1.1.1

- Fixed default config resolution so the installed binary can locate the published config under `/opt/NotiCLI/config/noticli.json` even when the executable is reached through symlinks.
- Kept Telegram topic delivery state next to the active config file, preserving the expected production layout under `/opt/NotiCLI/config/`.
- Added regression coverage for default config lookup through the published binary path.
- Verified the installed release with real `vaultgfs` sends for both Telegram topic delivery and private chat delivery.
