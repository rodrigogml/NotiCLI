package notify_test

import (
	"testing"

	"github.com/rodrigogml/NotiCLI/internal/channels/email"
	"github.com/rodrigogml/NotiCLI/internal/channels/slack"
	"github.com/rodrigogml/NotiCLI/internal/channels/telegram"
	"github.com/rodrigogml/NotiCLI/internal/notify"
)

func TestChannelSendersExposeStableNames(t *testing.T) {
	tests := []struct {
		name   string
		sender notify.ChannelSender
		want   string
	}{
		{name: "email", sender: email.Sender{}, want: notify.ChannelEmail},
		{name: "slack", sender: slack.Sender{}, want: notify.ChannelSlack},
		{name: "telegram", sender: telegram.Sender{}, want: notify.ChannelTelegram},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sender.Name(); got != tt.want {
				t.Fatalf("Name() = %q, want %q", got, tt.want)
			}
		})
	}
}
