package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWatcherPollsUpdatesAdvancesOffsetAndSummarizesMessages(t *testing.T) {
	requests := make([]map[string]any, 0, 2)
	call := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot123456:ABCDEF/getUpdates" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		requests = append(requests, payload)
		call++

		w.Header().Set("Content-Type", "application/json")
		switch call {
		case 1:
			w.Write([]byte(`{"ok":true,"result":[{"update_id":10,"message":{"message_id":1,"message_thread_id":77,"chat":{"id":-1001234567890,"type":"supergroup","title":"Operations"},"from":{"id":12345,"username":"alice","first_name":"Alice"},"text":"hello from topic"}}]}`))
		case 2:
			w.Write([]byte(`{"ok":true,"result":[{"update_id":11,"edited_message":{"message_id":2,"chat":{"id":12345,"type":"private","first_name":"Alice"},"from":{"id":12345,"first_name":"Alice"},"caption":"edited caption"}}]}`))
		default:
			w.Write([]byte(`{"ok":true,"result":[]}`))
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	watcher := NewWatcher(server.Client(), WithWatcherBaseURL(server.URL), WithWatcherClock(func() time.Time { return now }))
	var events []WatchUpdate
	err := watcher.Watch(ctx, WatchOptions{
		Token:       "123456:ABCDEF",
		AccountID:   "telegram-main",
		PollTimeout: 5 * time.Second,
		OnUpdate: func(update WatchUpdate) error {
			events = append(events, update)
			if len(events) == 2 {
				cancel()
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	if len(requests) != 2 {
		t.Fatalf("requests length = %d, want 2", len(requests))
	}
	if requests[0]["timeout"] != float64(5) || requests[0]["limit"] != float64(100) {
		t.Fatalf("first request = %#v", requests[0])
	}
	if _, ok := requests[0]["offset"]; ok {
		t.Fatalf("first request offset = %#v, want omitted", requests[0]["offset"])
	}
	if requests[1]["offset"] != float64(11) {
		t.Fatalf("second request offset = %#v, want 11", requests[1]["offset"])
	}

	if len(events) != 2 {
		t.Fatalf("events length = %d, want 2", len(events))
	}
	first := events[0]
	if first.AccountID != "telegram-main" || !first.ObservedAt.Equal(now) || first.UpdateID != 10 || first.UpdateType != "message" {
		t.Fatalf("first event = %#v", first)
	}
	if first.Summary.ChatID != "-1001234567890" || first.Summary.ChatType != "supergroup" || first.Summary.ChatTitle != "Operations" {
		t.Fatalf("first summary chat = %#v", first.Summary)
	}
	if first.Summary.MessageThreadID != 77 || first.Summary.FromID != "12345" || first.Summary.FromUsername != "alice" || first.Summary.Text != "hello from topic" {
		t.Fatalf("first summary = %#v", first.Summary)
	}

	second := events[1]
	if second.UpdateID != 11 || second.UpdateType != "edited_message" {
		t.Fatalf("second event = %#v", second)
	}
	if second.Summary.ChatID != "12345" || second.Summary.ChatType != "private" || second.Summary.Caption != "edited caption" {
		t.Fatalf("second summary = %#v", second.Summary)
	}
}

func TestWatcherReturnsProviderErrorsWithoutToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"ok":false,"description":"Conflict: can't use getUpdates method while webhook is active"}`))
	}))
	defer server.Close()

	watcher := NewWatcher(server.Client(), WithWatcherBaseURL(server.URL))
	err := watcher.Watch(context.Background(), WatchOptions{
		Token:       "123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ",
		AccountID:   "telegram-main",
		PollTimeout: time.Second,
		MaxDuration: time.Second,
	})
	if err == nil {
		t.Fatal("Watch() error = nil, want error")
	}
	if got := err.Error(); got == "" || strings.Contains(got, "123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		t.Fatalf("error leaked token or was empty: %q", got)
	}
}
