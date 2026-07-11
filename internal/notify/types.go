package notify

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
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
	Category     string
	Priority     string
	Title        string
	Message      string
	Attachments  []Attachment
}

const (
	PriorityHigh   = "HIGH"
	PriorityNormal = "NORMAL"
	PriorityLow    = "LOW"
)

func (r Request) Validate() error {
	if strings.TrimSpace(r.SenderSystem) == "" {
		return diagnostics.New(diagnostics.CategoryInvalidInput, "sender_system is required")
	}
	if len([]rune(strings.TrimSpace(r.SenderSystem))) > MaxSenderSystemLength {
		return diagnostics.New(diagnostics.CategoryInvalidInput, fmt.Sprintf("sender_system must be at most %d characters", MaxSenderSystemLength))
	}
	if !IsValidPriority(r.EffectivePriority()) {
		return diagnostics.New(diagnostics.CategoryInvalidInput, fmt.Sprintf("unsupported priority %q", r.Priority))
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

func (r Request) EffectivePriority() string {
	priority := strings.ToUpper(strings.TrimSpace(r.Priority))
	if priority == "" {
		return PriorityNormal
	}
	return priority
}

type Configuration struct {
	Destinations     map[string]Destination
	DeliveryAccounts map[string]DeliveryAccount
	Routes           []Route
	CatchAll         Route
	Logging          LoggingConfig
}

type ResolvedRequest struct {
	Request    Request
	Deliveries []ResolvedDelivery
}

type ResolvedDelivery struct {
	RouteID       string
	AccountID     string
	DestinationID string
	Account       DeliveryAccount
	Destination   Destination
}

func (c Configuration) Validate() error {
	if len(c.Destinations) == 0 {
		return diagnostics.New(diagnostics.CategoryInvalidConfig, "at least one destination is required")
	}
	if len(c.DeliveryAccounts) == 0 {
		return diagnostics.New(diagnostics.CategoryInvalidConfig, "at least one delivery account is required")
	}
	if len(c.CatchAll.Deliveries) == 0 {
		return diagnostics.New(diagnostics.CategoryInvalidConfig, "catch_all deliveries are required")
	}
	for id, destination := range c.Destinations {
		if strings.TrimSpace(destination.ID) == "" {
			destination.ID = id
		}
		if err := destination.Validate(); err != nil {
			return err
		}
	}
	for id, account := range c.DeliveryAccounts {
		if strings.TrimSpace(account.ID) == "" {
			account.ID = id
		}
		if err := account.Validate(); err != nil {
			return err
		}
	}
	for _, route := range append(c.Routes, c.CatchAll) {
		if err := c.validateRoute(route); err != nil {
			return err
		}
	}
	return nil
}

func (c Configuration) Resolve(request Request) (ResolvedRequest, error) {
	if err := request.Validate(); err != nil {
		return ResolvedRequest{}, err
	}

	matched := make([]Route, 0, len(c.Routes))
	for _, route := range c.Routes {
		if route.Matches(request) {
			matched = append(matched, route)
		}
	}
	if len(matched) == 0 {
		matched = append(matched, c.CatchAll)
	}

	deliveries := make([]ResolvedDelivery, 0)
	for _, route := range matched {
		for _, delivery := range route.Deliveries {
			account, ok := c.DeliveryAccounts[delivery.Account]
			if !ok {
				return ResolvedRequest{}, diagnostics.New(diagnostics.CategoryMissingConfig, fmt.Sprintf("delivery account %q is not configured", delivery.Account))
			}
			destination, ok := c.Destinations[delivery.Destination]
			if !ok {
				return ResolvedRequest{}, diagnostics.New(diagnostics.CategoryMissingConfig, fmt.Sprintf("destination %q is not configured", delivery.Destination))
			}
			if err := account.ValidateForDelivery(destination.Type); err != nil {
				return ResolvedRequest{}, err
			}
			if err := destination.ValidateForAccount(account.Type); err != nil {
				return ResolvedRequest{}, err
			}
			deliveries = append(deliveries, ResolvedDelivery{
				RouteID:       route.ID,
				AccountID:     delivery.Account,
				DestinationID: delivery.Destination,
				Account:       account,
				Destination:   destination,
			})
		}
	}

	return ResolvedRequest{
		Request:    request,
		Deliveries: deliveries,
	}, nil
}

func (c Configuration) SecretValues() []string {
	var secrets []string
	for _, account := range c.DeliveryAccounts {
		secrets = append(secrets, account.SecretValues()...)
	}
	return secrets
}

func (c Configuration) validateRoute(route Route) error {
	if len(route.Deliveries) == 0 {
		return diagnostics.New(diagnostics.CategoryInvalidConfig, fmt.Sprintf("route %q deliveries are required", route.ID))
	}
	for _, priority := range route.Match.Priorities {
		if !IsValidPriority(priority) {
			return diagnostics.New(diagnostics.CategoryInvalidConfig, fmt.Sprintf("route %q has unsupported priority %q", route.ID, priority))
		}
	}
	for _, delivery := range route.Deliveries {
		account, ok := c.DeliveryAccounts[delivery.Account]
		if !ok {
			return diagnostics.New(diagnostics.CategoryInvalidConfig, fmt.Sprintf("route %q references unknown delivery account %q", route.ID, delivery.Account))
		}
		destination, ok := c.Destinations[delivery.Destination]
		if !ok {
			return diagnostics.New(diagnostics.CategoryInvalidConfig, fmt.Sprintf("route %q references unknown destination %q", route.ID, delivery.Destination))
		}
		if account.Type != destination.Type {
			return diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, destination.Type, fmt.Sprintf("route %q uses %s account %q for %s destination %q", route.ID, account.Type, delivery.Account, destination.Type, delivery.Destination))
		}
	}
	return nil
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

func ValidateAttachments(request Request) ([]Attachment, error) {
	if len(request.Attachments) == 0 {
		return nil, nil
	}

	attachments := make([]Attachment, 0, len(request.Attachments))
	for _, attachment := range request.Attachments {
		validated, err := validateAttachment(attachment)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, validated)
	}
	return attachments, nil
}

func validateAttachment(attachment Attachment) (Attachment, error) {
	if err := attachment.Validate(); err != nil {
		return Attachment{}, err
	}

	path := strings.TrimSpace(attachment.Path)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Attachment{}, diagnostics.New(diagnostics.CategoryAttachmentError, fmt.Sprintf("attachment not found: %s", path))
		}
		return Attachment{}, diagnostics.New(diagnostics.CategoryAttachmentError, fmt.Sprintf("attachment cannot be read: %s", path))
	}
	if info.IsDir() {
		return Attachment{}, diagnostics.New(diagnostics.CategoryAttachmentError, fmt.Sprintf("attachment is a directory: %s", path))
	}

	file, err := os.Open(path)
	if err != nil {
		return Attachment{}, diagnostics.New(diagnostics.CategoryAttachmentError, fmt.Sprintf("attachment cannot be read: %s", path))
	}
	defer file.Close()

	contentType := strings.TrimSpace(attachment.ContentType)
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(path))
	}
	if contentType == "" {
		buffer := make([]byte, 512)
		n, _ := io.ReadFull(file, buffer)
		contentType = http.DetectContentType(buffer[:n])
	}

	attachment.Path = path
	attachment.Size = info.Size()
	attachment.ContentType = contentType
	if strings.TrimSpace(attachment.Filename) == "" {
		attachment.Filename = filepath.Base(path)
	}
	return attachment, nil
}

type Destination struct {
	ID                       string
	Name                     string
	Type                     string
	Email                    string
	TelegramChatID           string
	TelegramDeliveryMode     string
	TelegramTopicGroupChatID string
	TelegramTopicGroupName   string
	MessageThreadID          int
	SlackDest                string
	Enabled                  bool
}

const (
	TelegramDeliveryModePrivate = "private"
	TelegramDeliveryModeTopics  = "topics"
	TelegramDeliveryModeThread  = "thread"
)

func (d Destination) Validate() error {
	if strings.TrimSpace(d.ID) == "" {
		return diagnostics.New(diagnostics.CategoryInvalidConfig, "destination id is required")
	}
	if !IsSupportedChannel(d.Type) {
		return diagnostics.New(diagnostics.CategoryInvalidConfig, fmt.Sprintf("unsupported destination type %q", d.Type))
	}
	if !d.Enabled {
		return diagnostics.New(diagnostics.CategoryInvalidConfig, fmt.Sprintf("destination %q is disabled", d.ID))
	}
	if d.Type == ChannelTelegram && !d.hasValidTelegramDeliveryMode() {
		return diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, ChannelTelegram, fmt.Sprintf("destination %q has unsupported telegram_delivery_mode %q", d.ID, d.TelegramDeliveryMode))
	}
	if _, ok := d.Address(); !ok {
		return diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, d.Type, fmt.Sprintf("destination %q has no configured address", d.ID))
	}
	if d.EffectiveTelegramDeliveryMode() == TelegramDeliveryModeThread && d.MessageThreadID <= 0 {
		return diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, ChannelTelegram, fmt.Sprintf("destination %q uses thread mode without message_thread_id", d.ID))
	}
	return nil
}

func (d Destination) ValidateForAccount(accountType string) error {
	if err := d.Validate(); err != nil {
		return err
	}
	if d.Type != accountType {
		return diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, accountType, fmt.Sprintf("destination type %q does not match delivery account type", d.Type))
	}
	return nil
}

func (d Destination) Address() (string, bool) {
	switch d.Type {
	case ChannelEmail:
		return strings.TrimSpace(d.Email), strings.TrimSpace(d.Email) != ""
	case ChannelTelegram:
		switch d.EffectiveTelegramDeliveryMode() {
		case TelegramDeliveryModePrivate:
			return strings.TrimSpace(d.TelegramChatID), strings.TrimSpace(d.TelegramChatID) != ""
		case TelegramDeliveryModeTopics, TelegramDeliveryModeThread:
			destination := strings.TrimSpace(d.TelegramTopicGroupChatID)
			return destination, destination != ""
		default:
			return "", false
		}
	case ChannelSlack:
		return strings.TrimSpace(d.SlackDest), strings.TrimSpace(d.SlackDest) != ""
	default:
		return "", false
	}
}

func (d Destination) EffectiveTelegramDeliveryMode() string {
	mode := strings.TrimSpace(d.TelegramDeliveryMode)
	if mode == "" {
		return TelegramDeliveryModePrivate
	}
	return mode
}

func (d Destination) hasValidTelegramDeliveryMode() bool {
	switch d.EffectiveTelegramDeliveryMode() {
	case TelegramDeliveryModePrivate, TelegramDeliveryModeTopics, TelegramDeliveryModeThread:
		return true
	default:
		return false
	}
}

type DeliveryAccount struct {
	ID               string
	Type             string
	Enabled          bool
	Settings         map[string]string
	Secrets          map[string]string
	AttachmentPolicy AttachmentPolicy
}

func (c DeliveryAccount) Validate() error {
	if strings.TrimSpace(c.ID) == "" {
		return diagnostics.New(diagnostics.CategoryInvalidConfig, "delivery account id is required")
	}
	if !IsSupportedChannel(c.Type) {
		return diagnostics.New(diagnostics.CategoryInvalidConfig, fmt.Sprintf("unsupported delivery account type %q", c.Type))
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

func (c DeliveryAccount) ValidateForDelivery(selectedChannel string) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.Type != selectedChannel {
		return diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, selectedChannel, fmt.Sprintf("delivery account type %q does not match destination type", c.Type))
	}
	if !c.Enabled {
		return diagnostics.ForChannel(diagnostics.CategoryInvalidConfig, selectedChannel, "delivery account is disabled")
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

func (c DeliveryAccount) SecretValues() []string {
	secrets := make([]string, 0, len(c.Secrets))
	for _, value := range c.Secrets {
		if strings.TrimSpace(value) != "" {
			secrets = append(secrets, value)
		}
	}
	return secrets
}

type Route struct {
	ID         string
	Match      RouteMatch
	Deliveries []Delivery
}

func (r Route) Matches(request Request) bool {
	return matchAny(r.Match.Senders, request.SenderSystem) &&
		matchAny(r.Match.Categories, request.Category) &&
		matchAny(r.Match.Priorities, request.EffectivePriority())
}

type RouteMatch struct {
	Senders    []string
	Categories []string
	Priorities []string
}

type Delivery struct {
	Account     string
	Destination string
}

type LoggingConfig struct {
	Path string
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
	Send(ctx context.Context, request Request, delivery ResolvedDelivery) (Result, error)
}

func IsSupportedChannel(channel string) bool {
	switch channel {
	case ChannelEmail, ChannelSlack, ChannelTelegram:
		return true
	default:
		return false
	}
}

func IsValidPriority(priority string) bool {
	switch strings.ToUpper(strings.TrimSpace(priority)) {
	case PriorityHigh, PriorityNormal, PriorityLow:
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

func matchAny(criteria []string, value string) bool {
	if len(criteria) == 0 {
		return true
	}
	value = strings.TrimSpace(value)
	for _, criterion := range criteria {
		if strings.TrimSpace(criterion) == value {
			return true
		}
	}
	return false
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
