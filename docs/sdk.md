# Luminka TypeScript SDK Guide

The Luminka TypeScript SDK is the frontend-facing helper layer for talking to the local Luminka runtime. It lets a web app use Luminka's localhost WebSocket bridge through ordinary TypeScript/JavaScript helpers instead of hand-writing binary protocol frames.

Use this guide when you are building the app frontend and want to read or write local files, observe file changes, run enabled scripts or shell commands, stream bytes, broadcast messages between connected clients, or coordinate multiple browser tabs.

The normal positioning is simple: import one SDK helper, create a client, then call SDK methods from your frontend code. You should not need to understand Luminka's Go internals for normal SDK use.

```ts
import { createLuminkaClient } from "./luminka.js";

const client = createLuminkaClient();
await client.connect();
```

The SDK source of truth is [`luminka/sdk/luminka.ts`](../luminka/sdk/luminka.ts). The generated browser artifact lives under [`sdk/dist/`](../sdk/dist/). If you import Luminka directly or consume generated SDK files, rebuild/import the current SDK so TypeScript source, generated browser output, and embedded copies stay aligned.

Version 3 broadcast support is additive. Normal SDK consumers should not need protocol changes by hand; rebuild against the current SDK and keep using the helper APIs.

## What the SDK Provides

The SDK wraps Luminka's local runtime capabilities:

| Area | SDK surface | Use it for |
|---|---|---|
| Connection | `createLuminkaClient()`, `connect()`, `disconnect()`, `appInfo()` | Opening the runtime bridge and reading app/runtime metadata |
| Files | `readText()`, `writeText()`, `read()`, `write()`, `readBytes()`, `writeBytes()`, `list()`, `remove()`, `exists()` | Local files relative to the app root |
| File watching | `watch()`, `unwatch()`, `onFileChanged()` | Raw file-change notifications |
| File-backed state | `trackedTextFile()` | Friendly text-file state with echo suppression |
| Streams | `createReadStream()`, `createWriteStream()`, `runScriptStream()`, `runShellStream()` | Large files and live process output |
| Scripts | `runScript()`, `runScriptStream()` | Enabled constrained script execution |
| Shell | `runShell()`, `runShellStream()` | Enabled full-trust command execution |
| Broadcast | `broadcast()`, `onBroadcast()` | Transient local pub/sub between connected frontend clients |
| Multi-tab | `createMultiTabCoordinator()` | Primary-tab election, peer events, and tab messages |

All filesystem paths are app-root-relative. The runtime rejects paths that try to escape the app root.

## Import and Connection Patterns

### Default hosted app

In a normal Luminka-hosted browser or webview app, the SDK can infer the WebSocket URL from `location.host` and the canonical `/ws` endpoint.

```ts
import { createLuminkaClient } from "./luminka.js";

const client = createLuminkaClient();
await client.connect();
```

`connect()` is idempotent while the socket is already open or connecting. Most request methods call `connect()` internally, but explicit connection at app startup makes failures easier to surface.

### Explicit URL

Outside a Luminka-hosted browser context, or when connecting to a known headless/runtime URL, pass the WebSocket URL explicitly.

```ts
import { createLuminkaClient } from "./luminka.js";

const client = createLuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
await client.connect();
```

If no browser `location.host` is available and no URL is provided, connection fails with an explicit error asking for an explicit URL.

### App info and disconnect

`appInfo()` returns runtime identity and resolved capabilities. Use it to adapt the UI before showing feature buttons.

```ts
const info = await client.appInfo();

console.log(info.name, info.mode, info.root);
console.log(info.app_version, info.runtime_version, info.protocol_version);
console.log(info.capabilities.fs, info.capabilities.scripts, info.capabilities.shell);
```

`disconnect()` closes the socket and rejects pending requests and stream completions.

```ts
client.disconnect();
```

## Capability Model from the Frontend

Luminka has three frontend-visible capability families:

| Capability | Frontend meaning | Default expectation |
|---|---|---|
| `fs` | File reads, writes, listing, deletion, existence checks, file watching, file streams | Enabled unless the app disables it |
| `scripts` | Constrained script execution through `runner + file + args` | Disabled unless compiled and configured on |
| `shell` | Full-trust local command execution | Disabled unless compiled and configured on |

The SDK does not silently fall back when a capability is disabled. If filesystem, scripts, or shell are unavailable, calls in that family fail with a runtime error response. For example, `readText()` fails when `fs` is false, `runScript()` fails when `scripts` is false, and `runShell()` fails when `shell` is false.

Prefer checking `appInfo().capabilities` and hiding or disabling UI affordances that cannot work.

```ts
const { capabilities } = await client.appInfo();

if (!capabilities.fs) {
  disableFileButtons("Filesystem is disabled for this app build.");
}
```

Shell is a full-trust lane. Do not expose shell commands to untrusted frontend code or user input unless your app is intentionally a trusted local power tool.

## File Helpers

### Text files

`readText()` and `writeText()` are the explicit text helpers. `read()` and `write()` are text aliases for the same operations.

```ts
const text = await client.readText("notes/today.md");
await client.writeText("notes/today.md", text + "\nDone.\n");

// Aliases:
const sameText = await client.read("notes/today.md");
await client.write("notes/today.md", sameText);
```

### Bytes

Use bytes for binary data or when you do not want text encoding assumptions.

```ts
const image = await client.readBytes("images/logo.png");
await client.writeBytes("backup/logo.png", image);
```

`readBytes()` collects a read stream into one `Uint8Array`. `writeBytes()` writes in chunks internally.

### List, delete, and exists

```ts
const files = await client.list("notes");
const hasConfig = await client.exists("config.json");

if (hasConfig) {
  await client.remove("config.json"); // runtime fs_delete; files only
}
```

The SDK method for deletion is `remove(path)`. It maps to Luminka's filesystem delete operation. The runtime rejects directory deletion.

### Watch, unwatch, and raw change events

Raw watch APIs tell you that a watched path changed. They do not tell you who caused the change, and they may include changes caused by the same SDK client.

```ts
await client.watch("workspace.json");

const unsubscribe = client.onFileChanged((path) => {
  if (path === "workspace.json") {
    console.log("workspace changed; reload if needed");
  }
});

// Later:
unsubscribe();
await client.unwatch("workspace.json");
```

For UI state bound to a text file, prefer `trackedTextFile()` instead of raw watch events.

## `trackedTextFile()`

`trackedTextFile()` is a higher-level helper for the common local-first loop: load a text file, save app state back to it, and react only to meaningful external content changes.

It layers on top of filesystem watching and text reads/writes. It debounces raw change notifications, re-reads the file, compares the content against the SDK's current known text, suppresses same-content write echoes from your own saves, and notifies listeners only when the file content actually changed outside the helper's current state.

```ts
import { createLuminkaClient } from "./luminka.js";

const client = createLuminkaClient();
const workspace = client.trackedTextFile("workspace.emptyspace.xml", {
  debounceMs: 150,
});

const initialText = await workspace.load();
renderWorkspace(initialText);

const unsubscribe = workspace.onExternalChange((text) => {
  renderWorkspace(text);
});

await workspace.save(serializeWorkspace());

// Later, when the view is gone:
unsubscribe();
await workspace.dispose();
```

Useful methods:

| Method | Behavior |
|---|---|
| `load()` | Starts the helper-managed watch, reads the file, stores the known text, and returns it |
| `save(text)` | Writes text and records it as SDK-originated known state |
| `getText()` | Returns the last known text, or `null` before loading |
| `onExternalChange(listener)` | Receives meaningful external text changes |
| `onRawChange(listener)` | Receives raw watched-path notifications for this helper's path |
| `dispose()` | Removes helper listeners and unwatch registration |

Calling methods after `dispose()` fails explicitly.

## Streams

Luminka's SDK exposes stream helpers for payloads that should not be forced through a single text response.

### File read streams

```ts
const stream = await client.createReadStream("large.bin");
const reader = stream.getReader();

try {
  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    consumeChunk(value);
  }
} finally {
  reader.releaseLock();
}
```

### File write streams

```ts
const writable = await client.createWriteStream("output.bin");
const writer = writable.getWriter();

try {
  await writer.write(new Uint8Array([1, 2, 3]));
  await writer.write(nextChunk);
  await writer.close();
} finally {
  writer.releaseLock();
}
```

### Script and shell streaming

`runScriptStream()` and `runShellStream()` return separate stdout/stderr streams plus a completion promise.

```ts
const result = await client.runScriptStream("python", "tools/generate.py", ["--watch"], 30);

void pipeText(result.stdout, appendStdout);
void pipeText(result.stderr, appendStderr);

const completed = await result.completed;
console.log(completed.code, completed.stdout, completed.stderr);
```

Completion result shape:

```ts
type ExecCompletion = {
  code: number | null;
  stdout: string;
  stderr: string;
};
```

`stdout` and `stderr` in the completion result are accumulated text decoded from streamed chunks. The streams themselves carry `Uint8Array` chunks for live display or custom processing.

Synchronous process helpers are also available:

```ts
const script = await client.runScript("python", "tools/generate.py", ["--once"], 30);
const shell = await client.runShell("powershell", ["-Command", "Get-Date"], 10);

console.log(script.code, script.stdout, script.stderr);
console.log(shell.code, shell.stdout, shell.stderr);
```

## Broadcast

Broadcast is a transient local pub/sub primitive between active frontend clients connected to the same Luminka runtime instance and resolved app root. It is useful for browser tabs, companion windows, or other connected frontend clients that need lightweight coordination.

Broadcast messages are not persisted, replayed, or treated as application state.

```ts
const unsubscribe = client.onBroadcast("workspace", (message) => {
  console.log(message.channel, message.data, message.contentType);
  console.log(message.payload);
});

await client.broadcast("workspace", { type: "refresh" });

// Later:
unsubscribe();
```

Payload and content type are optional:

```ts
const payload = new TextEncoder().encode("hello peers");

await client.broadcast(
  "workspace",
  { type: "note" },
  { contentType: "text/plain" },
  payload,
);
```

Echo behavior is explicit. The runtime normally delivers broadcasts to other clients, excluding the sender. Pass `{ echo: true }` when the sender should also receive its own broadcast through `onBroadcast()`.

```ts
await client.broadcast("workspace", { type: "self-test" }, { echo: true });
```

The SDK receives messages with this shape:

```ts
type LuminkaBroadcastMessage<T = unknown> = {
  channel: string;
  data?: T;
  payload: Uint8Array;
  contentType?: string;
};
```

## Multi-Tab Coordination

`createMultiTabCoordinator()` is an SDK helper built on top of broadcast. It gives browser-mode apps a simple coordination lane for multiple open tabs without making Luminka own app state or merge policy.

The coordinator:

- creates or accepts a `sessionId`,
- announces presence with `hello`,
- sends periodic heartbeats,
- removes stale peers after a timeout,
- emits peer joined/left events,
- elects a primary tab by oldest active session by default,
- sends small typed messages between tabs,
- stops cleanly with `stop()`.

```ts
const coordinator = client.createMultiTabCoordinator("workspace", {
  data: { route: location.pathname },
  heartbeatMs: 1000,
  staleMs: 3000,
});

coordinator.onPrimaryChanged((peer) => {
  console.log("primary tab", peer?.sessionId ?? "none");
});

coordinator.onPeerJoined((peer) => {
  console.log("peer joined", peer.sessionId, peer.data);
});

coordinator.onPeerLeft((peer) => {
  console.log("peer left", peer.sessionId);
});

coordinator.onMessage((message) => {
  console.log("message from", message.from.sessionId, message.type, message.data);
});

await coordinator.start();

if (coordinator.isPrimary()) {
  startOneTabOnlyWork();
}

await coordinator.send("workspace-opened", { file: "workspace.json" });
```

Lifecycle matters. Call `stop()` when the tab view or app section using the coordinator is done.

```ts
window.addEventListener("beforeunload", () => {
  void coordinator.stop();
});
```

The coordinator does not force secondary tabs to close, become read-only, reload, or block editing. Your app decides what primary status means.

## Error Handling and Lifecycle Gotchas

- **Use `appInfo()` for feature gating.** Capability-disabled calls fail explicitly; they do not no-op or fall back.
- **Default URL inference only works in a browser host context.** Use `createLuminkaClient({ url })` for headless tools, external pages, or tests.
- **`disconnect()` rejects pending work.** Pending requests, file streams, and process stream completions receive a connection-closed error.
- **Raw watch events are origin-unaware.** Use `trackedTextFile()` when you need same-content write echo suppression.
- **Remember cleanup.** Unsubscribe listeners, `unwatch()` paths you own, dispose tracked files, and stop coordinators when UI scopes end.
- **Streams must be closed or consumed.** Close writable streams after writing; consume or cancel readable streams if your view goes away.
- **Broadcast is transient.** Do not use it as durable state. Store real application data in your app's own files or model.
- **Shell is trusted.** Treat shell access as equivalent to giving the frontend local command execution.

## Practical Mini Example

```ts
import { createLuminkaClient } from "./luminka.js";

const client = createLuminkaClient();
await client.connect();

const info = await client.appInfo();

if (info.capabilities.fs) {
  const doc = client.trackedTextFile("notes/current.md");
  render(await doc.load());

  doc.onExternalChange((text) => render(text));

  saveButton.addEventListener("click", () => {
    void doc.save(readEditorText());
  });
}

const tabs = client.createMultiTabCoordinator("notes", {
  data: { screen: "editor" },
});

tabs.onPrimaryChanged((peer) => {
  setPrimaryBadge(peer?.sessionId === tabs.sessionId);
});

await tabs.start();
```

This is the intended frontend shape: import the SDK once, connect to Luminka, check capabilities, and use the helper that matches the job.
