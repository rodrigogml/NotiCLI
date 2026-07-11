package notify_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rodrigogml/NotiCLI/internal/channels/email"
	"github.com/rodrigogml/NotiCLI/internal/channels/slack"
	"github.com/rodrigogml/NotiCLI/internal/channels/telegram"
	"github.com/rodrigogml/NotiCLI/internal/diagnostics"
	"github.com/rodrigogml/NotiCLI/internal/notify"
)

func TestChannelSendersExposeStableNames(t *testing.T) {
	tests := []struct {
		name   string
		sender notify.ChannelSender
		want   string
	}{
		{name: "email", sender: email.Sender{}, want: notify.ChannelEmail},
		{name: "slack", sender: slack.Sender{}, want: notify.ChannelSlack},
		{name: "telegram", sender: telegram.Sender{}, want: notify.ChannelTelegram},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sender.Name(); got != tt.want {
				t.Fatalf("Name() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRequestValidateRequiresCoreFieldsAndValidPriority(t *testing.T) {
	request := validRequest()
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if request.EffectivePriority() != notify.PriorityNormal {
		t.Fatalf("EffectivePriority() = %q, want NORMAL", request.EffectivePriority())
	}

	request.Priority = "URGENT"
	assertDiagnosticCategory(t, request.Validate(), diagnostics.CategoryInvalidInput)

	request = validRequest()
	request.Title = ""
	assertDiagnosticCategory(t, request.Validate(), diagnostics.CategoryInvalidInput)
}

func TestConfigurationResolveMatchesMultipleRoutesAndCatchAll(t *testing.T) {
	config := validConfiguration()
	request := validRequest()
	request.Category = "backup"
	request.Priority = notify.PriorityHigh

	resolved, err := config.Resolve(request)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(resolved.Deliveries) != 2 {
		t.Fatalf("deliveries length = %d, want 2", len(resolved.Deliveries))
	}
	if resolved.Deliveries[0].RouteID != "backup-high" || resolved.Deliveries[1].DestinationID != "ops-slack" {
		t.Fatalf("deliveries = %#v", resolved.Deliveries)
	}

	request.Category = "deploy"
	request.Priority = notify.PriorityLow
	resolved, err = config.Resolve(request)
	if err != nil {
		t.Fatalf("Resolve() catch_all error = %v", err)
	}
	if len(resolved.Deliveries) != 1 || resolved.Deliveries[0].RouteID != "catch_all" {
		t.Fatalf("catch_all deliveries = %#v", resolved.Deliveries)
	}
}

func TestConfigurationValidateRejectsMissingCatchAllAndBadReferences(t *testing.T) {
	config := validConfiguration()
	config.CatchAll = notify.Route{}
	assertDiagnosticCategory(t, config.Validate(), diagnostics.CategoryInvalidConfig)

	config = validConfiguration()
	config.Routes[0].Deliveries[0].Destination = "missing"
	assertDiagnosticCategory(t, config.Validate(), diagnostics.CategoryInvalidConfig)

	config = validConfiguration()
	config.Routes[0].Deliveries[0].Account = "slack-main"
	assertDiagnosticCategory(t, config.Validate(), diagnostics.CategoryInvalidConfig)
}

func TestDestinationTelegramModes(t *testing.T) {
	destination := notify.Destination{
		ID:                       "ops-thread",
		Type:                     notify.ChannelTelegram,
		TelegramDeliveryMode:     notify.TelegramDeliveryModeThread,
		TelegramTopicGroupChatID: "-1001234567890",
		MessageThreadID:          42,
		Enabled:                  true,
	}
	if err := destination.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	destination.MessageThreadID = 0
	assertDiagnosticCategory(t, destination.Validate(), diagnostics.CategoryInvalidConfig)
}

func TestConfigurationSecretValuesCollectsAccountSecretValues(t *testing.T) {
	secrets := validConfiguration().SecretValues()
	if len(secrets) != 2 {
		t.Fatalf("SecretValues() = %#v, want 2 secrets", secrets)
	}
}

func TestValidateAttachmentsEnrichesReadableFiles(t *testing.T) {
	request := validRequest()
	request.Attachments = []notify.Attachment{{Path: writeTempFile(t, "report.txt", "plain text")}}

	attachments, err := notify.ValidateAttachments(request)
	if err != nil {
		t.Fatalf("ValidateAttachments() error = %v", err)
	}
	if len(attachments) != 1 || attachments[0].Filename != "report.txt" || attachments[0].ContentType == "" {
		t.Fatalf("attachments = %#v", attachments)
	}
}

func TestValidateAttachmentsRejectsMissingFileAndDirectory(t *testing.T) {
	request := validRequest()
	request.Attachments = []notify.Attachment{{Path: filepath.Join(t.TempDir(), "missing.txt")}}
	_, err := notify.ValidateAttachments(request)
	assertDiagnosticCategory(t, err, diagnostics.CategoryAttachmentError)

	request.Attachments = []notify.Attachment{{Path: t.TempDir()}}
	_, err = notify.ValidateAttachments(request)
	assertDiagnosticCategory(t, err, diagnostics.CategoryAttachmentError)
}

func TestDeliveryResultConstructorsSetStateAndExitCode(t *testing.T) {
	success := notify.SuccessResult(notify.ChannelEmail, "accepted")
	if !success.Success || success.State != notify.DeliveryStateSuccess || success.ExitCode != 0 {
		t.Fatalf("success result = %#v", success)
	}

	failure := notify.FailureResult(notify.ResultDeliveryFailure, notify.ChannelSlack, "rejected")
	if failure.Success || failure.State != notify.DeliveryStateFailure || failure.ExitCode != diagnostics.ExitDeliveryFailure {
		t.Fatalf("failure result = %#v", failure)
	}
	if !failure.Redacted {
		t.Fatal("failure.Redacted = false, want true")
	}
}

func validRequest() notify.Request {
	return notify.Request{
		SenderSystem: "BackupJob",
		Title:        "Backup failed",
		Message:      "Nightly backup failed",
	}
}

func validConfiguration() notify.Configuration {
	return notify.Configuration{
		Destinations: map[string]notify.Destination{
			"ops-email": {
				ID:      "ops-email",
				Type:    notify.ChannelEmail,
				Email:   "ops@example.com",
				Enabled: true,
			},
			"ops-slack": {
				ID:        "ops-slack",
				Type:      notify.ChannelSlack,
				SlackDest: "#ops",
				Enabled:   true,
			},
		},
		DeliveryAccounts: map[string]notify.DeliveryAccount{
			"smtp-main": {
				ID:               "smtp-main",
				Type:             notify.ChannelEmail,
				Enabled:          true,
				Settings:         map[string]string{"from": "noticli@example.com"},
				Secrets:          map[string]string{"smtp_password": "secret"},
				AttachmentPolicy: notify.AttachmentPolicySupported,
			},
			"slack-main": {
				ID:               "slack-main",
				Type:             notify.ChannelSlack,
				Enabled:          true,
				Settings:         map[string]string{"workspace": "ops"},
				Secrets:          map[string]string{"webhook_url": "https://hooks.slack.com/services/T/B/S"},
				AttachmentPolicy: notify.AttachmentPolicyUnsupported,
			},
		},
		Routes: []notify.Route{
			{
				ID:    "backup-high",
				Match: notify.RouteMatch{Senders: []string{"BackupJob"}, Categories: []string{"backup"}, Priorities: []string{notify.PriorityHigh}},
				Deliveries: []notify.Delivery{
					{Account: "smtp-main", Destination: "ops-email"},
					{Account: "slack-main", Destination: "ops-slack"},
				},
			},
		},
		CatchAll: notify.Route{ID: "catch_all", Deliveries: []notify.Delivery{{Account: "smtp-main", Destination: "ops-email"}}},
	}
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

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
