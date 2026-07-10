# Production Notes: Telegram Topics

Estado local preparado em 2026-06-28 para validar Telegram topics no ambiente de producao em levantamento.

## Paths

```text
/opt/NotiCLI/releases/v1.1.2/config/noticli.json
/opt/NotiCLI/config/noticli.telegram-topics.json
/opt/NotiCLI/bin/noticli -> /opt/NotiCLI/testing/current/noticli
/usr/local/bin/noticli -> /opt/NotiCLI/bin/noticli
```

## Permissions

```sh
sudo chown root:noticli /opt/NotiCLI/config /opt/NotiCLI/config/noticli.json
sudo chmod 0770 /opt/NotiCLI/config
sudo chmod 0640 /opt/NotiCLI/config/noticli.json
sudo ln -sfn /opt/NotiCLI/config /opt/NotiCLI/releases/v1.1.2/config
sudo ln -sfn /opt/NotiCLI/config /opt/NotiCLI/testing/current/config
```

Do not create an empty state file. If `/opt/NotiCLI/config/noticli.telegram-topics.json` is absent, NotiCLI initializes it with valid JSON on first topic-mode use. If the file is pre-created manually, it must contain valid topic state JSON and be writable by group `noticli`.

Users and service accounts do not need direct read access to config or state. The installed binary is executable through `/usr/local/bin/noticli` and runs with the `noticli` effective group.

## Recipient

The existing private recipient remains configured as `rodrigogml`.

A separate topic-mode recipient was added:

```text
rodrigogml-topics
```

It uses `telegram_delivery_mode=topics` and the configured topic-enabled Telegram supergroup. Do not put bot tokens, chat IDs, or email credentials in this document.

## Rollback

Return the executable symlink to the current testing binary:

```sh
sudo ln -sfn /opt/NotiCLI/testing/current/noticli /opt/NotiCLI/bin/noticli
sudo chown -h root:noticli /opt/NotiCLI/bin/noticli
sudo chmod 2755 /opt/NotiCLI/testing/current/noticli
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
