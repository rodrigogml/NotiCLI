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

func TestRecipientTelegramDeliveryModeDefaultsToPrivate(t *testing.T) {
	recipient := notify.Recipient{
		ID:             "ops",
		TelegramChatID: "12345",
		Enabled:        true,
	}

	if got := recipient.EffectiveTelegramDeliveryMode(); got != notify.TelegramDeliveryModePrivate {
		t.Fatalf("EffectiveTelegramDeliveryMode() = %q, want %q", got, notify.TelegramDeliveryModePrivate)
	}
	got, ok := recipient.DestinationFor(notify.ChannelTelegram)
	if !ok {
		t.Fatal("DestinationFor(telegram) ok = false")
	}
	if got != "12345" {
		t.Fatalf("DestinationFor(telegram) = %q, want private chat ID", got)
	}
	if err := recipient.ValidateForChannel(notify.ChannelTelegram); err != nil {
		t.Fatalf("ValidateForChannel(telegram) error = %v", err)
	}
}

func TestRecipientTelegramDeliveryModeAcceptsTopicsDestination(t *testing.T) {
	recipient := notify.Recipient{
		ID:                       "ops",
		TelegramDeliveryMode:     notify.TelegramDeliveryModeTopics,
		TelegramTopicGroupChatID: "-1001234567890",
		TelegramTopicGroupName:   "NotiCLI",
		Enabled:                  true,
	}

	if got := recipient.EffectiveTelegramDeliveryMode(); got != notify.TelegramDeliveryModeTopics {
		t.Fatalf("EffectiveTelegramDeliveryMode() = %q, want %q", got, notify.TelegramDeliveryModeTopics)
	}
	got, ok := recipient.DestinationFor(notify.ChannelTelegram)
	if !ok {
		t.Fatal("DestinationFor(telegram) ok = false")
	}
	if got != "-1001234567890" {
		t.Fatalf("DestinationFor(telegram) = %q, want topic group chat ID", got)
	}
	if err := recipient.ValidateForChannel(notify.ChannelTelegram); err != nil {
		t.Fatalf("ValidateForChannel(telegram) error = %v", err)
	}
}

func TestRecipientTelegramValidationRejectsUnsupportedDeliveryMode(t *testing.T) {
	recipient := notify.Recipient{
		ID:                   "ops",
		TelegramDeliveryMode: "broadcast",
		TelegramChatID:       "12345",
		Enabled:              true,
	}

	assertDiagnosticCategory(t, recipient.Validate(), diagnostics.CategoryInvalidConfig)
	assertDiagnosticCategory(t, recipient.ValidateForChannel(notify.ChannelTelegram), diagnostics.CategoryInvalidConfig)
}

func TestRecipientTelegramValidationRequiresDestinationForSelectedMode(t *testing.T) {
	recipient := notify.Recipient{
		ID:      "ops",
		Enabled: true,
	}
	assertDiagnosticCategory(t, recipient.ValidateForChannel(notify.ChannelTelegram), diagnostics.CategoryInvalidConfig)

	recipient = notify.Recipient{
		ID:                   "ops",
		TelegramDeliveryMode: notify.TelegramDeliveryModePrivate,
		Enabled:              true,
	}
	assertDiagnosticCategory(t, recipient.ValidateForChannel(notify.ChannelTelegram), diagnostics.CategoryInvalidConfig)

	recipient = notify.Recipient{
		ID:                   "ops",
		TelegramDeliveryMode: notify.TelegramDeliveryModeTopics,
		Enabled:              true,
	}
	assertDiagnosticCategory(t, recipient.ValidateForChannel(notify.ChannelTelegram), diagnostics.CategoryInvalidConfig)
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
	attachment := notify.Attachment{Path: filepath.Join("tmp", "report.txt")}
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

func TestValidateAttachmentsAcceptsRelativeAndAbsolutePaths(t *testing.T) {
	baseDir := t.TempDir()
	absolute := filepath.Join(baseDir, "absolute.txt")
	if err := os.WriteFile(absolute, []byte("absolute"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	relativeDir := filepath.Join(baseDir, "relative")
	if err := os.Mkdir(relativeDir, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	relative := filepath.Join("relative", "report.txt")
	if err := os.WriteFile(filepath.Join(baseDir, relative), []byte("relative"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	originalWorkingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(baseDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		os.Chdir(originalWorkingDir)
	})

	request := validRequest()
	request.Attachments = []notify.Attachment{
		{Path: absolute},
		{Path: relative},
	}
	attachments, err := notify.ValidateAttachments(request, validConfiguration().Channels[notify.ChannelEmail])
	if err != nil {
		t.Fatalf("ValidateAttachments() error = %v", err)
	}
	if attachments[0].Path != absolute {
		t.Fatalf("absolute path = %q, want %q", attachments[0].Path, absolute)
	}
	if attachments[1].Path != relative {
		t.Fatalf("relative path = %q, want %q", attachments[1].Path, relative)
	}
	if attachments[1].Filename != "report.txt" {
		t.Fatalf("relative filename = %q, want report.txt", attachments[1].Filename)
	}
}

func TestValidateAttachmentsEnrichesMultipleReadableFiles(t *testing.T) {
	first := writeTempFile(t, "report.txt", "plain text")
	second := writeTempFile(t, "data.json", `{"ok":true}`)
	request := validRequest()
	request.Attachments = []notify.Attachment{
		{Path: first},
		{Path: second, Filename: "custom.json"},
	}

	attachments, err := notify.ValidateAttachments(request, validConfiguration().Channels[notify.ChannelEmail])
	if err != nil {
		t.Fatalf("ValidateAttachments() error = %v", err)
	}
	if len(attachments) != 2 {
		t.Fatalf("attachments length = %d, want 2", len(attachments))
	}
	if attachments[0].Filename != "report.txt" {
		t.Fatalf("Filename = %q, want report.txt", attachments[0].Filename)
	}
	if attachments[0].Size == 0 {
		t.Fatal("Size = 0, want file size")
	}
	if attachments[0].ContentType == "" {
		t.Fatal("ContentType is empty")
	}
	if attachments[1].Filename != "custom.json" {
		t.Fatalf("Filename = %q, want custom.json", attachments[1].Filename)
	}
}

func TestValidateAttachmentsRejectsMissingFileAndDirectory(t *testing.T) {
	request := validRequest()
	request.Attachments = []notify.Attachment{{Path: filepath.Join(t.TempDir(), "missing.txt")}}

	_, err := notify.ValidateAttachments(request, validConfiguration().Channels[notify.ChannelEmail])
	assertDiagnosticCategory(t, err, diagnostics.CategoryAttachmentError)

	request.Attachments = []notify.Attachment{{Path: t.TempDir()}}
	_, err = notify.ValidateAttachments(request, validConfiguration().Channels[notify.ChannelEmail])
	assertDiagnosticCategory(t, err, diagnostics.CategoryAttachmentError)
}

func TestValidateAttachmentsRejectsUnreadableFile(t *testing.T) {
	path := writeTempFile(t, "blocked.txt", "secret")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	t.Cleanup(func() {
		os.Chmod(path, 0o600)
	})

	request := validRequest()
	request.Attachments = []notify.Attachment{{Path: path}}
	_, err := notify.ValidateAttachments(request, validConfiguration().Channels[notify.ChannelEmail])
	assertDiagnosticCategory(t, err, diagnostics.CategoryAttachmentError)
}

func TestValidateAttachmentsRejectsUnsupportedChannelPolicy(t *testing.T) {
	request := validRequest()
	request.Channel = notify.ChannelSlack
	request.Attachments = []notify.Attachment{{Path: writeTempFile(t, "report.txt", "plain text")}}
	channel := notify.ChannelConfig{
		Type:             notify.ChannelSlack,
		Enabled:          true,
		Settings:         map[string]string{"workspace": "ops"},
		Secrets:          map[string]string{"webhook_url": "secret"},
		AttachmentPolicy: notify.AttachmentPolicyUnsupported,
	}

	_, err := notify.ValidateAttachments(request, channel)
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

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
