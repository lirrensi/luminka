# ADR-0003: Extension API for Custom Events

## Status

Accepted

## Context

Luminka's runtime has a hardcoded `switch` statement in `handleWebSocketSession` that dispatches every known event to its handler. Unknown events hit a `default` case and are rejected with `"unknown event"`. There is no way for an app developer importing `github.com/lirrensi/luminka/luminka` into their own Go application to add custom event handlers without forking the source.

This is a common need. A kanban app might want `goal:set` and `goal:list` events. A database tool might want `db:query`. A local AI tool might want `ai:complete`. Currently all of these require modifying Luminka's source.

ADR-0002 establishes the `namespace:action` naming convention. Custom events now have a natural slot (`goal:set`, `db:query`). The remaining gap is a registration mechanism.

## Decision

Luminka will add a **handler registration API** on the `Runtime` type. App developers can register handlers for custom events before calling `Run()`. Custom handlers run before the built-in dispatch, enabling both new events and overrides of existing events.

### 1. Exported types

Two currently-unexported types must be exported so handler authors can reference them:

```go
// WSMessage — the full request/response message (was wsMessage)
type WSMessage struct {
    Event   string          `json:"event"`
    ID      json.RawMessage `json:"id,omitempty"`
    Ok      *bool           `json:"ok,omitempty"`
    Error   string          `json:"error,omitempty"`
    Data    json.RawMessage `json:"data,omitempty"`
    // ... all existing fields ...
    Payload []byte          `json:"-"` // raw binary after JSON header
}

// WSConnection — a connected WebSocket client (was wsConnection)
type WSConnection struct { ... }
```

Response helpers are also exported:

```go
func (conn *WSConnection) WriteMessage(msg WSMessage) error
func (conn *WSConnection) WriteFrame(msg WSMessage, payload []byte) error
func (conn *WSConnection) WriteError(id json.RawMessage, message string) error
```

### 2. Handler type

```go
type HandlerFunc func(conn *WSConnection, msg *WSMessage) error
```

The handler receives the full message (JSON header fields + raw payload). It is responsible for writing its own response (or delegating to fallthrough). The handler signature uses the exported types so callers can construct and reference them.

### 3. Registration API

One function on `Runtime`:

```go
func (rt *Runtime) RegisterHandler(event string, handler HandlerFunc, opts ...RegisterOption)

type RegisterOption func(*registerConfig)
func WithOverride() RegisterOption
```

- `RegisterHandler("goal:set", handler)` — adds a custom handler. If the event matches a built-in event, the custom handler fires first. If it returns `nil`, the built-in handler does NOT run (custom handler took responsibility). If the custom handler wants to fall through to the built-in, it returns a sentinel `ErrUnhandled`.
- `RegisterHandler("file:read", handler, WithOverride())` — same mechanism but carries explicit intent that the caller is replacing a built-in handler. The option is purely a documentation signal; the dispatch mechanics are identical.

The handler map lives on `Runtime`. Registration must happen before `Run()` is called (or at least before the first WebSocket connection is established).

### 4. Dispatch order

```
Incoming message
  → Check registered handlers (custom first, then override-registered)
    → Handler returns nil → done (handler took responsibility)
    → Handler returns ErrUnhandled → fall through
  → Built-in switch (existing dispatch for known events)
  → Unknown event → response:error
```

`ErrUnhandled` is a sentinel error:

```go
var ErrUnhandled = errors.New("event not handled by custom handler")
```

### 5. SDK-side API

Two methods on `LuminkaClient`:

```ts
// Send an event and await the response (synchronous-style)
async call(event: string, data?: Record<string, unknown>, payload?: Uint8Array): Promise<WSMessage>

// Listen for server-pushed events (no request ID)
onEvent(event: string, listener: (msg: WSMessage) => void): () => void
```

`call()` wraps `request()` — generates the ID internally, sends the frame, waits for `response:<event>`, resolves the promise. This is the primary API for custom events.

```ts
// Usage
const resp = await client.call("goal:set", { goal: "ship it" });
if (resp.ok) { ... }

client.onEvent("goal:changed", (msg) => { ... });
```

`registerEvent()` can be added later as a typed wrapper around `call()`.

### 6. `app:info` capability reporting

`RegisterHandler` does NOT automatically add the event to `app:info` capabilities. The capability object remains `{fs, scripts, shell}`. Custom event discovery is left to the application layer (the app knows what it registered).

## Consequences

### Positive

- App developers can extend Luminka without forking.
- The namespace convention (ADR-0002) gives custom events a clean home.
- Override support enables virtual filesystems, custom script runners, etc.
- The SDK `call()` / `onEvent()` API gives frontend code a clean way to invoke custom events.
- Exported `WSConnection`/`WSMessage` types enable proper typed handler code outside the package.

### Negative

- Previously-unexported types become part of the public API surface. Changes to `WSMessage` or `WSConnection` become breaking changes.
- The `Payload` field on `WSMessage` is raw bytes — handlers must know what they're getting.
- No capability introspection for custom events (at least in v1).

## Follow-on work

1. Export `wsMessage` → `WSMessage` and `wsConnection` → `WSConnection`.
2. Export response helpers as methods on `WSConnection`.
3. Add `HandlerFunc`, `RegisterHandler`, `RegisterOption`, `WithOverride`, `ErrUnhandled`.
4. Modify the main dispatch loop in `handleWebSocketSession` to check registered handlers before the built-in switch.
5. Add `call()` and `onEvent()` to the TypeScript SDK.
6. Update `docs/spec.md` with the extension API contract.
7. Update `docs/arch.md` with the new types and dispatch flow.
