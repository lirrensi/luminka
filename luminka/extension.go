// FILE: luminka/extension.go
// PURPOSE: Define the Extension API types for custom event handler registration.
// OWNS: HandlerFunc, RegisterHandler, and associated types.
// EXPORTS: ErrUnhandled, HandlerFunc, RegisterOption, WithOverride
// DOCS: docs/spec.md

package luminka

import (
	"errors"
)

// ErrUnhandled is returned by a registered handler to signal that it did not
// handle the event and the built-in dispatch should take over.
var ErrUnhandled = errors.New("event not handled by custom handler")

// HandlerFunc is the signature for custom event handlers.
type HandlerFunc func(conn *WSConnection, msg *WSMessage) error

type registerConfig struct {
	override bool
}

// RegisterOption modifies the behavior of RegisterHandler.
type RegisterOption func(*registerConfig)

// WithOverride signals that the handler intentionally replaces a built-in handler.
func WithOverride() RegisterOption {
	return func(c *registerConfig) {
		c.override = true
	}
}

type handlerEntry struct {
	handler  HandlerFunc
	override bool
}

// RegisterHandler registers a custom handler for the given event.
// The handler runs before the built-in dispatch. If it returns nil,
// the built-in handler is skipped. If it returns ErrUnhandled, the
// built-in dispatch proceeds normally.
func (rt *Runtime) RegisterHandler(event string, handler HandlerFunc, opts ...RegisterOption) {
	cfg := &registerConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	rt.handlerMu.Lock()
	defer rt.handlerMu.Unlock()
	if rt.customHandlers == nil {
		rt.customHandlers = make(map[string]handlerEntry)
	}
	rt.customHandlers[event] = handlerEntry{handler: handler, override: cfg.override}
}
