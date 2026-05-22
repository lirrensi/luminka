# ADR-0001: Filesystem API targets `node:fs/promises` parity

## Status

Accepted

## Context

Luminka's current filesystem API is a small Node-inspired subset exposed over the runtime WebSocket bridge. It currently supports basic text and byte reads and writes, byte streams, directory listing, existence checks, file-only deletion, and path watching.

That surface is too small for rich local desktop-style applications. In practice it blocks or weakens functionality that is routine in Electron apps and normal in code written against Node's filesystem APIs. The known gaps include:

- no directory-capable delete,
- no rename or move,
- no explicit directory creation,
- no metadata inspection,
- no copy operations,
- no append or truncate helpers,
- no open-handle workflows,
- no Node-style directory iteration,
- weak watch semantics for directory-heavy external sync.

Luminka's transport model is inherently asynchronous. Frontend code talks to the runtime over WebSocket and every filesystem operation crosses a request boundary. Because of that, synchronous Node filesystem APIs are not a natural or honest fit for Luminka. Fake sync wrappers would misrepresent the runtime model and encourage the wrong calling style.

The project therefore needs a clear filesystem parity target that matches how Luminka actually works and is strong enough to guide a major-version redesign.

## Decision

Luminka will target **Promise-first parity with Node's `node:fs/promises` API shape** for its next major filesystem API.

This is a deliberate major-version change. **Backward compatibility is not required** for the filesystem surface in that release. SDK method names, runtime request names, watcher behavior, return shapes, and helper layering may all change where needed to align with the new direction.

The goal is that a frontend written for Luminka should be able to use a filesystem API that feels as close as practical to `node:fs/promises`, while still respecting Luminka's local-runtime constraints.

### 1. Promise-based parity target

- Luminka should expose a filesystem API shaped as closely as practical to `node:fs/promises`.
- The canonical reference is the Node `fs/promises` surface, not Luminka's current ad hoc method set.
- Luminka-specific helpers may still exist, but they are secondary conveniences, not the canonical filesystem contract.

### 2. No synchronous filesystem API target

Luminka will not target synchronous filesystem APIs such as:

- `readFileSync`
- `writeFileSync`
- `statSync`
- `readdirSync`
- `mkdirSync`
- `rmSync`
- `openSync`

The runtime transport is asynchronous by construction, so synchronous API parity is not a design goal.

### 3. File handles are included

Luminka will support `open()` and a `FileHandle`-style async workflow as part of the parity target.

Handle-based operations are considered part of the intended filesystem model, not an optional extra.

### 4. Ownership-changing APIs are excluded for now

Ownership-changing APIs are excluded from the initial parity commitment:

- `chown()`
- `lchown()`
- `filehandle.chown()`

They may be revisited later if a concrete product need appears.

### 5. Major-version breakage is explicitly allowed

The next major filesystem API may:

- replace current method names with Node names,
- remove or demote Luminka-only names from the canonical API,
- change watcher shapes,
- split broad helpers into more precise Node-like operations,
- change error behavior where needed to match the new contract more closely.

### 6. Parity means API-shape alignment under Luminka constraints

Parity does **not** mean Luminka claims to be a literal in-process Node runtime.

Luminka must still preserve:

- root-relative path safety,
- capability gating,
- transport-based execution,
- explicit runtime-managed resources,
- cross-platform behavior where feasible.

So parity means: **closest practical `fs/promises` API and semantic alignment inside Luminka's local runtime model**.

## Canonical target surface

The following sections record the intended target surface so this ADR alone is sufficient to begin implementation planning.

### A. Path-based methods to add or replace

These methods are part of the target public filesystem API.

#### Core file and directory operations

- `access(path[, mode])`
- `appendFile(path, data[, options])`
- `chmod(path, mode)`
- `copyFile(src, dest[, mode])`
- `cp(src, dest[, options])`
- `glob(pattern[, options])`
- `lchmod(path, mode)` when supported by the target platform/runtime contract
- `link(existingPath, newPath)`
- `lstat(path[, options])`
- `lutimes(path, atime, mtime)`
- `mkdir(path[, options])`
- `mkdtemp(prefix[, options])`
- `open(path, flags[, mode])`
- `opendir(path[, options])`
- `readdir(path[, options])`
- `readFile(path[, options])`
- `readlink(path[, options])`
- `realpath(path[, options])`
- `rename(oldPath, newPath)`
- `rm(path[, options])`
- `rmdir(path[, options])` for compatibility, even if `rm()` is the preferred recursive removal API
- `stat(path[, options])`
- `statfs(path[, options])`
- `symlink(target, path[, type])`
- `truncate(path[, len])`
- `unlink(path)`
- `utimes(path, atime, mtime)`
- `watch(path[, options])`
- `writeFile(path, data[, options])`

#### Explicitly excluded from the initial commitment

- `chown(path, uid, gid)`
- `lchown(path, uid, gid)`

### B. `FileHandle` methods to add

When `open()` returns a runtime-backed file handle, Luminka should support the corresponding async handle workflow.

#### Required handle methods

- `filehandle.appendFile(data[, options])`
- `filehandle.chmod(mode)`
- `filehandle.close()`
- `filehandle.createReadStream([options])`
- `filehandle.createWriteStream([options])`
- `filehandle.datasync()`
- `filehandle.read(...)`
- `filehandle.readFile([options])`
- `filehandle.readLines([options])` when practical
- `filehandle.readableWebStream([options])` when practical
- `filehandle.readv(buffers[, position])`
- `filehandle.stat([options])`
- `filehandle.sync()`
- `filehandle.truncate([len])`
- `filehandle.utimes(atime, mtime)`
- `filehandle.write(...)`
- `filehandle.writeFile(data[, options])`
- `filehandle.writev(buffers[, position])`

#### Explicitly excluded from the initial commitment

- `filehandle.chown(uid, gid)`

#### Not part of the parity target

The parity target does not include synchronous handle helpers or sync-only convenience methods.

### C. Existing Luminka methods that are not Node-canonical

The current SDK filesystem surface includes:

- `readText()`
- `writeText()`
- `readBytes()`
- `writeBytes()`
- `createReadStream()`
- `createWriteStream()`
- `list()`
- `remove()`
- `exists()`
- `watch()` + `unwatch()` + `onFileChanged()`
- `trackedTextFile()`

For the next major version:

1. **Node-style names become canonical.**
2. Existing Luminka-specific names may be removed, renamed, or kept only as optional aliases.
3. If an old name remains, it must be documented as a convenience wrapper, not the primary contract.

Expected direction:

- `readText()` / `writeText()` may remain as text-focused conveniences layered over `readFile()` / `writeFile()`.
- `readBytes()` / `writeBytes()` may remain as byte-focused conveniences layered over `readFile()` / `writeFile()` or handle/stream helpers.
- `createReadStream()` / `createWriteStream()` may remain as Luminka extras even though they are not `fs/promises` core methods.
- `trackedTextFile()` may remain as a Luminka-specific higher-level helper.
- `list()` should yield to `readdir()`.
- `remove()` should not remain the only delete API; Node-like `unlink()` and `rm()` become canonical.
- `exists()` should not remain canonical; callers should use `access()`, `stat()`, `lstat()`, or `throwIfNoEntry`-style behavior instead.
- the current `watch()` / `unwatch()` / `onFileChanged()` trio may be replaced by a Node-style watch contract and reintroduced only as wrappers if still useful.

### D. Watching direction

The current watch surface is too narrow for desktop-class external sync and directory-aware applications.

The major-version filesystem redesign should move toward Node-style watch behavior, including:

- directory watching as a first-class concept,
- support for newly created children according to the documented watch contract,
- explicit watch options,
- stronger create/delete/rename visibility,
- a Node-style async consumption model where practical.

Luminka is allowed to diverge internally in implementation strategy, but the public watch API should stop being a bespoke helper pair and move toward the Node model.

### E. Wire protocol freedom

This ADR governs the **public filesystem API direction**, not a requirement that the WebSocket wire event names must literally mirror Node names.

The runtime protocol may be redesigned in the major version if that makes the implementation cleaner. The important contract is the public SDK surface and semantics.

## Missing functionality covered by this decision

Relative to Luminka's current API, this ADR commits the project to fill the following missing or weak capability areas:

### Directory lifecycle

- explicit directory creation,
- empty directory removal,
- recursive tree removal,
- temporary directory creation,
- directory iteration through `opendir()` and richer `readdir()`.

### File movement and copying

- atomic rename where the host OS permits it,
- move semantics via `rename()`,
- direct file copy,
- recursive copy.

### Metadata and inspection

- file or directory stats,
- symlink-aware stats,
- file-system stats,
- path accessibility checks,
- realpath resolution,
- symlink inspection.

### File mutation beyond overwrite

- append,
- truncate,
- timestamp updates,
- mode updates,
- link and symlink creation.

### Open-handle workflows

- open file handles,
- partial reads and writes,
- vectored reads and writes,
- sync and datasync,
- handle-level streams,
- handle-level file metadata.

### Watching

- better folder watching,
- more useful external sync behavior,
- less bespoke watch lifecycle.

## Backward compatibility and release policy

This filesystem redesign is a **major version change**.

The project is explicitly allowed to:

- break the old filesystem SDK naming,
- remove methods that do not fit the new canonical contract,
- replace `exists()` with Node-style patterns,
- split the current broad delete story into `unlink()` and `rm()`,
- replace the current watch API shape,
- change runtime responses and error text where needed.

Backward compatibility is optional, not required.

If compatibility aliases are kept, they should be treated as migration aids rather than part of the long-term canonical API.

## Consequences

### Positive

- The product promise becomes clearer: Luminka offers a serious async local filesystem surface suitable for Electron-class application behavior.
- The SDK direction becomes coherent with the transport architecture: async all the way down.
- The project avoids inventing a one-off filesystem API that drifts from what developers already know from Node.
- Future implementation planning can now proceed from a named target instead of patching gaps one by one.

### Negative

- This is significantly larger in scope than adding `mkdir()` or `rename()` alone.
- The runtime protocol, Go bridge, TypeScript SDK, tests, and canon docs will all need substantial expansion.
- Some Node semantics will need careful translation because Luminka has a root sandbox, capability gating, and a transport boundary.
- Watch semantics, handle lifecycle, and error mapping will require deliberate design rather than mechanical API copying.

## Follow-on work required

This ADR does not itself update the behavioral or architecture canon. It records the decision that future canon and implementation work must follow.

Follow-on work must include:

1. update `docs/spec.md` to replace the current narrow filesystem event table with a broader Promise-first filesystem contract,
2. update `docs/arch.md` to describe the expanded bridge, handle lifecycle model, and watch architecture,
3. update `docs/sdk.md` to document the new canonical Node-style surface and the status of any Luminka convenience wrappers,
4. design the runtime handle/session protocol for `open()` and `FileHandle`,
5. plan staged implementation of the new API in the Go runtime and TypeScript SDK,
6. cut the work as a major-version filesystem program rather than a one-off patch.
