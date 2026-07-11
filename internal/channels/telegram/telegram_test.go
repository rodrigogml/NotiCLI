package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rodrigogml/NotiCLI/internal/diagnostics"
	"github.com/rodrigogml/NotiCLI/internal/notify"
	"github.com/rodrigogml/NotiCLI/internal/telegramtopics"
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
	result, err := sender.Send(context.Background(), validRequest(), validDelivery())
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
	if gotPayload["text"] != "[BackupJob] [NORMAL] Backup failed\n\nNightly backup failed" {
		t.Fatalf("text = %q", gotPayload["text"])
	}
	if gotPayload["parse_mode"] != "HTML" {
		t.Fatalf("parse_mode = %q, want HTML", gotPayload["parse_mode"])
	}
}

func TestSendExplicitPrivateModeDoesNotRequireTopicStore(t *testing.T) {
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	recipient := validRecipient()
	recipient.TelegramDeliveryMode = notify.TelegramDeliveryModePrivate
	sender := Sender{client: server.Client(), baseURL: server.URL}
	result, err := sender.Send(context.Background(), validRequest(), deliveryWithDestination(recipient))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("result = %#v, want success", result)
	}
	if _, ok := gotPayload["message_thread_id"]; ok {
		t.Fatalf("message_thread_id present for private send: %#v", gotPayload)
	}
	if gotPayload["text"] != "[BackupJob] [NORMAL] Backup failed\n\nNightly backup failed" {
		t.Fatalf("text = %q", gotPayload["text"])
	}
}

func TestSendOmitsSenderPrefixForTopicMode(t *testing.T) {
	var gotPath string
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	recipient := notify.Destination{
		ID:                       "ops",
		Type:                     notify.ChannelTelegram,
		TelegramDeliveryMode:     notify.TelegramDeliveryModeTopics,
		TelegramTopicGroupChatID: "-1001234567890",
		Enabled:                  true,
	}
	store := &memoryTopicStore{
		state: telegramtopics.State{
			Version:   telegramtopics.StateVersion,
			UpdatedAt: time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC),
			Associations: []telegramtopics.Association{
				validTopicAssociation("ops", "-1001234567890", "BackupJob", 4),
			},
		},
	}
	sender := Sender{client: server.Client(), baseURL: server.URL, topicStore: store}
	result, err := sender.Send(context.Background(), validRequest(), deliveryWithDestination(recipient))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("result = %#v, want success", result)
	}
	if gotPath != "/bot123456:ABCDEF/sendMessage" {
		t.Fatalf("path = %q, want sendMessage without topic creation", gotPath)
	}
	if gotPayload["chat_id"] != "-1001234567890" {
		t.Fatalf("chat_id = %q, want topic group chat ID", gotPayload["chat_id"])
	}
	if gotPayload["text"] != "Backup failed\n\nNightly backup failed" {
		t.Fatalf("text = %q", gotPayload["text"])
	}
	if gotPayload["message_thread_id"] != float64(4) {
		t.Fatalf("message_thread_id = %#v, want 4", gotPayload["message_thread_id"])
	}
	if store.state.Associations[0].LastUsedAt == nil || store.state.Associations[0].LastVerifiedAt == nil {
		t.Fatalf("topic timestamps were not updated: %#v", store.state.Associations[0])
	}
}

func TestSendTopicModeRequiresKnownAssociationScopedByRecipientAndChat(t *testing.T) {
	var sentThreadID any
	var createCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/bot123456:ABCDEF/createForumTopic":
			createCalls++
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true,"result":{"message_thread_id":6,"name":"BackupJob"}}`))
		case "/bot123456:ABCDEF/sendMessage":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode(send) error = %v", err)
			}
			sentThreadID = payload["message_thread_id"]
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	store := &memoryTopicStore{
		state: telegramtopics.State{
			Version:   telegramtopics.StateVersion,
			UpdatedAt: time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC),
			Associations: []telegramtopics.Association{
				validTopicAssociation("other-recipient", "-1001234567890", "BackupJob", 4),
				validTopicAssociation("ops", "-200", "BackupJob", 5),
			},
		},
	}
	recipient := notify.Destination{
		ID:                       "ops",
		Type:                     notify.ChannelTelegram,
		TelegramDeliveryMode:     notify.TelegramDeliveryModeTopics,
		TelegramTopicGroupChatID: "-1001234567890",
		Enabled:                  true,
	}
	sender := Sender{client: server.Client(), baseURL: server.URL, topicStore: store}

	result, err := sender.Send(context.Background(), validRequest(), deliveryWithDestination(recipient))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("result = %#v, want success", result)
	}
	if store.loadCalls != 1 {
		t.Fatalf("loadCalls = %d, want 1", store.loadCalls)
	}
	if createCalls != 1 {
		t.Fatalf("createCalls = %d, want one new topic for scoped cache miss", createCalls)
	}
	if sentThreadID != float64(6) {
		t.Fatalf("message_thread_id = %#v, want newly created thread", sentThreadID)
	}
}

func TestSendTopicModeCreatesTopicOnCacheMissAndSendsToCreatedThread(t *testing.T) {
	var paths []string
	var sendPayload map[string]any
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/bot123456:ABCDEF/createForumTopic":
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode(create) error = %v", err)
			}
			if payload["name"] != "BackupJob" {
				t.Fatalf("topic name = %q, want BackupJob", payload["name"])
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true,"result":{"message_thread_id":4,"name":"BackupJob"}}`))
		case "/bot123456:ABCDEF/sendMessage":
			if err := json.NewDecoder(r.Body).Decode(&sendPayload); err != nil {
				t.Fatalf("Decode(send) error = %v", err)
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	store := &memoryTopicStore{state: telegramtopics.NewState(time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC))}
	recipient := notify.Destination{
		ID:                       "ops",
		Type:                     notify.ChannelTelegram,
		TelegramDeliveryMode:     notify.TelegramDeliveryModeTopics,
		TelegramTopicGroupChatID: "-1001234567890",
		Enabled:                  true,
	}
	sender := Sender{client: server.Client(), baseURL: server.URL, topicStore: store}

	result, err := sender.Send(context.Background(), validRequest(), deliveryWithDestination(recipient))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("result = %#v, want success", result)
	}
	if len(paths) != 2 || paths[0] != "/bot123456:ABCDEF/createForumTopic" || paths[1] != "/bot123456:ABCDEF/sendMessage" {
		t.Fatalf("paths = %#v, want create then send", paths)
	}
	if sendPayload["message_thread_id"] != float64(4) {
		t.Fatalf("message_thread_id = %#v, want created thread ID", sendPayload["message_thread_id"])
	}
	association, ok := store.state.FindAssociation("ops", "-1001234567890", "BackupJob")
	if !ok {
		t.Fatal("created topic association was not persisted")
	}
	if association.TopicName != "BackupJob" || association.MessageThreadID != 4 || !association.CreatedByNotiCLI {
		t.Fatalf("created association = %#v", association)
	}
}

func TestSendTopicModeAbortsBeforeCreatingTopicWhenStateWriteIsNotPrepared(t *testing.T) {
	var createCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bot123456:ABCDEF/createForumTopic" {
			createCalls++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true,"result":{"message_thread_id":4,"name":"BackupJob"}}`))
	}))
	defer server.Close()

	store := &memoryTopicStore{
		state:      telegramtopics.NewState(time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)),
		prepareErr: errors.New("state file is not writable"),
	}
	recipient := notify.Destination{
		ID:                       "ops",
		Type:                     notify.ChannelTelegram,
		TelegramDeliveryMode:     notify.TelegramDeliveryModeTopics,
		TelegramTopicGroupChatID: "-1001234567890",
		Enabled:                  true,
	}
	sender := Sender{client: server.Client(), baseURL: server.URL, topicStore: store}

	result, err := sender.Send(context.Background(), validRequest(), deliveryWithDestination(recipient))
	if err == nil {
		t.Fatal("Send() error = nil, want state preparation failure")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryInternalError)
	if result.Success || result.Category != diagnostics.CategoryInternalError {
		t.Fatalf("result = %#v, want internal_error", result)
	}
	if createCalls != 0 {
		t.Fatalf("createCalls = %d, want no topic creation before writable state is verified", createCalls)
	}
}

func TestSendTopicModeCreatesAtMostOneTopicForConcurrentCacheMiss(t *testing.T) {
	var createCalls int
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if r.URL.Path == "/bot123456:ABCDEF/createForumTopic" {
			createCalls++
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/bot123456:ABCDEF/createForumTopic":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true,"result":{"message_thread_id":4,"name":"BackupJob"}}`))
		case "/bot123456:ABCDEF/sendMessage":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	store := &memoryTopicStore{state: telegramtopics.NewState(time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC))}
	recipient := notify.Destination{
		ID:                       "ops",
		Type:                     notify.ChannelTelegram,
		TelegramDeliveryMode:     notify.TelegramDeliveryModeTopics,
		TelegramTopicGroupChatID: "-1001234567890",
		Enabled:                  true,
	}
	sender := Sender{client: server.Client(), baseURL: server.URL, topicStore: store}

	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := sender.Send(context.Background(), validRequest(), deliveryWithDestination(recipient))
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("Send() error = %v", err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", createCalls)
	}
}

func TestSendMessageIncludesMessageThreadIDWhenPresent(t *testing.T) {
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	err := sendMessage(context.Background(), server.Client(), server.URL, Message{
		Token:           "123456:ABCDEF",
		ChatID:          "-1001234567890",
		MessageThreadID: 4,
		Text:            "Backup failed",
		ParseMode:       "HTML",
	})
	if err != nil {
		t.Fatalf("sendMessage() error = %v", err)
	}
	if gotPayload["message_thread_id"] != float64(4) {
		t.Fatalf("message_thread_id = %#v, want 4", gotPayload["message_thread_id"])
	}
}

func TestCreateForumTopicPostsPayloadAndReturnsThreadID(t *testing.T) {
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
		w.Write([]byte(`{"ok":true,"result":{"message_thread_id":4,"name":"ProdSmoke","icon_color":7322096}}`))
	}))
	defer server.Close()

	threadID, err := createForumTopic(context.Background(), server.Client(), server.URL, ForumTopic{
		Token:  "123456:ABCDEF",
		ChatID: "-1001234567890",
		Name:   "ProdSmoke",
	})
	if err != nil {
		t.Fatalf("createForumTopic() error = %v", err)
	}
	if threadID != 4 {
		t.Fatalf("threadID = %d, want 4", threadID)
	}
	if gotPath != "/bot123456:ABCDEF/createForumTopic" {
		t.Fatalf("path = %q, want Telegram createForumTopic path", gotPath)
	}
	if gotPayload["chat_id"] != "-1001234567890" {
		t.Fatalf("chat_id = %q, want group chat ID", gotPayload["chat_id"])
	}
	if gotPayload["name"] != "ProdSmoke" {
		t.Fatalf("name = %q, want topic name", gotPayload["name"])
	}
}

func TestCreateForumTopicMapsProviderFailureWithoutLeakingToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"ok":false,"description":"bad token 123456:ABCDEF"}`))
	}))
	defer server.Close()

	_, err := createForumTopic(context.Background(), server.Client(), server.URL, ForumTopic{
		Token:  "123456:ABCDEF",
		ChatID: "-1001234567890",
		Name:   "ProdSmoke",
	})
	if err == nil {
		t.Fatal("createForumTopic() error = nil, want provider error")
	}
	if strings.Contains(err.Error(), "123456:ABCDEF") {
		t.Fatalf("telegram token leaked in err=%q", err.Error())
	}
}

func TestSendTopicModeFullFlowUsesTemporaryStateRepository(t *testing.T) {
	var createCalls int
	var sendThreadIDs []any
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/bot123456:ABCDEF/createForumTopic":
			createCalls++
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true,"result":{"message_thread_id":4,"name":"BackupJob"}}`))
		case "/bot123456:ABCDEF/sendMessage":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode(send) error = %v", err)
			}
			sendThreadIDs = append(sendThreadIDs, payload["message_thread_id"])
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	repository := telegramtopics.NewFileRepository(filepath.Join(t.TempDir(), "telegram-topics.json"))
	recipient := notify.Destination{
		ID:                       "ops",
		Type:                     notify.ChannelTelegram,
		TelegramDeliveryMode:     notify.TelegramDeliveryModeTopics,
		TelegramTopicGroupChatID: "-1001234567890",
		Enabled:                  true,
	}
	sender := Sender{client: server.Client(), baseURL: server.URL, topicStore: repository}

	for i := 0; i < 2; i++ {
		result, err := sender.Send(context.Background(), validRequest(), deliveryWithDestination(recipient))
		if err != nil {
			t.Fatalf("Send(%d) error = %v", i, err)
		}
		if !result.Success {
			t.Fatalf("Send(%d) result = %#v, want success", i, result)
		}
	}

	if createCalls != 1 {
		t.Fatalf("createCalls = %d, want one topic creation across two sends", createCalls)
	}
	if len(sendThreadIDs) != 2 || sendThreadIDs[0] != float64(4) || sendThreadIDs[1] != float64(4) {
		t.Fatalf("sendThreadIDs = %#v, want two sends to thread 4", sendThreadIDs)
	}
	state, err := repository.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(state.Associations) != 1 {
		t.Fatalf("associations length = %d, want 1", len(state.Associations))
	}
}

func TestSendTopicModeRecoversStaleKnownTopicWithOneReplacement(t *testing.T) {
	var paths []string
	var retryThreadID any
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/bot123456:ABCDEF/sendMessage":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode(send) error = %v", err)
			}
			if payload["message_thread_id"] == float64(4) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"ok":false,"description":"Bad Request: message thread not found"}`))
				return
			}
			retryThreadID = payload["message_thread_id"]
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true}`))
		case "/bot123456:ABCDEF/createForumTopic":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true,"result":{"message_thread_id":6,"name":"BackupJob"}}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	store := &memoryTopicStore{
		state: telegramtopics.State{
			Version:      telegramtopics.StateVersion,
			UpdatedAt:    time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC),
			Associations: []telegramtopics.Association{validTopicAssociation("ops", "-1001234567890", "BackupJob", 4)},
		},
	}
	recipient := notify.Destination{
		ID:                       "ops",
		Type:                     notify.ChannelTelegram,
		TelegramDeliveryMode:     notify.TelegramDeliveryModeTopics,
		TelegramTopicGroupChatID: "-1001234567890",
		Enabled:                  true,
	}
	sender := Sender{client: server.Client(), baseURL: server.URL, topicStore: store}

	result, err := sender.Send(context.Background(), validRequest(), deliveryWithDestination(recipient))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("result = %#v, want success", result)
	}
	if len(paths) != 3 || paths[0] != "/bot123456:ABCDEF/sendMessage" || paths[1] != "/bot123456:ABCDEF/createForumTopic" || paths[2] != "/bot123456:ABCDEF/sendMessage" {
		t.Fatalf("paths = %#v, want send, create, retry send", paths)
	}
	if retryThreadID != float64(6) {
		t.Fatalf("retry thread ID = %#v, want replacement thread", retryThreadID)
	}
	association, ok := store.state.FindAssociation("ops", "-1001234567890", "BackupJob")
	if !ok {
		t.Fatal("association missing after recovery")
	}
	if association.MessageThreadID != 6 {
		t.Fatalf("MessageThreadID = %d, want replacement thread", association.MessageThreadID)
	}
	if association.LastUsedAt == nil || association.LastVerifiedAt == nil {
		t.Fatalf("association timestamps not updated after recovery: %#v", association)
	}
}

func TestSendTopicModeReturnsDeliveryFailureWhenRecoveryRetryFails(t *testing.T) {
	var sendCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/bot123456:ABCDEF/sendMessage":
			sendCalls++
			if sendCalls == 1 {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"ok":false,"description":"Bad Request: message thread not found"}`))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"ok":false,"description":"retry failed"}`))
		case "/bot123456:ABCDEF/createForumTopic":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true,"result":{"message_thread_id":6,"name":"BackupJob"}}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	store := &memoryTopicStore{
		state: telegramtopics.State{
			Version:      telegramtopics.StateVersion,
			UpdatedAt:    time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC),
			Associations: []telegramtopics.Association{validTopicAssociation("ops", "-1001234567890", "BackupJob", 4)},
		},
	}
	recipient := notify.Destination{ID: "ops", Type: notify.ChannelTelegram, TelegramDeliveryMode: notify.TelegramDeliveryModeTopics, TelegramTopicGroupChatID: "-1001234567890", Enabled: true}
	sender := Sender{client: server.Client(), baseURL: server.URL, topicStore: store}

	result, err := sender.Send(context.Background(), validRequest(), deliveryWithDestination(recipient))
	if err == nil {
		t.Fatal("Send() error = nil, want retry delivery failure")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryDeliveryFailure)
	if result.Success || result.Category != diagnostics.CategoryDeliveryFailure {
		t.Fatalf("result = %#v, want delivery_failure", result)
	}
	if sendCalls != 2 {
		t.Fatalf("sendCalls = %d, want one original send and one retry", sendCalls)
	}
}

func TestSendTopicModeDoesNotRecoverNonStaleProviderFailure(t *testing.T) {
	var createCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/bot123456:ABCDEF/sendMessage":
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"ok":false,"description":"bad token 123456:ABCDEF"}`))
		case "/bot123456:ABCDEF/createForumTopic":
			createCalls++
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true,"result":{"message_thread_id":6,"name":"BackupJob"}}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	store := &memoryTopicStore{
		state: telegramtopics.State{
			Version:      telegramtopics.StateVersion,
			UpdatedAt:    time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC),
			Associations: []telegramtopics.Association{validTopicAssociation("ops", "-1001234567890", "BackupJob", 4)},
		},
	}
	recipient := notify.Destination{ID: "ops", Type: notify.ChannelTelegram, TelegramDeliveryMode: notify.TelegramDeliveryModeTopics, TelegramTopicGroupChatID: "-1001234567890", Enabled: true}
	sender := Sender{client: server.Client(), baseURL: server.URL, topicStore: store}

	result, err := sender.Send(context.Background(), validRequest(), deliveryWithDestination(recipient))
	if err == nil {
		t.Fatal("Send() error = nil, want delivery failure")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryDeliveryFailure)
	if createCalls != 0 {
		t.Fatalf("createCalls = %d, want no recovery for non-stale provider failure", createCalls)
	}
	if strings.Contains(result.Message, "123456:ABCDEF") || strings.Contains(err.Error(), "123456:ABCDEF") {
		t.Fatalf("telegram token leaked in result=%q err=%q", result.Message, err.Error())
	}
}

func TestSendTopicModeMapsCreatePermissionFailureWithoutLeakingToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/bot123456:ABCDEF/createForumTopic":
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"ok":false,"description":"forbidden for token 123456:ABCDEF"}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	store := &memoryTopicStore{state: telegramtopics.NewState(time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC))}
	recipient := notify.Destination{ID: "ops", Type: notify.ChannelTelegram, TelegramDeliveryMode: notify.TelegramDeliveryModeTopics, TelegramTopicGroupChatID: "-1001234567890", Enabled: true}
	sender := Sender{client: server.Client(), baseURL: server.URL, topicStore: store}

	result, err := sender.Send(context.Background(), validRequest(), deliveryWithDestination(recipient))
	if err == nil {
		t.Fatal("Send() error = nil, want delivery failure")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryDeliveryFailure)
	if result.Success || result.Category != diagnostics.CategoryDeliveryFailure {
		t.Fatalf("result = %#v, want delivery_failure", result)
	}
	if strings.Contains(result.Message, "123456:ABCDEF") || strings.Contains(err.Error(), "123456:ABCDEF") {
		t.Fatalf("telegram token leaked in result=%q err=%q", result.Message, err.Error())
	}
}

func TestSendTopicModeMapsMalformedStateToInternalError(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "telegram-topics.json")
	if err := os.WriteFile(statePath, []byte(`{"version":`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	repository := telegramtopics.NewFileRepository(statePath)
	recipient := notify.Destination{ID: "ops", Type: notify.ChannelTelegram, TelegramDeliveryMode: notify.TelegramDeliveryModeTopics, TelegramTopicGroupChatID: "-1001234567890", Enabled: true}
	sender := Sender{client: failingClient{}, topicStore: repository}

	result, err := sender.Send(context.Background(), validRequest(), deliveryWithDestination(recipient))
	if err == nil {
		t.Fatal("Send() error = nil, want internal state error")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryInternalError)
	if result.Success || result.Category != diagnostics.CategoryInternalError {
		t.Fatalf("result = %#v, want internal_error", result)
	}
	if strings.Contains(result.Message, "123456:ABCDEF") || strings.Contains(err.Error(), "123456:ABCDEF") {
		t.Fatalf("telegram token leaked in result=%q err=%q", result.Message, err.Error())
	}
}

func TestSendReturnsInvalidConfigForMissingTokenOrDestination(t *testing.T) {
	config := validConfig()
	delete(config.Secrets, secretToken)

	result, err := NewSender(nil).Send(context.Background(), validRequest(), deliveryWithAccount(config))
	if err == nil {
		t.Fatal("Send() error = nil, want invalid_config")
	}
	assertDiagnosticCategory(t, err, diagnostics.CategoryInvalidConfig)
	if result.Success || result.Category != diagnostics.CategoryInvalidConfig {
		t.Fatalf("result = %#v, want invalid_config", result)
	}

	recipient := validRecipient()
	recipient.TelegramChatID = ""
	_, err = NewSender(nil).Send(context.Background(), validRequest(), deliveryWithDestination(recipient))
	assertDiagnosticCategory(t, err, diagnostics.CategoryInvalidConfig)
}

func TestSendMapsProviderHTTPFailureToDeliveryFailureWithoutLeakingToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"ok":false,"description":"bad token 123456:ABCDEF"}`))
	}))
	defer server.Close()

	sender := Sender{client: server.Client(), baseURL: server.URL}
	result, err := sender.Send(context.Background(), validRequest(), validDelivery())
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

	result, err := sender.Send(context.Background(), validRequest(), validDelivery())
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
	result, err := sender.Send(context.Background(), validRequest(), validDelivery())
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

	result, err := NewSender(failingClient{}).Send(context.Background(), request, validDelivery())
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

type memoryTopicStore struct {
	mu          sync.Mutex
	state       telegramtopics.State
	prepareErr  error
	loadCalls   int
	updateCalls int
}

func (s *memoryTopicStore) Load(context.Context) (telegramtopics.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadCalls++
	return s.state, nil
}

func (s *memoryTopicStore) PrepareForUpdate(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.prepareErr
}

func (s *memoryTopicStore) Update(_ context.Context, mutate func(*telegramtopics.State) error) (telegramtopics.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateCalls++
	if mutate != nil {
		if err := mutate(&s.state); err != nil {
			return telegramtopics.State{}, err
		}
	}
	return s.state, nil
}

func validRequest() notify.Request {
	return notify.Request{
		SenderSystem: "BackupJob",
		Priority:     notify.PriorityNormal,
		Title:        "Backup failed",
		Message:      "Nightly backup failed",
	}
}

func validRecipient() notify.Destination {
	return notify.Destination{
		ID:             "ops",
		Type:           notify.ChannelTelegram,
		TelegramChatID: "12345",
		Enabled:        true,
	}
}

func validConfig() notify.DeliveryAccount {
	return notify.DeliveryAccount{
		ID:      "telegram-main",
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

func validDelivery() notify.ResolvedDelivery {
	return deliveryWithDestination(validRecipient())
}

func deliveryWithAccount(account notify.DeliveryAccount) notify.ResolvedDelivery {
	delivery := validDelivery()
	delivery.Account = account
	delivery.AccountID = account.ID
	return delivery
}

func deliveryWithDestination(destination notify.Destination) notify.ResolvedDelivery {
	account := validConfig()
	return notify.ResolvedDelivery{
		RouteID:       "backup-high",
		AccountID:     account.ID,
		DestinationID: destination.ID,
		Account:       account,
		Destination:   destination,
	}
}

func validTopicAssociation(recipientID, chatID, sender string, threadID int) telegramtopics.Association {
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	return telegramtopics.Association{
		RecipientID:      recipientID,
		ChatID:           chatID,
		Sender:           sender,
		TopicName:        sender,
		MessageThreadID:  threadID,
		CreatedByNotiCLI: true,
		CreatedAt:        now,
		Status:           telegramtopics.TopicStatusActive,
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
