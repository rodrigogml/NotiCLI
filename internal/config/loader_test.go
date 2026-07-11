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

func TestLoadReadsValidBroadcastConfiguration(t *testing.T) {
	path := writeTempConfig(t, `{
		"destinations": {
			"ops-email": {"type": "email", "email": "ops@example.com"},
			"ops-telegram-thread": {
				"type": "telegram",
				"telegram_delivery_mode": "thread",
				"telegram_topic_group_chat_id": "-1001234567890",
				"message_thread_id": 42
			}
		},
		"delivery_accounts": {
			"smtp-main": {
				"type": "email",
				"settings": {"from": "noticli@example.com", "host": "smtp.example.com", "port": "587"},
				"secrets": {"smtp_password": "secret"},
				"attachments": "supported"
			},
			"telegram-main": {
				"type": "telegram",
				"settings": {"parse_mode": "HTML"},
				"secrets": {"token": "123456:ABCDEF"},
				"attachments": "unsupported"
			}
		},
		"routes": [
			{
				"id": "backup-high",
				"match": {"senders": ["BackupJob"], "categories": ["backup"], "priorities": ["HIGH"]},
				"deliveries": [
					{"account": "smtp-main", "destination": "ops-email"},
					{"account": "telegram-main", "destination": "ops-telegram-thread"}
				]
			}
		],
		"catch_all": {"deliveries": [{"account": "smtp-main", "destination": "ops-email"}]},
		"logging": {"path": "/tmp/noticli.log"}
	}`)

	configuration, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if configuration.Destinations["ops-email"].ID != "ops-email" {
		t.Fatalf("destination ID = %q", configuration.Destinations["ops-email"].ID)
	}
	if configuration.DeliveryAccounts["smtp-main"].AttachmentPolicy != notify.AttachmentPolicySupported {
		t.Fatalf("smtp attachment policy = %q", configuration.DeliveryAccounts["smtp-main"].AttachmentPolicy)
	}
	if configuration.Routes[0].ID != "backup-high" {
		t.Fatalf("route ID = %q", configuration.Routes[0].ID)
	}
	if configuration.Logging.Path != "/tmp/noticli.log" {
		t.Fatalf("logging path = %q", configuration.Logging.Path)
	}
}

func TestLoadDefaultsLoggingPathBesideConfig(t *testing.T) {
	path := writeTempConfig(t, minimalConfigJSON())

	configuration, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := filepath.Join(filepath.Dir(path), "noticli.delivery.log")
	if configuration.Logging.Path != want {
		t.Fatalf("Logging.Path = %q, want %q", configuration.Logging.Path, want)
	}
}

func TestLoadRejectsLegacyConfiguration(t *testing.T) {
	path := writeTempConfig(t, `{"recipients": {"ops": {"email": "ops@example.com"}}, "channels": {}}`)

	_, err := config.Load(path)
	assertDiagnosticCategory(t, err, diagnostics.CategoryInvalidConfig)
	if !strings.Contains(err.Error(), "legacy configuration") {
		t.Fatalf("error = %q, want legacy configuration context", err.Error())
	}
}

func TestLoadRejectsMissingCatchAllAndBadReferences(t *testing.T) {
	path := writeTempConfig(t, strings.Replace(minimalConfigJSON(), `"catch_all": {"deliveries": [{"account": "smtp-main", "destination": "ops-email"}]}`, `"catch_all": {}`, 1))
	_, err := config.Load(path)
	assertDiagnosticCategory(t, err, diagnostics.CategoryInvalidConfig)

	path = writeTempConfig(t, strings.Replace(minimalConfigJSON(), `"destination": "ops-email"`, `"destination": "missing"`, 1))
	_, err = config.Load(path)
	assertDiagnosticCategory(t, err, diagnostics.CategoryInvalidConfig)
}

func TestLoadRejectsTelegramThreadWithoutMessageThreadID(t *testing.T) {
	path := writeTempConfig(t, `{
		"destinations": {
			"ops-thread": {
				"type": "telegram",
				"telegram_delivery_mode": "thread",
				"telegram_topic_group_chat_id": "-1001234567890"
			}
		},
		"delivery_accounts": {
			"telegram-main": {
				"type": "telegram",
				"settings": {"parse_mode": "HTML"},
				"secrets": {"token": "123456:ABCDEF"},
				"attachments": "unsupported"
			}
		},
		"catch_all": {"deliveries": [{"account": "telegram-main", "destination": "ops-thread"}]}
	}`)

	_, err := config.Load(path)
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
	path := writeTempConfig(t, `{"destinations":`)

	_, err := config.Load(path)
	assertDiagnosticCategory(t, err, diagnostics.CategoryInvalidConfig)
}

func minimalConfigJSON() string {
	return `{
		"destinations": {
			"ops-email": {"type": "email", "email": "ops@example.com"}
		},
		"delivery_accounts": {
			"smtp-main": {
				"type": "email",
				"settings": {"from": "noticli@example.com", "host": "smtp.example.com", "port": "587"},
				"secrets": {"smtp_password": "secret"},
				"attachments": "supported"
			}
		},
		"catch_all": {"deliveries": [{"account": "smtp-main", "destination": "ops-email"}]}
	}`
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
