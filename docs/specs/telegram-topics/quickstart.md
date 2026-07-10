# Quickstart: Telegram Topics por Recipient

Cenarios de teste para validar a implementacao end-to-end da feature.

## Preparacao Operacional

### Obter chat privado do Telegram

1. Abra uma conversa direta com o bot.
2. Envie `/start`.
3. Obtenha o `chat_id` por uma chamada controlada ao Bot API ou por uma ferramenta operacional que leia `getUpdates`.
4. Configure o recipient com:
   ```json
   {
     "telegram_delivery_mode": "private",
     "telegram_chat_id": "TELEGRAM_PRIVATE_CHAT_ID"
   }
   ```

### Preparar supergrupo com topicos

1. Crie ou escolha um supergrupo Telegram.
2. Habilite topicos no grupo.
3. Adicione o bot ao grupo.
4. Conceda ao bot permissao de administrador suficiente para criar/gerenciar topicos e enviar mensagens.
5. Obtenha o `chat_id` do supergrupo por uma chamada controlada ao Bot API ou por evento recebido do grupo.
6. Configure o recipient com:
   ```json
   {
     "telegram_delivery_mode": "topics",
     "telegram_topic_group_chat_id": "TELEGRAM_SUPERGROUP_CHAT_ID",
     "telegram_topic_group_name": "Operations Notifications"
   }
   ```

### Estado local e backup

O estado de topicos fica fora do arquivo principal de configuracao:

```text
/opt/NotiCLI/config/noticli.telegram-topics.json
```

Inclua esse arquivo nos backups do host. Antes de testes destrutivos ou mudanca manual de topicos, faca uma copia:

```sh
sudo cp -a /opt/NotiCLI/config/noticli.telegram-topics.json /opt/NotiCLI/config/noticli.telegram-topics.json.bak
```

Para restaurar o ultimo backup conhecido:

```sh
sudo install -o root -g noticli -m 0660 /opt/NotiCLI/config/noticli.telegram-topics.json.bak /opt/NotiCLI/config/noticli.telegram-topics.json
```

Se o estado for perdido sem backup, NotiCLI pode criar novos topicos para senders ja existentes. Antes de criar um topico novo, NotiCLI verifica se consegue escrever o arquivo de estado irmao; se nao conseguir, a notificacao falha antes da chamada de criacao. O Bot API nao oferece listagem completa por nome de todos os topicos; comandos assistidos de bind/list/unbind sao trabalho futuro.

## Scenario 1: Private Chat Keeps Sender Prefix

1. Configure a recipient with `telegram_delivery_mode` omitted or set to `private` and a valid `telegram_chat_id`.
2. Run:
   ```sh
   noticli send --config /opt/NotiCLI/releases/v1.1.2/config/noticli.json --sender BackupJob --recipient RECIPIENT_PRIVATE --channel telegram --title "Backup failed" --message "Nightly backup failed"
   ```
3. **Expected**: Command exits with code `0`; recipient receives a Telegram message whose title starts with `[BackupJob] Backup failed`.

## Scenario 2: Topic-Based Delivery Creates Sender Topic

1. Create or use a Telegram supergroup with topics enabled.
2. Add the bot as admin with permission to create/manage topics and send messages.
3. Configure a recipient with `telegram_delivery_mode: "topics"` and a valid topic group chat ID.
4. Ensure the topic state file has no association for sender `DeployBot`.
5. Run:
   ```sh
   noticli send --config /opt/NotiCLI/releases/v1.1.2/config/noticli.json --sender DeployBot --recipient RECIPIENT_TOPICS --channel telegram --title "Deploy complete" --message "Release completed"
   ```
6. **Expected**: Command exits with code `0`; a `DeployBot` topic exists in the group; message appears inside that topic; title does not include `[DeployBot]`.

## Scenario 3: Topic-Based Delivery Reuses Known Topic

1. Complete Scenario 2 so topic state contains an association for `DeployBot`.
2. Run another send with the same sender and recipient:
   ```sh
   noticli send --config /opt/NotiCLI/releases/v1.1.2/config/noticli.json --sender DeployBot --recipient RECIPIENT_TOPICS --channel telegram --title "Deploy started" --message "Release started"
   ```
3. **Expected**: Command exits with code `0`; message appears in the existing `DeployBot` topic; no duplicate `DeployBot` topic is intentionally created.

## Scenario 4: Missing Topic Group Configuration

1. Configure a recipient with `telegram_delivery_mode: "topics"` but no topic group chat ID.
2. Run a Telegram send to that recipient.
3. **Expected**: Command exits with configuration failure; no Telegram provider send is attempted; diagnostic identifies Telegram recipient configuration without exposing the bot token.

## Scenario 5: Bot Cannot Create Topic

1. Configure a recipient for topic-based delivery to a group where the bot can send messages but cannot create/manage topics.
2. Ensure no association exists for the sender.
3. Run a Telegram send to that recipient.
4. **Expected**: Command exits with delivery failure; diagnostic identifies Telegram delivery failure and does not expose the bot token.

## Scenario 6: Stale Topic Association Recovery

1. Configure a valid topic-based recipient.
2. Create and store an association for sender `ProdSmoke`.
3. Make the known topic unusable by deleting or closing it in Telegram.
4. Run:
   ```sh
   noticli send --config /opt/NotiCLI/releases/v1.1.2/config/noticli.json --sender ProdSmoke --recipient RECIPIENT_TOPICS --channel telegram --title "Smoke test" --message "Testing stale topic recovery"
   ```
5. **Expected**: NotiCLI either creates a replacement topic and sends once successfully or exits with a clear Telegram delivery failure according to the implemented recovery policy. Token remains redacted.

## Validacao Local Sem Envio Real

Estes comandos validam a forma da CLI sem chamar provedores externos:

```sh
go run ./cmd/noticli
go run ./cmd/noticli send --config /tmp/noticli-missing.json --sender BackupJob --recipient RECIPIENT_PRIVATE --channel telegram --title "Backup failed" --message "Nightly backup failed"
```

Resultados esperados:

- O primeiro comando retorna `invalid_input` com usage.
- O segundo comando retorna `missing_config` depois de aceitar a forma do comando.

## Scenario 7: Concurrent Sends For Same Sender

1. Configure a topic-based recipient with no association for sender `ConcurrentJob`.
2. Start two local sends at nearly the same time for the same recipient and sender.
3. **Expected**: Topic state remains valid; at most one active association exists for `recipient + chat_id + ConcurrentJob`; both commands either succeed using one topic or one reports a controlled failure without corrupting state.

## Scenario 8: Sender Display Name Collision

1. Configure a topic-based recipient.
2. Send two notifications with different valid sender values that sanitize to the same topic display name.
3. **Expected**: Topic state keeps separate associations by original sender identity; NotiCLI-created topic display names are deterministically disambiguated.

## Scenario 9: Lost Topic State

1. Configure a topic-based recipient and send a notification that creates a sender topic.
2. Remove the local topic state file without restoring backup.
3. Send another notification for the same sender.
4. **Expected**: NotiCLI may create a new topic for that sender; documentation explains that restoring topic state backup avoids duplicates and assisted bind commands are future work.
