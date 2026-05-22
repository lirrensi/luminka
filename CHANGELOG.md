# Changelog

All notable changes to Luminka are documented here.

## [3.2.0] - 2026-05-22

Version 3.2 is a major filesystem expansion that brings the Luminka SDK to
full `node:fs/promises` API parity. The filesystem bridge is no longer a thin
utility — it is a mature, framework-grade local capability that gives your
web app proper hands to work with files: read, write, move, copy, link,
browse, inspect, watch, truncate, append, and open handles with partial
read/write, streaming, and line-level iteration.

### Added

#### Canonical Node `fs/promises`-shaped API (24 methods)

The SDK now exposes a complete Promise-based filesystem surface modelled on
Node's `fs/promises`. All methods are app-root-relative and capability-gated:

| Category | Methods |
|---|---|
| Read/write | `readFile()`, `writeFile()`, `appendFile()` |
| Directory lifecycle | `mkdir()` (with `recursive`), `readdir()` (with `withFileTypes`), `rmdir()`, `mkdtemp()` |
| Remove | `unlink()` (file), `rm()` (with `recursive`/`force`) |
| Move and copy | `rename()`, `copyFile()`, `cp()` (with `recursive`) |
| Metadata | `stat()`, `lstat()` (symlink-aware), `access()`, `realpath()` |
| Permission/timestamp | `chmod()`, `utimes()`, `truncate()` |
| Links | `symlink()`, `readlink()`, `link()` |

- Added `open(path, flags?, mode?)` returning a `FileHandle` for fine-grained
  handle-based workflows.

#### FileHandle class

The `FileHandle` returned by `open()` supports the full async handle surface:

- `read(options?)` — partial read at optional position
- `write(data, position?)` — partial write at optional position
- `close()` — release the handle
- `stat()` — metadata for the open file
- `truncate(len?)` — truncate through the handle
- `sync()` / `datasync()` — flush to disk
- `readFile()` / `writeFile()` / `appendFile()` — whole-file helpers on a handle
- `chmod()` / `utimes()` — permission and timestamp changes on a handle
- `readLines()` — async generator for lazy line-by-line iteration
- `createReadStream()` / `createWriteStream()` — Web Streams API integration

#### Wire protocol extensions

Added 23 new filesystem request types to the WebSocket transport, all using
the existing v2 binary frame format:

`fs_access`, `fs_append_file`, `fs_chmod`, `fs_copy_file`, `fs_cp`,
`fs_link`, `fs_lstat`, `fs_mkdir`, `fs_mkdtemp`, `fs_open`, `fs_read_file`,
`fs_readdir`, `fs_readlink`, `fs_realpath`, `fs_rename`, `fs_rm`, `fs_rmdir`,
`fs_stat`, `fs_symlink`, `fs_truncate`, `fs_unlink`, `fs_utimes`,
`fs_write_file`

#### Go runtime bridge

- Added `statToMap()` helper for serialising `os.FileInfo` to structured JSON.
- Added `flagToOpenFlags()` for POSIX-style flag string (`r`, `r+`, `w`, `w+`,
  `a`, `a+`) to Go `os.OpenFile` flag conversion.
- Extended `FSBridge` with full directory lifecycle, copy/move/link, metadata
  and mutation, symlink, and open-handle methods.

### Changed

- `readText()` / `writeText()` / `readBytes()` / `writeBytes()` / `list()` /
  `remove()` / `exists()` are preserved as backward-compatible convenience
  wrappers that delegate to the canonical Node-style methods. They may be
  removed in a future major version.
- The SDK `StatResult` now includes `isSymlink` in addition to the existing
  `size`, `mode`, `modTime`, and `isDirectory` fields.
- `readdir()` with `{ withFileTypes: true }` now returns `DirEntry[]` with
  `name`, `isDirectory`, `isFile`, `isSymlink`, and `isBlockDevice` on each
  entry.

### Compatibility

- **Wire protocol**: Protocol version remains `"2"`. The new filesystem
  operations extend the existing binary frame format — no framing or transport
  changes.
- **SDK**: The new canonical methods are additive. Existing code using
  `readText()`, `writeText()`, `list()`, `remove()`, `exists()`, streams,
  `trackedTextFile()`, broadcast, and multi-tab coordination continues to
  work without changes.
- **Go runtime**: All new `FSBridge` methods are additive. Existing apps that
  embed Luminka as a Go module will compile against the expanded bridge
  without regressions.
- **Breaking scope**: This release intentionally breaks the old ad-hoc
  filesystem naming convention by introducing Node-canonical names. Old
  Luminka-specific names are preserved as wrappers.

### Documentation

- Added `docs/adr/0001-fs-promises-parity.md` recording the decision to
  target Node `fs/promises` parity.
- Extended `docs/sdk.md` with full documentation for all new canonical
  methods, `FileHandle` workflows, `readLines()`, `opendir()`, and the
  backward-compatible legacy wrappers.
- Updated `docs/arch.md` to describe the expanded filesystem bridge
  architecture and `FileHandle` lifecycle.

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
