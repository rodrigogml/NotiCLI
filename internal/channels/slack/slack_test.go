package slack

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

func TestSendPostsMessageToSlackWebhook(t *testing.T) {
	var gotPayload map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	config := validConfig()
	config.Secrets[secretWebhookURL] = server.URL

	result, err := NewSender(server.Client()).Send(context.Background(), validRequest(), validDelivery(config))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !result.Success || result.Category != diagnostics.CategorySuccess {
		t.Fatalf("result = %#v, want success", result)
	}
	if gotPayload["channel"] != "#ops" {
		t.Fatalf("channel = %q, want #ops", gotPayload["channel"])
	}
	if gotPayload["text"] != "*Deploy complete*\nRelease finished" {
		t.Fatalf("text = %q", gotPayload["text"])
	}
}

func TestSendReturnsInvalidConfigForMissingWebhookOrDestination(t *testing.T) {
	config := validConfig()
	delete(config.Secrets, secretWebhookURL)

	result, err := NewSender(nil).Send(context.Background(), validRequest(), validDelivery(config))
	if err == nil {
		t.Fatal("Send() error = nil, want invalid_config")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryInvalidConfig)
	if result.Success || result.Category != diagnostics.CategoryInvalidConfig {
		t.Fatalf("result = %#v, want invalid_config", result)
	}

	recipient := validRecipient()
	recipient.SlackDest = ""
	_, err = NewSender(nil).Send(context.Background(), validRequest(), validDeliveryWithDestination(recipient))
	assertDiagnosticCategory(t, err, diagnostics.CategoryInvalidConfig)
}

func TestSendReturnsInvalidConfigForInvalidWebhookURL(t *testing.T) {
	config := validConfig()
	config.Secrets[secretWebhookURL] = "not a url"

	_, err := NewSender(nil).Send(context.Background(), validRequest(), validDelivery(config))
	assertDiagnosticCategory(t, err, diagnostics.CategoryInvalidConfig)
}

func TestSendMapsProviderHTTPFailureToDeliveryFailureWithoutLeakingWebhook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("invalid webhook https://hooks.slack.com/services/T000/B000/secret"))
	}))
	defer server.Close()

	config := validConfig()
	config.Secrets[secretWebhookURL] = server.URL + "/services/T000/B000/secret"

	result, err := NewSender(server.Client()).Send(context.Background(), validRequest(), validDelivery(config))
	if err == nil {
		t.Fatal("Send() error = nil, want delivery_failure")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryDeliveryFailure)
	if result.Success || result.Category != diagnostics.CategoryDeliveryFailure {
		t.Fatalf("result = %#v, want delivery_failure", result)
	}
	if strings.Contains(result.Message, "T000/B000/secret") || strings.Contains(err.Error(), "T000/B000/secret") {
		t.Fatalf("webhook URL leaked in result=%q err=%q", result.Message, err.Error())
	}
}

func TestSendMapsProviderNonOKBodyToDeliveryFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid_payload"))
	}))
	defer server.Close()

	config := validConfig()
	config.Secrets[secretWebhookURL] = server.URL

	result, err := NewSender(server.Client()).Send(context.Background(), validRequest(), validDelivery(config))
	if err == nil {
		t.Fatal("Send() error = nil, want delivery_failure")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryDeliveryFailure)
	if result.Success || result.Category != diagnostics.CategoryDeliveryFailure {
		t.Fatalf("result = %#v, want delivery_failure", result)
	}
}

func TestSendMapsClientFailureToDeliveryFailureWithoutLeakingWebhook(t *testing.T) {
	result, err := NewSender(failingClient{}).Send(context.Background(), validRequest(), validDelivery(validConfig()))
	if err == nil {
		t.Fatal("Send() error = nil, want delivery_failure")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryDeliveryFailure)
	if strings.Contains(result.Message, "T000/B000/secret") || strings.Contains(err.Error(), "T000/B000/secret") {
		t.Fatalf("webhook URL leaked in result=%q err=%q", result.Message, err.Error())
	}
}

func TestSendReturnsAttachmentErrorWhenAttachmentsAreRequested(t *testing.T) {
	request := validRequest()
	request.Attachments = []notify.Attachment{{Path: filepath.Join("tmp", "report.txt"), Filename: "report.txt"}}

	result, err := NewSender(failingClient{}).Send(context.Background(), request, validDelivery(validConfig()))
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
	return nil, errors.New("dial https://hooks.slack.com/services/T000/B000/secret")
}

func validRequest() notify.Request {
	return notify.Request{
		SenderSystem: "DeployBot",
		Priority:     notify.PriorityNormal,
		Title:        "Deploy complete",
		Message:      "Release finished",
	}
}

func validRecipient() notify.Destination {
	return notify.Destination{
		ID:        "ops",
		Type:      notify.ChannelSlack,
		SlackDest: "#ops",
		Enabled:   true,
	}
}

func validConfig() notify.DeliveryAccount {
	return notify.DeliveryAccount{
		ID:      "slack-main",
		Type:    notify.ChannelSlack,
		Enabled: true,
		Settings: map[string]string{
			"workspace": "ops",
		},
		Secrets: map[string]string{
			secretWebhookURL: "https://hooks.slack.com/services/T000/B000/secret",
		},
		AttachmentPolicy: notify.AttachmentPolicyUnsupported,
	}
}

func validDelivery(account notify.DeliveryAccount) notify.ResolvedDelivery {
	return resolvedDelivery(validRecipient(), account)
}

func validDeliveryWithDestination(destination notify.Destination) notify.ResolvedDelivery {
	return resolvedDelivery(destination, validConfig())
}

func resolvedDelivery(destination notify.Destination, account notify.DeliveryAccount) notify.ResolvedDelivery {
	return notify.ResolvedDelivery{
		RouteID:       "deploy",
		AccountID:     account.ID,
		DestinationID: destination.ID,
		Account:       account,
		Destination:   destination,
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
