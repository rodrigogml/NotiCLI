package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rodrigogml/NotiCLI/internal/config"
	"github.com/rodrigogml/NotiCLI/internal/diagnostics"
	"github.com/rodrigogml/NotiCLI/internal/notify"
)

func TestLoadReadsValidJSONConfiguration(t *testing.T) {
	path := writeTempConfig(t, `{
		"recipients": {
			"ops": {
				"name": "Operations",
				"email": "ops@example.com",
				"telegram_chat_id": "12345",
				"slack_destination": "#ops"
			},
			"dev": {
				"id": "dev-team",
				"email": "dev@example.com",
				"enabled": false
			}
		},
		"channels": {
			"email": {
				"settings": {"from": "noticli@example.com", "host": "smtp.example.com"},
				"secrets": {"smtp_password": "secret"},
				"attachments": "supported"
			},
			"telegram": {
				"type": "telegram",
				"settings": {"parse_mode": "plain"},
				"secrets": {"token": "secret"},
				"attachments": "limited"
			}
		},
		"defaults": {"channel": "email"}
	}`)

	configuration, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	ops := configuration.Recipients["ops"]
	if ops.ID != "ops" {
		t.Fatalf("ops.ID = %q, want ops", ops.ID)
	}
	if !ops.Enabled {
		t.Fatal("ops.Enabled = false, want true default")
	}
	if got := ops.EffectiveTelegramDeliveryMode(); got != notify.TelegramDeliveryModePrivate {
		t.Fatalf("ops telegram delivery mode = %q, want private default", got)
	}
	if ops.TelegramChatID != "12345" {
		t.Fatalf("ops.TelegramChatID = %q, want 12345", ops.TelegramChatID)
	}
	dev := configuration.Recipients["dev"]
	if dev.ID != "dev-team" {
		t.Fatalf("dev.ID = %q, want dev-team", dev.ID)
	}
	if dev.Enabled {
		t.Fatal("dev.Enabled = true, want false")
	}

	email := configuration.Channels[notify.ChannelEmail]
	if !email.Enabled {
		t.Fatal("email.Enabled = false, want true default")
	}
	if email.Type != notify.ChannelEmail {
		t.Fatalf("email.Type = %q", email.Type)
	}
	if email.AttachmentPolicy != notify.AttachmentPolicySupported {
		t.Fatalf("email.AttachmentPolicy = %q", email.AttachmentPolicy)
	}
	if configuration.Defaults["channel"] != notify.ChannelEmail {
		t.Fatalf("default channel = %q", configuration.Defaults["channel"])
	}
}

func TestLoadMapsTopicTelegramRecipient(t *testing.T) {
	path := writeTempConfig(t, `{
		"recipients": {
			"rodrigogml-topics": {
				"name": "Rodrigo GML Topics",
				"telegram_delivery_mode": "topics",
				"telegram_topic_group_chat_id": "-1001234567890",
				"telegram_topic_group_name": "NotiCLI"
			}
		},
		"channels": {
			"telegram": {
				"type": "telegram",
				"settings": {"parse_mode": "HTML"},
				"secrets": {"token": "123456:ABCDEF"},
				"attachments": "unsupported"
			}
		}
	}`)

	configuration, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	recipient := configuration.Recipients["rodrigogml-topics"]
	if recipient.TelegramDeliveryMode != notify.TelegramDeliveryModeTopics {
		t.Fatalf("TelegramDeliveryMode = %q, want topics", recipient.TelegramDeliveryMode)
	}
	if recipient.TelegramTopicGroupChatID != "-1001234567890" {
		t.Fatalf("TelegramTopicGroupChatID = %q", recipient.TelegramTopicGroupChatID)
	}
	if recipient.TelegramTopicGroupName != "NotiCLI" {
		t.Fatalf("TelegramTopicGroupName = %q, want NotiCLI", recipient.TelegramTopicGroupName)
	}
}

func TestLoadRejectsInvalidTelegramDeliveryMode(t *testing.T) {
	path := writeTempConfig(t, `{
		"recipients": {
			"ops": {
				"telegram_chat_id": "12345",
				"telegram_delivery_mode": "broadcast"
			}
		},
		"channels": {
			"telegram": {
				"type": "telegram",
				"settings": {"parse_mode": "HTML"},
				"secrets": {"token": "123456:ABCDEF"},
				"attachments": "unsupported"
			}
		}
	}`)

	_, err := config.Load(path)
	assertDiagnosticCategory(t, err, diagnostics.CategoryInvalidConfig)
	assertDoesNotContain(t, err, "123456:ABCDEF")
}

func TestLoadAllowsIncompleteTopicDestinationButResolveRejectsWithoutTokenLeak(t *testing.T) {
	path := writeTempConfig(t, `{
		"recipients": {
			"ops": {
				"telegram_delivery_mode": "topics"
			}
		},
		"channels": {
			"telegram": {
				"type": "telegram",
				"settings": {"parse_mode": "HTML"},
				"secrets": {"token": "123456:ABCDEF"},
				"attachments": "unsupported"
			}
		}
	}`)

	configuration, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	_, err = configuration.Resolve(notify.Request{
		SenderSystem: "BackupJob",
		RecipientID:  "ops",
		Channel:      notify.ChannelTelegram,
		Title:        "Backup failed",
		Message:      "Nightly backup failed",
	})
	assertDiagnosticCategory(t, err, diagnostics.CategoryInvalidConfig)
	assertDoesNotContain(t, err, "123456:ABCDEF")
}

func TestLoadReturnsMissingConfigForAbsentFile(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "missing.json"))
	assertDiagnosticCategory(t, err, diagnostics.CategoryMissingConfig)
}

func TestLoadReturnsMissingConfigForUnreadablePath(t *testing.T) {
	_, err := config.Load(t.TempDir())
	assertDiagnosticCategory(t, err, diagnostics.CategoryMissingConfig)
}

func TestLoadReturnsInvalidConfigForMalformedJSON(t *testing.T) {
	path := writeTempConfig(t, `{"recipients":`)

	_, err := config.Load(path)
	assertDiagnosticCategory(t, err, diagnostics.CategoryInvalidConfig)
}

func TestLoadReturnsInvalidConfigForIncompleteConfiguration(t *testing.T) {
	path := writeTempConfig(t, `{"recipients": {}, "channels": {}}`)

	_, err := config.Load(path)
	assertDiagnosticCategory(t, err, diagnostics.CategoryInvalidConfig)
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "noticli.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func assertDiagnosticCategory(t *testing.T, err error, want diagnostics.Category) {
	t.Helper()

	if err == nil {
		t.Fatalf("error = nil, want %q", want)
	}
	var diagnostic diagnostics.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error type = %T, want diagnostics.Diagnostic", err)
	}
	if diagnostic.Category != want {
		t.Fatalf("Category = %q, want %q", diagnostic.Category, want)
	}
}

func assertDoesNotContain(t *testing.T, err error, value string) {
	t.Helper()

	if err == nil {
		return
	}
	if value != "" && strings.Contains(err.Error(), value) {
		t.Fatalf("error %q contains sensitive value %q", err.Error(), value)
	}
}
