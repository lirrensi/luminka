# Luminka

<img src="./assets/lumi.png" alt="Luminka" width="100%" />

## ✨ TL;DR

Luminka is a tiny Go runtime for turning a built web app into a portable `.exe`.

It gives you a simple way to ship a web app like a desktop app, without Electron, Tauri, or a heavier framework shell.

## 💡 Why it exists

Luminka exists to make web apps feel more powerful while staying simple. 🐾

It is meant for the “I just want to bundle this web app and run it anywhere” case:

- one portable executable 📦
- no installer 🧊
- runs from any folder 📁
- can live in many copies across many folders 🔁
- keeps the app lightweight and unopinionated 🌿

If you need more power, you bring it yourself through scripts like Python, TypeScript, PowerShell, or Bash.

### 🆚 Compared with Wails

- Wails is broader and more framework-like.
- Luminka does one job: package your web app into a portable `.exe`.
- Luminka stays lightweight and unopinionated; complex system work lives in scripts.
- Wails is the big, serious ship. Luminka is the tiny, quick little boat. 🚤

At its core, Luminka ships built web apps as local desktop-style apps.

## 🧩 What you can build

Luminka is a great fit for apps that benefit from local files and persistent storage, like:

- task managers and personal productivity apps ✅
- media libraries and media players 🎵
- note apps, trackers, and dashboards 🗂️
- tools that want local saving instead of browser-only storage 💾
- any app that feels cramped as a pure website 🌍

If the browser is limiting you, Luminka gives your web app a more capable home.

## 📦 Portable by design

Distribution is simple: copy the `.exe` into a folder with your data, and you are done.

- no installer
- no setup wizard
- easy to duplicate
- easy to move between folders and machines

That makes it a very natural fit for portable apps and self-contained workflows.

## 🌐 If PWA feels limiting

If a PWA is close, but not quite enough, Luminka can be the next step.

It keeps the web app model you already know, while giving you local app packaging, filesystem access, and real portability.

It gives you:

- a **single executable** with embedded frontend assets,
- bundled internal scripts that can live inside the `.exe` itself,
- a **localhost WebSocket bridge** from frontend to host runtime,
- two app shells: **browser** and **webview**,
- explicit capability gates for **filesystem**, **scripts**, and **shell**.

## Start here

If you are new to the repo, use this order:

1. Read this README.
2. Follow [`docs/onboarding.md`](docs/onboarding.md).
3. Start from `starter/` unless you specifically want an example.

## Which app should I open?

| Path | Use it when | Notes |
|---|---|---|
| `starter/` | You want to make your own app | Canonical clone-and-edit starting point |
| `examples/hello/` | You want the smallest runnable example | Filesystem capability is disabled on purpose |
| `examples/kanban/` | You want a more real app shape | Uses filesystem-backed persistence |

## Prerequisites

### Required

- **Go 1.22+**
- **Node.js + npm**
- `npm install` in the repo root to install local dev dependencies like **`tsx`** and **TypeScript**

### Required for webview builds

- **CGO enabled**
- a working **native C/C++ toolchain** for your platform
- native libraries required by `github.com/webview/webview_go`
- on **Windows**, webview builds typically expect **Microsoft Edge WebView2** to be available at runtime

If you want the lowest-friction first run, start with a **browser build**.

## Quick start

### 1) Install JS tooling

```bash
npm install
```

### 2) Rebuild the in-repo SDK outputs

```bash
npm run build:sdk
```

This transpiles `luminka/sdk/luminka.ts` and writes generated `luminka.js` copies into:

- `starter/dist/`
- `examples/hello/dist/`
- `examples/kanban/dist/`

### 3) Build the starter app

```bash
go build ./starter
```

### 4) Run it

Run the produced binary from the repo root or your output location.

By default, `starter/` builds in **browser mode**.

## Build modes and tags

Luminka has two separate ideas:

1. **display mode**: browser vs webview
2. **capability support**: filesystem vs scripts vs shell

They are related, but not the same thing.

### Display mode

`starter/`, `examples/hello/`, and `examples/kanban/` choose mode through build tags:

- default build: **browser**
- `-tags webview`: **webview**

Examples:

```bash
go build ./starter
go build -tags webview ./starter
```

The Windows helper scripts reflect that split:

- `build.bat` -> browser build
- `build_webview.bat` -> webview build

### Capability support

Capabilities are controlled by both:

- **compile-time tags**, and
- **runtime config** in the app's `main.go`

| Capability | Compile-time requirement | Runtime config requirement | Default in repo |
|---|---|---|---|
| Filesystem | none | leave `DisableFS` as `false` | enabled in `starter/` and `kanban/`, disabled in `hello/` |
| Scripts | build with `-tags scripts` | set `EnableScripts: true` | off |
| Shell | build with `-tags shell` | set `EnableShell: true` | off |

Examples:

```bash
go build -tags scripts ./starter
go build -tags shell ./starter
go build -tags "webview scripts shell" ./starter
```

Important:

- **Filesystem is exposed by default** unless the app sets `DisableFS: true`.
- `examples/hello/` intentionally disables filesystem access.
- **Scripts are not enabled just because code exists**. You must compile with `-tags scripts` **and** set `EnableScripts: true`.
- **Shell is full-trust mode**. It is not a sandbox or “safe mode”. You must compile with `-tags shell` **and** set `EnableShell: true`.

### Script locations

Scripts can be resolved in two ways:

- **External scripts**: plain relative paths under the app root, usually living in the same folder as the executable.
- **Internal scripts**: bundled into the final Go build and referenced with `internal:<path>`.

That means you can ship scripts inside the `.exe` itself for release builds, while still keeping loose scripts beside the app when you want them editable.

The runtime may materialize an internal script to a temporary file when execution needs a real path, but the selector stays `internal:...`.

## First-run editing map

If you are turning `starter/` into your app, these are the first places to edit:

| What you want to change | Where to change it |
|---|---|
| App name / runtime identity | `starter/main.go` -> `Name` |
| Window title | `starter/main.go` -> `WindowTitle` |
| Window size / behavior | `starter/main.go` |
| Capability defaults | `starter/main.go` -> `DisableFS`, `EnableScripts`, `EnableShell` |
| Frontend HTML entry | `starter/dist/index.html` |
| Frontend behavior | `starter/dist/app.js` |
| Frontend styles | `starter/dist/style.css` |
| SDK source of truth | `luminka/sdk/luminka.ts` |
| Regenerate embedded SDK copies | `npm run build:sdk` |

## SDK behavior you should know early

The frontend SDK lives at `luminka/sdk/luminka.ts`.

Key expectations:

- In a normal Luminka-hosted browser/webview app, `createLuminkaClient()` can infer the WebSocket URL from `location.host`.
- Outside that host context, pass an explicit URL, for example:

```ts
new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" })
```

- `client.connect()` manages the socket lifecycle.
- `client.disconnect()` closes the socket and rejects pending requests.
- Capability-disabled calls fail explicitly.
- `appInfo()` reports the resolved runtime capabilities so your frontend can adapt.

If you call filesystem, script, or shell APIs when the capability is unavailable, expect an explicit runtime error rather than silent fallback.

## Browser vs webview: what should I choose?

### Choose browser mode when

- you want the easiest first run,
- you are debugging startup,
- you do not want native webview dependencies yet.

### Choose webview mode when

- you want a single-window desktop app feel,
- you are ready to deal with native toolchain setup,
- you understand that build or launch failures may be platform-specific.

## Repo shape

- `luminka/` — Go runtime core
- `luminka/sdk/` — TypeScript SDK source of truth
- `starter/` — main adoption path
- `examples/hello/` — smallest example, no filesystem capability
- `examples/kanban/` — fuller filesystem-backed example
- `docs/` — product/spec/architecture/onboarding docs
- `scripts/build_sdk.ts` — regenerates browser-ready SDK copies

## Common commands

```bash
npm install
npm run build:sdk
go build ./starter
go build ./examples/hello
go build ./examples/kanban
go build -tags webview ./starter
go build -tags scripts ./starter
go build -tags shell ./starter
```

Windows helpers live in each app folder as `build.bat` and `build_webview.bat`.

## Webview friction notes

Webview builds are usually the highest-friction path.

If a browser build works but a webview build or launch fails, check:

- CGO is enabled
- your native compiler toolchain is installed
- platform webview dependencies are present
- on Windows, WebView2 is available

If you are onboarding a new developer, have them prove the browser build first.

## More docs

- [`docs/onboarding.md`](docs/onboarding.md) — practical setup and first edits
- [`docs/product.md`](docs/product.md) — product overview
- [`docs/spec.md`](docs/spec.md) — behavior and protocol spec
- [`docs/arch.md`](docs/arch.md) — repository and runtime architecture
- [`docs/glossary.md`](docs/glossary.md) — canonical terms
