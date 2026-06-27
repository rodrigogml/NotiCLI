package app

import (
	"context"
	"fmt"

	"github.com/rodrigogml/NotiCLI/internal/diagnostics"
	"github.com/rodrigogml/NotiCLI/internal/notify"
)

type App struct {
	configuration notify.Configuration
	senders       map[string]notify.ChannelSender
}

func New(configuration notify.Configuration, senders ...notify.ChannelSender) App {
	app := App{
		configuration: configuration,
		senders:       make(map[string]notify.ChannelSender, len(senders)),
	}
	for _, sender := range senders {
		if sender == nil {
			continue
		}
		app.senders[sender.Name()] = sender
	}
	return app
}

func (a App) Notify(ctx context.Context, request notify.Request) (notify.Result, error) {
	resolved, err := a.configuration.Resolve(request)
	if err != nil {
		return resultFromError(request.Channel, err), err
	}
	attachments, err := notify.ValidateAttachments(resolved.Request, resolved.Channel)
	if err != nil {
		return resultFromError(request.Channel, err), err
	}
	resolved.Request.Attachments = attachments

	sender, ok := a.senders[request.Channel]
	if !ok {
		err := diagnostics.ForChannel(diagnostics.CategoryInternalError, request.Channel, fmt.Sprintf("no sender registered for channel %q", request.Channel))
		return resultFromError(request.Channel, err), err
	}

	result, err := sender.Send(ctx, resolved.Request, resolved.Recipient, resolved.Channel)
	if err != nil {
		return resultFromError(request.Channel, err), err
	}
	if result.Category == "" {
		err := diagnostics.ForChannel(diagnostics.CategoryInternalError, request.Channel, "sender returned an empty delivery result")
		return resultFromError(request.Channel, err), err
	}
	return result, nil
}

func resultFromError(channel string, err error) notify.Result {
	diagnostic := diagnostics.FromError(err)
	if diagnostic.Channel != "" {
		channel = diagnostic.Channel
	}
	return notify.FailureResult(diagnostic.Category, channel, diagnostic.Message)
}
