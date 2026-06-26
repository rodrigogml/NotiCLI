package app

import (
	"context"

	"github.com/rodrigogml/NotiCLI/internal/notify"
)

type Dispatcher interface {
	Dispatch(ctx context.Context, request notify.Request) (notify.Result, error)
}

type App struct {
	dispatcher Dispatcher
}

func New(dispatcher Dispatcher) App {
	return App{dispatcher: dispatcher}
}

func (a App) Notify(ctx context.Context, request notify.Request) (notify.Result, error) {
	return a.dispatcher.Dispatch(ctx, request)
}
