package email

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rodrigogml/NotiCLI/internal/diagnostics"
	"github.com/rodrigogml/NotiCLI/internal/notify"
)

func TestSendBuildsMessageAndUsesTransport(t *testing.T) {
	transport := &fakeTransport{}
	sender := NewSender(transport)

	result, err := sender.Send(context.Background(), validRequest(), validDelivery(validConfig()))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !result.Success || result.Category != diagnostics.CategorySuccess {
		t.Fatalf("result = %#v, want success", result)
	}
	if transport.calls != 1 {
		t.Fatalf("transport calls = %d, want 1", transport.calls)
	}
	if transport.message.Host != "smtp.example.com" {
		t.Fatalf("Host = %q, want smtp.example.com", transport.message.Host)
	}
	if transport.message.Port != "587" {
		t.Fatalf("Port = %q, want 587", transport.message.Port)
	}
	if transport.message.From != "noticli@example.com" {
		t.Fatalf("From = %q, want noticli@example.com", transport.message.From)
	}
	if transport.message.FromName != "NotiCLI" {
		t.Fatalf("FromName = %q, want NotiCLI", transport.message.FromName)
	}
	if transport.message.To != "ops@example.com" {
		t.Fatalf("To = %q, want ops@example.com", transport.message.To)
	}
	if transport.message.Subject != "[BackupJob] Backup failed" {
		t.Fatalf("Subject = %q, want [BackupJob] Backup failed", transport.message.Subject)
	}
	if transport.message.Body != "Nightly backup failed" {
		t.Fatalf("Body = %q, want Nightly backup failed", transport.message.Body)
	}
	if transport.message.Username != "smtp-user" {
		t.Fatalf("Username = %q, want smtp-user", transport.message.Username)
	}
}

func TestSendIncludesAttachmentsInTransportMessage(t *testing.T) {
	transport := &fakeTransport{}
	request := validRequest()
	request.Attachments = []notify.Attachment{{
		Path:        writeTempFile(t, "report.txt", "plain text"),
		Filename:    "report.txt",
		Size:        10,
		ContentType: "text/plain; charset=utf-8",
	}}

	result, err := NewSender(transport).Send(context.Background(), request, validDelivery(validConfig()))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("result = %#v, want success", result)
	}
	if len(transport.message.Attachments) != 1 {
		t.Fatalf("attachments length = %d, want 1", len(transport.message.Attachments))
	}
	if transport.message.Attachments[0].Filename != "report.txt" {
		t.Fatalf("Filename = %q, want report.txt", transport.message.Attachments[0].Filename)
	}
}

func TestSendDefaultsUsernameToFromAddress(t *testing.T) {
	transport := &fakeTransport{}
	config := validConfig()
	delete(config.Settings, settingUsername)

	_, err := NewSender(transport).Send(context.Background(), validRequest(), validDelivery(config))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if transport.message.Username != "noticli@example.com" {
		t.Fatalf("Username = %q, want from address", transport.message.Username)
	}
}

func TestSendReturnsInvalidConfigForMissingEmailSettings(t *testing.T) {
	config := validConfig()
	delete(config.Settings, settingHost)

	result, err := NewSender(&fakeTransport{}).Send(context.Background(), validRequest(), validDelivery(config))
	if err == nil {
		t.Fatal("Send() error = nil, want invalid_config")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryInvalidConfig)
	if result.Success || result.Category != diagnostics.CategoryInvalidConfig {
		t.Fatalf("result = %#v, want invalid_config failure", result)
	}
}

func TestSendMapsTransportFailureToDeliveryFailure(t *testing.T) {
	transport := &fakeTransport{err: errors.New("smtp rejected credentials password=secret")}

	result, err := NewSender(transport).Send(context.Background(), validRequest(), validDelivery(validConfig()))
	if err == nil {
		t.Fatal("Send() error = nil, want delivery_failure")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryDeliveryFailure)
	if result.Success || result.Category != diagnostics.CategoryDeliveryFailure {
		t.Fatalf("result = %#v, want delivery_failure", result)
	}
	if !strings.Contains(result.Message, "provider rejected email") {
		t.Fatalf("Message = %q, want provider failure context", result.Message)
	}
}

func TestFormatPlainTextMessageIncludesHeadersAndBody(t *testing.T) {
	formatted := formatPlainTextMessage(Message{
		From:         "noticli@example.com",
		FromName:     "NotiCLI",
		To:           "ops@example.com",
		Subject:      "[BackupJob] Backup failed",
		Body:         "Nightly backup failed",
		SenderSystem: "BackupJob",
	})

	for _, want := range []string{
		"From: \"NotiCLI\" <noticli@example.com>\r\n",
		"To: ops@example.com\r\n",
		"Subject: [BackupJob] Backup failed\r\n",
		"X-NotiCLI-Sender: BackupJob\r\n",
		"\r\nNightly backup failed\r\n",
	} {
		if !strings.Contains(formatted, want) {
			t.Fatalf("formatted message missing %q in %q", want, formatted)
		}
	}
}

func TestFormatSMTPMessageIncludesAttachmentPart(t *testing.T) {
	data, err := formatSMTPMessage(Message{
		From:     "noticli@example.com",
		FromName: "NotiCLI",
		To:       "ops@example.com",
		Subject:  "[BackupJob] Backup failed",
		Body:     "Nightly backup failed",
		Attachments: []notify.Attachment{{
			Path:        writeTempFile(t, "report.txt", "plain text"),
			Filename:    "report.txt",
			ContentType: "text/plain; charset=utf-8",
		}},
	})
	if err != nil {
		t.Fatalf("formatSMTPMessage() error = %v", err)
	}
	formatted := string(data)
	for _, want := range []string{
		"From: \"NotiCLI\" <noticli@example.com>",
		"Subject: [BackupJob] Backup failed",
		"Content-Type: multipart/mixed;",
		"Content-Disposition: attachment; filename=\"report.txt\"",
		"Content-Transfer-Encoding: base64",
		"cGxhaW4gdGV4dA==",
	} {
		if !strings.Contains(formatted, want) {
			t.Fatalf("formatted message missing %q in %q", want, formatted)
		}
	}
}

type fakeTransport struct {
	calls   int
	message Message
	err     error
}

func (f *fakeTransport) Send(_ context.Context, message Message) error {
	f.calls++
	f.message = message
	return f.err
}

func validRequest() notify.Request {
	return notify.Request{
		SenderSystem: "BackupJob",
		Priority:     notify.PriorityNormal,
		Title:        "Backup failed",
		Message:      "Nightly backup failed",
	}
}

func validDestination() notify.Destination {
	return notify.Destination{
		ID:      "ops",
		Type:    notify.ChannelEmail,
		Email:   "ops@example.com",
		Enabled: true,
	}
}

func validConfig() notify.DeliveryAccount {
	return notify.DeliveryAccount{
		ID:      "smtp-main",
		Type:    notify.ChannelEmail,
		Enabled: true,
		Settings: map[string]string{
			settingFrom:     "noticli@example.com",
			settingFromName: "NotiCLI",
			settingHost:     "smtp.example.com",
			settingPort:     "587",
			settingUsername: "smtp-user",
		},
		Secrets: map[string]string{
			secretSMTPPassword: "secret",
		},
		AttachmentPolicy: notify.AttachmentPolicySupported,
	}
}

func validDelivery(account notify.DeliveryAccount) notify.ResolvedDelivery {
	return notify.ResolvedDelivery{
		RouteID:       "backup-high",
		AccountID:     account.ID,
		DestinationID: "ops-email",
		Account:       account,
		Destination:   validDestination(),
	}
}

func assertDiagnosticCategory(t *testing.T, err error, want diagnostics.Category) {
	t.Helper()

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
