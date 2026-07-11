package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rodrigogml/NotiCLI/internal/diagnostics"
	"github.com/rodrigogml/NotiCLI/internal/notify"
)

type File struct {
	Recipients       map[string]Destination     `json:"recipients"`
	Channels         map[string]DeliveryAccount `json:"channels"`
	Destinations     map[string]Destination     `json:"destinations"`
	DeliveryAccounts map[string]DeliveryAccount `json:"delivery_accounts"`
	Routes           []Route                    `json:"routes"`
	CatchAll         CatchAll                   `json:"catch_all"`
	Logging          Logging                    `json:"logging"`
}

type Destination struct {
	ID                       string `json:"id"`
	Name                     string `json:"name"`
	Type                     string `json:"type"`
	Email                    string `json:"email"`
	TelegramChatID           string `json:"telegram_chat_id"`
	TelegramDeliveryMode     string `json:"telegram_delivery_mode"`
	TelegramTopicGroupChatID string `json:"telegram_topic_group_chat_id"`
	TelegramTopicGroupName   string `json:"telegram_topic_group_name"`
	MessageThreadID          int    `json:"message_thread_id"`
	SlackDest                string `json:"slack_destination"`
	Enabled                  *bool  `json:"enabled"`
}

type DeliveryAccount struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Enabled     *bool             `json:"enabled"`
	Settings    map[string]string `json:"settings"`
	Secrets     map[string]string `json:"secrets"`
	Attachments string            `json:"attachments"`
}

type Route struct {
	ID         string     `json:"id"`
	Match      RouteMatch `json:"match"`
	Deliveries []Delivery `json:"deliveries"`
}

type RouteMatch struct {
	Senders    []string `json:"senders"`
	Categories []string `json:"categories"`
	Priorities []string `json:"priorities"`
}

type Delivery struct {
	Account     string `json:"account"`
	Destination string `json:"destination"`
}

type CatchAll struct {
	Deliveries []Delivery `json:"deliveries"`
}

type Logging struct {
	Path string `json:"path"`
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
	if len(file.Recipients) > 0 || len(file.Channels) > 0 {
		return notify.Configuration{}, diagnostics.New(diagnostics.CategoryInvalidConfig, "legacy configuration keys recipients/channels are no longer supported; use destinations/delivery_accounts/routes/catch_all")
	}

	configuration := file.toDomain(defaultLogPath(path))
	if err := configuration.Validate(); err != nil {
		return notify.Configuration{}, err
	}

	return configuration, nil
}

func (f File) toDomain(defaultLoggingPath string) notify.Configuration {
	destinations := make(map[string]notify.Destination, len(f.Destinations))
	for key, destination := range f.Destinations {
		enabled := true
		if destination.Enabled != nil {
			enabled = *destination.Enabled
		}
		id := destination.ID
		if id == "" {
			id = key
		}
		destinations[key] = notify.Destination{
			ID:                       id,
			Name:                     destination.Name,
			Type:                     destination.Type,
			Email:                    destination.Email,
			TelegramChatID:           destination.TelegramChatID,
			TelegramDeliveryMode:     destination.TelegramDeliveryMode,
			TelegramTopicGroupChatID: destination.TelegramTopicGroupChatID,
			TelegramTopicGroupName:   destination.TelegramTopicGroupName,
			MessageThreadID:          destination.MessageThreadID,
			SlackDest:                destination.SlackDest,
			Enabled:                  enabled,
		}
	}

	accounts := make(map[string]notify.DeliveryAccount, len(f.DeliveryAccounts))
	for key, account := range f.DeliveryAccounts {
		enabled := true
		if account.Enabled != nil {
			enabled = *account.Enabled
		}
		id := account.ID
		if id == "" {
			id = key
		}
		accounts[key] = notify.DeliveryAccount{
			ID:               id,
			Type:             account.Type,
			Enabled:          enabled,
			Settings:         cloneMap(account.Settings),
			Secrets:          cloneMap(account.Secrets),
			AttachmentPolicy: attachmentPolicyOrDefault(account.Attachments),
		}
	}

	logPath := f.Logging.Path
	if logPath == "" {
		logPath = defaultLoggingPath
	}
	return notify.Configuration{
		Destinations:     destinations,
		DeliveryAccounts: accounts,
		Routes:           routesToDomain(f.Routes),
		CatchAll:         notify.Route{ID: "catch_all", Deliveries: deliveriesToDomain(f.CatchAll.Deliveries)},
		Logging:          notify.LoggingConfig{Path: logPath},
	}
}

func routesToDomain(routes []Route) []notify.Route {
	out := make([]notify.Route, 0, len(routes))
	for _, route := range routes {
		out = append(out, notify.Route{
			ID: route.ID,
			Match: notify.RouteMatch{
				Senders:    append([]string(nil), route.Match.Senders...),
				Categories: append([]string(nil), route.Match.Categories...),
				Priorities: append([]string(nil), route.Match.Priorities...),
			},
			Deliveries: deliveriesToDomain(route.Deliveries),
		})
	}
	return out
}

func deliveriesToDomain(deliveries []Delivery) []notify.Delivery {
	out := make([]notify.Delivery, 0, len(deliveries))
	for _, delivery := range deliveries {
		out = append(out, notify.Delivery{
			Account:     delivery.Account,
			Destination: delivery.Destination,
		})
	}
	return out
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

func defaultLogPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "noticli.delivery.log")
}
