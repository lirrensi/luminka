# Luminka Specification

## Abstract

Luminka specifies a small local runtime for built web applications. A conforming Luminka app embeds static frontend assets into a local executable, serves them over a localhost-only interface, and exposes selected local capabilities over a canonical WebSocket transport.

The product supports two equal first-class display profiles: browser and webview. It also supports portable and detached root policies, normal and headless launch behavior, byte-capable filesystem transfer, constrained script execution, and unrestricted shell execution.

## Introduction

Luminka exists to let developers keep their application logic in the web layer while crossing the browser boundary in a small, explicit, local-first way.

The specification defines:

- what a Luminka app artifact is,
- how the runtime behaves,
- how roots and launch behavior are resolved,
- how capabilities are exposed and gated,
- how frontend code communicates with the runtime,
- what an implementation must preserve to remain conforming.

This document defines behavior, not a specific language implementation. The reference implementation is expected to be written in Go, but the protocol and runtime behavior are not Go-specific.

## Scope

This specification covers:

- embedded static asset serving,
- localhost runtime behavior,
- browser and webview display profiles,
- portable and detached root policies,
- normal and headless launch behavior,
- single-instance behavior per resolved app root,
- best-effort foregrounding of existing webview instances,
- runtime capability gating,
- WebSocket transport contracts,
- chunked byte streams,
- SDK-level tracked text file behavior,
- runtime broadcast events and SDK multi-tab coordination primitives,
- default external data location,
- lifecycle and shutdown behavior.

This specification does not cover:

- frontend build tooling,
- application-specific file schemas,
- authentication or multi-user network access,
- package registry behavior,
- cloud synchronization,
- interactive PTY semantics,
- general stdin streaming for local processes in v1.

## Terminology

Key terms are defined in [glossary.md](glossary.md). In this document, "portable root policy", "detached root policy", "resolved app root", "headless launch", "stream", "stream session", "binary frame envelope", and "trusted frontend" use the glossary definitions.

## Normative Language

The key words **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** in this document are to be interpreted as described in RFC 2119.

## System Model

### Actors

- **Developer**: builds the frontend, configures Luminka, and produces an app executable.
- **End user**: launches the produced executable and interacts with the frontend.
- **Frontend**: the embedded static web app.
- **Runtime**: the local Luminka executable process.
- **Host OS**: the local machine on which the runtime executes.

### App Artifact Model

A Luminka app instance consists of:

1. a single executable containing embedded frontend assets, and
2. external files located in the resolved app root.

The runtime MUST serve the embedded assets from inside the executable. The runtime MUST NOT require a separate frontend development server for normal operation.

### Display Profiles

Luminka defines two display profiles:

| Profile | Description |
|---|---|
| Browser | The runtime opens the app in the system browser. |
| Webview | The runtime opens the app in a native WebView window. |

These profiles are equal first-class product modes. A specific built binary conforms to exactly one display profile at runtime.

### Root Policies

Luminka defines two root policies:

| Policy | Description |
|---|---|
| Portable | The runtime resolves the app root from the executable folder. |
| Detached | The runtime resolves the app root from the current working directory. |

An app MAY choose either policy as its default. A launch-time override MAY select either policy regardless of the app default.

### Launch Behavior

Luminka defines two launch behaviors:

| Behavior | Description |
|---|---|
| Normal | Start the runtime and open the configured browser or webview shell. |
| Headless | Start the runtime without opening a browser tab or webview window. |

Headless launch is independent of display profile. A browser or webview build MAY both be launched headlessly.

### Capability Model

Luminka defines three capability families:

| Capability | Default state | Purpose |
|---|---|---|
| Filesystem | Enabled | Read, write, list, delete, exists, and watch files relative to the app root. |
| Scripts | Disabled | Run approved local script files or interpreters against approved local files. |
| Shell | Disabled | Run arbitrary local commands with no command validation. |

Capability availability is determined by implementation support plus app configuration.

An implementation MUST support the concept of capability gating. If a capability is disabled or unavailable, the runtime MUST reject calls to it with an explicit error response.

Filesystem support is part of the runtime and MAY be disabled as an exposed frontend capability. Disabling filesystem capability does not imply that script execution loses internal access to local files required for validation and execution.

## Conformance

### Runtime Conformance

A conforming Luminka runtime MUST:

1. serve embedded assets from a localhost-only runtime,
2. expose the canonical WebSocket transport,
3. preserve single-instance behavior per resolved app root,
4. resolve the external root according to root policy and launch overrides,
5. report capability availability accurately,
6. enforce capability gating consistently,
7. implement at least one display profile,
8. implement normal launch behavior,
9. reject unsupported or disabled capability calls explicitly.

### Product Conformance

A conforming Luminka product implementation SHOULD provide both browser and webview display profiles.

The reference repository is expected to provide both profiles even though a single built binary uses one profile at runtime.

The reference repository's public Go module identity is `github.com/lirrensi/luminka`.

The reference repository's direct runtime import lane is expected to remain `github.com/lirrensi/luminka/luminka`.

A conforming product implementation SHOULD provide both root policies and headless launch behavior.

### Capability Conformance

If a runtime claims support for a capability in `app_info`, that capability MUST behave according to this specification.

If a runtime does not claim support for a capability, calls to that capability MUST fail explicitly and MUST NOT silently fall back to another capability.

In particular:

- `script_exec` MUST NOT degrade into `shell_exec`,
- `shell_exec` MUST NOT degrade into `script_exec`,
- disabled filesystem APIs MUST NOT remain reachable.

## Behavioral Specification

### 1. Asset Packaging and Serving

The developer MAY use any frontend stack. Luminka does not define or own the frontend build process.

For normal operation, the built frontend assets MUST be embedded into the executable before distribution.

At runtime, the executable MUST serve those assets directly. The runtime MAY expose them over HTTP internally for browser or webview loading, but the assets are conceptually part of the executable artifact.

The runtime MUST prefer exact embedded file matches first. If a `GET` or `HEAD` request does not match an exact file, the runtime MUST attempt to serve an embedded `index.html` entry document using these candidates in order:

1. `index.html`
2. `dist/index.html`
3. `static/index.html`

If an entry document is found, the runtime MUST return `200 OK`. `HEAD` requests MUST return headers only. If no entry document exists, the runtime MUST return `404 Not Found`.

### 2. Root Resolution and External Files

The runtime MUST resolve an effective app root for each launch.

The resolved app root MUST be determined in this order:

1. explicit launch override,
2. app-configured default root policy,
3. portable behavior if no root policy is specified.

Portable behavior resolves the app root from the executable folder.

Detached behavior resolves the app root from the current working directory.

Unless explicitly configured otherwise by the app author, all default filesystem operations, script path validation, lock files, and related runtime-local artifacts MUST resolve relative to the resolved app root.

### 3. Single-Instance Behavior

A Luminka app MUST behave as a single instance within its resolved app root.

If the executable is launched and no live instance exists for that resolved root, the runtime MUST start normally.

If the executable is launched and a live instance already exists for that resolved root, the runtime MUST NOT start a second independent instance for that same root.

In that case:

- a browser build SHOULD open the existing app URL in a new browser tab or window and then exit,
- a webview build SHOULD attempt to bring the existing native window to the foreground when platform support exists,
- if native focus is unavailable or fails, the implementation MAY exit after detecting the existing live instance.

If stale instance state is found, the runtime MUST recover by discarding the stale state and proceeding with normal startup.

### 4. Browser Display Profile

In the browser profile, the runtime MUST:

1. start the localhost runtime,
2. serve the embedded frontend,
3. open the system browser to the app URL,
4. remain alive while clients are active,
5. apply idle shutdown behavior after all clients disconnect.

The runtime SHOULD use a configurable idle timeout. The default idle timeout SHOULD be 180 seconds.

### 5. Webview Display Profile

In the webview profile, the runtime MUST:

1. start the localhost runtime,
2. serve the embedded frontend,
3. open a native WebView window to the app URL,
4. remain alive while the window is open,
5. shut down when the window is closed unless another foreground shell policy is explicitly implemented.

### 6. Headless Launch Behavior

In headless launch behavior, the runtime MUST:

1. start the localhost runtime,
2. serve the embedded frontend,
3. expose the enabled capability bridge,
4. refrain from opening a browser tab or webview window,
5. remain alive while the foreground host process remains alive.

In headless launch behavior, browser idle timeout semantics and webview window-lifecycle semantics MUST NOT be the primary lifetime controller.

### 7. Localhost Transport

The runtime MUST listen only on loopback interfaces.

The frontend communicates with the runtime over WebSocket. The canonical endpoint path is `/ws`.

The runtime MAY use HTTP to serve embedded assets, but REST-style HTTP APIs are not the canonical capability surface.

The runtime MAY support transient broadcast messages between active WebSocket clients connected to the same runtime instance and resolved app root.

### 8. WebSocket Frame Envelope

The canonical WebSocket frame format MUST be binary and MUST follow this layout:

```text
[4-byte big-endian JSON header length][UTF-8 JSON header][payload bytes]
```

The JSON header MUST describe the message semantics. The payload MAY be empty.

Control-only messages MUST use an empty payload.

The JSON header for requests MUST contain at least:

```json
{ "event": "<event_name>", "id": "<request_id>" }
```

The JSON header for successful responses MUST contain at least:

```json
{ "event": "<response_event>", "id": "<request_id>", "ok": true }
```

The JSON header for failed responses MUST contain at least:

```json
{ "event": "<response_event>", "id": "<request_id>", "ok": false, "error": "<message>" }
```

Server-pushed notifications MAY omit `id` when they are not direct responses to a request.

### 9. Stream Sessions

Luminka MUST support stream-oriented transfers for payload-bearing operations.

A stream session MUST be identified by a `stream_id`.

The JSON header MAY include fields such as:

- `stream_id`
- `seq`
- `lane`
- `eof`
- `content_type`

Chunked transfers MUST preserve ordering within a stream session.

The payload bytes of a frame MUST be interpreted according to the JSON header.

### 10. Runtime Introspection

The runtime MUST support `app_info`.

Request header:

```json
{ "event": "app_info", "id": "a1" }
```

Response header:

```json
{
  "event": "app_info",
  "id": "a1",
  "ok": true,
  "name": "<app_name>",
  "mode": "browser",
  "root": "<resolved_root>",
  "capabilities": {
    "fs": true,
    "scripts": false,
    "shell": false
  }
}
```

`mode` MUST be either `browser` or `webview`.

### 11. Filesystem Capability

If filesystem capability is enabled, the runtime MUST support the following events:

| Request event | Response event | Behavior |
|---|---|---|
| `fs_read_text` | `fs_response` | Read a text file |
| `fs_write_text` | `fs_response` | Write a text file |
| `fs_list` | `fs_response` | List a directory |
| `fs_delete` | `fs_response` | Delete a file; directories are rejected |
| `fs_exists` | `fs_response` | Check path existence |
| `fs_watch` | `fs_response` | Register a watched path |
| `fs_unwatch` | `fs_response` | Remove a watched path |
| `fs_open_read` | `fs_response` or equivalent | Open a chunked read stream |
| `fs_open_write` | `fs_response` or equivalent | Open a chunked write stream |

Paths MUST be interpreted relative to the resolved app root.

The runtime MUST reject:

- absolute paths,
- parent traversal outside the root,
- paths that escape the root through symlink resolution.

The runtime MUST NOT impose an application schema on file contents.

`fs_read_text` and `fs_write_text` are convenience operations layered on top of the runtime's byte-capable transport model.

Chunked filesystem transfer MUST be supported from the start. A conforming implementation MUST NOT require that large files fit into one request or one response frame.

Example text read header:

```json
{ "event": "fs_read_text", "id": "f1", "path": "data.yaml" }
```

If filesystem capability is disabled, any `fs_*` call MUST fail explicitly.

### 12. Filesystem Change Notifications

If filesystem capability is enabled and a path is being watched, the runtime MUST notify the frontend when it observes that the path changed.

Notification header:

```json
{ "event": "fs_changed", "path": "data.yaml" }
```

The implementation MAY use polling or native OS file watching. The observable contract is the notification, not the detection strategy.

The raw filesystem watch contract is origin-unaware. A runtime notification MAY be caused by a write performed through the same frontend client that registered the watch. The runtime MUST NOT be required to distinguish same-client writes from other local modifications.

### 13. SDK Tracked Text File Helper

A conforming product implementation SHOULD provide a higher-level SDK helper for text files that supports the common local-first pattern of loading a file, saving new text to that file, and subscribing to meaningful external content changes.

The helper is layered on top of the filesystem capability. It does not change the raw `fs_watch` or `fs_changed` transport contract.

The helper SHOULD expose operations equivalent to:

| Operation | Behavior |
|---|---|
| create tracked text file | Bind helper state to one relative path |
| load | Read current file text and record it as the helper's current known text |
| save | Write text to the file and record the written text as SDK-originated state |
| subscribe | Register a listener for meaningful external text changes |
| dispose | Remove helper-managed watches and listeners |

For watched paths, the helper SHOULD debounce rapid raw change notifications before re-reading the file.

After a debounced raw change notification, the helper SHOULD re-read the file and compare the current file text to its current known text or equivalent content hash. If the content matches the current known text, the helper SHOULD suppress the notification as a write echo or duplicate event. If the content differs, the helper SHOULD update its current known text and notify subscribers with the new text.

When `save` completes successfully, the helper MUST update its current known text to the saved text. Subsequent raw watch notifications for identical content SHOULD NOT be reported as external changes.

The helper MAY expose raw watch events separately for advanced callers, but raw events MUST remain distinguishable from meaningful external content-change callbacks.

### 14. Runtime Broadcast Events

Luminka MAY support a runtime-local broadcast primitive for connected frontend clients.

If broadcast support is implemented, the runtime MUST accept a `broadcast` request containing a channel name, arbitrary JSON metadata, and optional payload bytes.

Example request header:

```json
{
  "event": "broadcast",
  "id": "b1",
  "channel": "workspace",
  "data": { "type": "active-session", "session_id": "tab-1" }
}
```

Successful response header:

```json
{ "event": "broadcast_response", "id": "b1", "ok": true }
```

Server-pushed broadcast header:

```json
{
  "event": "broadcast",
  "channel": "workspace",
  "data": { "type": "active-session", "session_id": "tab-1" }
}
```

The runtime MUST deliver a broadcast only to active WebSocket clients connected to the same runtime instance and resolved app root.

The runtime SHOULD default to delivering a broadcast to clients other than the sender. An implementation MAY expose an explicit echo option if sender delivery is useful.

Broadcasts MUST be transient. The runtime MUST NOT persist broadcast messages or treat them as authoritative application state.

The runtime MUST NOT define application-level semantics for broadcast messages. Multi-tab coordination, dirty-state handling, primary-tab behavior, stale-data handling, and merge policy remain frontend or application responsibilities.

### 15. SDK Multi-Tab Coordination Helper

A conforming product implementation SHOULD provide an opt-in SDK helper built on top of runtime broadcast events for browser multi-tab coordination.

The helper SHOULD expose behavior equivalent to:

| Operation | Behavior |
|---|---|
| create multi-tab coordinator | Bind coordination to a named channel or scope |
| start | Generate or use a frontend session ID and announce presence |
| stop/dispose | Stop heartbeats and remove listeners |
| send peer message | Broadcast arbitrary app-defined coordination data |
| observe peers | Notify the app when peers appear, update, or disappear |
| observe primary session | Notify the app when the primary session changes |

The default primary election rule SHOULD treat the oldest active session as primary. Newer sessions SHOULD become non-primary unless the application implements a different policy on top of the helper.

The helper MUST NOT force secondary tabs to close, become read-only, reload, or block editing. It exposes coordination state; the application decides UI and data policy.

### 16. Script Execution Capability

If script capability is enabled, the runtime MUST support synchronous execution and MAY additionally support stream-mode execution.

The constrained execution lane is distinct from shell execution.

The request model is a triplet:

1. `runner`
2. `file`
3. `args`

The `file` value is a script selector.

- If `file` begins with `internal:`, the runtime MUST resolve the remainder against the embedded script bundle.
- Otherwise, the runtime MUST resolve `file` as an external path relative to the resolved app root.

The runtime MUST validate the selected script before execution.

Synchronous request header:

```json
{
  "event": "script_exec",
  "id": "s1",
  "runner": "python",
  "file": "tools/generate.py",
  "args": ["--verbose"],
  "timeout": 30
}
```

If `timeout` is omitted or non-positive, the runtime MUST use its configured default execution timeout.

The runtime MUST validate that an external script resolves inside the app root and exists before execution.

The runtime MUST validate that an internal script exists in the embedded bundle before execution.

The runtime MUST invoke the `runner` with the validated `file` as the next argument.

If `args` are present, the runtime MUST append them after the validated file.

The spawned process MUST run with the resolved app root as its working directory.

The runtime MAY materialize an internal script to a temporary local file before execution, but the observable behavior MUST remain equivalent to running the bundled script itself.

The runtime MUST NOT apply additional semantic validation to `args` beyond normal message parsing.

Synchronous response header:

```json
{
  "event": "script_response",
  "id": "s1",
  "ok": true,
  "stdout": "generated 42 files\n",
  "stderr": "",
  "code": 0
}
```

If stream execution is supported for scripts, the runtime MUST:

1. allow a script process to be started in a stream mode,
2. emit stdout and stderr as ordered stream chunks,
3. emit a terminal completion event with final exit status.

General stdin streaming is not required in v1 of this stream model.

If the selected external script is missing from the allowed root, or the selected internal script is missing from the embedded bundle, the runtime MUST reject the request.

If script capability is disabled, `script_exec` MUST fail explicitly.

### 17. Shell Execution Capability

If shell capability is enabled, the runtime MUST support synchronous execution and MAY additionally support stream-mode execution.

`shell_exec` is the unrestricted execution lane.

Synchronous request header:

```json
{
  "event": "shell_exec",
  "id": "h1",
  "cmd": "powershell",
  "args": ["-Command", "Get-Process | Select -First 5"],
  "timeout": 10
}
```

If `timeout` is omitted or non-positive, the runtime MUST use its configured default execution timeout.

The runtime MUST pass the command directly to local process spawning without command validation beyond normal execution setup.

The spawned process MUST run with the resolved app root as its working directory.

Synchronous response header:

```json
{
  "event": "shell_response",
  "id": "h1",
  "ok": true,
  "stdout": "...",
  "stderr": "",
  "code": 0
}
```

If stream execution is supported for shell commands, the runtime MUST:

1. allow a process to be started in a stream mode,
2. emit stdout and stderr as ordered stream chunks,
3. emit a terminal completion event with final exit status.

General stdin streaming is not required in v1 of this stream model.

If shell capability is disabled, `shell_exec` MUST fail explicitly.

### 18. Idle and Shutdown Behavior

Browser builds SHOULD shut down after a configurable idle period once all active frontend connections are gone.

Webview builds SHOULD shut down when the owning window is closed.

Headless launches SHOULD shut down when the owning foreground process exits.

On clean shutdown, the runtime MUST clean up its instance state.

## Data and State Model

### Runtime Configuration Model

A Luminka app configuration MUST include conceptually equivalent fields to the following:

| Field | Meaning | Default |
|---|---|---|
| `name` | App identity used for runtime-local artifacts | implementation-defined |
| `mode` | `browser` or `webview` | app-defined |
| `root_policy` | `portable` or `detached` default root policy | `portable` |
| `root` | Explicit external app root override | none |
| `headless` | Launch without opening browser/webview shell | `false` |
| `idle_timeout` | Browser idle shutdown timeout | 180s |
| `fs_enabled` | Exposed filesystem capability | true |
| `scripts_enabled` | Exposed script capability | false |
| `shell_enabled` | Exposed shell capability | false |
| `exec_timeout` | Default process execution timeout | 30s |

The exact configuration surface MAY differ by implementation language.

### SDK Distribution Surface

Luminka's canonical SDK source is the TypeScript file `luminka/sdk/luminka.ts` in the reference repository.

A conforming repository implementation MAY additionally provide generated JavaScript artifacts for browser-ready or tool-light consumption.

If generated JavaScript artifacts are provided, the reference repository SHOULD expose them through a stable path under `sdk/dist/`.

Providing generated artifacts does not replace the canonical TypeScript source. Both source consumption and generated-artifact consumption are valid adoption lanes.

### Instance State

An implementation MUST persist enough state to detect whether another live instance already owns the current resolved app root.

The reference model uses a structured instance record in the resolved app root.

The record SHOULD include at least:

| Field | Meaning |
|---|---|
| `pid` | Owning runtime process ID |
| `port` | Localhost runtime port |
| `mode` | Display profile for the running instance |
| `window` | Optional platform window identity for native focus behavior |

Equivalent mechanisms are permitted if they preserve the same observable behavior.

### Capability State

Capability state is the resolved availability of `fs`, `scripts`, and `shell` for the current app instance.

That resolved state MUST be reflected by `app_info` and by actual runtime behavior.

## Error Handling and Edge Cases

The runtime MUST handle the following situations explicitly:

- stale instance state,
- no available port,
- malformed frame envelopes,
- malformed JSON headers,
- unknown events,
- disabled capabilities,
- unsupported capabilities,
- invalid or escaping paths,
- missing script files,
- stream chunk ordering errors,
- premature stream termination,
- process timeout,
- frontend disconnects.

General rules:

1. Invalid requests MUST produce a structured failure response when tied to a request.
2. Unknown events MUST NOT crash the runtime.
3. Missing or disabled capabilities MUST fail clearly rather than silently no-op.
4. Filesystem path checks MUST occur server-side.
5. A watched-path notification MAY be coalesced, but change pushes MUST eventually reflect observable modifications.

## Security Considerations

Luminka assumes a trusted frontend.

The runtime MUST be localhost-only.

Luminka does not define authentication for local capability access in v1.

Filesystem capability exposes local file access within the allowed root.

Script capability exposes controlled local execution against allowed local files.

Shell capability exposes unrestricted local command execution and is therefore a full-trust mode.

Detached mode changes locality by making the current working directory the default app root, but it does not change the trust model.

Headless mode changes lifecycle behavior but does not make the runtime safer or more network-exposed.

Implementations MUST NOT claim stronger security properties than this model actually provides.

## References

### Normative References

- [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119) — Key words for use in RFCs to Indicate Requirement Levels
- [Product](product.md) — Product canon for Luminka
- [Glossary](glossary.md) — Canonical terminology

### Informative References

- Electron
- Tauri
