# Changelog

All notable changes to Luminka are documented here.

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
