package app_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rodrigogml/NotiCLI/internal/app"
	"github.com/rodrigogml/NotiCLI/internal/diagnostics"
	"github.com/rodrigogml/NotiCLI/internal/notify"
)

func TestNotifyValidatesAndDispatchesResolvedRequest(t *testing.T) {
	sender := &fakeSender{
		result: notify.SuccessResult(notify.ChannelEmail, "accepted"),
	}
	service := app.New(validConfiguration(), sender)

	result, err := service.Notify(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if !result.Success || result.Category != notify.ResultSuccess {
		t.Fatalf("result = %#v, want success", result)
	}
	if sender.calls != 1 {
		t.Fatalf("sender calls = %d, want 1", sender.calls)
	}
	if sender.recipient.ID != "ops" {
		t.Fatalf("recipient ID = %q, want ops", sender.recipient.ID)
	}
	if sender.config.Type != notify.ChannelEmail {
		t.Fatalf("channel type = %q, want email", sender.config.Type)
	}
}

func TestNotifyValidatesAttachmentsBeforeDispatch(t *testing.T) {
	sender := &fakeSender{
		result: notify.SuccessResult(notify.ChannelEmail, "accepted"),
	}
	service := app.New(validConfiguration(), sender)
	request := validRequest()
	request.Attachments = []notify.Attachment{{Path: writeTempFile(t, "report.txt", "plain text")}}

	result, err := service.Notify(context.Background(), request)
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("result = %#v, want success", result)
	}
	if len(sender.request.Attachments) != 1 {
		t.Fatalf("sender attachments length = %d, want 1", len(sender.request.Attachments))
	}
	attachment := sender.request.Attachments[0]
	if attachment.Filename != "report.txt" {
		t.Fatalf("Filename = %q, want report.txt", attachment.Filename)
	}
	if attachment.Size == 0 {
		t.Fatal("Size = 0, want file size")
	}
	if attachment.ContentType == "" {
		t.Fatal("ContentType is empty")
	}
}

func TestNotifyReturnsValidationFailureBeforeDispatch(t *testing.T) {
	sender := &fakeSender{}
	service := app.New(validConfiguration(), sender)
	request := validRequest()
	request.RecipientID = "missing"

	result, err := service.Notify(context.Background(), request)
	if err == nil {
		t.Fatal("Notify() error = nil, want missing recipient error")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryMissingConfig)
	if sender.calls != 0 {
		t.Fatalf("sender calls = %d, want 0", sender.calls)
	}
	if result.Success || result.Category != diagnostics.CategoryMissingConfig {
		t.Fatalf("result = %#v, want missing_config failure", result)
	}
}

func TestNotifyReturnsSenderFailure(t *testing.T) {
	wantErr := diagnostics.ForChannel(diagnostics.CategoryDeliveryFailure, notify.ChannelEmail, "provider rejected request")
	sender := &fakeSender{err: wantErr}
	service := app.New(validConfiguration(), sender)

	result, err := service.Notify(context.Background(), validRequest())
	if err == nil {
		t.Fatal("Notify() error = nil, want delivery error")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryDeliveryFailure)
	if result.Success || result.Category != diagnostics.CategoryDeliveryFailure || result.Channel != notify.ChannelEmail {
		t.Fatalf("result = %#v, want delivery_failure for email", result)
	}
}

func TestNotifyRejectsMissingSenderRegistration(t *testing.T) {
	service := app.New(validConfiguration())

	result, err := service.Notify(context.Background(), validRequest())
	if err == nil {
		t.Fatal("Notify() error = nil, want registration error")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryInternalError)
	if result.Success || result.Category != diagnostics.CategoryInternalError {
		t.Fatalf("result = %#v, want internal_error failure", result)
	}
}

func TestNotifyRejectsInvalidAttachmentsBeforeDispatch(t *testing.T) {
	sender := &fakeSender{}
	service := app.New(validConfiguration(), sender)
	request := validRequest()
	request.Attachments = []notify.Attachment{{Path: filepath.Join(t.TempDir(), "missing.txt")}}

	result, err := service.Notify(context.Background(), request)
	if err == nil {
		t.Fatal("Notify() error = nil, want attachment error")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryAttachmentError)
	if sender.calls != 0 {
		t.Fatalf("sender calls = %d, want 0", sender.calls)
	}
	if result.Success || result.Category != diagnostics.CategoryAttachmentError {
		t.Fatalf("result = %#v, want attachment_error failure", result)
	}
}

type fakeSender struct {
	calls     int
	result    notify.Result
	err       error
	request   notify.Request
	recipient notify.Recipient
	config    notify.ChannelConfig
}

func (f *fakeSender) Name() string {
	return notify.ChannelEmail
}

func (f *fakeSender) Send(_ context.Context, request notify.Request, recipient notify.Recipient, config notify.ChannelConfig) (notify.Result, error) {
	f.calls++
	f.request = request
	f.recipient = recipient
	f.config = config
	return f.result, f.err
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
