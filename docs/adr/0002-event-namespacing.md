# ADR-0002: Event Namespacing Convention

## Status

Accepted

## Context

Luminka's WebSocket protocol currently uses flat event names across all capability domains:

```
fs_read_file    fs_write_file   fs_stat     fs_watch
script_exec     shell_exec      app_info    ws_auth
broadcast       stream_chunk    handle_read ...
```

This is fine at ~47 events but has problems going forward:

- **No organizational structure.** Events from different domains sit in one flat namespace. Adding custom events (see ADR-0003) without a convention would create collisions.
- **Custom events have no natural slot.** An app developer adding `goal_set` has no guidance on naming. When two extensions both want `sync`, someone loses.
- **Response events are inconsistent.** `fs_response` is a catch-all for ~40 filesystem operations, while `script_response` and `shell_response` are per-domain. There's no rule — just ad hoc decisions.
- **The SDK dispatch (`handleMessage`) hardcodes event names.** Future-proofing means predictable prefixes, not memorizing 47 strings.

Moving to a `namespace:action` convention solves all of these.

## Decision

Luminka will adopt a **`namespace:action` event naming convention** across the entire protocol. This is a **major-version breaking change** with no backward compatibility.

### 1. Request events: `namespace:action`

Every request event (has `id`, client→server) follows `<namespace>:<action>`:

| Namespace | Scope | Examples |
|---|---|---|
| `file:` | Path-based filesystem operations | `file:read`, `file:write`, `file:watch`, `file:stat` |
| `handle:` | Open file handle operations | `handle:read`, `handle:write`, `handle:close` |
| `stream:` | Stream lifecycle | `stream:chunk`, `stream:close` |
| `script:` | Script execution | `script:exec`, `script:stream` |
| `shell:` | Shell execution | `shell:exec`, `shell:stream` |
| `ws:` | WebSocket/runtime services | `ws:auth`, `ws:broadcast` |
| `app:` | Runtime introspection | `app:info` |

Custom events follow the same convention: `goal:set`, `db:query`, `ai:complete`.

### 2. Response events: `response:<namespace>:<action>`

Every response event (has `id` + `ok`, server→client) mirrors the request event with a `response:` prefix:

```
Request:  file:read
Response: response:file:read

Request:  script:exec
Response: response:script:exec

Request:  ws:broadcast
Response: response:ws:broadcast
```

This makes dispatch trivial — strip the `response:` prefix to find the original request. The SDK resolves pending promises by request ID anyway, but the prefix makes response filtering in `handleMessage` a single prefix check instead of 47 case statements.

Malformed frames (no event, bad JSON) that cannot map to a request use `response:error`.

### 3. Push events: `namespace:action` (no `id`)

Push events (no `id`, server→client) stay under their namespace without `response:` prefix. They are distinguished from request events by the absence of `id`:

```
file:changed       ws:broadcast       stream:chunk       stream:close
```

`ws:broadcast` is both a request event and a push event — the push variant has no `id`.

### 4. Complete rename map

#### Request events

| Old | New |
|---|---|
| `ws_auth` | `ws:auth` |
| `app_info` | `app:info` |
| `broadcast` | `ws:broadcast` |
| `script_exec` | `script:exec` |
| `script_exec_stream` | `script:stream` |
| `shell_exec` | `shell:exec` |
| `shell_exec_stream` | `shell:stream` |
| `fs_read_text` | `file:read_text` |
| `fs_write_text` | `file:write_text` |
| `fs_list` | `file:list` |
| `fs_delete` | `file:delete` |
| `fs_exists` | `file:exists` |
| `fs_watch` | `file:watch` |
| `fs_unwatch` | `file:unwatch` |
| `fs_open_read` | `file:open_read` |
| `fs_open_write` | `file:open_write` |
| `fs_access` | `file:access` |
| `fs_append_file` | `file:append` |
| `fs_chmod` | `file:chmod` |
| `fs_copy_file` | `file:copy` |
| `fs_cp` | `file:cp` |
| `fs_link` | `file:link` |
| `fs_lstat` | `file:lstat` |
| `fs_mkdir` | `file:mkdir` |
| `fs_mkdtemp` | `file:mkdtemp` |
| `fs_open` | `file:open` |
| `fs_read_file` | `file:read` |
| `fs_readdir` | `file:readdir` |
| `fs_readlink` | `file:readlink` |
| `fs_realpath` | `file:realpath` |
| `fs_rename` | `file:rename` |
| `fs_rm` | `file:rm` |
| `fs_rmdir` | `file:rmdir` |
| `fs_stat` | `file:stat` |
| `fs_symlink` | `file:symlink` |
| `fs_truncate` | `file:truncate` |
| `fs_unlink` | `file:unlink` |
| `fs_utimes` | `file:utimes` |
| `fs_write_file` | `file:write` |
| `stream_chunk` | `stream:chunk` |
| `stream_close` | `stream:close` |
| `handle_read` | `handle:read` |
| `handle_write` | `handle:write` |
| `handle_close` | `handle:close` |
| `handle_stat` | `handle:stat` |
| `handle_truncate` | `handle:truncate` |
| `handle_sync` | `handle:sync` |
| `handle_datasync` | `handle:datasync` |
| `handle_chmod` | `handle:chmod` |
| `handle_utimes` | `handle:utimes` |

#### Response events

| Old | New |
|---|---|
| `fs_response` | `response:file:...` (per-operation) |
| `broadcast_response` | `response:ws:broadcast` |
| `script_response` | `response:script:exec` |
| `shell_response` | `response:shell:exec` |
| `error` | `response:error` |

#### Push events

| Old | New |
|---|---|
| `fs_changed` | `file:changed` |
| `broadcast` (push) | `ws:broadcast` |
| `stream_chunk` | `stream:chunk` |
| `stream_close` | `stream:close` |

## Consequences

### Positive

- Custom events have a clear slot and naming convention.
- Response dispatch becomes predictable: one prefix check instead of 47 case statements.
- SDK `handleMessage` can route responses generically.
- Documentation and debugging benefit from organized event logs.

### Negative

- Every event string changes. Every file that references an event name must be updated: Go runtime, TypeScript SDK, tests, spec, architecture docs.
- No backward compatibility. All existing apps, examples, and starters must be updated.
- `fs_response` was a convenient catch-all for filesystem responses. Now each operation gets its own `response:file:<action>`, which means ~30 different response events for the filesystem domain. This costs nothing in implementation (the response is keyed to the request ID, not the event name), but it's more verbose in wire logs.

## Follow-on work

1. Update `docs/spec.md` — all event tables, examples, response shapes.
2. Update `docs/arch.md` — any event name references.
3. Implement the rename across Go runtime, TypeScript SDK, and all tests.
4. Update example apps and starter.
