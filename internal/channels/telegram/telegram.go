package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rodrigogml/NotiCLI/internal/diagnostics"
	"github.com/rodrigogml/NotiCLI/internal/notify"
	"github.com/rodrigogml/NotiCLI/internal/telegramtopics"
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
	client     HTTPClient
	baseURL    string
	topicStore TopicStore
	now        func() time.Time
}

type SenderOption func(*Sender)

type TopicStore interface {
	Load(ctx context.Context) (telegramtopics.State, error)
	PrepareForUpdate(ctx context.Context) error
	Update(ctx context.Context, mutate func(*telegramtopics.State) error) (telegramtopics.State, error)
}

func NewSender(client HTTPClient, options ...SenderOption) Sender {
	sender := Sender{
		client:  client,
		baseURL: defaultBaseURL,
		now:     func() time.Time { return time.Now().UTC() },
	}
	for _, option := range options {
		if option != nil {
			option(&sender)
		}
	}
	return sender
}

func NewSenderWithTopicStore(client HTTPClient, topicStore TopicStore) Sender {
	return NewSender(client, WithTopicStore(topicStore))
}

func WithTopicStore(topicStore TopicStore) SenderOption {
	return func(sender *Sender) {
		sender.topicStore = topicStore
	}
}

func WithBaseURL(baseURL string) SenderOption {
	return func(sender *Sender) {
		sender.baseURL = baseURL
	}
}

func (Sender) Name() string {
	return notify.ChannelTelegram
}

func (s Sender) Send(ctx context.Context, request notify.Request, delivery notify.ResolvedDelivery) (notify.Result, error) {
	if len(request.Attachments) > 0 {
		err := diagnostics.ForChannel(diagnostics.CategoryAttachmentError, notify.ChannelTelegram, "attachments are not supported for channel")
		return notify.FailureResult(err.Category, err.Channel, err.Message), err
	}
	if delivery.Destination.EffectiveTelegramDeliveryMode() == notify.TelegramDeliveryModeTopics {
		return s.sendTopicMessage(ctx, request, delivery)
	}
	message, err := buildMessage(request, delivery.Destination, delivery.Account)
	if err != nil {
		return notify.FailureResult(diagnostics.CategoryInvalidConfig, notify.ChannelTelegram, diagnostics.FromError(err).Message), err
	}
	if delivery.Destination.EffectiveTelegramDeliveryMode() == notify.TelegramDeliveryModeThread {
		message.MessageThreadID = delivery.Destination.MessageThreadID
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

func (s Sender) sendTopicMessage(ctx context.Context, request notify.Request, delivery notify.ResolvedDelivery) (notify.Result, error) {
	destination := delivery.Destination
	message, err := buildMessage(request, destination, delivery.Account)
	if err != nil {
		return notify.FailureResult(diagnostics.CategoryInvalidConfig, notify.ChannelTelegram, diagnostics.FromError(err).Message), err
	}
	if s.topicStore == nil {
		err := invalidConfig("telegram topic state repository is not configured")
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

	state, err := s.topicStore.Load(ctx)
	if err != nil {
		diagnostic := diagnostics.ForChannel(diagnostics.CategoryInternalError, notify.ChannelTelegram, err.Error())
		return notify.FailureResult(diagnostic.Category, diagnostic.Channel, diagnostic.Message), diagnostic
	}
	association, ok := state.FindAssociation(destination.ID, message.ChatID, request.SenderSystem)
	knownAssociation := ok
	if !ok {
		association, err = s.createTopicAssociation(ctx, client, baseURL, request, destination, message, state)
		if err != nil {
			diagnostic := diagnostics.FromError(err)
			if diagnostic.Category == diagnostics.CategoryInternalError || diagnostic.Category == diagnostics.CategoryDeliveryFailure {
				return notify.FailureResult(diagnostic.Category, notify.ChannelTelegram, diagnostic.Message), diagnostic
			}
			return notify.FailureResult(diagnostics.CategoryInvalidConfig, notify.ChannelTelegram, diagnostic.Message), err
		}
	}
	message.MessageThreadID = association.MessageThreadID

	if err := sendMessage(ctx, client, baseURL, message); err != nil {
		if knownAssociation && isStaleTopicError(err) {
			replacement, recoveryErr := s.replaceTopicAssociation(ctx, client, baseURL, request, destination, message)
			if recoveryErr != nil {
				diagnostic := diagnostics.FromError(recoveryErr)
				return notify.FailureResult(diagnostic.Category, notify.ChannelTelegram, diagnostic.Message), diagnostic
			}
			message.MessageThreadID = replacement.MessageThreadID
			if retryErr := sendMessage(ctx, client, baseURL, message); retryErr != nil {
				diagnostic := diagnostics.ForChannel(diagnostics.CategoryDeliveryFailure, notify.ChannelTelegram, retryErr.Error())
				return notify.FailureResult(diagnostic.Category, diagnostic.Channel, diagnostic.Message), diagnostic
			}
		} else {
			diagnostic := diagnostics.ForChannel(diagnostics.CategoryDeliveryFailure, notify.ChannelTelegram, err.Error())
			return notify.FailureResult(diagnostic.Category, diagnostic.Channel, diagnostic.Message), diagnostic
		}
	}
	if _, err := s.topicStore.Update(ctx, func(state *telegramtopics.State) error {
		state.TouchAssociation(destination.ID, message.ChatID, request.SenderSystem, s.currentTime())
		return nil
	}); err != nil {
		diagnostic := diagnostics.ForChannel(diagnostics.CategoryInternalError, notify.ChannelTelegram, err.Error())
		return notify.FailureResult(diagnostic.Category, diagnostic.Channel, diagnostic.Message), diagnostic
	}

	return notify.SuccessResult(notify.ChannelTelegram, "telegram accepted"), nil
}

func (s Sender) createTopicAssociation(ctx context.Context, client HTTPClient, baseURL string, request notify.Request, destination notify.Destination, message Message, observedState telegramtopics.State) (telegramtopics.Association, error) {
	if err := s.topicStore.PrepareForUpdate(ctx); err != nil {
		return telegramtopics.Association{}, diagnostics.ForChannel(diagnostics.CategoryInternalError, notify.ChannelTelegram, err.Error())
	}

	var association telegramtopics.Association
	_, err := s.topicStore.Update(ctx, func(state *telegramtopics.State) error {
		if existing, ok := state.FindAssociation(destination.ID, message.ChatID, request.SenderSystem); ok {
			association = existing
			return nil
		}

		topicName, disambiguator := telegramtopics.TopicNameForSender(destination.ID, message.ChatID, request.SenderSystem, state.Associations)
		threadID, err := createForumTopic(ctx, client, baseURL, ForumTopic{
			Token:  message.Token,
			ChatID: message.ChatID,
			Name:   topicName,
		})
		if err != nil {
			return diagnostics.ForChannel(diagnostics.CategoryDeliveryFailure, notify.ChannelTelegram, err.Error())
		}

		now := s.currentTime()
		association = telegramtopics.Association{
			RecipientID:            destination.ID,
			ChatID:                 message.ChatID,
			Sender:                 request.SenderSystem,
			TopicName:              topicName,
			TopicNameDisambiguator: disambiguator,
			MessageThreadID:        threadID,
			CreatedByNotiCLI:       true,
			CreatedAt:              now,
			Status:                 telegramtopics.TopicStatusActive,
		}
		state.Associations = append(state.Associations, association)
		return nil
	})
	if err != nil {
		return telegramtopics.Association{}, err
	}
	if association.MessageThreadID <= 0 {
		if existing, ok := observedState.FindAssociation(destination.ID, message.ChatID, request.SenderSystem); ok {
			return existing, nil
		}
		return telegramtopics.Association{}, diagnostics.ForChannel(diagnostics.CategoryInternalError, notify.ChannelTelegram, "telegram topic association was not created")
	}
	return association, nil
}

func (s Sender) replaceTopicAssociation(ctx context.Context, client HTTPClient, baseURL string, request notify.Request, destination notify.Destination, message Message) (telegramtopics.Association, error) {
	if err := s.topicStore.PrepareForUpdate(ctx); err != nil {
		return telegramtopics.Association{}, diagnostics.ForChannel(diagnostics.CategoryInternalError, notify.ChannelTelegram, err.Error())
	}

	var replacement telegramtopics.Association
	_, err := s.topicStore.Update(ctx, func(state *telegramtopics.State) error {
		topicName, disambiguator := telegramtopics.TopicNameForSender(destination.ID, message.ChatID, request.SenderSystem, state.Associations)
		threadID, err := createForumTopic(ctx, client, baseURL, ForumTopic{
			Token:  message.Token,
			ChatID: message.ChatID,
			Name:   topicName,
		})
		if err != nil {
			return diagnostics.ForChannel(diagnostics.CategoryDeliveryFailure, notify.ChannelTelegram, err.Error())
		}

		now := s.currentTime()
		for index, association := range state.Associations {
			if association.Key() == telegramtopics.AssociationKey(destination.ID, message.ChatID, request.SenderSystem) {
				state.Associations[index].Status = telegramtopics.TopicStatusReplaced
				state.Associations[index].TopicName = topicName
				state.Associations[index].TopicNameDisambiguator = disambiguator
				state.Associations[index].MessageThreadID = threadID
				state.Associations[index].CreatedByNotiCLI = true
				state.Associations[index].CreatedAt = now
				state.Associations[index].LastUsedAt = nil
				state.Associations[index].LastVerifiedAt = nil
				state.Associations[index].Status = telegramtopics.TopicStatusActive
				replacement = state.Associations[index]
				return nil
			}
		}
		return diagnostics.ForChannel(diagnostics.CategoryInternalError, notify.ChannelTelegram, "telegram topic association disappeared during recovery")
	})
	if err != nil {
		return telegramtopics.Association{}, err
	}
	if replacement.MessageThreadID <= 0 {
		return telegramtopics.Association{}, diagnostics.ForChannel(diagnostics.CategoryInternalError, notify.ChannelTelegram, "telegram topic association was not replaced")
	}
	return replacement, nil
}

type Message struct {
	Token           string
	ChatID          string
	MessageThreadID int
	Text            string
	ParseMode       string
}

type ForumTopic struct {
	Token  string
	ChatID string
	Name   string
}

func buildMessage(request notify.Request, destination notify.Destination, config notify.DeliveryAccount) (Message, error) {
	if config.Type != notify.ChannelTelegram {
		return Message{}, invalidConfig("channel config type must be telegram")
	}
	token := strings.TrimSpace(config.Secrets[secretToken])
	if token == "" {
		return Message{}, invalidConfig(fmt.Sprintf("required secret %q is missing", secretToken))
	}
	chatID, ok := destination.Address()
	if !ok {
		return Message{}, invalidConfig("destination has no telegram destination")
	}

	return Message{
		Token:     token,
		ChatID:    chatID,
		Text:      formatText(request, destination),
		ParseMode: strings.TrimSpace(config.Settings[settingParseMode]),
	}, nil
}

func formatText(request notify.Request, destination notify.Destination) string {
	title := strings.TrimSpace(request.Title)
	body := strings.TrimSpace(request.Message)
	if destination.EffectiveTelegramDeliveryMode() == notify.TelegramDeliveryModePrivate && strings.TrimSpace(request.SenderSystem) != "" && title != "" {
		title = fmt.Sprintf("[%s] [%s] %s", strings.TrimSpace(request.SenderSystem), request.EffectivePriority(), title)
	}
	if title == "" {
		return body
	}
	if body == "" {
		return title
	}
	return title + "\n\n" + body
}

func sendMessage(ctx context.Context, client HTTPClient, baseURL string, message Message) error {
	body := map[string]any{
		"chat_id": message.ChatID,
		"text":    message.Text,
	}
	if message.ParseMode != "" {
		body["parse_mode"] = message.ParseMode
	}
	if message.MessageThreadID > 0 {
		body["message_thread_id"] = message.MessageThreadID
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

	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("provider response could not be read")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return providerFailure(response.StatusCode, data, fmt.Sprintf("provider returned HTTP status %d", response.StatusCode))
	}
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("provider returned invalid response")
	}
	if !result.OK {
		return providerError{message: "provider rejected telegram request", description: result.Description}
	}
	return nil
}

func createForumTopic(ctx context.Context, client HTTPClient, baseURL string, topic ForumTopic) (int, error) {
	body := map[string]string{
		"chat_id": topic.ChatID,
		"name":    topic.Name,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return 0, fmt.Errorf("provider request could not be encoded")
	}

	endpoint := fmt.Sprintf("%s/bot%s/createForumTopic", baseURL, topic.Token)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return 0, fmt.Errorf("provider request could not be created")
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("provider request failed")
	}
	defer response.Body.Close()

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      struct {
			MessageThreadID int `json:"message_thread_id"`
		} `json:"result"`
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return 0, fmt.Errorf("provider response could not be read")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return 0, providerFailure(response.StatusCode, data, fmt.Sprintf("provider returned HTTP status %d", response.StatusCode))
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return 0, fmt.Errorf("provider returned invalid response")
	}
	if !result.OK || result.Result.MessageThreadID <= 0 {
		return 0, providerError{message: "provider rejected telegram request", description: result.Description}
	}
	return result.Result.MessageThreadID, nil
}

func invalidConfig(message string) error {
	return diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, notify.ChannelTelegram, message)
}

func (s Sender) currentTime() time.Time {
	if s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
}

type providerError struct {
	status      int
	description string
	message     string
}

func (e providerError) Error() string {
	if e.message != "" {
		return e.message
	}
	if e.status != 0 {
		return fmt.Sprintf("provider returned HTTP status %d", e.status)
	}
	return "provider rejected telegram request"
}

func providerFailure(status int, data []byte, fallback string) error {
	var result struct {
		Description string `json:"description"`
	}
	_ = json.Unmarshal(data, &result)
	return providerError{status: status, description: result.Description, message: fallback}
}

func isStaleTopicError(err error) bool {
	var provider providerError
	if !errors.As(err, &provider) {
		return false
	}
	description := strings.ToLower(provider.description)
	if provider.status != http.StatusBadRequest && provider.status != 0 {
		return false
	}
	return strings.Contains(description, "message thread") ||
		strings.Contains(description, "thread not found") ||
		strings.Contains(description, "topic not found") ||
		strings.Contains(description, "message thread not found")
}
