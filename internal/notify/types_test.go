package notify_test

import (
	"errors"
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

func TestRequestValidateRequiresCoreFields(t *testing.T) {
	request := notify.Request{
		SenderSystem: "BackupJob",
		RecipientID:  "ops",
		Channel:      notify.ChannelEmail,
		Title:        "Backup failed",
		Message:      "Nightly backup failed",
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	request.Title = ""
	err := request.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want title error")
	}

	var diagnostic diagnostics.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error type = %T, want diagnostics.Diagnostic", err)
	}
	if diagnostic.Category != diagnostics.CategoryInvalidInput {
		t.Fatalf("Category = %q, want %q", diagnostic.Category, diagnostics.CategoryInvalidInput)
	}
}

func TestRequestValidateRequiresSenderSystem(t *testing.T) {
	request := notify.Request{
		RecipientID: "ops",
		Channel:     notify.ChannelEmail,
		Title:       "Backup failed",
		Message:     "Nightly backup failed",
	}
	if err := request.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want sender_system error")
	}
}

func TestRequestValidateRejectsLongSenderSystem(t *testing.T) {
	request := notify.Request{
		SenderSystem: "SystemNameLongerThan20",
		RecipientID:  "ops",
		Channel:      notify.ChannelEmail,
		Title:        "Backup failed",
		Message:      "Nightly backup failed",
	}
	if err := request.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want sender_system length error")
	}
}

func TestRequestValidateRejectsUnsupportedChannel(t *testing.T) {
	request := notify.Request{
		SenderSystem: "BackupJob",
		RecipientID:  "ops",
		Channel:      "sms",
		Title:        "Backup failed",
		Message:      "Nightly backup failed",
	}
	if err := request.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unsupported channel error")
	}
}

func TestRecipientDestinationForChannel(t *testing.T) {
	recipient := notify.Recipient{
		ID:             "ops",
		Email:          "ops@example.com",
		TelegramChatID: "12345",
		SlackDest:      "#ops",
		Enabled:        true,
	}

	tests := []struct {
		channel string
		want    string
	}{
		{channel: notify.ChannelEmail, want: "ops@example.com"},
		{channel: notify.ChannelTelegram, want: "12345"},
		{channel: notify.ChannelSlack, want: "#ops"},
	}

	for _, tt := range tests {
		t.Run(tt.channel, func(t *testing.T) {
			got, ok := recipient.DestinationFor(tt.channel)
			if !ok {
				t.Fatal("DestinationFor() ok = false")
			}
			if got != tt.want {
				t.Fatalf("DestinationFor() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfigurationValidateAllowsMultipleRecipientsAndGlobalChannels(t *testing.T) {
	config := notify.Configuration{
		Recipients: map[string]notify.Recipient{
			"ops": {
				Name:  "Operations",
				Email: "ops@example.com",
			},
			"dev": {
				ID:             "dev",
				TelegramChatID: "12345",
			},
		},
		Channels: map[string]notify.ChannelConfig{
			notify.ChannelEmail: {
				Settings:         map[string]string{"from": "noticli@example.com"},
				Secrets:          map[string]string{"smtp_password": "secret"},
				AttachmentPolicy: notify.AttachmentPolicySupported,
			},
			notify.ChannelTelegram: {
				Type:             notify.ChannelTelegram,
				Settings:         map[string]string{"parse_mode": "plain"},
				Secrets:          map[string]string{"token": "secret"},
				AttachmentPolicy: notify.AttachmentPolicyLimited,
			},
		},
		Defaults: map[string]string{"channel": notify.ChannelEmail},
	}

	if err := config.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigurationValidateRequiresRecipientsAndChannels(t *testing.T) {
	if err := (notify.Configuration{}).Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing recipients error")
	}

	config := notify.Configuration{
		Recipients: map[string]notify.Recipient{"ops": {ID: "ops"}},
	}
	if err := config.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing channels error")
	}
}

func TestConfigurationResolveReturnsRecipientChannelAndDestination(t *testing.T) {
	config := validConfiguration()
	request := validRequest()

	resolved, err := config.Resolve(request)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.Recipient.ID != "ops" {
		t.Fatalf("Recipient.ID = %q", resolved.Recipient.ID)
	}
	if resolved.Channel.Type != notify.ChannelEmail {
		t.Fatalf("Channel.Type = %q", resolved.Channel.Type)
	}
	if resolved.Destination != "ops@example.com" {
		t.Fatalf("Destination = %q", resolved.Destination)
	}
}

func TestConfigurationResolveRejectsUnknownRecipientAndChannel(t *testing.T) {
	config := validConfiguration()

	request := validRequest()
	request.RecipientID = "unknown"
	_, err := config.Resolve(request)
	assertDiagnosticCategory(t, err, diagnostics.CategoryMissingConfig)

	request = validRequest()
	request.Channel = notify.ChannelSlack
	_, err = config.Resolve(request)
	assertDiagnosticCategory(t, err, diagnostics.CategoryMissingConfig)
}

func TestConfigurationResolveRejectsDisabledRecipientAndMissingDestination(t *testing.T) {
	config := validConfiguration()
	config.Recipients["ops"] = notify.Recipient{
		ID:      "ops",
		Email:   "ops@example.com",
		Enabled: false,
	}
	_, err := config.Resolve(validRequest())
	assertDiagnosticCategory(t, err, diagnostics.CategoryInvalidConfig)

	config = validConfiguration()
	config.Recipients["ops"] = notify.Recipient{
		ID:      "ops",
		Enabled: true,
	}
	_, err = config.Resolve(validRequest())
	assertDiagnosticCategory(t, err, diagnostics.CategoryInvalidConfig)
}

func TestConfigurationResolveRejectsDisabledChannelAndMissingRequiredSecret(t *testing.T) {
	config := validConfiguration()
	channel := config.Channels[notify.ChannelEmail]
	channel.Enabled = false
	config.Channels[notify.ChannelEmail] = channel
	_, err := config.Resolve(validRequest())
	assertDiagnosticCategory(t, err, diagnostics.CategoryInvalidConfig)

	config = validConfiguration()
	channel = config.Channels[notify.ChannelEmail]
	channel.Secrets = map[string]string{"api_key": "secret"}
	config.Channels[notify.ChannelEmail] = channel
	_, err = config.Resolve(validRequest())
	assertDiagnosticCategory(t, err, diagnostics.CategoryInvalidConfig)
}

func TestConfigurationSecretValuesCollectsChannelSecretValues(t *testing.T) {
	config := validConfiguration()
	secrets := config.SecretValues()
	if len(secrets) != 1 || secrets[0] != "secret" {
		t.Fatalf("SecretValues() = %#v", secrets)
	}
}

func TestChannelConfigValidateRequiresSupportedTypeAndMaps(t *testing.T) {
	config := notify.ChannelConfig{
		Type:             notify.ChannelSlack,
		Enabled:          true,
		Settings:         map[string]string{"workspace": "ops"},
		Secrets:          map[string]string{"webhook_url": "secret"},
		AttachmentPolicy: notify.AttachmentPolicyUnsupported,
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	config.Secrets = nil
	if err := config.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing secrets error")
	}
}

func TestAttachmentValidateAndEffectiveFilename(t *testing.T) {
	attachment := notify.Attachment{Path: "/tmp/report.txt"}
	if err := attachment.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got := attachment.EffectiveFilename(); got != "report.txt" {
		t.Fatalf("EffectiveFilename() = %q, want report.txt", got)
	}

	if err := (notify.Attachment{}).Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing path error")
	}
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
		RecipientID:  "ops",
		Channel:      notify.ChannelEmail,
		Title:        "Backup failed",
		Message:      "Nightly backup failed",
	}
}

func validConfiguration() notify.Configuration {
	return notify.Configuration{
		Recipients: map[string]notify.Recipient{
			"ops": {
				ID:      "ops",
				Email:   "ops@example.com",
				Enabled: true,
			},
		},
		Channels: map[string]notify.ChannelConfig{
			notify.ChannelEmail: {
				Type:             notify.ChannelEmail,
				Enabled:          true,
				Settings:         map[string]string{"from": "noticli@example.com"},
				Secrets:          map[string]string{"smtp_password": "secret"},
				AttachmentPolicy: notify.AttachmentPolicySupported,
			},
		},
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
