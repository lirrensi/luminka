# Changelog

## v2 — protocol update

This is a standalone update with a breaking transport change.

### Breaking

- The canonical WebSocket transport is now binary: `[header length][JSON header][payload bytes]`.
- File and stream payloads now move as raw bytes, so changes stay byte-accurate.
- Apps that depended on older bridge assumptions should be rebuilt or rewritten against the v2 SDK.

### Updated

- Runtime, SDK, and starter/example apps were brought forward together.
- Small plumbing changes landed across filesystem, stream, shell, and launch paths.
