package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/rodrigogml/NotiCLI/internal/diagnostics"
	"github.com/rodrigogml/NotiCLI/internal/notify"
)

const (
	secretWebhookURL = "webhook_url"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Sender struct {
	client HTTPClient
}

func NewSender(client HTTPClient) Sender {
	return Sender{client: client}
}

func (Sender) Name() string {
	return notify.ChannelSlack
}

func (s Sender) Send(ctx context.Context, request notify.Request, delivery notify.ResolvedDelivery) (notify.Result, error) {
	if len(request.Attachments) > 0 {
		err := diagnostics.ForChannel(diagnostics.CategoryAttachmentError, notify.ChannelSlack, "attachments are not supported for channel")
		return notify.FailureResult(err.Category, err.Channel, err.Message), err
	}
	message, err := buildMessage(request, delivery.Destination, delivery.Account)
	if err != nil {
		return notify.FailureResult(diagnostics.CategoryInvalidConfig, notify.ChannelSlack, diagnostics.FromError(err).Message), err
	}

	client := s.client
	if client == nil {
		client = http.DefaultClient
	}
	if err := sendWebhook(ctx, client, message); err != nil {
		diagnostic := diagnostics.ForChannel(diagnostics.CategoryDeliveryFailure, notify.ChannelSlack, err.Error())
		return notify.FailureResult(diagnostic.Category, diagnostic.Channel, diagnostic.Message), diagnostic
	}

	return notify.SuccessResult(notify.ChannelSlack, "slack accepted"), nil
}

type Message struct {
	WebhookURL string
	Dest       string
	Text       string
}

func buildMessage(request notify.Request, destination notify.Destination, config notify.DeliveryAccount) (Message, error) {
	if config.Type != notify.ChannelSlack {
		return Message{}, invalidConfig("channel config type must be slack")
	}
	webhookURL := strings.TrimSpace(config.Secrets[secretWebhookURL])
	if webhookURL == "" {
		return Message{}, invalidConfig(fmt.Sprintf("required secret %q is missing", secretWebhookURL))
	}
	parsed, err := url.Parse(webhookURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return Message{}, invalidConfig(fmt.Sprintf("required secret %q must be a URL", secretWebhookURL))
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return Message{}, invalidConfig(fmt.Sprintf("required secret %q must be an HTTP URL", secretWebhookURL))
	}

	dest, ok := destination.Address()
	if !ok {
		return Message{}, invalidConfig("destination has no slack destination")
	}

	return Message{
		WebhookURL: webhookURL,
		Dest:       dest,
		Text:       formatText(request),
	}, nil
}

func formatText(request notify.Request) string {
	title := strings.TrimSpace(request.Title)
	body := strings.TrimSpace(request.Message)
	if title == "" {
		return body
	}
	if body == "" {
		return title
	}
	return "*" + title + "*\n" + body
}

func sendWebhook(ctx context.Context, client HTTPClient, message Message) error {
	payload := map[string]string{
		"text":    message.Text,
		"channel": message.Dest,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("provider request could not be encoded")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, message.WebhookURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("provider request could not be created")
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("provider request failed")
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("provider response could not be read")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("provider returned HTTP status %d", response.StatusCode)
	}
	if strings.TrimSpace(string(body)) != "ok" {
		return fmt.Errorf("provider rejected slack request")
	}
	return nil
}

func invalidConfig(message string) error {
	return diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, notify.ChannelSlack, message)
}
