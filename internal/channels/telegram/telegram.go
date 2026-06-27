package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/rodrigogml/NotiCLI/internal/diagnostics"
	"github.com/rodrigogml/NotiCLI/internal/notify"
)

const (
	defaultBaseURL = "https://api.telegram.org"

	settingParseMode = "parse_mode"

	secretToken = "token"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Sender struct {
	client  HTTPClient
	baseURL string
}

func NewSender(client HTTPClient) Sender {
	return Sender{
		client:  client,
		baseURL: defaultBaseURL,
	}
}

func (Sender) Name() string {
	return notify.ChannelTelegram
}

func (s Sender) Send(ctx context.Context, request notify.Request, recipient notify.Recipient, config notify.ChannelConfig) (notify.Result, error) {
	if len(request.Attachments) > 0 {
		err := diagnostics.ForChannel(diagnostics.CategoryAttachmentError, notify.ChannelTelegram, "attachments are not supported for channel")
		return notify.FailureResult(err.Category, err.Channel, err.Message), err
	}
	message, err := buildMessage(request, recipient, config)
	if err != nil {
		return notify.FailureResult(diagnostics.CategoryInvalidConfig, notify.ChannelTelegram, diagnostics.FromError(err).Message), err
	}

	client := s.client
	if client == nil {
		client = http.DefaultClient
	}
	baseURL := strings.TrimRight(s.baseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	if err := sendMessage(ctx, client, baseURL, message); err != nil {
		diagnostic := diagnostics.ForChannel(diagnostics.CategoryDeliveryFailure, notify.ChannelTelegram, err.Error())
		return notify.FailureResult(diagnostic.Category, diagnostic.Channel, diagnostic.Message), diagnostic
	}

	return notify.SuccessResult(notify.ChannelTelegram, "telegram accepted"), nil
}

type Message struct {
	Token     string
	ChatID    string
	Text      string
	ParseMode string
}

func buildMessage(request notify.Request, recipient notify.Recipient, config notify.ChannelConfig) (Message, error) {
	if config.Type != notify.ChannelTelegram {
		return Message{}, invalidConfig("channel config type must be telegram")
	}
	token := strings.TrimSpace(config.Secrets[secretToken])
	if token == "" {
		return Message{}, invalidConfig(fmt.Sprintf("required secret %q is missing", secretToken))
	}
	chatID, ok := recipient.DestinationFor(notify.ChannelTelegram)
	if !ok {
		return Message{}, invalidConfig("recipient has no telegram destination")
	}

	return Message{
		Token:     token,
		ChatID:    chatID,
		Text:      formatText(request),
		ParseMode: strings.TrimSpace(config.Settings[settingParseMode]),
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
	return title + "\n\n" + body
}

func sendMessage(ctx context.Context, client HTTPClient, baseURL string, message Message) error {
	body := map[string]string{
		"chat_id": message.ChatID,
		"text":    message.Text,
	}
	if message.ParseMode != "" {
		body["parse_mode"] = message.ParseMode
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("provider request could not be encoded")
	}

	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", baseURL, message.Token)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("provider request could not be created")
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("provider request failed")
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("provider returned HTTP status %d", response.StatusCode)
	}

	var result struct {
		OK bool `json:"ok"`
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("provider response could not be read")
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("provider returned invalid response")
	}
	if !result.OK {
		return fmt.Errorf("provider rejected telegram request")
	}
	return nil
}

func invalidConfig(message string) error {
	return diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, notify.ChannelTelegram, message)
}
