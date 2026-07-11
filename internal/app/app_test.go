package app_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rodrigogml/NotiCLI/internal/app"
	"github.com/rodrigogml/NotiCLI/internal/diagnostics"
	"github.com/rodrigogml/NotiCLI/internal/notify"
)

func TestNotifyDispatchesAllResolvedDeliveries(t *testing.T) {
	emailSender := &fakeSender{name: notify.ChannelEmail, result: notify.SuccessResult(notify.ChannelEmail, "email accepted")}
	slackSender := &fakeSender{name: notify.ChannelSlack, result: notify.SuccessResult(notify.ChannelSlack, "slack accepted")}
	service := app.New(validConfiguration(t), emailSender, slackSender)

	request := validRequest()
	request.Category = "backup"
	request.Priority = notify.PriorityHigh
	result, err := service.Notify(context.Background(), request)
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if !result.Success || result.Category != notify.ResultSuccess {
		t.Fatalf("result = %#v, want success", result)
	}
	if emailSender.calls != 1 || slackSender.calls != 1 {
		t.Fatalf("calls email=%d slack=%d, want 1 each", emailSender.calls, slackSender.calls)
	}
	if emailSender.delivery.DestinationID != "ops-email" || slackSender.delivery.DestinationID != "ops-slack" {
		t.Fatalf("deliveries email=%#v slack=%#v", emailSender.delivery, slackSender.delivery)
	}
}

func TestNotifyUsesCatchAllWhenNoRouteMatches(t *testing.T) {
	emailSender := &fakeSender{name: notify.ChannelEmail, result: notify.SuccessResult(notify.ChannelEmail, "email accepted")}
	service := app.New(validConfiguration(t), emailSender)

	result, err := service.Notify(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("result = %#v, want success", result)
	}
	if emailSender.calls != 1 || emailSender.delivery.RouteID != "catch_all" {
		t.Fatalf("email calls=%d delivery=%#v", emailSender.calls, emailSender.delivery)
	}
}

func TestNotifyValidatesAttachmentsAndStripsUnsupportedDestinations(t *testing.T) {
	emailSender := &fakeSender{name: notify.ChannelEmail, result: notify.SuccessResult(notify.ChannelEmail, "email accepted")}
	slackSender := &fakeSender{name: notify.ChannelSlack, result: notify.SuccessResult(notify.ChannelSlack, "slack accepted")}
	service := app.New(validConfiguration(t), emailSender, slackSender)
	request := validRequest()
	request.Category = "backup"
	request.Priority = notify.PriorityHigh
	request.Attachments = []notify.Attachment{{Path: writeTempFile(t, "report.txt", "plain text")}}

	result, err := service.Notify(context.Background(), request)
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("result = %#v, want success", result)
	}
	if len(emailSender.request.Attachments) != 1 {
		t.Fatalf("email attachments = %d, want 1", len(emailSender.request.Attachments))
	}
	if len(slackSender.request.Attachments) != 0 {
		t.Fatalf("slack attachments = %d, want stripped", len(slackSender.request.Attachments))
	}
	logData := readLog(t, serviceLogPath(t))
	if !strings.Contains(logData, "attachments omitted") {
		t.Fatalf("log = %q, want attachment omission", logData)
	}
}

func TestNotifyAttemptsAllDeliveriesAndReturnsPartialFailure(t *testing.T) {
	wantErr := diagnostics.ForChannel(diagnostics.CategoryDeliveryFailure, notify.ChannelEmail, "provider rejected password=super-secret")
	emailSender := &fakeSender{name: notify.ChannelEmail, err: wantErr}
	slackSender := &fakeSender{name: notify.ChannelSlack, result: notify.SuccessResult(notify.ChannelSlack, "slack accepted")}
	service := app.New(validConfiguration(t), emailSender, slackSender)
	request := validRequest()
	request.Category = "backup"
	request.Priority = notify.PriorityHigh

	result, err := service.Notify(context.Background(), request)
	if err == nil {
		t.Fatal("Notify() error = nil, want partial delivery failure")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryDeliveryFailure)
	if result.Success || result.Category != diagnostics.CategoryDeliveryFailure {
		t.Fatalf("result = %#v, want delivery_failure", result)
	}
	if emailSender.calls != 1 || slackSender.calls != 1 {
		t.Fatalf("calls email=%d slack=%d, want all attempted", emailSender.calls, slackSender.calls)
	}
	logData := readLog(t, serviceLogPath(t))
	if strings.Contains(logData, "super-secret") {
		t.Fatalf("log leaked secret: %q", logData)
	}
	if !strings.Contains(logData, diagnostics.Redacted) {
		t.Fatalf("log = %q, want redacted value", logData)
	}
}

func TestNotifyReturnsValidationFailureBeforeDispatch(t *testing.T) {
	sender := &fakeSender{name: notify.ChannelEmail}
	service := app.New(validConfiguration(t), sender)
	request := validRequest()
	request.SenderSystem = ""

	result, err := service.Notify(context.Background(), request)
	if err == nil {
		t.Fatal("Notify() error = nil, want validation error")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryInvalidInput)
	if sender.calls != 0 {
		t.Fatalf("sender calls = %d, want 0", sender.calls)
	}
	if result.Success || result.Category != diagnostics.CategoryInvalidInput {
		t.Fatalf("result = %#v, want invalid_input failure", result)
	}
}

func TestNotifyRejectsMissingSenderRegistration(t *testing.T) {
	service := app.New(validConfiguration(t))

	result, err := service.Notify(context.Background(), validRequest())
	if err == nil {
		t.Fatal("Notify() error = nil, want registration error")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryInternalError)
	if result.Success || result.Category != diagnostics.CategoryInternalError {
		t.Fatalf("result = %#v, want internal_error failure", result)
	}
}

type fakeSender struct {
	name     string
	calls    int
	result   notify.Result
	err      error
	request  notify.Request
	delivery notify.ResolvedDelivery
}

func (f *fakeSender) Name() string {
	return f.name
}

func (f *fakeSender) Send(_ context.Context, request notify.Request, delivery notify.ResolvedDelivery) (notify.Result, error) {
	f.calls++
	f.request = request
	f.delivery = delivery
	return f.result, f.err
}

func validRequest() notify.Request {
	return notify.Request{
		SenderSystem: "BackupJob",
		Title:        "Backup failed",
		Message:      "Nightly backup failed",
	}
}

func validConfiguration(t *testing.T) notify.Configuration {
	lastLogPath = filepath.Join(t.TempDir(), "noticli.delivery.log")
	return notify.Configuration{
		Destinations: map[string]notify.Destination{
			"ops-email": {ID: "ops-email", Type: notify.ChannelEmail, Email: "ops@example.com", Enabled: true},
			"ops-slack": {ID: "ops-slack", Type: notify.ChannelSlack, SlackDest: "#ops", Enabled: true},
		},
		DeliveryAccounts: map[string]notify.DeliveryAccount{
			"smtp-main": {
				ID:               "smtp-main",
				Type:             notify.ChannelEmail,
				Enabled:          true,
				Settings:         map[string]string{"from": "noticli@example.com", "host": "smtp.example.com", "port": "587"},
				Secrets:          map[string]string{"smtp_password": "super-secret"},
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
		Logging:  notify.LoggingConfig{Path: lastLogPath},
	}
}

var lastLogPath string

func serviceLogPath(t *testing.T) string {
	t.Helper()
	if lastLogPath == "" {
		t.Fatal("log path was not captured")
	}
	return lastLogPath
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(data)
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
