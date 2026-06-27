# Quickstart: CLI Notifications

Cenarios de teste que validam a implementacao end-to-end. Os exemplos usam nomes ilustrativos; valores reais devem vir de configuracoes locais do operador.

## Scenario 1: Send Email Notification

1. Create a local configuration file with one enabled recipient containing an email destination and one enabled email channel.
2. Run `noticli send --config ./noticli.json --sender TestRunner --recipient ops --channel email --title "Test" --message "Email notification test"`.
3. **Expected**: Command exits with code `0`; recipient receives the email; no prompt appears.

## Scenario 2: Send Telegram Notification

1. Create a local configuration file with one enabled recipient containing a Telegram destination and one enabled Telegram channel.
2. Run `noticli send --config ./noticli.json --sender TestRunner --recipient ops --channel telegram --title "Test" --message "Telegram notification test"`.
3. **Expected**: Command exits with code `0`; configured Telegram destination receives the message; no prompt appears.

## Scenario 3: Send Slack Notification

1. Create a local configuration file with one enabled recipient or destination for Slack and one enabled Slack channel.
2. Run `noticli send --config ./noticli.json --sender TestRunner --recipient ops --channel slack --title "Test" --message "Slack notification test"`.
3. **Expected**: Command exits with code `0`; configured Slack destination receives the message; no prompt appears.

## Scenario 4: Missing Required Argument

1. Run `noticli send --config ./noticli.json --sender TestRunner --channel email --title "Test" --message "Missing recipient"`.
2. **Expected**: Command exits with code `2`; diagnostic category is `invalid_input`; no notification is attempted.

## Scenario 5: Missing Configuration

1. Run `noticli send --config ./missing.json --sender TestRunner --recipient ops --channel email --title "Test" --message "Missing config"`.
2. **Expected**: Command exits with code `3`; diagnostic category is `missing_config`; no secret values are printed.

## Scenario 6: Invalid Channel Configuration

1. Create a configuration file where the selected channel is missing required credentials.
2. Run `noticli send --config ./noticli.json --sender TestRunner --recipient ops --channel telegram --title "Test" --message "Invalid config"`.
3. **Expected**: Command exits with code `4`; diagnostic category is `invalid_config`; missing fields are identified without printing configured secret values.

## Scenario 7: Attachment Error

1. Run `noticli send --config ./noticli.json --sender TestRunner --recipient ops --channel email --title "Test" --message "Attachment test" --attach ./missing-file.txt`.
2. **Expected**: Command exits with code `5`; diagnostic category is `attachment_error`; no delivery is attempted.

## Scenario 8: Channel Delivery Failure

1. Configure a channel with invalid provider credentials.
2. Run `noticli send --config ./noticli.json --sender TestRunner --recipient ops --channel slack --title "Test" --message "Delivery failure test"`.
3. **Expected**: Command exits with code `6`; diagnostic category is `delivery_failure`; output identifies `slack` and redacts credentials.

## Scenario 9: Future Service Compatibility Check

1. Review that notification request validation, configuration resolution and channel dispatch can be exercised independently of CLI argument parsing.
2. **Expected**: The implementation can expose the same notification behavior through a future local service without duplicating delivery rules.
