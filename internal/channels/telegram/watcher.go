package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultUpdatesLimit = 100

type Watcher struct {
	client  HTTPClient
	baseURL string
	now     func() time.Time
}

type WatcherOption func(*Watcher)

type WatchOptions struct {
	Token       string
	AccountID   string
	PollTimeout time.Duration
	MaxDuration time.Duration
	OnStart     func(WatchStart)
	OnUpdate    func(WatchUpdate) error
}

type WatchStart struct {
	AccountID   string
	PollTimeout time.Duration
	MaxDuration time.Duration
}

type WatchUpdate struct {
	AccountID  string
	ObservedAt time.Time
	UpdateID   int64
	UpdateType string
	Raw        json.RawMessage
	Summary    UpdateSummary
}

type UpdateSummary struct {
	ChatID          string `json:"chat_id,omitempty"`
	ChatType        string `json:"chat_type,omitempty"`
	ChatTitle       string `json:"chat_title,omitempty"`
	MessageThreadID int64  `json:"message_thread_id,omitempty"`
	FromID          string `json:"from_id,omitempty"`
	FromUsername    string `json:"from_username,omitempty"`
	FromFirstName   string `json:"from_first_name,omitempty"`
	Text            string `json:"text,omitempty"`
	Caption         string `json:"caption,omitempty"`
}

func NewWatcher(client HTTPClient, options ...WatcherOption) Watcher {
	watcher := Watcher{
		client:  client,
		baseURL: defaultBaseURL,
		now:     func() time.Time { return time.Now().UTC() },
	}
	for _, option := range options {
		if option != nil {
			option(&watcher)
		}
	}
	return watcher
}

func WithWatcherBaseURL(baseURL string) WatcherOption {
	return func(watcher *Watcher) {
		watcher.baseURL = baseURL
	}
}

func WithWatcherClock(now func() time.Time) WatcherOption {
	return func(watcher *Watcher) {
		watcher.now = now
	}
}

func (w Watcher) Watch(ctx context.Context, options WatchOptions) error {
	token := strings.TrimSpace(options.Token)
	if token == "" {
		return invalidConfig(fmt.Sprintf("required secret %q is missing", secretToken))
	}
	pollTimeout := options.PollTimeout
	if pollTimeout <= 0 {
		pollTimeout = 3 * time.Second
	}

	client := w.client
	if client == nil {
		client = http.DefaultClient
	}
	baseURL := strings.TrimRight(w.baseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	if options.OnStart != nil {
		options.OnStart(WatchStart{
			AccountID:   strings.TrimSpace(options.AccountID),
			PollTimeout: pollTimeout,
			MaxDuration: options.MaxDuration,
		})
	}

	watchCtx := ctx
	cancel := func() {}
	if options.MaxDuration > 0 {
		watchCtx, cancel = context.WithTimeout(ctx, options.MaxDuration)
	}
	defer cancel()

	var offset int64
	for {
		select {
		case <-watchCtx.Done():
			return nil
		default:
		}

		updates, err := getUpdates(watchCtx, client, baseURL, token, offset, pollTimeout)
		if err != nil {
			if watchCtx.Err() != nil {
				return nil
			}
			return err
		}
		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			if options.OnUpdate == nil {
				continue
			}
			raw := append(json.RawMessage(nil), update.Raw...)
			event := WatchUpdate{
				AccountID:  strings.TrimSpace(options.AccountID),
				ObservedAt: w.currentTime(),
				UpdateID:   update.UpdateID,
				UpdateType: updateType(raw),
				Raw:        raw,
				Summary:    summarizeUpdate(raw),
			}
			if err := options.OnUpdate(event); err != nil {
				return err
			}
		}
	}
}

func (w Watcher) currentTime() time.Time {
	if w.now == nil {
		return time.Now().UTC()
	}
	return w.now().UTC()
}

type updateEnvelope struct {
	UpdateID int64           `json:"update_id"`
	Raw      json.RawMessage `json:"-"`
}

func getUpdates(ctx context.Context, client HTTPClient, baseURL, token string, offset int64, pollTimeout time.Duration) ([]updateEnvelope, error) {
	body := map[string]any{
		"allowed_updates": []string{},
		"limit":           defaultUpdatesLimit,
		"timeout":         int(pollTimeout / time.Second),
	}
	if offset > 0 {
		body["offset"] = offset
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("provider request could not be encoded")
	}

	endpoint := fmt.Sprintf("%s/bot%s/getUpdates", baseURL, token)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("provider request could not be created")
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("provider request failed")
	}
	defer response.Body.Close()

	data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("provider response could not be read")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, providerFailure(response.StatusCode, data, fmt.Sprintf("provider returned HTTP status %d", response.StatusCode))
	}
	var result struct {
		OK          bool              `json:"ok"`
		Description string            `json:"description"`
		Result      []json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("provider returned invalid response")
	}
	if !result.OK {
		return nil, providerError{message: "provider rejected telegram request", description: result.Description}
	}

	updates := make([]updateEnvelope, 0, len(result.Result))
	for _, raw := range result.Result {
		var update struct {
			UpdateID int64 `json:"update_id"`
		}
		if err := json.Unmarshal(raw, &update); err != nil {
			return nil, fmt.Errorf("provider returned invalid update")
		}
		updates = append(updates, updateEnvelope{
			UpdateID: update.UpdateID,
			Raw:      append(json.RawMessage(nil), raw...),
		})
	}
	return updates, nil
}

func updateType(raw json.RawMessage) string {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return "unknown"
	}
	for _, key := range []string{
		"message",
		"edited_message",
		"channel_post",
		"edited_channel_post",
		"callback_query",
		"my_chat_member",
		"chat_member",
		"chat_join_request",
		"message_reaction",
		"message_reaction_count",
		"poll",
		"poll_answer",
		"inline_query",
		"chosen_inline_result",
		"pre_checkout_query",
		"shipping_query",
	} {
		if _, ok := fields[key]; ok {
			return key
		}
	}
	return "unknown"
}

func summarizeUpdate(raw json.RawMessage) UpdateSummary {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return UpdateSummary{}
	}
	for _, key := range []string{"message", "edited_message", "channel_post", "edited_channel_post"} {
		if payload, ok := fields[key]; ok {
			return summarizeMessage(payload)
		}
	}
	if payload, ok := fields["callback_query"]; ok {
		return summarizeCallbackQuery(payload)
	}
	if payload, ok := fields["my_chat_member"]; ok {
		return summarizeChatMemberUpdate(payload)
	}
	if payload, ok := fields["chat_member"]; ok {
		return summarizeChatMemberUpdate(payload)
	}
	if payload, ok := fields["chat_join_request"]; ok {
		return summarizeChatJoinRequest(payload)
	}
	return UpdateSummary{}
}

func summarizeMessage(raw json.RawMessage) UpdateSummary {
	var message struct {
		MessageThreadID int64  `json:"message_thread_id"`
		Text            string `json:"text"`
		Caption         string `json:"caption"`
		Chat            struct {
			ID        json.Number `json:"id"`
			Type      string      `json:"type"`
			Title     string      `json:"title"`
			Username  string      `json:"username"`
			FirstName string      `json:"first_name"`
		} `json:"chat"`
		From struct {
			ID        json.Number `json:"id"`
			Username  string      `json:"username"`
			FirstName string      `json:"first_name"`
		} `json:"from"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&message); err != nil {
		return UpdateSummary{}
	}
	title := strings.TrimSpace(message.Chat.Title)
	if title == "" {
		title = strings.TrimSpace(message.Chat.Username)
	}
	if title == "" {
		title = strings.TrimSpace(message.Chat.FirstName)
	}
	return UpdateSummary{
		ChatID:          message.Chat.ID.String(),
		ChatType:        strings.TrimSpace(message.Chat.Type),
		ChatTitle:       title,
		MessageThreadID: message.MessageThreadID,
		FromID:          message.From.ID.String(),
		FromUsername:    strings.TrimSpace(message.From.Username),
		FromFirstName:   strings.TrimSpace(message.From.FirstName),
		Text:            trimSummaryText(message.Text),
		Caption:         trimSummaryText(message.Caption),
	}
}

func summarizeCallbackQuery(raw json.RawMessage) UpdateSummary {
	var callback struct {
		From struct {
			ID        json.Number `json:"id"`
			Username  string      `json:"username"`
			FirstName string      `json:"first_name"`
		} `json:"from"`
		Message json.RawMessage `json:"message"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&callback); err != nil {
		return UpdateSummary{}
	}
	summary := summarizeMessage(callback.Message)
	summary.FromID = callback.From.ID.String()
	summary.FromUsername = strings.TrimSpace(callback.From.Username)
	summary.FromFirstName = strings.TrimSpace(callback.From.FirstName)
	return summary
}

func summarizeChatMemberUpdate(raw json.RawMessage) UpdateSummary {
	var update struct {
		Chat struct {
			ID    json.Number `json:"id"`
			Type  string      `json:"type"`
			Title string      `json:"title"`
		} `json:"chat"`
		From struct {
			ID        json.Number `json:"id"`
			Username  string      `json:"username"`
			FirstName string      `json:"first_name"`
		} `json:"from"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&update); err != nil {
		return UpdateSummary{}
	}
	return UpdateSummary{
		ChatID:        update.Chat.ID.String(),
		ChatType:      strings.TrimSpace(update.Chat.Type),
		ChatTitle:     strings.TrimSpace(update.Chat.Title),
		FromID:        update.From.ID.String(),
		FromUsername:  strings.TrimSpace(update.From.Username),
		FromFirstName: strings.TrimSpace(update.From.FirstName),
	}
}

func summarizeChatJoinRequest(raw json.RawMessage) UpdateSummary {
	var request struct {
		Chat struct {
			ID    json.Number `json:"id"`
			Type  string      `json:"type"`
			Title string      `json:"title"`
		} `json:"chat"`
		From struct {
			ID        json.Number `json:"id"`
			Username  string      `json:"username"`
			FirstName string      `json:"first_name"`
		} `json:"from"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		return UpdateSummary{}
	}
	return UpdateSummary{
		ChatID:        request.Chat.ID.String(),
		ChatType:      strings.TrimSpace(request.Chat.Type),
		ChatTitle:     strings.TrimSpace(request.Chat.Title),
		FromID:        request.From.ID.String(),
		FromUsername:  strings.TrimSpace(request.From.Username),
		FromFirstName: strings.TrimSpace(request.From.FirstName),
	}
}

func trimSummaryText(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	const maxLength = 240
	if len([]rune(value)) <= maxLength {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxLength]) + "..."
}
