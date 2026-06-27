package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rodrigogml/NotiCLI/internal/diagnostics"
	"github.com/rodrigogml/NotiCLI/internal/notify"
)

func TestSendPostsMessageToTelegramAPI(t *testing.T) {
	var gotPath string
	var gotPayload map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	sender := Sender{client: server.Client(), baseURL: server.URL}
	result, err := sender.Send(context.Background(), validRequest(), validRecipient(), validConfig())
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !result.Success || result.Category != diagnostics.CategorySuccess {
		t.Fatalf("result = %#v, want success", result)
	}
	if gotPath != "/bot123456:ABCDEF/sendMessage" {
		t.Fatalf("path = %q, want Telegram sendMessage path", gotPath)
	}
	if gotPayload["chat_id"] != "12345" {
		t.Fatalf("chat_id = %q, want 12345", gotPayload["chat_id"])
	}
	if gotPayload["text"] != "Backup failed\n\nNightly backup failed" {
		t.Fatalf("text = %q", gotPayload["text"])
	}
	if gotPayload["parse_mode"] != "HTML" {
		t.Fatalf("parse_mode = %q, want HTML", gotPayload["parse_mode"])
	}
}

func TestSendReturnsInvalidConfigForMissingTokenOrDestination(t *testing.T) {
	config := validConfig()
	delete(config.Secrets, secretToken)

	result, err := NewSender(nil).Send(context.Background(), validRequest(), validRecipient(), config)
	if err == nil {
		t.Fatal("Send() error = nil, want invalid_config")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryInvalidConfig)
	if result.Success || result.Category != diagnostics.CategoryInvalidConfig {
		t.Fatalf("result = %#v, want invalid_config", result)
	}

	recipient := validRecipient()
	recipient.TelegramChatID = ""
	_, err = NewSender(nil).Send(context.Background(), validRequest(), recipient, validConfig())
	assertDiagnosticCategory(t, err, diagnostics.CategoryInvalidConfig)
}

func TestSendMapsProviderHTTPFailureToDeliveryFailureWithoutLeakingToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"ok":false,"description":"bad token 123456:ABCDEF"}`))
	}))
	defer server.Close()

	sender := Sender{client: server.Client(), baseURL: server.URL}
	result, err := sender.Send(context.Background(), validRequest(), validRecipient(), validConfig())
	if err == nil {
		t.Fatal("Send() error = nil, want delivery_failure")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryDeliveryFailure)
	if result.Success || result.Category != diagnostics.CategoryDeliveryFailure {
		t.Fatalf("result = %#v, want delivery_failure", result)
	}
	if strings.Contains(result.Message, "123456:ABCDEF") || strings.Contains(err.Error(), "123456:ABCDEF") {
		t.Fatalf("telegram token leaked in result=%q err=%q", result.Message, err.Error())
	}
}

func TestSendMapsClientFailureToDeliveryFailureWithoutLeakingToken(t *testing.T) {
	sender := NewSender(failingClient{})

	result, err := sender.Send(context.Background(), validRequest(), validRecipient(), validConfig())
	if err == nil {
		t.Fatal("Send() error = nil, want delivery_failure")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryDeliveryFailure)
	if strings.Contains(result.Message, "123456:ABCDEF") || strings.Contains(err.Error(), "123456:ABCDEF") {
		t.Fatalf("telegram token leaked in result=%q err=%q", result.Message, err.Error())
	}
}

func TestSendMapsTelegramOKFalseToDeliveryFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":false}`))
	}))
	defer server.Close()

	sender := Sender{client: server.Client(), baseURL: server.URL}
	result, err := sender.Send(context.Background(), validRequest(), validRecipient(), validConfig())
	if err == nil {
		t.Fatal("Send() error = nil, want delivery_failure")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryDeliveryFailure)
	if result.Success || result.Category != diagnostics.CategoryDeliveryFailure {
		t.Fatalf("result = %#v, want delivery_failure", result)
	}
}

func TestSendReturnsAttachmentErrorWhenAttachmentsAreRequested(t *testing.T) {
	request := validRequest()
	request.Attachments = []notify.Attachment{{Path: filepath.Join("tmp", "report.txt"), Filename: "report.txt"}}

	result, err := NewSender(failingClient{}).Send(context.Background(), request, validRecipient(), validConfig())
	if err == nil {
		t.Fatal("Send() error = nil, want attachment_error")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryAttachmentError)
	if result.Success || result.Category != diagnostics.CategoryAttachmentError {
		t.Fatalf("result = %#v, want attachment_error", result)
	}
}

type failingClient struct{}

func (failingClient) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("dial https://api.telegram.org/bot123456:ABCDEF/sendMessage")
}

func validRequest() notify.Request {
	return notify.Request{
		SenderSystem: "BackupJob",
		RecipientID:  "ops",
		Channel:      notify.ChannelTelegram,
		Title:        "Backup failed",
		Message:      "Nightly backup failed",
	}
}

func validRecipient() notify.Recipient {
	return notify.Recipient{
		ID:             "ops",
		TelegramChatID: "12345",
		Enabled:        true,
	}
}

func validConfig() notify.ChannelConfig {
	return notify.ChannelConfig{
		Type:    notify.ChannelTelegram,
		Enabled: true,
		Settings: map[string]string{
			settingParseMode: "HTML",
		},
		Secrets: map[string]string{
			secretToken: "123456:ABCDEF",
		},
		AttachmentPolicy: notify.AttachmentPolicyLimited,
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
