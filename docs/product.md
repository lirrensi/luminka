# Luminka

## Overview

Luminka is a small Go-based framework and product for turning a built web app into a local desktop-style app with a tiny runtime.

It exists for developers who like building in HTML, CSS, and JavaScript, but need a clean way to cross the browser boundary and talk to the local machine. Browsers are great at UI and application logic, but they cannot directly read local files, run scripts, or manage a local app lifecycle. Electron solves that with a large bundled runtime. Tauri solves it with a more opinionated native stack. Luminka solves it with a thin Go executable and a WebSocket bridge.

The result is intentionally simple:

- build your frontend however you want,
- embed the built static assets into a small executable,
- expose local capabilities through Luminka,
- keep user data outside the binary, beside the app.

Luminka grew out of the original PortableKanban concept, but the framework is now the product. PortableKanban becomes an example app built on top of it.

## Target Users

Luminka is for developers who:

- already want to build UI as a web app,
- want a small local runtime instead of a heavy browser app shell,
- want explicit control over local capabilities,
- want a simple starter they can clone and edit,
- may later want to import the framework directly into their own repo.

It is especially aimed at small local-first tools, internal utilities, mini-apps, personal software, and workflow helpers.

## Core Capabilities

Luminka fundamentally provides five things:

1. **Static app hosting inside a single binary**  
   A built frontend is embedded into the executable and served from inside it.

2. **A local capability bridge for web apps**  
   The frontend talks to the runtime over WebSocket. The runtime can expose filesystem access, script execution, and shell execution depending on configuration and build mode.

3. **Two equal app shells**  
   The same app can be built either as:
   - a **browser build**, which opens in the system browser, or
   - a **webview build**, which opens in a native WebView window.

4. **Portable local app behavior**  
   The app behaves like a small portable local program. Data lives beside the binary by default. Multiple copies can exist independently in different folders.

5. **A minimal developer experience layer**  
   Luminka exposes a canonical WebSocket protocol and also ships a small TypeScript SDK that wraps it with easier request/response style calls.

## Design Principles

### Web-first, not native-first

Luminka assumes the main app logic and UI live in the web layer. The Go side is a bridge and runtime shell, not the center of the app.

### Small, explicit, and local

Capabilities are exposed deliberately. Filesystem access is the happy path and is enabled by default, but it can be turned off. Script and shell execution are opt-in. The runtime listens only on localhost.

### One binary for the app, external files for the data

The app binary should be easy to copy around. The UI ships inside the executable. Data, logs, and project files stay outside it.

### Framework first, starter friendly

Luminka is a framework, but it should still be easy to adopt by cloning a starter, dropping in a frontend build, and editing from there.

### No fake safety story

If a developer enables powerful local capabilities, the frontend is trusted local code. Luminka does not pretend otherwise.

## Main User Flows

### 1. Build a simple local app

A developer creates or builds a frontend app, places the output into the expected static/dist location, configures Luminka, and builds a single executable. The app opens in browser mode or webview mode and can use the SDK to call the runtime.

### 2. Build a portable app with local storage

A developer uses Luminka's default filesystem capability so the frontend can read and write files beside the binary. The app remains self-contained and portable across folders and machines.

### 3. Build a frontend that runs project scripts

A developer enables script execution and places scripts alongside the app or project files. The frontend calls those scripts through Luminka without needing a full Node or Electron-style runtime.

### 4. Build a fully trusted local power tool

If the developer wants full system command execution, they can explicitly enable shell access. This turns Luminka into a very thin local bridge for trusted software.

### 5. Start from the starter, then grow beyond it

A developer can begin by cloning the official starter, replacing the frontend, renaming the app, and building it. If they outgrow that flow, they can import Luminka more directly and structure their own repo around it.

## System Shape

At a high level, Luminka has four parts:

1. **Go runtime/framework**  
   Handles lifecycle, capability exposure, static serving, local process behavior, and browser/webview launching.

2. **Canonical WebSocket protocol**  
   The wire-level contract between frontend and runtime.

3. **TypeScript SDK**  
   A small in-repo client helper that hides WebSocket request/response details behind simple functions.

4. **Starter and example apps**  
   A minimal starter for adoption and example apps, including kanban, that prove the framework shape in practice.

The frontend build system is not part of Luminka. React, Vue, vanilla HTML, or any other stack may be used as long as the final result is static assets that can be embedded into the executable.

## Non-Goals

Luminka is not trying to be:

- a full frontend build tool,
- an npm-style SDK product,
- a secure sandbox for untrusted frontend code,
- a replacement for every native desktop framework,
- a large plugin ecosystem in v1,
- a cloud app platform,
- a schema-aware application runtime.

It also does not try to own the application's internal data model. Luminka provides transport and local capability access. The app decides what its files mean.

## Initial Product Shape

The initial Luminka repository is expected to contain:

- the core `luminka/` runtime,
- the in-repo TypeScript SDK,
- a `starter/` app scaffold,
- an `examples/` area with at least a kanban example.

That shape supports both intended adoption paths:

- clone the starter and make it yours,
- or import Luminka more directly in a custom repo.
