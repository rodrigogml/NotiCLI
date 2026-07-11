# Production Notes: Telegram Topics

Estado local preparado em 2026-06-28 para validar Telegram topics no ambiente de producao em levantamento.

## Paths

```text
/opt/NotiCLI/releases/v1.1.2/config/noticli.json
/opt/NotiCLI/config/noticli.telegram-topics.json
/opt/NotiCLI/current -> /opt/NotiCLI/releases/<active-release>
/opt/NotiCLI/bin/noticli -> /opt/NotiCLI/current/noticli
/usr/local/bin/noticli -> /opt/NotiCLI/bin/noticli
```

## Permissions

```sh
sudo chown root:root /opt/NotiCLI /opt/NotiCLI/bin /opt/NotiCLI/releases
sudo chmod 0755 /opt/NotiCLI /opt/NotiCLI/bin /opt/NotiCLI/releases

sudo chown root:root /opt/NotiCLI/releases/<active-release>
sudo chmod 0755 /opt/NotiCLI/releases/<active-release>

sudo chown noticli:noticli /opt/NotiCLI/releases/<active-release>/noticli
sudo chmod 6755 /opt/NotiCLI/releases/<active-release>/noticli

sudo chown root:noticli /opt/NotiCLI/config /opt/NotiCLI/config/noticli.json
sudo chmod 0770 /opt/NotiCLI/config
sudo chmod 0640 /opt/NotiCLI/config/noticli.json

sudo ln -sfn /opt/NotiCLI/config /opt/NotiCLI/releases/<active-release>/config
sudo ln -sfn /opt/NotiCLI/releases/<active-release> /opt/NotiCLI/current
sudo ln -sfn /opt/NotiCLI/current/noticli /opt/NotiCLI/bin/noticli
```

Do not create an empty state file. If `/opt/NotiCLI/config/noticli.telegram-topics.json` is absent, NotiCLI initializes it with valid JSON on first topic-mode use. If the file is pre-created manually, it must contain valid topic state JSON and be writable by group `noticli`.

Users and service accounts do not need direct read access to config or state. The installed binary is executable through `/usr/local/bin/noticli` and runs with the `noticli` effective user and group because the release binary has the `setuid` and `setgid` bits.

Symlink permissions are not the access boundary. The target release binary owner/mode and the traversed directory permissions determine execution, while `/opt/NotiCLI/releases/<active-release>/config -> /opt/NotiCLI/config` keeps the config and topic state stable across releases.

## Nightly Builds

Night builds for production validation should be installed as their own release directory, for example:

```text
/opt/NotiCLI/releases/nightly-YYYYMMDD-HHMMSS/
```

The nightly directory must contain the built `noticli`, a `BUILD-INFO` file with the source commit/tree state, and a `config` symlink to `/opt/NotiCLI/config`. Activate the nightly by moving `/opt/NotiCLI/current` to that directory and keeping `/opt/NotiCLI/bin/noticli` pointed at `/opt/NotiCLI/current/noticli`.

## Recipient

The existing private recipient remains configured as `rodrigogml`.

A separate topic-mode recipient was added:

```text
rodrigogml-topics
```

It uses `telegram_delivery_mode=topics` and the configured topic-enabled Telegram supergroup. Do not put bot tokens, chat IDs, or email credentials in this document.

## Rollback

Return the active symlink to the previous release:

```sh
sudo ln -sfn /opt/NotiCLI/releases/v1.1.2 /opt/NotiCLI/current
sudo ln -sfn /opt/NotiCLI/current/noticli /opt/NotiCLI/bin/noticli
sudo chown noticli:noticli /opt/NotiCLI/releases/v1.1.2/noticli
sudo chmod 6755 /opt/NotiCLI/releases/v1.1.2/noticli
```

Remove the topic-mode recipient without changing the private recipient:

```sh
sudo python3 - <<'PY'
import json
from pathlib import Path

path = Path('/opt/NotiCLI/releases/v1.1.2/config/noticli.json')
data = json.loads(path.read_text())
data.get('recipients', {}).pop('rodrigogml-topics', None)
tmp = path.with_suffix('.json.tmp')
tmp.write_text(json.dumps(data, indent=2, ensure_ascii=False) + '\n')
tmp.replace(path)
PY
sudo chown root:noticli /opt/NotiCLI/config/noticli.json
sudo chmod 0640 /opt/NotiCLI/config/noticli.json
```

Restore topic state from backup when one exists:

```sh
sudo install -o root -g noticli -m 0660 /opt/NotiCLI/config/noticli.telegram-topics.json.bak /opt/NotiCLI/config/noticli.telegram-topics.json
```
