package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/rodrigogml/NotiCLI/internal/diagnostics"
	"github.com/rodrigogml/NotiCLI/internal/notify"
)

type File struct {
	Recipients map[string]Recipient `json:"recipients"`
	Channels   map[string]Channel   `json:"channels"`
	Defaults   map[string]string    `json:"defaults"`
}

type Recipient struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	TelegramChatID string `json:"telegram_chat_id"`
	SlackDest      string `json:"slack_destination"`
	Enabled        *bool  `json:"enabled"`
}

type Channel struct {
	Type        string            `json:"type"`
	Enabled     *bool             `json:"enabled"`
	Settings    map[string]string `json:"settings"`
	Secrets     map[string]string `json:"secrets"`
	Attachments string            `json:"attachments"`
}

func Load(path string) (notify.Configuration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return notify.Configuration{}, diagnostics.New(diagnostics.CategoryMissingConfig, fmt.Sprintf("configuration file not found: %s", path))
		}
		return notify.Configuration{}, diagnostics.New(diagnostics.CategoryMissingConfig, fmt.Sprintf("configuration file unreadable: %s", path))
	}

	var file File
	if err := json.Unmarshal(data, &file); err != nil {
		return notify.Configuration{}, diagnostics.New(diagnostics.CategoryInvalidConfig, fmt.Sprintf("configuration file is not valid JSON: %s", path))
	}

	configuration := file.toDomain()
	if err := configuration.Validate(); err != nil {
		return notify.Configuration{}, err
	}

	return configuration, nil
}

func (f File) toDomain() notify.Configuration {
	recipients := make(map[string]notify.Recipient, len(f.Recipients))
	for key, recipient := range f.Recipients {
		enabled := true
		if recipient.Enabled != nil {
			enabled = *recipient.Enabled
		}
		id := recipient.ID
		if id == "" {
			id = key
		}
		recipients[key] = notify.Recipient{
			ID:             id,
			Name:           recipient.Name,
			Email:          recipient.Email,
			TelegramChatID: recipient.TelegramChatID,
			SlackDest:      recipient.SlackDest,
			Enabled:        enabled,
		}
	}

	channels := make(map[string]notify.ChannelConfig, len(f.Channels))
	for key, channel := range f.Channels {
		enabled := true
		if channel.Enabled != nil {
			enabled = *channel.Enabled
		}
		channelType := channel.Type
		if channelType == "" {
			channelType = key
		}
		channels[key] = notify.ChannelConfig{
			Type:             channelType,
			Enabled:          enabled,
			Settings:         cloneMap(channel.Settings),
			Secrets:          cloneMap(channel.Secrets),
			AttachmentPolicy: attachmentPolicyOrDefault(channel.Attachments),
		}
	}

	return notify.Configuration{
		Recipients: recipients,
		Channels:   channels,
		Defaults:   cloneMap(f.Defaults),
	}
}

func attachmentPolicyOrDefault(value string) notify.AttachmentPolicy {
	if value == "" {
		return notify.AttachmentPolicyUnsupported
	}
	return notify.AttachmentPolicy(value)
}

func cloneMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	target := make(map[string]string, len(source))
	for key, value := range source {
		target[key] = value
	}
	return target
}
