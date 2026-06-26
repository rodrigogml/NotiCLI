package slack

import (
	"context"

	"github.com/rodrigogml/NotiCLI/internal/notify"
)

type Sender struct{}

func (Sender) Name() string {
	return notify.ChannelSlack
}

func (Sender) Send(context.Context, notify.Request, notify.Recipient, notify.ChannelConfig) (notify.Result, error) {
	return notify.Result{}, nil
}
