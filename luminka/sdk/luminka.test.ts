import test from "node:test";
import assert from "node:assert/strict";

import { LuminkaClient } from "./luminka.ts";

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
  sent: string[] = [];
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

  send(data: string): void {
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

  message(data: unknown): void {
    this.emit("message", { data: JSON.stringify(data) });
  }

  rawMessage(data: string): void {
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

test("LuminkaClient appInfo sends request and parses response", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const connectPromise = client.appInfo();
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket, "expected WebSocket instance");

  socket.open();
  await flushAsyncWork();
  const request = JSON.parse(socket.sent[0] ?? "{}");
  assert.equal(request.event, "app_info");
  assert.equal(request.id, "luminka-1");

  socket.message({
    event: "app_info",
    id: request.id,
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

test("LuminkaClient rejects failed fs responses", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const readPromise = client.read("secret.txt");
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket, "expected WebSocket instance");

  socket.open();
  await flushAsyncWork();
  const request = JSON.parse(socket.sent[0] ?? "{}");
  assert.equal(request.event, "fs_read");
  assert.equal(request.path, "secret.txt");

  socket.message({
    event: "fs_response",
    id: request.id,
    ok: false,
    error: "filesystem capability is disabled",
  });

  await assert.rejects(
    readPromise,
    /filesystem capability is disabled\. Enable the filesystem capability in the Luminka host app, or avoid calling this SDK method when that capability is unavailable\./,
  );
});

test("LuminkaClient dispatches fs_changed notifications to listeners", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const seen: string[] = [];
  const dispose = client.onFileChanged((path) => seen.push(path));

  const watchPromise = client.watch("kanban.json");
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket, "expected WebSocket instance");

  socket.open();
  await flushAsyncWork();
  const request = JSON.parse(socket.sent[0] ?? "{}");
  socket.message({ event: "fs_response", id: request.id, ok: true });
  await watchPromise;

  socket.message({ event: "fs_changed", path: "kanban.json" });
  assert.deepEqual(seen, ["kanban.json"]);

  dispose();
  socket.message({ event: "fs_changed", path: "ignored.json" });
  assert.deepEqual(seen, ["kanban.json"]);
});

test("LuminkaClient disconnect rejects pending requests", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const pending = client.list("");
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket, "expected WebSocket instance");

  socket.open();
  await flushAsyncWork();
  client.disconnect();

  await assert.rejects(
    pending,
    /Luminka connection closed while waiting for a response\. Reconnect and retry the request\./,
  );
});

test("LuminkaClient derives default url from location.host", async () => {
  const originalLocation = globalThis.location;
  (globalThis as any).location = { host: "127.0.0.1:8787" };
  try {
    const client = new LuminkaClient();
    const appInfoPromise = client.appInfo();
    const socket = FakeWebSocket.instances[0];
    assert.ok(socket, "expected WebSocket instance");
    assert.equal(socket.url, "ws://127.0.0.1:8787/ws");

    socket.open();
    await flushAsyncWork();
    const request = JSON.parse(socket.sent[0] ?? "{}");
    socket.message({
      event: "app_info",
      id: request.id,
      ok: true,
      name: "starter",
      mode: "browser",
      root: "C:/apps/starter",
      capabilities: { fs: true, scripts: false, shell: false },
    });

    await appInfoPromise;
  } finally {
    (globalThis as any).location = originalLocation;
  }
});

test("LuminkaClient requires explicit url outside browser hosts", async () => {
  const originalLocation = globalThis.location;
  delete (globalThis as any).location;
  try {
    const client = new LuminkaClient();
    await assert.rejects(
      client.appInfo(),
      /could not infer a WebSocket URL outside a browser host context\. Pass an explicit url, for example new LuminkaClient\(\{ url: "ws:\/\/127\.0\.0\.1:7777\/ws" \}\)\./,
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
    /Failed to connect to Luminka at ws:\/\/127.0.0.1:7777\/ws\. Confirm the host app is running and serving the SDK WebSocket endpoint\./,
  );
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
    /Luminka connection closed while waiting for a response\. Reconnect and retry the request\./,
  );
});

test("LuminkaClient disconnect closes open socket once", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const connectPromise = client.connect();
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket, "expected WebSocket instance");

  socket.open();
  await connectPromise;

  client.disconnect();

  assert.equal(socket.closeCalls, 1);
  assert.equal(socket.readyState, FakeWebSocket.CLOSED);
});

test("LuminkaClient ignores malformed websocket messages until a valid response arrives", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const existsPromise = client.exists("kanban.json");
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket, "expected WebSocket instance");

  socket.open();
  await flushAsyncWork();
  const request = JSON.parse(socket.sent[0] ?? "{}");

  socket.rawMessage("not json");
  socket.message({ event: "fs_response", id: request.id, ok: true, exists: true });

  await assert.doesNotReject(existsPromise);
  assert.equal(await existsPromise, true);
});

test("LuminkaClient rejects incomplete app_info responses", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const appInfoPromise = client.appInfo();
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket, "expected WebSocket instance");

  socket.open();
  await flushAsyncWork();
  const request = JSON.parse(socket.sent[0] ?? "{}");

  socket.message({
    event: "app_info",
    id: request.id,
    ok: true,
    name: "starter",
    mode: "browser",
    root: "C:/apps/starter",
  });

  await assert.rejects(
    appInfoPromise,
    /Luminka app_info response was incomplete: missing capabilities\. Check that the host returns name, mode, root, and capabilities\./,
  );
});

test("LuminkaClient runScript and runShell preserve wrapper request shapes", async () => {
  const client = new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" });
  const scriptPromise = client.runScript("python", "tools/demo.py", ["--flag"], 12);
  const socket = FakeWebSocket.instances[0];
  assert.ok(socket, "expected WebSocket instance");

  socket.open();
  await flushAsyncWork();
  const scriptRequest = JSON.parse(socket.sent[0] ?? "{}");
  assert.deepEqual(scriptRequest, {
    event: "script_exec",
    id: "luminka-1",
    runner: "python",
    file: "tools/demo.py",
    args: ["--flag"],
    timeout: 12,
  });
  socket.message({ event: "script_response", id: scriptRequest.id, ok: true, stdout: "ok", stderr: "", code: 0 });
  await assert.doesNotReject(scriptPromise);

  const shellPromise = client.runShell("node", ["script.js"], 8);
  await flushAsyncWork();
  const shellRequest = JSON.parse(socket.sent[1] ?? "{}");
  assert.deepEqual(shellRequest, {
    event: "shell_exec",
    id: "luminka-2",
    cmd: "node",
    args: ["script.js"],
    timeout: 8,
  });
  socket.message({ event: "shell_response", id: shellRequest.id, ok: true, stdout: "done", stderr: "", code: 0 });
  await assert.doesNotReject(shellPromise);
});
