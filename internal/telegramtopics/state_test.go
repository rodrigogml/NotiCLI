package telegramtopics_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/rodrigogml/NotiCLI/internal/telegramtopics"
)

func TestStateSerializesAndDeserializesTopicAssociations(t *testing.T) {
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	usedAt := now.Add(time.Second)
	state := telegramtopics.State{
		Version:   telegramtopics.StateVersion,
		UpdatedAt: now,
		Associations: []telegramtopics.Association{
			{
				RecipientID:            "rodrigogml-topics",
				ChatID:                 "-1001234567890",
				Sender:                 "ProdSmoke",
				TopicName:              "ProdSmoke",
				TopicNameDisambiguator: "01",
				MessageThreadID:        4,
				CreatedByNotiCLI:       true,
				CreatedAt:              now,
				LastUsedAt:             &usedAt,
				LastVerifiedAt:         &usedAt,
				Status:                 telegramtopics.TopicStatusActive,
			},
		},
	}

	var buffer bytes.Buffer
	if err := telegramtopics.EncodeState(&buffer, state); err != nil {
		t.Fatalf("EncodeState() error = %v", err)
	}
	data := buffer.String()
	if strings.Contains(data, "token") || strings.Contains(data, "password") || strings.Contains(data, "secret") {
		t.Fatalf("encoded state contains secret-like field: %s", data)
	}

	decoded, err := telegramtopics.DecodeState(strings.NewReader(data))
	if err != nil {
		t.Fatalf("DecodeState() error = %v", err)
	}
	if decoded.Version != telegramtopics.StateVersion {
		t.Fatalf("Version = %d, want %d", decoded.Version, telegramtopics.StateVersion)
	}
	if !decoded.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt = %s, want %s", decoded.UpdatedAt, now)
	}
	if len(decoded.Associations) != 1 {
		t.Fatalf("Associations length = %d, want 1", len(decoded.Associations))
	}
	association := decoded.Associations[0]
	if association.RecipientID != "rodrigogml-topics" || association.ChatID != "-1001234567890" || association.Sender != "ProdSmoke" {
		t.Fatalf("association routing fields = %#v", association)
	}
	if association.MessageThreadID != 4 {
		t.Fatalf("MessageThreadID = %d, want 4", association.MessageThreadID)
	}
	if association.Status != telegramtopics.TopicStatusActive {
		t.Fatalf("Status = %q, want active", association.Status)
	}
}

func TestNewStateCreatesValidEmptyState(t *testing.T) {
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	state := telegramtopics.NewState(now)

	if err := state.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if state.Version != telegramtopics.StateVersion {
		t.Fatalf("Version = %d, want %d", state.Version, telegramtopics.StateVersion)
	}
	if state.Associations == nil {
		t.Fatal("Associations = nil, want empty slice")
	}
}

func TestDecodeStateRejectsMalformedState(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{name: "invalid json", json: `{"version":`},
		{name: "unknown field", json: `{"version":1,"updated_at":"2026-06-28T12:00:00Z","associations":[],"token":"secret"}`},
		{name: "missing associations", json: `{"version":1,"updated_at":"2026-06-28T12:00:00Z"}`},
		{name: "unsupported version", json: `{"version":2,"updated_at":"2026-06-28T12:00:00Z","associations":[]}`},
		{name: "missing routing field", json: `{"version":1,"updated_at":"2026-06-28T12:00:00Z","associations":[{"recipient_id":"ops","chat_id":"","sender":"ProdSmoke","topic_name":"ProdSmoke","message_thread_id":4,"created_by_noticli":true,"created_at":"2026-06-28T12:00:00Z","status":"active"}]}`},
		{name: "invalid message thread", json: `{"version":1,"updated_at":"2026-06-28T12:00:00Z","associations":[{"recipient_id":"ops","chat_id":"-100","sender":"ProdSmoke","topic_name":"ProdSmoke","message_thread_id":0,"created_by_noticli":true,"created_at":"2026-06-28T12:00:00Z","status":"active"}]}`},
		{name: "invalid status", json: `{"version":1,"updated_at":"2026-06-28T12:00:00Z","associations":[{"recipient_id":"ops","chat_id":"-100","sender":"ProdSmoke","topic_name":"ProdSmoke","message_thread_id":4,"created_by_noticli":true,"created_at":"2026-06-28T12:00:00Z","status":"deleted"}]}`},
		{name: "duplicate key", json: `{"version":1,"updated_at":"2026-06-28T12:00:00Z","associations":[{"recipient_id":"ops","chat_id":"-100","sender":"ProdSmoke","topic_name":"ProdSmoke","message_thread_id":4,"created_by_noticli":true,"created_at":"2026-06-28T12:00:00Z","status":"active"},{"recipient_id":"ops","chat_id":"-100","sender":"ProdSmoke","topic_name":"ProdSmoke 2","message_thread_id":5,"created_by_noticli":true,"created_at":"2026-06-28T12:00:00Z","status":"active"}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := telegramtopics.DecodeState(strings.NewReader(tt.json)); err == nil {
				t.Fatal("DecodeState() error = nil, want malformed state error")
			}
		})
	}
}
