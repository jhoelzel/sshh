package wailsnotification

import (
	"context"
	"errors"
	"sync"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"shh-h/internal/domain/notification"
)

type Gateway struct {
	mu          sync.RWMutex
	ctx         context.Context
	initialized bool
}

func New() *Gateway {
	return &Gateway{}
}

func (gateway *Gateway) Initialize(ctx context.Context) error {
	if ctx == nil {
		return errors.New("notification context is required")
	}
	if err := wailsruntime.InitializeNotifications(ctx); err != nil {
		return err
	}
	gateway.mu.Lock()
	gateway.ctx = ctx
	gateway.initialized = true
	gateway.mu.Unlock()
	return nil
}

func (gateway *Gateway) Cleanup() {
	gateway.mu.Lock()
	ctx := gateway.ctx
	initialized := gateway.initialized
	gateway.ctx = nil
	gateway.initialized = false
	gateway.mu.Unlock()
	if initialized {
		wailsruntime.CleanupNotifications(ctx)
	}
}

func (gateway *Gateway) Available() bool {
	ctx, initialized := gateway.context()
	return initialized && wailsruntime.IsNotificationAvailable(ctx)
}

func (gateway *Gateway) Authorized() (bool, error) {
	ctx, initialized := gateway.context()
	if !initialized {
		return false, errors.New("notifications are not initialized")
	}
	return wailsruntime.CheckNotificationAuthorization(ctx)
}

func (gateway *Gateway) RequestAuthorization() (bool, error) {
	ctx, initialized := gateway.context()
	if !initialized {
		return false, errors.New("notifications are not initialized")
	}
	return wailsruntime.RequestNotificationAuthorization(ctx)
}

func (gateway *Gateway) Send(message notification.Message) error {
	if err := message.Validate(); err != nil {
		return err
	}
	ctx, initialized := gateway.context()
	if !initialized {
		return errors.New("notifications are not initialized")
	}
	return wailsruntime.SendNotification(ctx, wailsruntime.NotificationOptions{
		ID: message.ID, Title: message.Title, Subtitle: message.Subtitle, Body: message.Body,
	})
}

func (gateway *Gateway) context() (context.Context, bool) {
	gateway.mu.RLock()
	defer gateway.mu.RUnlock()
	return gateway.ctx, gateway.initialized
}
