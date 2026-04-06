# Changelog

All notable changes to Luminka are documented here.

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
