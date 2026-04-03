# Luminka Architecture

## Overview

This document defines the intended repository and runtime architecture for Luminka as the codebase moves from the current prototype into the framework-first structure described in the canon.

Luminka's implementation is organized around a small Go runtime, an in-repo TypeScript SDK, a starter scaffold, and example apps. The runtime is the bridge. The frontend remains the primary app layer.

The current architecture target is no longer a text-only request/response bridge. It is a stream-capable local runtime with portable and detached root policies, optional headless launch behavior, and packaging hooks for cross-platform desktop distribution.

## Scope Boundary

**Owns**: embedded asset serving, localhost runtime, binary WebSocket framing, stream session management, capability gating, filesystem bridge, script bridge, shell bridge, single-instance handling, root policy resolution, browser/webview shell launching, headless launch behavior, starter scaffold, example apps, TS SDK, packaging hooks for app icons

**Does not own**: frontend build pipelines, application-specific data schemas, remote networking, auth, cloud sync, npm distribution, PTY emulation

**Boundary interfaces**: embedded assets from app entrypoints, WebSocket transport to frontend, local OS file/process APIs underneath, platform packaging tools during build

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

  sdk/
    dist/
      luminka.js

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

The shared runtime package intentionally remains under `luminka/` so the public import path stays `github.com/lirrensi/luminka/luminka` while the repository root remains tidy and framework-oriented.

## Components

### 1. App Entrypoints

`starter/` and each example app own a small Go entrypoint.

Responsibilities:

- embed built frontend assets from `dist/`,
- define app-level runtime configuration,
- define default root policy and launch behavior,
- invoke the Luminka runtime,
- remain intentionally thin.

The app entrypoint is where app identity and embedded assets meet the shared runtime.

### 2. Runtime Core (`luminka/`)

The runtime core owns:

- startup orchestration,
- root policy resolution,
- launch behavior resolution,
- capability resolution,
- localhost server startup,
- transport lifecycle control,
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

### 4. Transport and Framing Layer

`ws.go` and adjacent transport code are responsible for:

- WebSocket connection management,
- parsing the binary frame envelope,
- decoding the JSON header,
- preserving request/response correlation,
- handing off payload-bearing frames to stream sessions,
- pushing notifications such as `fs_changed`.

The transport layer uses the canonical frame shape:

```text
[4-byte big-endian JSON header length][UTF-8 JSON header][payload bytes]
```

This layer owns framing. Higher layers own message semantics.

### 5. Stream Session Manager

The stream manager owns:

- stream ID creation,
- stream registration and cleanup,
- chunk ordering,
- EOF and terminal signaling,
- lane tagging such as file, stdout, and stderr,
- mapping transport events to active producers and consumers.

This is the architectural seam that allows filesystem bytes and live process output to share one underlying transport model.

### 6. Filesystem Bridge

`fs.go` owns path validation and allowed file operations relative to the resolved app root.

Responsibilities:

- sanitize and resolve paths,
- reject escaping paths,
- perform byte-oriented file access,
- provide text helpers layered over byte transfer,
- perform list/delete/exists,
- integrate with watch registration.

The filesystem code is always present in the runtime, but the frontend-facing filesystem capability is gated by configuration.

### 7. Script Bridge

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
- return stdout, stderr, and exit status for synchronous mode,
- optionally emit stdout/stderr through stream sessions in stream mode.

`script_other.go` provides a stub when script support is not compiled into the current build profile.

### 8. Shell Bridge

`shell.go` owns unrestricted execution.

Responsibilities:

- spawn the requested command directly,
- apply timeout handling,
- return stdout, stderr, and exit status for synchronous mode,
- optionally emit stdout/stderr through stream sessions in stream mode.

It MUST remain separate from the script bridge. No implicit fallback is allowed.

`shell_other.go` provides a stub when shell support is not compiled into the current build profile.

### 9. Lifecycle and Instance Management

`lock.go` and related lifecycle orchestration own:

- resolved-root calculation,
- single-instance detection,
- stale lock recovery,
- runtime-local artifact cleanup,
- instance state based on app name and resolved root.

The canonical state location is the resolved app root.

If a live instance is already present for the current resolved root, startup short-circuits and opens the preserved localhost URL in the default browser instead of starting a second server when that behavior is appropriate for the active display and launch mode.

### 10. Display Shells and Headless Mode

`browser.go` and `webview.go` are peer shell adapters.

Browser responsibilities:

- open the app URL in the default browser,
- cooperate with idle shutdown logic.

Webview responsibilities:

- open a native WebView window,
- own process lifetime through the window lifecycle.

Headless responsibilities inside runtime orchestration:

- bypass browser/webview shell opening,
- leave process lifetime tied to the foreground process,
- avoid browser idle timeout and webview window-close rules as primary lifetime controllers.

These are launch-behavior choices, not extra display profiles.

### 11. TypeScript SDK

`luminka/sdk/luminka.ts` is the ergonomic frontend layer.

Responsibilities:

- open and manage the WebSocket connection,
- encode and decode the binary frame envelope,
- provide promise-style control requests,
- expose Node-inspired text, binary, and stream helpers,
- wrap filesystem, script, shell, and app-info calls,
- hide request IDs and stream/session mechanics from normal app code,
- stay thin enough that direct protocol access remains possible.

The SDK is in-repo and first-class, but not a standalone npm product.

The TypeScript file remains the source of truth. The repository also owns a generated JavaScript distribution surface under `sdk/dist/` for consumers who want a ready-to-embed browser artifact without importing the TypeScript source directly.

Both consumption lanes are first-class:

- direct source consumption from `luminka/sdk/luminka.ts`,
- generated artifact consumption from `sdk/dist/*`.

### 12. Packaging Hooks

Build tooling is expected to support platform packaging resources, especially app icons.

Canonical direction:

- keep a single source icon asset or source icon set under repository control,
- generate the platform-specific packaging outputs from that source,
- support Windows, macOS, and Linux packaging targets through adapters,
- keep this as build architecture, not runtime architecture.

The exact toolchain MAY differ by platform, but the repo should converge on one canonical icon pipeline rather than ad hoc per-app scripts.

## Data Models / Storage

### Embedded Assets

Frontend assets are embedded into each app binary at build time.

They are immutable at runtime from Luminka's perspective.

### External App Root

The external app root is resolved at launch time.

By default this is the executable folder in portable mode or the current working directory in detached mode.

This location is used for:

- app data,
- scripts,
- logs,
- lock files,
- other app-local files.

### Lock State

The implementation uses a lock artifact representing instance ownership for the current resolved app root.

Reference format:

```text
<app_name>.lock => PID:port
```

Equivalent internal representation is acceptable if observable behavior remains the same.

### Connection and Stream State

The runtime tracks active WebSocket connections, watched paths, and open stream sessions in memory.

Idle behavior in browser mode depends on active connection count. Stream behavior depends on active stream state and chunk ordering.

## Relationships and Flow

### Startup Flow

```text
app entrypoint
  -> embed dist assets
  -> create Luminka config
  -> call runtime
      -> resolve root policy and launch overrides
      -> resolve effective app root
      -> acquire or validate instance lock
      -> resolve capabilities
      -> start localhost server
      -> expose /ws and static assets
      -> open browser or webview shell unless headless
      -> manage lifecycle until shutdown
```

### Frontend Capability Flow

```text
frontend code
  -> TS SDK or direct WS
  -> binary frame envelope
  -> transport/framing layer
  -> dispatcher / stream manager
  -> capability bridge
  -> local OS/file/process action
  -> structured header + optional payload bytes
```

### Filesystem Watch Flow

```text
frontend registers watch
  -> runtime stores watched path
  -> watch subsystem detects modification
  -> dispatcher pushes fs_changed
  -> frontend decides whether to re-read
```

### Streaming Process Flow

```text
frontend starts script/shell in stream mode
  -> runtime spawns process
  -> stream manager assigns stream_id
  -> stdout/stderr chunks emitted over transport
  -> terminal completion event returned
```

## Dependencies

### External Dependencies

Expected runtime dependencies include:

- Go standard library,
- Gorilla WebSocket or equivalent WebSocket implementation,
- optional WebView bindings for webview builds,
- platform packaging helpers for icons and app metadata where needed.

### Internal Dependencies

Key internal relationships:

- app entrypoints depend on `luminka/`,
- runtime core depends on transport, streams, filesystem, and execution bridges,
- display shells depend on server startup,
- SDK depends on the canonical transport framing and protocol.

## Contracts / Invariants

| Invariant | Description |
|---|---|
| Embedded frontend | App UI assets are embedded into the executable for normal operation. |
| Localhost-only runtime | Runtime interfaces are exposed only on loopback. |
| Single instance per resolved root | The same resolved app root must not create competing live instances. |
| Portable-first locality | Portable mode remains the default locality model unless overridden. |
| App-agnostic runtime | Luminka does not interpret application file schemas. |
| Strict capability separation | FS, script, and shell lanes stay distinct. No silent fallback. |
| Stream-first payload model | Payload-bearing operations use the shared stream transport model. |
| Capability truthfulness | Reported capabilities must match actual behavior. |
| Thin SDK | SDK improves DX without replacing the canonical protocol. |

## Configuration / Operations

### Runtime Configuration

The runtime configuration layer is expected to cover at least:

- app name,
- display mode,
- default root policy,
- explicit root override,
- headless launch flag,
- idle timeout,
- filesystem capability enable/disable,
- script capability enable/disable,
- shell capability enable/disable,
- execution timeout,
- stream and chunk sizing defaults.

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
- headless launches should die with the foreground process,
- stale locks should be recoverable,
- failures to open a shell or bind a port should fail fast and clearly,
- large payloads should flow through chunked streams rather than one-shot buffers.

## Design Decisions

| Decision | Why | Confidence |
|---|---|---|
| Framework-first repo | The product is Luminka, not the original kanban app | High |
| Starter plus examples | Supports clone-and-edit onboarding without making root an app | High |
| In-repo TS SDK | Best DX without turning the SDK into a separate package product | High |
| Portable-first with detached override | Preserves portable behavior while allowing one installed binary to serve many roots | High |
| Headless as launch behavior, not display mode | Keeps shell behavior separate from browser/webview identity | High |
| Binary frame envelope with JSON header | Keeps transport dependency-light while supporting raw payload bytes | High |
| Shared stream model for files and process output | Avoids separate transport designs for similar payload problems | High |
| Canonical cross-platform icon pipeline | Converges packaging behavior across Windows, macOS, and Linux | Medium |
| Strict script vs shell separation | Keeps capability semantics honest and predictable | High |

## Implementation Pointers

- Current runtime package: `luminka/*`
- Current SDK source of truth: `luminka/sdk/luminka.ts`
- Generated SDK distribution surface: `sdk/dist/*`
- Transitional architecture source: `agent_chat/plan_luminka_architecture_2026-03-30.md`

These pointers are informative only. The canon is this document plus the rest of `docs/`.
