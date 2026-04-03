# Luminka Onboarding

This is the practical “get me productive quickly” guide.

## 1. What you are looking at

Luminka is a framework-first repo, not a single app repo.

The important paths are:

- `starter/` — the thing you should usually edit first
- `examples/hello/` — tiny example, intentionally limited
- `examples/kanban/` — richer example with real file persistence
- `luminka/` — the shared Go runtime
- `luminka/sdk/luminka.ts` — the SDK source of truth

## 2. Install prerequisites

### Core prerequisites

- Go 1.22+
- Node.js + npm

Then from repo root:

```bash
npm install
```

That installs the local JS tooling used by this repo, including `tsx` and `typescript`.

## 3. Pick the lowest-friction first run

Start with the **starter app in browser mode**.

```bash
npm run build:starter
```

That gives you a browser-mode starter `.exe` at repo root.

Why this first?

- no webview-native setup required
- easiest way to prove the repo is healthy
- best place to start editing your own app

## 4. Starter recipes you can copy-paste

If you want a specific starter outcome, use one of these.

### Browser starter build at repo root

```bash
npm run build:starter
```

Result:

- builds `luminka-starter.exe` at repo root
- browser mode
- Windows GUI build path
- includes SDK regeneration, icon generation, and Windows resource generation

### Webview starter build at repo root

```bash
npm run build:starter:webview
```

Result:

- builds `luminka-starter-webview.exe` at repo root
- webview mode
- Windows GUI build path

### Rebuild the SDK only

```bash
npm run build:sdk
```

Use this when you changed `luminka/sdk/luminka.ts` and want fresh generated `luminka.js` copies in `starter/` and the examples.

### Rebuild icons only

```bash
npm run build:icons
```

Use this when you changed the starter icon source asset and want fresh Windows, macOS, and Linux icon outputs.

### Plain generic Go starter build

```bash
go build ./starter
```

Use this when you want the simplest generic Go compilation path.

Important on Windows:

- this is **not** the canonical no-console starter build
- it produces a foreground console build unless you supply the GUI subsystem flag yourself

### Run the starter in detached mode

```bash
.\luminka-starter.exe --detached
```

Use this when the current working directory should become the app root.

### Run the starter in headless mode

```bash
.\luminka-starter.exe --headless
```

Use this when you want the local server and capability bridge without opening browser or webview shell.

### Run the starter in detached + headless mode

```bash
.\luminka-starter.exe --detached --headless
```

Use this when you want one installed binary serving the current directory without opening UI.

## 5. Understand the two build axes

There are two different choices in Luminka.

### A. Display mode

- default build = `browser`
- `-tags webview` = `webview`

Examples:

```bash
go build ./starter
go build -tags webview ./starter
```

### Luminka launch flags

The runtime also recognizes these flags:

- `--root <path>`
- `--root-policy <portable|detached>`
- `--portable`
- `--detached`
- `--headless`

### B. Capability support

These are separate from browser/webview.

- filesystem
- scripts
- shell

Capabilities are not only a runtime toggle. Some are also compile-time gated.

| Capability | Needs build tag? | Needs config flag? |
|---|---|---|
| Filesystem | No | Optional: disable with `DisableFS: true` |
| Scripts | Yes: `scripts` | Yes: `EnableScripts: true` |
| Shell | Yes: `shell` | Yes: `EnableShell: true` |

## 6. Know the default behavior before you are surprised

### Filesystem

- Enabled by default.
- Relative to the app root.
- `examples/hello/` disables it on purpose.

### Scripts

- Disabled by default.
- Requires `-tags scripts` and `EnableScripts: true`.
- Intended as the constrained execution lane.

### Shell

- Disabled by default.
- Requires `-tags shell` and `EnableShell: true`.
- This is unrestricted local command execution.
- It is **full trust**, not a sandbox.

## 7. If you want to make a real app, start in `starter/`

`starter/` is the handoff path from framework repo to your app.

### Edit these first

#### Rename the app

Edit `starter/main.go`:

- `Name`
- `WindowTitle`

#### Change app behavior

Also in `starter/main.go`:

- `Mode`
- `DisableFS`
- `EnableScripts`
- `EnableShell`
- window size/debug options

#### Change the frontend

Edit:

- `starter/dist/index.html`
- `starter/dist/app.js`
- `starter/dist/style.css`

Those are the embedded frontend assets for the starter app right now.

## 8. Where the examples differ

### `starter/`

Use this when you want a base app to rename and reshape.

### `examples/hello/`

Use this when you want the smallest possible SDK + runtime example.

Important: it disables filesystem capability, so do not treat it as the “normal default app”.

### `examples/kanban/`

Use this when you want to see a more complete local-first app flow:

- file-backed state
- reload on file change
- persistence through the SDK
- deep links still resolve through the embedded entry document on unknown `GET` and `HEAD` routes

## 9. SDK ergonomics and expectations

The SDK is browser-first.

### Default connection behavior

In a Luminka-hosted frontend, `createLuminkaClient()` uses `location.host` to infer `ws://<host>/ws`.

### Outside the host context

If you use the SDK elsewhere, pass an explicit URL:

```ts
import { LuminkaClient } from "./luminka.js";

const client = new LuminkaClient({
  url: "ws://127.0.0.1:7777/ws",
});
```

### Connection lifecycle

- `connect()` opens or reuses the socket
- `disconnect()` closes it and rejects pending work
- failed connection attempts produce explicit errors
- `readText()` / `writeText()` are the primary text helpers; `read()` / `write()` remain aliases
- `readBytes()` / `writeBytes()` handle raw file bytes
- `createReadStream()` / `createWriteStream()` and `runScriptStream()` / `runShellStream()` expose byte streams

### Capability failures

If the host disables a capability, the SDK surfaces that as an explicit failure.

Practical pattern:

1. call `appInfo()` early
2. inspect `capabilities`
3. adapt the UI before calling capability-specific methods

## 10. When to regenerate the SDK

If you edit `luminka/sdk/luminka.ts`, run:

```bash
npm run build:sdk
```

That updates the generated `luminka.js` copies embedded by:

- `starter/`
- `examples/hello/`
- `examples/kanban/`

If you forget this step, the runtime may embed stale frontend SDK code.

## 11. Webview troubleshooting first principles

Webview is the most likely onboarding pain point.

If `go build ./starter` works but `go build -tags webview ./starter` does not, think in this order:

1. Is CGO enabled?
2. Is a native compiler toolchain installed?
3. Are platform webview dependencies present?
4. On Windows, is WebView2 available?

Do not debug all of Luminka first. Prove the browser build, then isolate the webview-specific problem.

## 12. Suggested first-hour flow for a new developer

1. `npm install`
2. `npm run build:starter`
3. run the starter app
4. edit `starter/main.go` to rename the app
5. edit `starter/dist/app.js` or `starter/dist/index.html`
6. rebuild and rerun
7. only then try `npm run build:starter:webview`

If you prefer the longer explicit path instead of the recipe script:

1. `npm run build:sdk`
2. `npm run build:icons` if you want the starter icon outputs and Windows resource files
3. `starter\build.bat`
4. run the starter app
5. edit `starter/main.go` to rename the app
6. edit `starter/dist/app.js` or `starter/dist/index.html`
7. rebuild and rerun
8. only then try `npm run build:starter:webview`

## 13. Canon docs if you need deeper truth

- `docs/product.md` — what Luminka is for
- `docs/spec.md` — exact behavior contract
- `docs/arch.md` — intended repo/runtime shape
- `docs/glossary.md` — terminology
