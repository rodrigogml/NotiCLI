package notify

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rodrigogml/NotiCLI/internal/diagnostics"
)

const (
	ChannelEmail    = "email"
	ChannelSlack    = "slack"
	ChannelTelegram = "telegram"

	MaxSenderSystemLength = 20
)

type AttachmentPolicy string

const (
	AttachmentPolicySupported   AttachmentPolicy = "supported"
	AttachmentPolicyLimited     AttachmentPolicy = "limited"
	AttachmentPolicyUnsupported AttachmentPolicy = "unsupported"
)

type ResultCategory = diagnostics.Category

const (
	ResultSuccess         = diagnostics.CategorySuccess
	ResultInvalidInput    = diagnostics.CategoryInvalidInput
	ResultMissingConfig   = diagnostics.CategoryMissingConfig
	ResultInvalidConfig   = diagnostics.CategoryInvalidConfig
	ResultAttachmentError = diagnostics.CategoryAttachmentError
	ResultDeliveryFailure = diagnostics.CategoryDeliveryFailure
	ResultInternalError   = diagnostics.CategoryInternalError
)

type DeliveryState string

const (
	DeliveryStatePending DeliveryState = "pending"
	DeliveryStateSuccess DeliveryState = "success"
	DeliveryStateFailure DeliveryState = "failure"
)

type Request struct {
	ConfigPath   string
	SenderSystem string
	RecipientID  string
	Channel      string
	Title        string
	Message      string
	Attachments  []Attachment
}

func (r Request) Validate() error {
	if strings.TrimSpace(r.SenderSystem) == "" {
		return diagnostics.New(diagnostics.CategoryInvalidInput, "sender_system is required")
	}
	if len([]rune(strings.TrimSpace(r.SenderSystem))) > MaxSenderSystemLength {
		return diagnostics.New(diagnostics.CategoryInvalidInput, fmt.Sprintf("sender_system must be at most %d characters", MaxSenderSystemLength))
	}
	if strings.TrimSpace(r.RecipientID) == "" {
		return diagnostics.New(diagnostics.CategoryInvalidInput, "recipient_id is required")
	}
	if !IsSupportedChannel(r.Channel) {
		return diagnostics.New(diagnostics.CategoryInvalidInput, fmt.Sprintf("unsupported channel %q", r.Channel))
	}
	if strings.TrimSpace(r.Title) == "" {
		return diagnostics.New(diagnostics.CategoryInvalidInput, "title is required")
	}
	if strings.TrimSpace(r.Message) == "" {
		return diagnostics.New(diagnostics.CategoryInvalidInput, "message is required")
	}
	for _, attachment := range r.Attachments {
		if err := attachment.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type Configuration struct {
	Recipients map[string]Recipient
	Channels   map[string]ChannelConfig
	Defaults   map[string]string
}

type ResolvedRequest struct {
	Request     Request
	Recipient   Recipient
	Channel     ChannelConfig
	Destination string
}

func (c Configuration) Validate() error {
	if len(c.Recipients) == 0 {
		return diagnostics.New(diagnostics.CategoryInvalidConfig, "at least one recipient is required")
	}
	if len(c.Channels) == 0 {
		return diagnostics.New(diagnostics.CategoryInvalidConfig, "at least one channel config is required")
	}
	for id, recipient := range c.Recipients {
		if strings.TrimSpace(recipient.ID) == "" {
			recipient.ID = id
		}
		if err := recipient.Validate(); err != nil {
			return err
		}
	}
	for channel, config := range c.Channels {
		if strings.TrimSpace(config.Type) == "" {
			config.Type = channel
		}
		if err := config.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (c Configuration) Resolve(request Request) (ResolvedRequest, error) {
	if err := request.Validate(); err != nil {
		return ResolvedRequest{}, err
	}

	recipient, ok := c.Recipients[request.RecipientID]
	if !ok {
		return ResolvedRequest{}, diagnostics.New(diagnostics.CategoryMissingConfig, fmt.Sprintf("recipient %q is not configured", request.RecipientID))
	}

	channel, ok := c.Channels[request.Channel]
	if !ok {
		return ResolvedRequest{}, diagnostics.New(diagnostics.CategoryMissingConfig, fmt.Sprintf("channel %q is not configured", request.Channel))
	}
	if err := recipient.ValidateForChannel(request.Channel); err != nil {
		return ResolvedRequest{}, err
	}
	if err := channel.ValidateForDelivery(request.Channel); err != nil {
		return ResolvedRequest{}, err
	}

	destination, _ := recipient.DestinationFor(request.Channel)
	return ResolvedRequest{
		Request:     request,
		Recipient:   recipient,
		Channel:     channel,
		Destination: destination,
	}, nil
}

func (c Configuration) SecretValues() []string {
	var secrets []string
	for _, channel := range c.Channels {
		secrets = append(secrets, channel.SecretValues()...)
	}
	return secrets
}

type Attachment struct {
	Path        string
	Filename    string
	Size        int64
	ContentType string
}

func (a Attachment) Validate() error {
	if strings.TrimSpace(a.Path) == "" {
		return diagnostics.New(diagnostics.CategoryAttachmentError, "attachment path is required")
	}
	return nil
}

func (a Attachment) EffectiveFilename() string {
	if strings.TrimSpace(a.Filename) != "" {
		return a.Filename
	}
	return filepath.Base(a.Path)
}

type Recipient struct {
	ID             string
	Name           string
	Email          string
	TelegramChatID string
	SlackDest      string
	Enabled        bool
}

func (r Recipient) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return diagnostics.New(diagnostics.CategoryInvalidConfig, "recipient id is required")
	}
	return nil
}

func (r Recipient) ValidateForChannel(channel string) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if !r.Enabled {
		return diagnostics.New(diagnostics.CategoryInvalidConfig, fmt.Sprintf("recipient %q is disabled", r.ID))
	}
	if _, ok := r.DestinationFor(channel); !ok {
		return diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, channel, fmt.Sprintf("recipient %q has no configured destination", r.ID))
	}
	return nil
}

func (r Recipient) DestinationFor(channel string) (string, bool) {
	switch channel {
	case ChannelEmail:
		return strings.TrimSpace(r.Email), strings.TrimSpace(r.Email) != ""
	case ChannelTelegram:
		return strings.TrimSpace(r.TelegramChatID), strings.TrimSpace(r.TelegramChatID) != ""
	case ChannelSlack:
		return strings.TrimSpace(r.SlackDest), strings.TrimSpace(r.SlackDest) != ""
	default:
		return "", false
	}
}

type ChannelConfig struct {
	Type             string
	Enabled          bool
	Settings         map[string]string
	Secrets          map[string]string
	AttachmentPolicy AttachmentPolicy
}

func (c ChannelConfig) Validate() error {
	if !IsSupportedChannel(c.Type) {
		return diagnostics.New(diagnostics.CategoryInvalidConfig, fmt.Sprintf("unsupported channel config type %q", c.Type))
	}
	if c.Settings == nil {
		return diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, c.Type, "settings are required")
	}
	if c.Secrets == nil {
		return diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, c.Type, "secrets are required")
	}
	if !IsValidAttachmentPolicy(c.AttachmentPolicy) {
		return diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, c.Type, fmt.Sprintf("unsupported attachment policy %q", c.AttachmentPolicy))
	}
	return nil
}

func (c ChannelConfig) ValidateForDelivery(selectedChannel string) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.Type != selectedChannel {
		return diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, selectedChannel, fmt.Sprintf("channel config type %q does not match selected channel", c.Type))
	}
	if !c.Enabled {
		return diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, selectedChannel, "channel is disabled")
	}
	if len(c.Settings) == 0 {
		return diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, selectedChannel, "settings must not be empty")
	}
	if len(c.Secrets) == 0 {
		return diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, selectedChannel, "secrets must not be empty")
	}
	for _, key := range RequiredSecretKeys(selectedChannel) {
		if strings.TrimSpace(c.Secrets[key]) == "" {
			return diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, selectedChannel, fmt.Sprintf("required secret %q is missing", key))
		}
	}
	return nil
}

func (c ChannelConfig) SecretValues() []string {
	secrets := make([]string, 0, len(c.Secrets))
	for _, value := range c.Secrets {
		if strings.TrimSpace(value) != "" {
			secrets = append(secrets, value)
		}
	}
	return secrets
}

type Result struct {
	Success  bool
	ExitCode int
	Category ResultCategory
	Channel  string
	Message  string
	State    DeliveryState
	Redacted bool
}

func SuccessResult(channel, message string) Result {
	return Result{
		Success:  true,
		ExitCode: diagnostics.ExitSuccess,
		Category: ResultSuccess,
		Channel:  channel,
		Message:  message,
		State:    DeliveryStateSuccess,
	}
}

func FailureResult(category ResultCategory, channel, message string) Result {
	return Result{
		Success:  false,
		ExitCode: diagnostics.ExitCode(category),
		Category: category,
		Channel:  channel,
		Message:  message,
		State:    DeliveryStateFailure,
		Redacted: true,
	}
}

type ChannelSender interface {
	Name() string
	Send(ctx context.Context, request Request, recipient Recipient, config ChannelConfig) (Result, error)
}

func IsSupportedChannel(channel string) bool {
	switch channel {
	case ChannelEmail, ChannelSlack, ChannelTelegram:
		return true
	default:
		return false
	}
}

func IsValidAttachmentPolicy(policy AttachmentPolicy) bool {
	switch policy {
	case AttachmentPolicySupported, AttachmentPolicyLimited, AttachmentPolicyUnsupported:
		return true
	default:
		return false
	}
}

func RequiredSecretKeys(channel string) []string {
	switch channel {
	case ChannelEmail:
		return []string{"smtp_password"}
	case ChannelTelegram:
		return []string{"token"}
	case ChannelSlack:
		return []string{"webhook_url"}
	default:
		return nil
	}
}
