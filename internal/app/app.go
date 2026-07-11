package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
		return resultFromError("", err), err
	}
	attachments, err := notify.ValidateAttachments(resolved.Request)
	if err != nil {
		return resultFromError("", err), err
	}
	resolved.Request.Attachments = attachments

	var firstErr error
	var firstResult notify.Result
	successes := 0
	for _, delivery := range resolved.Deliveries {
		deliveryRequest := resolved.Request
		if len(deliveryRequest.Attachments) > 0 && delivery.Account.AttachmentPolicy == notify.AttachmentPolicyUnsupported {
			deliveryRequest.Attachments = nil
			a.logDeliveryEvent(resolved.Request, delivery, diagnostics.CategoryAttachmentError, "attachments omitted for unsupported delivery account")
		}

		sender, ok := a.senders[delivery.Account.Type]
		if !ok {
			err := diagnostics.ForChannel(diagnostics.CategoryInternalError, delivery.Account.Type, fmt.Sprintf("no sender registered for channel %q", delivery.Account.Type))
			result := resultFromError(delivery.Account.Type, err)
			a.captureFailure(resolved.Request, delivery, result, err)
			if firstErr == nil {
				firstErr = err
				firstResult = result
			}
			continue
		}

		result, err := sender.Send(ctx, deliveryRequest, delivery)
		if err != nil {
			result = resultFromError(delivery.Account.Type, err)
			a.captureFailure(resolved.Request, delivery, result, err)
			if firstErr == nil {
				firstErr = err
				firstResult = result
			}
			continue
		}
		if result.Category == "" {
			err := diagnostics.ForChannel(diagnostics.CategoryInternalError, delivery.Account.Type, "sender returned an empty delivery result")
			result = resultFromError(delivery.Account.Type, err)
			a.captureFailure(resolved.Request, delivery, result, err)
			if firstErr == nil {
				firstErr = err
				firstResult = result
			}
			continue
		}
		if !result.Success {
			err := diagnostics.ForChannel(result.Category, result.Channel, result.Message)
			a.captureFailure(resolved.Request, delivery, result, err)
			if firstErr == nil {
				firstErr = err
				firstResult = result
			}
			continue
		}
		successes++
	}
	if firstErr != nil {
		if firstResult.Message == "" {
			firstResult = resultFromError("", firstErr)
		}
		if successes > 0 {
			firstResult.Message = fmt.Sprintf("partial delivery failure: %s", firstResult.Message)
			return firstResult, diagnostics.ForChannel(firstResult.Category, firstResult.Channel, firstResult.Message)
		}
		return firstResult, firstErr
	}
	return notify.SuccessResult("", "notification accepted"), nil
}

func resultFromError(channel string, err error) notify.Result {
	diagnostic := diagnostics.FromError(err)
	if diagnostic.Channel != "" {
		channel = diagnostic.Channel
	}
	return notify.FailureResult(diagnostic.Category, channel, diagnostic.Message)
}

func (a App) captureFailure(request notify.Request, delivery notify.ResolvedDelivery, result notify.Result, err error) {
	category := result.Category
	message := result.Message
	if err != nil {
		diagnostic := diagnostics.FromError(err)
		category = diagnostic.Category
		message = diagnostic.Message
	}
	a.logDeliveryEvent(request, delivery, category, message)
}

func (a App) logDeliveryEvent(request notify.Request, delivery notify.ResolvedDelivery, category diagnostics.Category, message string) {
	path := strings.TrimSpace(a.configuration.Logging.Path)
	if path == "" {
		return
	}
	redactor := diagnostics.NewRedactor(a.configuration.SecretValues()...)
	event := map[string]any{
		"timestamp":      time.Now().UTC().Format(time.RFC3339Nano),
		"sender":         request.SenderSystem,
		"category":       request.Category,
		"priority":       request.EffectivePriority(),
		"route_id":       delivery.RouteID,
		"destination_id": delivery.DestinationID,
		"account_id":     delivery.AccountID,
		"channel_type":   delivery.Account.Type,
		"error_category": string(category),
		"message":        redactor.Redact(message),
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.Write(append(data, '\n'))
}
