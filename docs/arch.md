# Luminka Architecture

## Overview

This document defines the intended repository and runtime architecture for Luminka as the codebase moves from the current PortableKanban prototype to the framework-first structure described in the canon.

Luminka's implementation is organized around a small Go runtime, an in-repo TypeScript SDK, a starter scaffold, and example apps. The runtime is the bridge. The frontend remains the primary app layer.

## Scope Boundary

**Owns**: embedded asset serving, localhost runtime, WebSocket transport, capability gating, filesystem bridge, script bridge, shell bridge, single-instance handling, browser/webview shell launching, starter scaffold, example apps, TS SDK

**Does not own**: frontend build pipelines, application-specific data schemas, remote networking, auth, cloud sync, npm distribution

**Boundary interfaces**: embedded assets from app entrypoints, WebSocket protocol to frontend, local OS file/process APIs underneath

## Repository Shape

The target repository shape is:

```text
/
  README.md
  docs/
    product.md
    glossary.md
    spec.md
    arch.md

  luminka/
    app.go
    server.go
    ws.go
    lock.go
    watch.go
    fs.go
    script.go
    script_other.go
    shell.go
    shell_other.go
    browser.go
    webview.go
    webview_other.go
    sdk/
      luminka.ts

  starter/
    main.go
    dist/

  examples/
    hello/
      main.go
      dist/
    kanban/
      main.go
      dist/
```

There SHOULD be no application Go entrypoint at repository root.

## Components

### 1. App Entrypoints

`starter/` and each example app own a small Go entrypoint.

Responsibilities:

- embed built frontend assets from `dist/`,
- define app-level runtime configuration,
- invoke the Luminka runtime,
- remain intentionally thin.

The app entrypoint is where app identity and embedded assets meet the shared runtime.

### 2. Runtime Core (`luminka/`)

The runtime core owns:

- startup orchestration,
- capability resolution,
- localhost server startup,
- WebSocket dispatch,
- lifecycle control,
- shutdown cleanup.

It MUST remain app-agnostic. It does not know application file schemas.

### 3. Asset Serving Layer

The runtime serves assets provided by the app entrypoint. It does not build frontend assets itself.

Implementation direction:

- app entrypoint embeds `dist/*`,
- app entrypoint passes embedded assets into Luminka,
- Luminka serves them via a local HTTP layer for browser/webview loading,
- the handler prefers exact file matches first, then serves the embedded `index.html` entry document for unknown `GET` and `HEAD` routes,
- real assets always win over the entry-document fallback.

### 4. WebSocket Dispatcher

`ws.go` is responsible for:

- connection management,
- request parsing,
- request/response correlation,
- dispatch to capability bridges,
- push notifications such as `fs_changed`.

The dispatcher is the central routing boundary between frontend protocol and local runtime behavior.

### 5. Filesystem Bridge

`fs.go` owns path validation and allowed file operations relative to the resolved app root.

Responsibilities:

- sanitize and resolve paths,
- reject escaping paths,
- perform read/write/list/delete/exists,
- integrate with watch registration.

The filesystem code is always present in the runtime, but the frontend-facing filesystem capability is gated by configuration.

### 6. Script Bridge

`script.go` owns constrained execution.

Responsibilities:

- accept the `runner` + `file` + `args` request model,
- resolve `file` as either an external path under the app root or an `internal:` selector into the embedded script bundle,
- validate external script paths against the app root,
- resolve internal scripts from the embedded `scripts/` tree without requiring startup extraction,
- materialize internal scripts to a temporary local file only when execution needs a real path,
- invoke the runner against the validated script path,
- append provided args after the validated file without additional semantic validation,
- execute with timeout,
- return stdout, stderr, and exit status.

`script_other.go` provides a stub when script support is not compiled into the current build profile.

### 7. Shell Bridge

`shell.go` owns unrestricted execution.

Responsibilities:

- spawn the requested command directly,
- apply timeout handling,
- return stdout, stderr, and exit status.

It MUST remain separate from the script bridge. No implicit fallback is allowed.

`shell_other.go` provides a stub when shell support is not compiled into the current build profile.

### 8. Lifecycle and Instance Management

`lock.go` and related lifecycle orchestration own:

- single-instance detection,
- stale lock recovery,
- runtime-local artifact cleanup,
- instance state based on app name and root.

The canonical state location is the binary folder.

If a live instance is already present for the current app folder, startup short-circuits and opens the preserved localhost URL in the default browser instead of starting a second server.

### 9. Display Shells

`browser.go` and `webview.go` are peer shell adapters.

Browser responsibilities:

- open the app URL in the default browser,
- cooperate with idle shutdown logic.

Webview responsibilities:

- open a native WebView window,
- own process lifetime through the window lifecycle.

These are equal product modes, not primary and fallback modes.

### 10. TypeScript SDK

`luminka/sdk/luminka.ts` is the ergonomic frontend layer.

Responsibilities:

- open and manage the WebSocket connection,
- provide promise-style request/response helpers,
- wrap filesystem, script, shell, and app-info calls,
- hide request IDs and event correlation from normal app code,
- stay thin enough that direct protocol access remains possible.

The SDK is in-repo and first-class, but not a standalone npm product.

## Data Models / Storage

### Embedded Assets

Frontend assets are embedded into each app binary at build time.

They are immutable at runtime from Luminka's perspective.

### External App Root

The default external app root is the binary folder.

This location is used for:

- app data,
- scripts,
- logs,
- lock files,
- other app-local files.

### Lock State

The implementation uses a lock artifact representing instance ownership for the current app root.

Reference format:

```text
<app_name>.lock => PID:port
```

Equivalent internal representation is acceptable if observable behavior remains the same.

### Connection State

The runtime tracks active WebSocket connections and watched paths in memory.

Idle behavior in browser mode depends on active connection count.

## Relationships and Flow

### Startup Flow

```text
app entrypoint
  -> embed dist assets
  -> create Luminka config
  -> call runtime
      -> resolve root to binary folder
      -> acquire or validate instance lock
      -> resolve capabilities
      -> start localhost server
      -> expose /ws and static assets
      -> open browser or webview shell
      -> manage lifecycle until shutdown
```

### Frontend Capability Flow

```text
frontend code
  -> TS SDK or direct WS
  -> /ws
  -> dispatcher
  -> capability bridge
  -> local OS/file/process action
  -> structured response
```

### Filesystem Watch Flow

```text
frontend registers watch
  -> runtime stores watched path
  -> watch subsystem detects modification
  -> dispatcher pushes fs_changed
  -> frontend decides whether to re-read
```

## Dependencies

### External Dependencies

Expected runtime dependencies include:

- Go standard library,
- Gorilla WebSocket or equivalent WebSocket implementation,
- optional WebView bindings for webview builds.

### Internal Dependencies

Key internal relationships:

- app entrypoints depend on `luminka/`,
- runtime core depends on filesystem and execution bridges,
- display shells depend on server startup,
- SDK depends on the canonical WebSocket protocol.

## Contracts / Invariants

| Invariant | Description |
|---|---|
| Embedded frontend | App UI assets are embedded into the executable for normal operation. |
| Localhost-only runtime | Runtime interfaces are exposed only on loopback. |
| Single instance per app folder | The same app folder must not create competing live instances. |
| Binary-folder default root | External state defaults to the binary folder. |
| App-agnostic runtime | Luminka does not interpret application file schemas. |
| Strict capability separation | FS, script, and shell lanes stay distinct. No silent fallback. |
| Capability truthfulness | Reported capabilities must match actual behavior. |
| Thin SDK | SDK improves DX without replacing the canonical protocol. |

## Configuration / Operations

### Runtime Configuration

The runtime configuration layer is expected to cover at least:

- app name,
- display mode,
- root override,
- idle timeout,
- filesystem capability enable/disable,
- script capability enable/disable,
- shell capability enable/disable,
- execution timeout.

### Build Profiles

The reference Go implementation is expected to use build profiles or tags to produce different binaries, especially for:

- browser vs webview builds,
- script support,
- shell support.

Filesystem support remains compiled into the runtime, even when frontend filesystem capability is disabled.

### Operations

Operational expectations:

- browser builds should cleanly auto-exit after idle timeout,
- webview builds should cleanly exit on window close,
- stale locks should be recoverable,
- failures to open a shell or bind a port should fail fast and clearly.

## Design Decisions

| Decision | Why | Confidence |
|---|---|---|
| Framework-first repo | The product is Luminka, not the original kanban app | High |
| Starter plus examples | Supports clone-and-edit onboarding without making root an app | High |
| In-repo TS SDK | Best DX without turning the SDK into a separate package product | High |
| Binary-folder default root | Preserves portable app behavior and predictable locality | High |
| Browser and webview parity | Both are first-class delivery modes | High |
| Filesystem compiled in, API gateable | Keeps implementation simple while allowing tighter exposed capability sets | High |
| Strict script vs shell separation | Keeps capability semantics honest and predictable | High |

## Implementation Pointers

- Current prototype to replace: `main.go` at repository root
- Current prototype frontend: `static/*`
- Transitional architecture source: `agent_chat/plan_luminka_architecture_2026-03-30.md`

These pointers are informative only. The canon is this document plus the rest of `docs/`.
