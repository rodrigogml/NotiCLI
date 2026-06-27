package config_test

import (
	"errors"
	"os"
	"path/filepath"
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
