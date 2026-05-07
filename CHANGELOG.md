# Changelog

All notable changes to Luminka are documented here.

## [3.1.0] - 2026-05-08

Version 3.1 adds configurable port selection, runtime event logging, native
file watching via fsnotify, SDK convenience methods, and practical how-to
recipes — all backwards-compatible additive improvements.

### Added

- Added `--port <number>` CLI flag and `Config.Port` field for setting a fixed
  localhost port. The default (0) still picks an OS-assigned port.
- Added `--logs` CLI flag and `Config.Logs` field for runtime event logging.
  When enabled, the runtime appends JSON Lines entries to `<root>/luminka.log`
  for startup, shutdown, WebSocket connections, filesystem operations, script
  and shell execution, and unknown-message errors.
- Added SDK `log(message)` convenience method. Each call appends a
  `[timestamp] message` line to `luminka.log` via the filesystem bridge.
- Added `docs/recipes.md` with practical how-to patterns for filesystem
  operations, file watching, tracked text files, script and shell execution,
  stream output, embedded binary bundling, and multi-tab coordination.

### Changed

- Upgraded the file watcher from a pure-polling implementation to an
  fsnotify-based backend with a polling fallback. Watched files now receive
  near-instant change notifications on platforms that support it, while
  directories that fsnotify cannot watch fall back transparently to polling.
- Bumped minimum Go version from 1.22 to 1.23 for the fsnotify dependency.
- The `pollOnce()` loop only polls files that fsnotify cannot watch, reducing
  filesystem overhead on capable platforms.

### Compatibility

- All changes in 3.1 are strictly additive and fully backwards-compatible
  with apps built against 3.0.0.
- Existing lock files, protocol frames, SDK call sites, and config structs
  continue to work without modification.

## [3.0.0] - 2026-04-27

Version 3 adds local broadcast coordination and quality-of-life runtime polish without changing the normal SDK adoption path.

### Added

- Added transient local broadcast events between active WebSocket clients in the same Luminka runtime instance.
- Added SDK helpers for `broadcast()`, `onBroadcast()`, and `createMultiTabCoordinator()` so browser-mode multi-tab apps can elect a primary tab, track peers, and exchange lightweight coordination messages.
- Added best-effort webview foregrounding for duplicate launches on Windows by remembering the existing native window identity and focusing it when the app is started again for the same root.

### Changed

- Updated single-instance lock records to carry richer runtime metadata, including mode and optional window identity, while still reading legacy `pid:port` lock records.
- Improved runtime tests around fresh, reused, stale, and legacy PID lock records.

### Compatibility

- Most apps built against the version 2 SDK should continue to work naturally after rebuilding against version 3.
- This is not expected to be a breaking release for normal frontend SDK consumers.
- The main compatibility risk is for projects that hand-edited Luminka's Go runtime internals, custom lock-file handling, or raw WebSocket protocol code instead of using the SDK.

## [2.0.0] - 2026-04-06

Version 2 is a breaking protocol release.

### Changed

- Replaced the old text-only WebSocket transport with a binary-only frame format: `[header length][JSON header][payload bytes]`.
- Moved file and stream payload transfer to raw bytes so filesystem and streaming operations stay byte-accurate.
- Brought the runtime, in-repo TypeScript SDK, starter app, and examples forward together onto the new transport.

### Breaking

- Apps built against the version 1 text transport are not wire-compatible with version 2.
- Existing integrations should be rebuilt or rewritten against the current v2 SDK and protocol shape.

## [1.0.0]

Initial public release.

### Added

- Go runtime for hosting desktop-style web apps.
- Initial SDK and starter flow for app integration.
- Text-only WebSocket transport layer used by the first protocol generation.
