package telegramtopics

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

const StateVersion = 1

type TopicStatus string

const (
	TopicStatusActive   TopicStatus = "active"
	TopicStatusStale    TopicStatus = "stale"
	TopicStatusReplaced TopicStatus = "replaced"
)

type State struct {
	Version          int           `json:"version"`
	UpdatedAt        time.Time     `json:"updated_at"`
	Associations     []Association `json:"associations"`
	PreviousBackupAt *time.Time    `json:"previous_backup_at,omitempty"`
}

type Association struct {
	RecipientID            string      `json:"recipient_id"`
	ChatID                 string      `json:"chat_id"`
	Sender                 string      `json:"sender"`
	TopicName              string      `json:"topic_name"`
	TopicNameDisambiguator string      `json:"topic_name_disambiguator,omitempty"`
	MessageThreadID        int         `json:"message_thread_id"`
	CreatedByNotiCLI       bool        `json:"created_by_noticli"`
	CreatedAt              time.Time   `json:"created_at"`
	LastUsedAt             *time.Time  `json:"last_used_at,omitempty"`
	LastVerifiedAt         *time.Time  `json:"last_verified_at,omitempty"`
	Status                 TopicStatus `json:"status"`
}

func NewState(now time.Time) State {
	return State{
		Version:      StateVersion,
		UpdatedAt:    now.UTC(),
		Associations: []Association{},
	}
}

func DecodeState(reader io.Reader) (State, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()

	var state State
	if err := decoder.Decode(&state); err != nil {
		return State{}, fmt.Errorf("telegram topic state is not valid JSON: %w", err)
	}
	if err := state.Validate(); err != nil {
		return State{}, err
	}
	return state, nil
}

func EncodeState(writer io.Writer, state State) error {
	if err := state.Validate(); err != nil {
		return err
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		return fmt.Errorf("telegram topic state could not be encoded: %w", err)
	}
	return nil
}

func (s State) Validate() error {
	if s.Version != StateVersion {
		return fmt.Errorf("telegram topic state version %d is unsupported", s.Version)
	}
	if s.UpdatedAt.IsZero() {
		return fmt.Errorf("telegram topic state updated_at is required")
	}
	if s.Associations == nil {
		return fmt.Errorf("telegram topic state associations are required")
	}

	seen := make(map[string]struct{}, len(s.Associations))
	for index, association := range s.Associations {
		if err := association.Validate(); err != nil {
			return fmt.Errorf("telegram topic state association %d is invalid: %w", index, err)
		}
		key := association.Key()
		if _, ok := seen[key]; ok {
			return fmt.Errorf("telegram topic state has duplicate association key %q", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (s State) FindAssociation(recipientID, chatID, sender string) (Association, bool) {
	key := AssociationKey(recipientID, chatID, sender)
	for _, association := range s.Associations {
		if association.Key() == key {
			return association, true
		}
	}
	return Association{}, false
}

func (s *State) TouchAssociation(recipientID, chatID, sender string, now time.Time) bool {
	key := AssociationKey(recipientID, chatID, sender)
	timestamp := now.UTC()
	for index, association := range s.Associations {
		if association.Key() == key {
			s.Associations[index].LastUsedAt = &timestamp
			s.Associations[index].LastVerifiedAt = &timestamp
			return true
		}
	}
	return false
}

func (a Association) Validate() error {
	if strings.TrimSpace(a.RecipientID) == "" {
		return fmt.Errorf("recipient_id is required")
	}
	if strings.TrimSpace(a.ChatID) == "" {
		return fmt.Errorf("chat_id is required")
	}
	if strings.TrimSpace(a.Sender) == "" {
		return fmt.Errorf("sender is required")
	}
	if strings.TrimSpace(a.TopicName) == "" {
		return fmt.Errorf("topic_name is required")
	}
	if a.MessageThreadID <= 0 {
		return fmt.Errorf("message_thread_id must be positive")
	}
	if a.CreatedAt.IsZero() {
		return fmt.Errorf("created_at is required")
	}
	if !isValidStatus(a.Status) {
		return fmt.Errorf("status %q is unsupported", a.Status)
	}
	return nil
}

func (a Association) Key() string {
	return AssociationKey(a.RecipientID, a.ChatID, a.Sender)
}

func isValidStatus(status TopicStatus) bool {
	switch status {
	case TopicStatusActive, TopicStatusStale, TopicStatusReplaced:
		return true
	default:
		return false
	}
}
