import test from "node:test";
import assert from "node:assert/strict";

import { decodeLuminkaFrame, encodeLuminkaFrame, LuminkaClient } from "./luminka.ts";

type Listener = (event?: any) => void;

async function flushAsyncWork(): Promise<void> {
  await new Promise((resolve) => globalThis.setTimeout(resolve, 0));
}

class FakeWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;

  static instances: FakeWebSocket[] = [];

  readonly url: string;
  readyState = FakeWebSocket.CONNECTING;
  binaryType = "";
  sent: Uint8Array[] = [];
  closeCalls = 0;
  private listeners = new Map<string, Set<Listener>>();

  constructor(url: string) {
    this.url = url;
    FakeWebSocket.instances.push(this);
  }

  addEventListener(type: string, listener: Listener): void {
    const listeners = this.listeners.get(type) ?? new Set<Listener>();
    listeners.add(listener);
    this.listeners.set(type, listeners);
  }

  removeEventListener(type: string, listener: Listener): void {
    this.listeners.get(type)?.delete(listener);
  }

  send(data: Uint8Array): void {
    this.sent.push(data);
  }

  close(): void {
    this.closeCalls += 1;
    this.readyState = FakeWebSocket.CLOSED;
    this.emit("close");
  }

  open(): void {
    this.readyState = FakeWebSocket.OPEN;
    this.emit("open");
  }

  message(header: any, payload: Uint8Array = new Uint8Array()): void {
    this.emit("message", { data: encodeLuminkaFrame(header, payload) });
  }

  rawMessage(data: unknown): void {
    this.emit("message", { data });
  }

  error(): void {
    this.emit("error");
  }

  emit(type: string, event?: any): void {
    for (const listener of this.listeners.get(type) ?? []) {
      listener(event);
    }
  }

  static reset(): void {
    FakeWebSocket.instances = [];
  }
}

test.beforeEach(() => {
  FakeWebSocket.reset();
  (globalThis as any).WebSocket = FakeWebSocket;
});

test("LuminkaClient frame helpers round trip", () => {
  const frame = encodeLuminkaFrame({ event: "fs_changed", path: "notes.txt" }, new Uint8Array([1, 2, 3]));
  const decoded = decodeLuminkaFrame(frame);
  assert.deepEqual(decoded.header, { event: "fs_changed", path: "notes.txt" });
  assert.deepEqual(Array.from(decoded.payload), [1, 2, 3]);
});

test("LuminkaClient appInfo sends a binary request and parses response", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const connectPromise = client.appInfo();
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket, "expected WebSocket instance");

  socket.open();
  await flushAsyncWork();
  const request = decodeLuminkaFrame(socket.sent[0] ?? new Uint8Array());
  assert.equal(request.header.event, "app_info");
  assert.equal(request.header.id, "luminka-1");

  socket.message({
    event: "app_info",
    id: request.header.id,
    ok: true,
    name: "starter",
    app_version: "1.2.3",
    runtime_version: "2.0.0",
    protocol_version: "2",
    mode: "webview",
    root: "C:/apps/starter",
    capabilities: { fs: true, scripts: false, shell: false },
  });

  const info = await connectPromise;
  assert.deepEqual(info, {
    name: "starter",
    app_version: "1.2.3",
    runtime_version: "2.0.0",
    protocol_version: "2",
    mode: "webview",
    root: "C:/apps/starter",
    capabilities: { fs: true, scripts: false, shell: false },
  });
});

test("LuminkaClient read/write aliases call text helpers", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const readAlias = client.read("secret.txt");
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket, "expected WebSocket instance");

  socket.open();
  await flushAsyncWork();
  const request = decodeLuminkaFrame(socket.sent[0] ?? new Uint8Array());
  assert.equal(request.header.event, "fs_read_text");
  assert.equal(request.header.path, "secret.txt");

  socket.message({ event: "fs_read_text", id: request.header.id, ok: true, data: "hello" });
  assert.equal(await readAlias, "hello");

  const writePromise = client.write("secret.txt", "updated");
  await flushAsyncWork();
  const writeRequest = decodeLuminkaFrame(socket.sent[1] ?? new Uint8Array());
  assert.equal(writeRequest.header.event, "fs_write_text");
  assert.equal(writeRequest.header.data, "updated");
  socket.message({ event: "fs_write_text", id: writeRequest.header.id, ok: true });
  await writePromise;
});

test("LuminkaClient trackedTextFile loads and saves text", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const file = client.trackedTextFile("workspace.emptyspace.xml", { debounceMs: 0 });
  const loadPromise = file.load();
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket, "expected WebSocket instance");

  socket.open();
  await flushAsyncWork();
  const watchRequest = decodeLuminkaFrame(socket.sent[0] ?? new Uint8Array());
  assert.equal(watchRequest.header.event, "fs_watch");
  assert.equal(watchRequest.header.path, "workspace.emptyspace.xml");
  socket.message({ event: "fs_watch", id: watchRequest.header.id, ok: true });

  await flushAsyncWork();
  const readRequest = decodeLuminkaFrame(socket.sent[1] ?? new Uint8Array());
  assert.equal(readRequest.header.event, "fs_read_text");
  assert.equal(readRequest.header.path, "workspace.emptyspace.xml");
  socket.message({ event: "fs_read_text", id: readRequest.header.id, ok: true, data: "<workspace />" });

  assert.equal(await loadPromise, "<workspace />");
  assert.equal(file.getText(), "<workspace />");

  const savePromise = file.save("<workspace name=\"Updated\" />");
  await flushAsyncWork();
  const writeRequest = decodeLuminkaFrame(socket.sent[2] ?? new Uint8Array());
  assert.equal(writeRequest.header.event, "fs_write_text");
  assert.equal(writeRequest.header.path, "workspace.emptyspace.xml");
  assert.equal(writeRequest.header.data, "<workspace name=\"Updated\" />");
  socket.message({ event: "fs_write_text", id: writeRequest.header.id, ok: true });

  await savePromise;
  assert.equal(file.getText(), "<workspace name=\"Updated\" />");
});

test("LuminkaClient trackedTextFile emits debounced external text changes", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const file = client.trackedTextFile("workspace.emptyspace.xml", { debounceMs: 0 });
  const loadPromise = file.load();
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket, "expected WebSocket instance");

  socket.open();
  await flushAsyncWork();
  const watchRequest = decodeLuminkaFrame(socket.sent[0] ?? new Uint8Array());
  socket.message({ event: "fs_watch", id: watchRequest.header.id, ok: true });
  await flushAsyncWork();
  const initialReadRequest = decodeLuminkaFrame(socket.sent[1] ?? new Uint8Array());
  socket.message({ event: "fs_read_text", id: initialReadRequest.header.id, ok: true, data: "before" });
  assert.equal(await loadPromise, "before");

  const changes: string[] = [];
  file.onExternalChange((text) => {
    changes.push(text);
  });
  socket.message({ event: "fs_changed", path: "workspace.emptyspace.xml" });
  await new Promise((resolve) => globalThis.setTimeout(resolve, 0));

  const externalReadRequest = decodeLuminkaFrame(socket.sent[2] ?? new Uint8Array());
  assert.equal(externalReadRequest.header.event, "fs_read_text");
  assert.equal(externalReadRequest.header.path, "workspace.emptyspace.xml");
  socket.message({ event: "fs_read_text", id: externalReadRequest.header.id, ok: true, data: "after" });

  await flushAsyncWork();
  assert.deepEqual(changes, ["after"]);
  assert.equal(file.getText(), "after");
});

test("LuminkaClient trackedTextFile suppresses self write echoes", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const file = client.trackedTextFile("workspace.emptyspace.xml", { debounceMs: 0 });
  const loadPromise = file.load();
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket, "expected WebSocket instance");

  socket.open();
  await flushAsyncWork();
  const watchRequest = decodeLuminkaFrame(socket.sent[0] ?? new Uint8Array());
  socket.message({ event: "fs_watch", id: watchRequest.header.id, ok: true });
  await flushAsyncWork();
  const initialReadRequest = decodeLuminkaFrame(socket.sent[1] ?? new Uint8Array());
  socket.message({ event: "fs_read_text", id: initialReadRequest.header.id, ok: true, data: "before" });
  assert.equal(await loadPromise, "before");

  const changes: string[] = [];
  file.onExternalChange((text) => {
    changes.push(text);
  });
  const savePromise = file.save("after");
  await flushAsyncWork();
  const writeRequest = decodeLuminkaFrame(socket.sent[2] ?? new Uint8Array());
  assert.equal(writeRequest.header.event, "fs_write_text");
  assert.equal(writeRequest.header.path, "workspace.emptyspace.xml");
  assert.equal(writeRequest.header.data, "after");
  socket.message({ event: "fs_write_text", id: writeRequest.header.id, ok: true });
  await savePromise;

  socket.message({ event: "fs_changed", path: "workspace.emptyspace.xml" });
  await new Promise((resolve) => globalThis.setTimeout(resolve, 0));

  const echoReadRequest = decodeLuminkaFrame(socket.sent[3] ?? new Uint8Array());
  assert.equal(echoReadRequest.header.event, "fs_read_text");
  assert.equal(echoReadRequest.header.path, "workspace.emptyspace.xml");
  socket.message({ event: "fs_read_text", id: echoReadRequest.header.id, ok: true, data: "after" });

  await flushAsyncWork();
  assert.deepEqual(changes, []);
  assert.equal(file.getText(), "after");
});

test("LuminkaClient trackedTextFile serializes concurrent watch startup", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const file = client.trackedTextFile("workspace.emptyspace.xml", { debounceMs: 0 });
  const operations = Promise.all([file.load(), file.save("after")]);
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket, "expected WebSocket instance");

  socket.open();
  await flushAsyncWork();
  const watchRequestsBeforeResponse = socket.sent
    .map((frame) => decodeLuminkaFrame(frame).header)
    .filter((header) => header.event === "fs_watch");
  assert.equal(watchRequestsBeforeResponse.length, 1);

  const watchRequest = watchRequestsBeforeResponse[0];
  socket.message({ event: "fs_watch", id: watchRequest.id, ok: true });
  await flushAsyncWork();

  const pendingRequests = socket.sent
    .map((frame) => decodeLuminkaFrame(frame).header)
    .filter((header) => header.event === "fs_read_text" || header.event === "fs_write_text");
  assert.equal(pendingRequests.length, 2);
  for (const request of pendingRequests) {
    assert.equal(request.path, "workspace.emptyspace.xml");
    if (request.event === "fs_read_text") {
      socket.message({ event: "fs_read_text", id: request.id, ok: true, data: "before" });
    } else {
      assert.equal(request.data, "after");
      socket.message({ event: "fs_write_text", id: request.id, ok: true });
    }
  }

  await operations;
  const allWatchRequests = socket.sent
    .map((frame) => decodeLuminkaFrame(frame).header)
    .filter((header) => header.event === "fs_watch");
  assert.equal(allWatchRequests.length, 1);
});

test("LuminkaClient assembles byte streams from chunks", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const bytesPromise = client.readBytes("payload.bin");
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket, "expected WebSocket instance");

  socket.open();
  await flushAsyncWork();
  const request = decodeLuminkaFrame(socket.sent[0] ?? new Uint8Array());
  socket.message({ event: "fs_open_read", id: request.header.id, ok: true, stream_id: "stream-1" });
  socket.message({ event: "stream_chunk", stream_id: "stream-1", seq: 0, eof: false }, new Uint8Array([1, 2]));
  socket.message({ event: "stream_chunk", stream_id: "stream-1", seq: 1, eof: false }, new Uint8Array([3, 4]));
  socket.message({ event: "stream_close", stream_id: "stream-1", ok: true });

  assert.deepEqual(Array.from(await bytesPromise), [1, 2, 3, 4]);
});

test("LuminkaClient rejects pending requests when the socket closes unexpectedly", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const pending = client.exists("kanban.json");
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket, "expected WebSocket instance");

  socket.open();
  await flushAsyncWork();
  socket.close();

  await assert.rejects(
    pending,
    /Luminka connection closed while waiting for a response|Luminka connection closed/,
  );
});

test("LuminkaClient ignores malformed websocket messages until a valid response arrives", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const existsPromise = client.exists("kanban.json");
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket, "expected WebSocket instance");

  socket.open();
  await flushAsyncWork();
  const request = decodeLuminkaFrame(socket.sent[0] ?? new Uint8Array());

  socket.rawMessage("not a frame");
  socket.message({ event: "fs_exists", id: request.header.id, ok: true, exists: true });

  await assert.doesNotReject(existsPromise);
  assert.equal(await existsPromise, true);
});

test("LuminkaClient requires explicit url outside browser hosts", async () => {
  const originalLocation = globalThis.location;
  delete (globalThis as any).location;
  try {
    const client = new LuminkaClient();
    await assert.rejects(
      client.appInfo(),
      /could not infer a WebSocket URL outside a browser host context\./,
    );
  } finally {
    (globalThis as any).location = originalLocation;
  }
});

test("LuminkaClient rejects connect failures from socket errors", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const connectPromise = client.connect();
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket, "expected WebSocket instance");

  socket.error();

  await assert.rejects(
    connectPromise,
    /failed to connect to Luminka at ws:\/\/127.0.0.1:7777\/ws/,
  );
});

test("LuminkaClient emits binary frames for script and shell streams", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const scriptPromise = client.runScriptStream("python", "tools/demo.py", ["--flag"], 12);
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket, "expected WebSocket instance");

  socket.open();
  await flushAsyncWork();
  const script = await scriptPromise;
  const request = decodeLuminkaFrame(socket.sent[0] ?? new Uint8Array());
  assert.equal(request.header.event, "script_exec_stream");

  socket.message({ event: "stream_chunk", stream_id: "stream-2", lane: "stdout", seq: 0, eof: false }, new Uint8Array([111, 107]));
  socket.message({ event: "script_response", id: request.header.id, ok: true, stream_id: "stream-2", code: 0 });

  const chunk = await collectStreamText(script.stdout);
  assert.equal(chunk, "ok");
  const result = await script.completed;
  assert.equal(result.code, 0);
});

test("LuminkaClient broadcast sends a JSON broadcast frame", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const pending = client.broadcast("workspace", { type: "ping" });
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket, "expected WebSocket instance");

  socket.open();
  await flushAsyncWork();
  const request = decodeLuminkaFrame(socket.sent[0] ?? new Uint8Array());
  assert.equal(request.header.event, "broadcast");
  assert.equal(request.header.channel, "workspace");
  assert.deepEqual(request.header.data, { type: "ping" });
  socket.message({ event: "broadcast_response", id: request.header.id, ok: true });
  await pending;
});

test("LuminkaClient onBroadcast receives pushed frames for matching channel only", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  await openClient(client);
  const socket = FakeWebSocket.instances[0];
  const messages: unknown[] = [];
  client.onBroadcast("workspace", (message) => messages.push(message.data));

  socket.message({ event: "broadcast", channel: "other", data: { type: "skip" } });
  socket.message({ event: "broadcast", channel: "workspace", data: { type: "ping" } }, new Uint8Array([7]));
  await flushAsyncWork();

  assert.deepEqual(messages, [{ type: "ping" }]);
});

test("LuminkaClient multi-tab coordinators elect the older session as primary", async () => {
  const older = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const first = older.createMultiTabCoordinator("main", { sessionId: "older", heartbeatMs: 10, staleMs: 50 });
  await new Promise((resolve) => globalThis.setTimeout(resolve, 2));
  const newer = new LuminkaClient({ url: "ws://127.0.0.1:7778/ws" });
  const second = newer.createMultiTabCoordinator("main", { sessionId: "newer", heartbeatMs: 10, staleMs: 50 });

  const startFirst = first.start();
  const firstSocket = FakeWebSocket.instances[0];
  firstSocket.open();
  await flushAsyncWork();
  acknowledgeLastBroadcast(firstSocket);
  await startFirst;

  const startSecond = second.start();
  const secondSocket = FakeWebSocket.instances[1];
  secondSocket.open();
  await flushAsyncWork();
  acknowledgeLastBroadcast(secondSocket);
  await startSecond;

  deliverLastBroadcast(secondSocket, firstSocket);
  deliverLastBroadcast(firstSocket, secondSocket);
  await flushAsyncWork();

  assert.equal(first.getPrimary()?.sessionId, "older");
  assert.equal(second.getPrimary()?.sessionId, "older");
  assert.equal(first.isPrimary(), true);
  assert.equal(second.isPrimary(), false);

  const stopFirst = first.stop();
  await flushAsyncWork();
  acknowledgeLastBroadcast(firstSocket);
  await stopFirst;
  const stopSecond = second.stop();
  await flushAsyncWork();
  acknowledgeLastBroadcast(secondSocket);
  await stopSecond;
});

test("LuminkaClient multi-tab coordinator removes peers on bye", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const coordinator = client.createMultiTabCoordinator("main", { sessionId: "local", heartbeatMs: 10, staleMs: 50 });
  const start = coordinator.start();
  const socket = FakeWebSocket.instances[0];
  socket.open();
  await flushAsyncWork();
  acknowledgeLastBroadcast(socket);
  await start;

  socket.message({ event: "broadcast", channel: "luminka:multi-tab:main", data: { kind: "hello", sessionId: "peer", startedAt: 1 } });
  await flushAsyncWork();
  assert.equal(coordinator.getPrimary()?.sessionId, "peer");

  socket.message({ event: "broadcast", channel: "luminka:multi-tab:main", data: { kind: "bye", sessionId: "peer", startedAt: 1 } });
  await flushAsyncWork();
  assert.equal(coordinator.getPrimary()?.sessionId, "local");
  assert.equal(coordinator.getPeers().some((peer) => peer.sessionId === "peer"), false);

  const stop = coordinator.stop();
  await flushAsyncWork();
  acknowledgeLastBroadcast(socket);
  await stop;
});

async function openClient(client: LuminkaClient): Promise<void> {
  const pending = client.connect();
  const socket = FakeWebSocket.instances.at(-1);
  assert.ok(socket, "expected WebSocket instance");
  socket.open();
  await pending;
}

function acknowledgeLastBroadcast(socket: FakeWebSocket): void {
  const request = decodeLuminkaFrame(socket.sent.at(-1) ?? new Uint8Array()).header;
  assert.equal(request.event, "broadcast");
  socket.message({ event: "broadcast_response", id: request.id, ok: true });
}

function deliverLastBroadcast(from: FakeWebSocket, to: FakeWebSocket): void {
  const frame = decodeLuminkaFrame(from.sent.at(-1) ?? new Uint8Array());
  assert.equal(frame.header.event, "broadcast");
  to.message({ event: "broadcast", channel: frame.header.channel, data: frame.header.data, content_type: frame.header.content_type }, frame.payload);
}

async function collectStreamText(stream: ReadableStream<Uint8Array>): Promise<string> {
  const reader = stream.getReader();
  const chunks: Uint8Array[] = [];
  try {
    while (true) {
      const next = await reader.read();
      if (next.done) {
        break;
      }
      chunks.push(next.value);
    }
  } finally {
    reader.releaseLock();
  }
  return new TextDecoder().decode(concat(chunks));
}

function concat(chunks: Uint8Array[]): Uint8Array {
  const size = chunks.reduce((total, chunk) => total + chunk.byteLength, 0);
  const output = new Uint8Array(size);
  let offset = 0;
  for (const chunk of chunks) {
    output.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return output;
}
