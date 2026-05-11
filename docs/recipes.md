# Luminka Recipes

Practical how-to patterns for common Luminka workflows.

## Filesystem

### Read and write text files

You want to persist text data to the local filesystem.

```ts
import { createLuminkaClient } from "./luminka.js";

const client = createLuminkaClient();
await client.connect();

// Write
await client.writeText("notes/todo.txt", "buy milk\nwalk dog");

// Read
const content = await client.readText("notes/todo.txt");
console.log(content);
```

### Read and write binary files

You want to read or write raw bytes — images, audio, binary data.

```ts
// Write binary data
const pngBytes = await fetch("/logo.png").then(r => r.arrayBuffer());
await client.writeBytes("assets/logo.png", new Uint8Array(pngBytes));

// Read binary data
const data = await client.readBytes("assets/logo.png");
```

### Watch a file for external changes

You want your frontend to react when a file changes on disk (e.g., edited by another tool).

```ts
await client.watch("data/config.json");

client.onFileChanged((path) => {
  if (path === "data/config.json") {
    console.log("config.json changed on disk — reloading");
    reloadConfig();
  }
});

// When done:
await client.unwatch("data/config.json");
```

### Use tracked text files for local-first state

You want a file-backed state object that handles external changes with debouncing and echo suppression.

```ts
const workspace = client.trackedTextFile("workspace.json");

// Load current state
const text = await workspace.load();
render(JSON.parse(text));

// React to external changes (file edited outside your app)
workspace.onExternalChange((text) => {
  render(JSON.parse(text));
});

// Save from your app (echo-suppressed — won't re-trigger onExternalChange)
await workspace.save(JSON.stringify(currentState));
```

### Append to a log file from the frontend

You want to write timestamped log entries to `luminka.log`.

```ts
await client.log("user clicked export");
await client.log("export completed — 42 records");
```

Each call appends a `[timestamp] message` line to `luminka.log`. This is a simple convenience for debug logging; it reads the existing file content, appends your message, and writes it back.

## Scripts and Shell

### Run a Python script from the frontend

You want to execute a local Python script and read its output.

```ts
const result = await client.runScript("python", "scripts/analyze.py", ["--input", "data.csv"]);
console.log("stdout:", result.stdout);
console.log("stderr:", result.stderr);
console.log("exit code:", result.code);
```

> Requires `-tags scripts` at build time and `EnableScripts: true` in config.

### Run a shell command

You want to run a shell command from the frontend.

```ts
const result = await client.runShell("ffmpeg", ["-i", "input.mp4", "output.mp3"]);
if (result.code === 0) {
  console.log("conversion succeeded");
} else {
  console.error("failed:", result.stderr);
}
```

> Requires `-tags shell` at build time and `EnableShell: true` in config. This is full-trust — the command runs with the user's permissions.

### Stream live process output to the frontend

You want to see stdout and stderr as the process runs, not just after it finishes.

```ts
const { stdout, stderr, completed } = await client.runShellStream("ping", ["-c", "4", "example.com"]);

// Read stdout as it arrives
const stdoutReader = stdout.getReader();
while (true) {
  const { done, value } = await stdoutReader.read();
  if (done) break;
  console.log(new TextDecoder().decode(value));
}

// Wait for process to finish
const result = await completed;
console.log("exit code:", result.code);
```

### Include and run a bundled binary executable

You want to embed a compiled Go binary (or any tool) in your Luminka app and execute it from the frontend.

**Step 1: Embed the binary.** Place your compiled tool in your `dist/` directory (or a subdirectory like `dist/tools/`). It gets embedded into the Go binary alongside your frontend assets.

**Step 2: Extract it at runtime.** Use `readBytes` to read the embedded binary and `writeBytes` to materialize it to disk.

```ts
// Read the embedded binary from the asset bundle
const toolData = await client.readBytes("tools/my-tool.exe");

// Write it to the app root so it's executable
await client.writeBytes("_tools/my-tool.exe", toolData);
```

**Step 3: Run it.**

```ts
const result = await client.runShell("_tools/my-tool.exe", ["--flag", "value"]);
console.log(result.stdout);
```

**Alternative for embedded scripts:** Use `runScript` with the `internal:` prefix to run scripts that are embedded in the `ScriptAssets` bundle.

```ts
const result = await client.runScript("python", "internal:tools/helper.py", ["--input", "data.csv"]);
```

The `internal:` prefix resolves from the embedded `ScriptAssets` filesystem. The runtime materializes the script to a temporary file when execution needs a real path.

## Multi-Tab Coordination

### Coordinate multiple browser tabs for the same app

You want multiple browser tabs connected to the same Luminka app to communicate and avoid stepping on each other.

```ts
const coordinator = client.createMultiTabCoordinator("workspace");

// Listen for primary tab changes
coordinator.onPrimaryChanged((peer) => {
  if (peer) {
    console.log("primary tab is now", peer.sessionId);
  }
});

// Start coordinating
await coordinator.start();

// Only the primary tab runs certain tasks
if (coordinator.isPrimary()) {
  startBackgroundSync();
}
```

### Make secondary tabs read-only

You want secondary tabs to show a read-only view while the primary tab handles writes.

```ts
const coordinator = client.createMultiTabCoordinator("editor");

await coordinator.start();

function updateUI() {
  if (coordinator.isPrimary()) {
    showEditControls();
  } else {
    showReadOnlyBanner();
  }
}

coordinator.onPrimaryChanged(updateUI);
updateUI();
```

## Building and Packaging

### Build a browser-mode app

You want to build your app so it opens in the default browser.

```bash
go run ./cmd/build ./starter
```

Or with pnpm:

```bash
pnpm run build
```

### Build a webview-mode app

You want to build your app with a native desktop window.

```bash
go run ./cmd/build ./starter --webview
```

Or with pnpm:

```bash
pnpm run build:webview
```

If your GCC is in a non-standard location, pass it directly:

```bash
go run ./cmd/build ./starter --webview --gcc C:\msys64\mingw64\bin\gcc.exe
```

Requires CGO enabled and native webview dependencies. See [`onboarding.md`](onboarding.md) for prerequisites.

### Build an imported module

When you import Luminka as a Go module (not cloned), use the remote path:

```bash
go run github.com/lirrensi/luminka/cmd/build@latest . --webview
```

Same command, works whether you cloned the repo or imported it.

### Package your app for distribution

You want to give your app to someone else.

Copy the built `.exe` into a folder with any data files your app needs. That's it — no installer required. The user runs the `.exe` from that folder, and the folder becomes the app root.
