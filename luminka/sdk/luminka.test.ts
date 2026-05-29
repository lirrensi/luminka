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
  const frame = encodeLuminkaFrame({ event: "file:changed", path: "notes.txt" }, new Uint8Array([1, 2, 3]));
  const decoded = decodeLuminkaFrame(frame);
  assert.deepEqual(decoded.header, { event: "file:changed", path: "notes.txt" });
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
	assert.equal(request.header.event, "app:info");
	assert.equal(request.header.id, "luminka-1");

	socket.message({
		event: "response:app:info",
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
  assert.equal(request.header.event, "file:read_text");
  assert.equal(request.header.path, "secret.txt");

  socket.message({ event: "file:read_text", id: request.header.id, ok: true, data: "hello" });
  assert.equal(await readAlias, "hello");

  const writePromise = client.write("secret.txt", "updated");
  await flushAsyncWork();
  const writeRequest = decodeLuminkaFrame(socket.sent[1] ?? new Uint8Array());
  assert.equal(writeRequest.header.event, "file:write_text");
  assert.equal(writeRequest.header.data, "updated");
  socket.message({ event: "file:write_text", id: writeRequest.header.id, ok: true });
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
  assert.equal(watchRequest.header.event, "file:watch");
  assert.equal(watchRequest.header.path, "workspace.emptyspace.xml");
  socket.message({ event: "file:watch", id: watchRequest.header.id, ok: true });

  await flushAsyncWork();
  const readRequest = decodeLuminkaFrame(socket.sent[1] ?? new Uint8Array());
  assert.equal(readRequest.header.event, "file:read_text");
  assert.equal(readRequest.header.path, "workspace.emptyspace.xml");
  socket.message({ event: "file:read_text", id: readRequest.header.id, ok: true, data: "<workspace />" });

  assert.equal(await loadPromise, "<workspace />");
  assert.equal(file.getText(), "<workspace />");

  const savePromise = file.save("<workspace name=\"Updated\" />");
  await flushAsyncWork();
  const writeRequest = decodeLuminkaFrame(socket.sent[2] ?? new Uint8Array());
  assert.equal(writeRequest.header.event, "file:write_text");
  assert.equal(writeRequest.header.path, "workspace.emptyspace.xml");
  assert.equal(writeRequest.header.data, "<workspace name=\"Updated\" />");
  socket.message({ event: "file:write_text", id: writeRequest.header.id, ok: true });

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
  socket.message({ event: "file:watch", id: watchRequest.header.id, ok: true });
  await flushAsyncWork();
  const initialReadRequest = decodeLuminkaFrame(socket.sent[1] ?? new Uint8Array());
  socket.message({ event: "file:read_text", id: initialReadRequest.header.id, ok: true, data: "before" });
  assert.equal(await loadPromise, "before");

  const changes: string[] = [];
  file.onExternalChange((text) => {
    changes.push(text);
  });
  socket.message({ event: "file:changed", path: "workspace.emptyspace.xml" });
  await new Promise((resolve) => globalThis.setTimeout(resolve, 0));

  const externalReadRequest = decodeLuminkaFrame(socket.sent[2] ?? new Uint8Array());
  assert.equal(externalReadRequest.header.event, "file:read_text");
  assert.equal(externalReadRequest.header.path, "workspace.emptyspace.xml");
  socket.message({ event: "file:read_text", id: externalReadRequest.header.id, ok: true, data: "after" });

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
  socket.message({ event: "file:watch", id: watchRequest.header.id, ok: true });
  await flushAsyncWork();
  const initialReadRequest = decodeLuminkaFrame(socket.sent[1] ?? new Uint8Array());
  socket.message({ event: "file:read_text", id: initialReadRequest.header.id, ok: true, data: "before" });
  assert.equal(await loadPromise, "before");

  const changes: string[] = [];
  file.onExternalChange((text) => {
    changes.push(text);
  });
  const savePromise = file.save("after");
  await flushAsyncWork();
  const writeRequest = decodeLuminkaFrame(socket.sent[2] ?? new Uint8Array());
  assert.equal(writeRequest.header.event, "file:write_text");
  assert.equal(writeRequest.header.path, "workspace.emptyspace.xml");
  assert.equal(writeRequest.header.data, "after");
  socket.message({ event: "file:write_text", id: writeRequest.header.id, ok: true });
  await savePromise;

  socket.message({ event: "file:changed", path: "workspace.emptyspace.xml" });
  await new Promise((resolve) => globalThis.setTimeout(resolve, 0));

  const echoReadRequest = decodeLuminkaFrame(socket.sent[3] ?? new Uint8Array());
  assert.equal(echoReadRequest.header.event, "file:read_text");
  assert.equal(echoReadRequest.header.path, "workspace.emptyspace.xml");
  socket.message({ event: "file:read_text", id: echoReadRequest.header.id, ok: true, data: "after" });

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
    .filter((header) => header.event === "file:watch");
  assert.equal(watchRequestsBeforeResponse.length, 1);

  const watchRequest = watchRequestsBeforeResponse[0];
  socket.message({ event: "file:watch", id: watchRequest.id, ok: true });
  await flushAsyncWork();

  const pendingRequests = socket.sent
    .map((frame) => decodeLuminkaFrame(frame).header)
    .filter((header) => header.event === "file:read_text" || header.event === "file:write_text");
  assert.equal(pendingRequests.length, 2);
  for (const request of pendingRequests) {
    assert.equal(request.path, "workspace.emptyspace.xml");
    if (request.event === "file:read_text") {
      socket.message({ event: "file:read_text", id: request.id, ok: true, data: "before" });
    } else {
      assert.equal(request.data, "after");
      socket.message({ event: "file:write_text", id: request.id, ok: true });
    }
  }

  await operations;
  const allWatchRequests = socket.sent
    .map((frame) => decodeLuminkaFrame(frame).header)
    .filter((header) => header.event === "file:watch");
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
  socket.message({ event: "file:open_read", id: request.header.id, ok: true, stream_id: "stream-1" });
  socket.message({ event: "stream:chunk", stream_id: "stream-1", seq: 0, eof: false }, new Uint8Array([1, 2]));
  socket.message({ event: "stream:chunk", stream_id: "stream-1", seq: 1, eof: false }, new Uint8Array([3, 4]));
  socket.message({ event: "stream:close", stream_id: "stream-1", ok: true });

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
  socket.message({ event: "file:exists", id: request.header.id, ok: true, exists: true });

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
  assert.equal(request.header.event, "script:stream");

  socket.message({ event: "stream:chunk", stream_id: "stream-2", lane: "stdout", seq: 0, eof: false }, new Uint8Array([111, 107]));
  socket.message({ event: "response:script:exec", id: request.header.id, ok: true, stream_id: "stream-2", code: 0 });

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
  assert.equal(request.header.event, "ws:broadcast");
  assert.equal(request.header.channel, "workspace");
  assert.deepEqual(request.header.data, { type: "ping" });
  socket.message({ event: "response:ws:broadcast", id: request.header.id, ok: true });
  await pending;
});

test("LuminkaClient onBroadcast receives pushed frames for matching channel only", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  await openClient(client);
  const socket = FakeWebSocket.instances[0];
  const messages: unknown[] = [];
  client.onBroadcast("workspace", (message) => messages.push(message.data));

  socket.message({ event: "ws:broadcast", channel: "other", data: { type: "skip" } });
  socket.message({ event: "ws:broadcast", channel: "workspace", data: { type: "ping" } }, new Uint8Array([7]));
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

// ---------------------------------------------------------------------------
// Extension API: call() and onEvent()
// ---------------------------------------------------------------------------

test("LuminkaClient call() sends custom event and returns response", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const callPromise = client.call("custom:event", { foo: "bar" });
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket);

  socket.open();
  await flushAsyncWork();
  const request = decodeLuminkaFrame(socket.sent[0] ?? new Uint8Array());
  assert.equal(request.header.event, "custom:event");
  assert.equal((request.header as Record<string, unknown>).foo, "bar");
  assert.equal(request.header.id, "luminka-1");

  socket.message({ event: "response:custom:event", id: request.header.id, ok: true, data: "ok" });
  const resp = await callPromise;
  assert.equal(resp.event, "response:custom:event");
  assert.equal(resp.data, "ok");
});

test("LuminkaClient call() with binary payload", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const payload = new Uint8Array([1, 2, 3]);
  const callPromise = client.call("custom:binary", {}, payload);
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket);

  socket.open();
  await flushAsyncWork();
  const frame = socket.sent[0] ?? new Uint8Array();
  const { header, payload: decodedPayload } = decodeLuminkaFrame(frame);
  assert.equal(header.event, "custom:binary");
  assert.deepEqual(Array.from(decodedPayload), [1, 2, 3]);

  socket.message({ event: "response:custom:binary", id: header.id, ok: true });
  await callPromise;
});

test("LuminkaClient call() without data sends event only", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const callPromise = client.call("simple:ping");
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket);

  socket.open();
  await flushAsyncWork();
  const request = decodeLuminkaFrame(socket.sent[0] ?? new Uint8Array());
  assert.equal(request.header.event, "simple:ping");
  assert.equal(request.header.id, "luminka-1");

  socket.message({ event: "response:simple:ping", id: request.header.id, ok: true });
  await callPromise;
});

test("LuminkaClient onEvent() receives pushed custom events", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const received: Array<{ msg: LuminkaFrame; payload: Uint8Array }> = [];
  client.onEvent("goal:changed", (msg, payload) => {
    received.push({ msg, payload });
  });

  await openClient(client);
  const socket = FakeWebSocket.instances[0];

  socket.message({ event: "goal:changed", data: { goal: "ship" } }, new Uint8Array([10]));

  await flushAsyncWork();
  assert.equal(received.length, 1);
  assert.deepEqual(received[0].msg.data, { goal: "ship" });
  assert.deepEqual(Array.from(received[0].payload), [10]);
});

test("LuminkaClient onEvent() unsubscribe stops receiving", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const received: string[] = [];
  const unsub = client.onEvent("custom:push", (msg) => {
    received.push(msg.event);
  });

  await openClient(client);
  const socket = FakeWebSocket.instances[0];

  socket.message({ event: "custom:push" });
  await flushAsyncWork();
  assert.equal(received.length, 1);

  unsub();
  socket.message({ event: "custom:push" });
  await flushAsyncWork();
  assert.equal(received.length, 1); // Still 1 — second message was ignored
});

test("LuminkaClient onEvent() multiple listeners on same event", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const results: string[] = [];
  const unsub1 = client.onEvent("multi:event", (msg) => { results.push("a"); });
  const unsub2 = client.onEvent("multi:event", (msg) => { results.push("b"); });

  await openClient(client);
  const socket = FakeWebSocket.instances[0];

  socket.message({ event: "multi:event" });
  await flushAsyncWork();
  assert.equal(results.length, 2);
  assert.ok(results.includes("a"));
  assert.ok(results.includes("b"));
});

test("LuminkaClient onEvent() does not intercept pending request resolution", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const received: string[] = [];
  client.onEvent("custom:event", (msg) => {
    received.push("listener");
  });

  const respPromise = client.call("custom:event", {});
  const socket = FakeWebSocket.instances[0];
  socket.open();
  await flushAsyncWork();

  const request = decodeLuminkaFrame(socket.sent[0] ?? new Uint8Array());
  // A response with matching ID — this should resolve the pending request,
  // NOT invoke the onEvent listener.
  socket.message({ event: "response:custom:event", id: request.header.id, ok: true, data: "done" });

  const resp = await respPromise;
  assert.deepEqual(resp, { event: "response:custom:event", id: request.header.id, ok: true, data: "done" });
  assert.equal(received.length, 0); // Listener should NOT fire for response with ID
});

test("LuminkaClient onEvent() fires for push without ID even when pending exists", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const received: string[] = [];
  client.onEvent("goal:changed", (msg) => {
    received.push(msg.event);
  });

  // Have a pending request too
  const respPromise = client.call("other:event");
  const socket = FakeWebSocket.instances[0];
  socket.open();
  await flushAsyncWork();

  // Push event (no id) should still reach the listener
  socket.message({ event: "goal:changed", data: "push" });
  await flushAsyncWork();
  assert.equal(received.length, 1);
  assert.equal(received[0], "goal:changed");

  // Resolve the pending request
  const request = decodeLuminkaFrame(socket.sent[0] ?? new Uint8Array());
  socket.message({ event: "response:other:event", id: request.header.id, ok: true });
  await respPromise;
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

  socket.message({ event: "ws:broadcast", channel: "luminka:multi-tab:main", data: { kind: "hello", sessionId: "peer", startedAt: 1 } });
  await flushAsyncWork();
  assert.equal(coordinator.getPrimary()?.sessionId, "peer");

  socket.message({ event: "ws:broadcast", channel: "luminka:multi-tab:main", data: { kind: "bye", sessionId: "peer", startedAt: 1 } });
  await flushAsyncWork();
  assert.equal(coordinator.getPrimary()?.sessionId, "local");
  assert.equal(coordinator.getPeers().some((peer) => peer.sessionId === "peer"), false);

  const stop = coordinator.stop();
  await flushAsyncWork();
  acknowledgeLastBroadcast(socket);
  await stop;
});

// ---------------------------------------------------------------------------
// FileHandle tests live in luminka/sdk/filehandle.verify.ts
// (run with: node luminka/sdk/filehandle.verify.ts)
// Separated because Node.js v24.15.0 test runner hangs on multi-request
// FileHandle test patterns (confirmed working outside test runner).
// ---------------------------------------------------------------------------

// FileHandle open/read/close round-trip — simple single-pass test retained
test("FileHandle open/read/close round-trip", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const content = new Uint8Array([72, 101, 108, 108, 111]); // "Hello"
  const openPromise = client.open("data.bin");
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket, "expected WebSocket instance");

  socket.open();
  await flushAsyncWork();
  const openRequest = decodeLuminkaFrame(socket.sent[0] ?? new Uint8Array()).header;
  assert.equal(openRequest.event, "file:open");
  assert.equal(openRequest.path, "data.bin");
  assert.equal(openRequest.flag, "r");

  socket.message({ event: "response:file:open", id: openRequest.id, ok: true, stream_id: "handle-1" });
  const handle = await openPromise;
  assert.ok(handle, "expected FileHandle");

  // Read from handle
  const readPromise = handle.read();
  await flushAsyncWork();
  const readRequest = decodeLuminkaFrame(socket.sent[1] ?? new Uint8Array()).header;
  assert.equal(readRequest.event, "handle:read");
  assert.equal(readRequest.stream_id, "handle-1");

  socket.message({ event: "response:handle:read", id: readRequest.id, ok: true, stream_id: "handle-1" }, content);
  const data = await readPromise;
  assert.deepEqual(Array.from(data), [72, 101, 108, 108, 111]);

  // Close handle
  const closePromise = handle.close();
  await flushAsyncWork();
  const closeRequest = decodeLuminkaFrame(socket.sent[2] ?? new Uint8Array()).header;
  assert.equal(closeRequest.event, "handle:close");
  assert.equal(closeRequest.stream_id, "handle-1");

  socket.message({ event: "response:handle:close", id: closeRequest.id, ok: true });
  await closePromise;
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
  assert.equal(request.event, "ws:broadcast");
  socket.message({ event: "response:ws:broadcast", id: request.id, ok: true });
}

function deliverLastBroadcast(from: FakeWebSocket, to: FakeWebSocket): void {
  const frame = decodeLuminkaFrame(from.sent.at(-1) ?? new Uint8Array());
  assert.equal(frame.header.event, "ws:broadcast");
  to.message({ event: "ws:broadcast", channel: frame.header.channel, data: frame.header.data, content_type: frame.header.content_type }, frame.payload);
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
