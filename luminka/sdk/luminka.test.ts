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
    mode: "webview",
    root: "C:/apps/starter",
    capabilities: { fs: true, scripts: false, shell: false },
  });

  const info = await connectPromise;
  assert.deepEqual(info, {
    name: "starter",
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
