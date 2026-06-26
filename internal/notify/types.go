package notify

import "context"

const (
	ChannelEmail    = "email"
	ChannelSlack    = "slack"
	ChannelTelegram = "telegram"
)

type Request struct {
	ConfigPath  string
	RecipientID string
	Channel     string
	Title       string
	Message     string
	Attachments []Attachment
}

type Attachment struct {
	Path string
}

type Recipient struct {
	ID string
}

type ChannelConfig struct {
	Type     string
	Settings map[string]string
	Secrets  map[string]string
}

type Result struct {
	Success  bool
	ExitCode int
	Category string
	Channel  string
	Message  string
}

type ChannelSender interface {
	Name() string
	Send(ctx context.Context, request Request, recipient Recipient, config ChannelConfig) (Result, error)
}
