# Glossary

## Luminka

The framework and product defined by this repository. Luminka packages a built web app into a small Go-based local runtime with a WebSocket bridge to local capabilities.

## Runtime

The Go executable portion of a Luminka app. It serves embedded frontend assets, exposes enabled capabilities, manages lifecycle, and opens either a browser or a webview shell.

## Frontend

The built static web app embedded into a Luminka executable. The frontend may be produced by any web stack as long as the final output is static assets.

## Embedded Assets

The built frontend files compiled into the executable and served from inside it at runtime.

## Binary Folder

The folder containing the built Luminka executable. This is the canonical default location for external data, logs, scripts, and related app files.

## Starter

The official minimal scaffold in this repository for creating a new Luminka app. It imports the Luminka runtime and provides the default adoption path.

## Example App

A concrete app included to demonstrate Luminka in practice. PortableKanban is expected to become one of these examples.

## Browser Build

A first-class Luminka build target that serves the embedded app and opens it in the system browser.

## Webview Build

A first-class Luminka build target that serves the embedded app and opens it in a native WebView window.

## Capability

A runtime feature that the frontend may call through the Luminka protocol, such as filesystem access, script execution, or shell execution.

## Filesystem Capability

The WebSocket-exposed ability for the frontend to read, write, list, delete, or watch files through Luminka. This capability is enabled by default but may be disabled by configuration.

## Script Execution

A constrained execution capability that runs a validated local file through a runner, with optional appended arguments. The file is validated inside the app's local project area. It is distinct from unrestricted shell execution.

## Shell Execution

An unrestricted execution capability that passes commands directly to local process spawning with no command validation beyond normal runtime handling.

## WebSocket Protocol

The canonical wire-level contract between the frontend and the Luminka runtime.

## TypeScript SDK

The in-repository client helper that wraps the WebSocket protocol with simpler request/response style functions. It is the ergonomic layer, not the canonical wire contract.

## Localhost-Only

The rule that Luminka listens only on loopback interfaces and is not intended to expose its runtime bridge on the network.

## Trusted Frontend

The security assumption that the embedded frontend is trusted application code. If powerful capabilities are enabled, Luminka does not try to sandbox that code.
