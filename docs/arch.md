# Luminka Architecture

## Overview

This document defines the intended repository and runtime architecture for Luminka as the codebase moves from the current prototype into the framework-first structure described in the canon.

Luminka's implementation is organized around a small Go runtime, an in-repo TypeScript SDK, a starter scaffold, and example apps. The runtime is the bridge. The frontend remains the primary app layer.

The current architecture target is no longer a text-only request/response bridge. It is a stream-capable local runtime with portable and detached root policies, optional headless launch behavior, and packaging hooks for cross-platform desktop distribution.

## Scope Boundary

**Owns**: embedded asset serving, localhost runtime, binary WebSocket framing, stream session management, capability gating, filesystem bridge, script bridge, shell bridge, single-instance handling, native window focus, runtime broadcast events, root policy resolution, browser/webview shell launching, headless launch behavior, starter scaffold, example apps, TS SDK including tracked text file helpers and multi-tab coordination helpers, packaging hooks for app icons, install script templates, platform packaging subcommands (zip, tar, deb, appdir)

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
    installation.md

  cmd/
    build/
      main.go
      package_zip.go
      package_tar.go
      package_deb.go
      package_appdir.go

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

  scripts/
    install/
      install-path.sh
      install-path.ps1
      install-portable.sh
      install-portable.ps1
      install-home.sh
      install-home.ps1
      uninstall.sh
      uninstall.ps1

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
- pushing notifications such as `fs_changed`,
- routing transient `broadcast` events between connected frontend clients in the same runtime instance.

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

`fs.go` owns path validation and all file operations relative to the resolved app root. The implementation targets practical parity with `node:fs/promises` (see [ADR-0001](adr/0001-fs-promises-parity.md)).

Responsibilities:

- sanitize and resolve paths,
- reject escaping paths (absolute, parent traversal, symlink escape),
- perform byte-oriented and text-oriented file access,
- directory lifecycle: create (`Mkdir`, `MkdirAll`), list (`ReadDir`), remove empty (`Rmdir`), recursive remove (`RemoveAll`, `Remove`),
- temporary directory creation (`Mkdtemp`),
- file lifecycle: read (`ReadBytes`, `Read`), write (`WriteBytes`, `Write`), append (`AppendFile`), truncate (`Truncate`), delete/unlink (`Remove`, `Delete`),
- copy and move: `CopyFile` (single file), `Rename` (cross-directory move),
- metadata and inspection: `Stat`, `Lstat`, `Access`, `Readlink`, `Realpath`,
- mode and timestamp mutation: `Chmod`, `Utimes`,
- symlink creation: `Symlink`, `Link`,
- open-handle workflows: `Open` returns an `*os.File` for handle-based I/O,
- integrate with watch registration.

The filesystem bridge is organized into method categories:

| Category | Methods |
|---|---|
| Read | `Read`, `ReadBytes`, `ReadFile` (os.ReadFile) |
| Write | `Write`, `WriteBytes`, `WriteFile` (os.WriteFile) |
| Directory | `Mkdir`, `MkdirAll`, `ReadDir`, `Rmdir`, `Mkdtemp` |
| Copy/Move/Delete | `Rename`, `CopyFile`, `Remove` (unlink), `RemoveAll` (rm), `Delete` (legacy, file-only) |
| Metadata | `Stat`, `Lstat`, `Access`, `Exists` (legacy) |
| Mutation | `Chmod`, `Utimes`, `Truncate`, `AppendFile` |
| Links | `Symlink`, `Readlink`, `Link`, `Realpath` |
| Open handle | `Open`, `OpenRead` (legacy), `OpenWrite` (legacy) |

The filesystem code is always present in the runtime, but the frontend-facing filesystem capability is gated by configuration.

#### FileHandle lifecycle (runtime side)

When `fs_open` is requested over the wire, the runtime:

1. resolves and validates the path through `FSBridge.Open(path, flags, perm)`,
2. stores the open `*os.File` in a handle registry keyed by a unique handle ID (reuses the stream management infrastructure),
3. returns the handle ID to the frontend,
4. forwards subsequent `handle_*` operations to the registered handle,
5. closes the OS handle and removes the registry entry on `handle_close`,
6. cleans up all handles owned by a connection when that WebSocket connection closes.

The handle registry is in-memory and per-runtime-instance. Handle IDs are unique within a runtime session.

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
- instance state based on app name and resolved root,
- best-effort foregrounding of existing native webview windows.

The canonical state location is the resolved app root.

Instance state is a structured record rather than a fixed text tuple. It stores process ID, port, mode, and optional platform window identity.

If a live browser instance is already present for the current resolved root, startup short-circuits and opens the preserved localhost URL in the default browser instead of starting a second server.

If a live webview instance is already present for the current resolved root, startup short-circuits and calls the platform focus adapter. Windows should use a stored window handle when available, with PID-based top-level window discovery as fallback. Platforms without a reliable focus mechanism may exit quietly after detecting the existing instance.

### 10. Display Shells and Headless Mode

`browser.go` and `webview.go` are peer shell adapters.

Browser responsibilities:

- open the app URL in the default browser,
- cooperate with idle shutdown logic.

Webview responsibilities:

- open a native WebView window,
- record platform window identity when available,
- own process lifetime through the window lifecycle.

The webview adapter also owns nonce injection. Before navigating the WebView window, the adapter constructs the navigation URL with a cryptographically random nonce query parameter (`?t=<nonce>`). This nonce is the only proof that the WebSocket client is the genuine WebView instance.

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
- expose a Node-style canonical filesystem API (`readFile`, `writeFile`, `readdir`, `mkdir`, `stat`, `rm`, `unlink`, `rename`, `copyFile`, `appendFile`, `access`, `open` for FileHandle, and all other `fs/promises`-shaped methods),
- expose backward-compatible wrapper methods for the legacy Luminka-specific names (`readText`, `writeText`, `list`, `remove`, `exists`),
- expose tracked text file helpers for two-way file-backed state,
- wrap filesystem, script, shell, and app-info calls,
- expose low-level runtime broadcast helpers,
- expose an opt-in multi-tab coordination helper built on broadcast,
- hide request IDs and stream/session mechanics from normal app code,
- stay thin enough that direct protocol access remains possible.

The SDK is in-repo and first-class, but not a standalone npm product.

The TypeScript file remains the source of truth. The repository also owns a generated JavaScript distribution surface under `sdk/dist/` for consumers who want a ready-to-embed browser artifact without importing the TypeScript source directly.

The SDK preserves three filesystem layers:

- **Node-style canonical methods** (the primary contract): `readFile`, `writeFile`, `readdir`, `mkdir`, `stat`, `lstat`, `rm`, `unlink`, `rename`, `copyFile`, `appendFile`, `access`, `chmod`, `utimes`, `truncate`, `symlink`, `readlink`, `realpath`, `link`, `open` (FileHandle), `mkdtemp`, `rmdir`,
- **Legacy convenience wrappers** (backward compat, may be removed in a future major version): `readText`, `writeText`, `readBytes`, `writeBytes`, `list`, `remove`, `exists`,
- **Tracked text file helpers** (`trackedTextFile`), which maintain per-file known text, debounce raw change notifications, re-read after changes, suppress echoes from SDK-originated saves, and notify subscribers only for meaningful external content changes.

Both consumption lanes are first-class:

- direct source consumption from `luminka/sdk/luminka.ts`,
- generated artifact consumption from `sdk/dist/*`.

The multi-tab helper owns session IDs, peer heartbeats, peer join/leave detection, arbitrary peer messages, and primary election. By default, the oldest active session is primary and newer active sessions are non-primary. The helper does not enforce UI or data policy. Apps decide whether secondary tabs become read-only, block, warn, reload, or continue.

### 12. Packaging Hooks and Install Templates

Build tooling is provided by a single Go CLI at `cmd/build/main.go` that ships inside the module. It handles tool resolution, icon generation, go-winres embedding, and Go compilation — identical whether used in-repo or imported remotely.

Both consumption paths use the same command:

```bash
# Cloned repo:
go run ./cmd/build ./starter --webview

# Imported module:
go run github.com/lirrensi/luminka/cmd/build@latest . --webview
```

The build CLI also exposes packaging subcommands that produce distribution-ready artifacts from a built binary. Each packaging format is its own entry point:

```bash
go run ./cmd/build zip    ./starter   # produces app-windows-x64.zip
go run ./cmd/build tar    ./starter   # produces app-linux-x64.tar.gz
go run ./cmd/build deb    ./starter   # produces app_1.0.0_amd64.deb
go run ./cmd/build appdir ./starter   # produces App.app/ (macOS)
```

Subcommands are platform-aware. The CLI only offers formats that can be produced on the current build host. An unsupported format errors out clearly.

Canonical direction:

- keep a single source icon asset under repository control (e.g., `assets/lumi.png`),
- the build CLI generates platform-specific packaging outputs from that source,
- it supports Windows, macOS, and Linux packaging targets,
- on Windows, it exhaustively scans for GCC across MSYS2, MinGW-w64, TDM-GCC, Chocolatey, Scoop, and Cygwin installations, with `--gcc` for manual override,
- the build CLI is standalone — no dependency on the Luminka runtime package, no Node.js required for Go builds.

**Install script templates** live under `scripts/install/` and are shipped in the repo as a library for app developers. They are not part of the build CLI or the built binary. Each template covers one install scenario and uses placeholder strings (`__APP_NAME__`, `__BINARY_NAME__`) that the developer replaces to match their app:

| Script | Scenario |
|---|---|
| `install-path.sh` / `.ps1` | Copy binary to `~/.local/bin/`, add to PATH, create desktop shortcut |
| `install-portable.sh` / `.ps1` | Create a desktop shortcut to the binary in-place, no copy |
| `install-home.sh` / `.ps1` | Copy binary to `~/.app-name/`, set that as the app root via `--root` |
| `uninstall.sh` / `.ps1` | Reverse the above — remove from PATH, delete shortcut, optionally delete data |

App developers pick the scripts that match their distribution model, customize the placeholders, and ship them alongside the binary in their release artifacts.

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

Raw watch events are origin-unaware and may include writes initiated by the same SDK client.

### Tracked Text File Flow

```text
frontend creates tracked text file helper
  -> helper registers a raw runtime watch for the path
  -> helper load reads text and records current known text
  -> helper save writes text and records SDK-originated known text
  -> runtime may later emit raw fs_changed for that same write
  -> helper debounces and re-reads the file
  -> helper suppresses the event if text matches known SDK state
  -> helper notifies subscribers if text differs from known SDK state
```

This flow is the recommended path for local-first apps that bind UI state to a text file. Raw watch APIs remain available for callers that need every filesystem event.

### Runtime Broadcast Flow

```text
frontend client A sends broadcast(channel, data, optional payload)
  -> runtime validates the broadcast envelope
  -> runtime snapshots active WebSocket connections for the same process/root
  -> runtime pushes the broadcast to matching connected clients, normally excluding sender
  -> frontend clients decide what the message means
```

Broadcast messages are transient. The runtime does not persist them, replay them, or treat them as authoritative state.

### Multi-Tab Coordination Flow

```text
browser tab creates multi-tab coordinator
  -> coordinator generates a session ID
  -> coordinator announces presence through runtime broadcast
  -> coordinators exchange heartbeats and peer metadata
  -> oldest active session is identified as primary by default
  -> app receives primary/peer events and chooses its own UI/data policy
```

This flow helps browser-mode apps handle stale tabs and duplicate windows without forcing Luminka to own application state or file merge semantics.

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
| Best-effort webview focus | Relaunching a webview instance should foreground the existing native window when platform support exists. |
| Portable-first locality | Portable mode remains the default locality model unless overridden. |
| App-agnostic runtime | Luminka does not interpret application file schemas. |
| Strict capability separation | FS, script, and shell lanes stay distinct. No silent fallback. |
| Stream-first payload model | Payload-bearing operations use the shared stream transport model. |
| Capability truthfulness | Reported capabilities must match actual behavior. |
| Thin SDK | SDK improves DX without replacing the canonical protocol. |
| Broadcast is not state | Runtime broadcast messages are transient coordination events, not persisted application state. |
| WebView nonce authentication | WebView connections must authenticate via a one-time nonce before any other messages are accepted. |

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
- port (fixed localhost port; 0 = OS-assigned),
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
- relaunching an existing webview instance should attempt platform-native focus and then exit,
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
| Structured instance records | Allows instance state to grow with mode and platform window identity without tuple parsing constraints | High |
| Broadcast as primitive, not state store | Solves browser multi-tab coordination without making Luminka own app state | High |
| Oldest active tab as default primary | Gives SDK multi-tab coordination a stable default while leaving policy to applications | Medium |
| Subcommand-per-format packaging | Each packaging format (zip, tar, deb, appdir) is its own build CLI subcommand, making it easy to call exactly one in CI without flags | High |
| Install script templates as repo library | Scripts live in the repo as templates that app developers copy and customize — the framework does not enforce one install model | High |
| Placeholder-based scripts instead of parameterized | Developers edit the script once per app instead of passing the app name every invocation; simpler to ship in a release artifact | Medium |
| WebView nonce auth, browser mode skipped | Nonce auth closes the realistic local-attacker gap for webview builds; browser mode address bar visibility makes nonce ineffective there | High |

## Implementation Pointers

- Current runtime package: `luminka/*`
- Current SDK source of truth: `luminka/sdk/luminka.ts`
- Generated SDK distribution surface: `sdk/dist/*`
- Build CLI: `cmd/build/main.go`
- Packaging subcommands: `cmd/build/package_*.go`
- Install script templates: `scripts/install/*`
- Install and packaging patterns guide: `docs/installation.md`
- Transitional architecture source: `agent_chat/plan_luminka_architecture_2026-03-30.md`

These pointers are informative only. The canon is this document plus the rest of `docs/`.
