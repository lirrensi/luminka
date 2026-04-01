// FILE: luminka/sdk/luminka.ts
// PURPOSE: Provide the browser-first Luminka WebSocket SDK for frontend apps.
// OWNS: Binary WebSocket framing, request/response helpers, and file/process capability wrappers.
// EXPORTS: LuminkaCapabilities, LuminkaAppInfo, LuminkaOptions, LuminkaFrame, LuminkaClient, createLuminkaClient, encodeLuminkaFrame, decodeLuminkaFrame
// DOCS: docs/spec.md, docs/arch.md, agent_chat/plan_luminka_stream_runtime_2026-04-01.md

const DEFAULT_WS_PATH = "/ws";
const DEFAULT_CHUNK_SIZE = 32 * 1024;
const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

export interface LuminkaCapabilities {
  fs: boolean;
  scripts: boolean;
  shell: boolean;
}

export interface LuminkaAppInfo {
  name: string;
  mode: string;
  root: string;
  capabilities: LuminkaCapabilities;
}

export interface LuminkaOptions {
  url?: string;
}

export interface LuminkaFrame {
  event: string;
  id?: string;
  ok?: boolean;
  error?: string;
  path?: string;
  data?: string;
  files?: string[];
  exists?: boolean;
  runner?: string;
  file?: string;
  cmd?: string;
  args?: string[];
  timeout?: number;
  stdout?: string;
  stderr?: string;
  code?: number | null;
  name?: string;
  mode?: string;
  root?: string;
  capabilities?: LuminkaCapabilities;
  stream_id?: string;
  seq?: number;
  lane?: string;
  eof?: boolean;
}

export interface ExecStreamResult {
  stdout: ReadableStream<Uint8Array>;
  stderr: ReadableStream<Uint8Array>;
  completed: Promise<{ code: number | null; stdout: string; stderr: string }>;
}

type RequestRecord = {
  resolve: (frame: LuminkaFrame) => void;
  reject: (error: Error) => void;
};

type Deferred<T> = {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (error: Error) => void;
};

type ReadStreamState = {
  kind: "read";
  streamId: string;
  chunks: Uint8Array[];
  closed: boolean;
  error: Error | null;
  controller: ReadableStreamDefaultController<Uint8Array> | null;
};

type WriteStreamState = {
  kind: "write";
  streamId: string;
  nextSeq: number;
  closed: boolean;
};

type ExecStreamState = {
  kind: "exec";
  requestId: string;
  event: "script_exec_stream" | "shell_exec_stream";
  streamId: string | null;
  stdoutChunks: Uint8Array[];
  stderrChunks: Uint8Array[];
  stdoutText: string;
  stderrText: string;
  stdoutController: ReadableStreamDefaultController<Uint8Array> | null;
  stderrController: ReadableStreamDefaultController<Uint8Array> | null;
  completed: Deferred<{ code: number | null; stdout: string; stderr: string }>;
  closed: boolean;
};

type StreamState = ReadStreamState | WriteStreamState | ExecStreamState;

export function encodeLuminkaFrame(header: LuminkaFrame, payload: Uint8Array = new Uint8Array()): Uint8Array {
  const headerBytes = textEncoder.encode(JSON.stringify(header));
  const frame = new Uint8Array(4 + headerBytes.length + payload.length);
  new DataView(frame.buffer, frame.byteOffset, frame.byteLength).setUint32(0, headerBytes.length, false);
  frame.set(headerBytes, 4);
  frame.set(payload, 4 + headerBytes.length);
  return frame;
}

export function decodeLuminkaFrame(frame: ArrayBuffer | ArrayBufferView): { header: LuminkaFrame; payload: Uint8Array } {
  const bytes = toUint8Array(frame);
  if (bytes.byteLength < 4) {
    throw new Error("frame is too short");
  }
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const headerLength = view.getUint32(0, false);
  if (headerLength > bytes.byteLength - 4) {
    throw new Error("frame header length exceeds frame body");
  }
  const headerBytes = bytes.slice(4, 4 + headerLength);
  const header = JSON.parse(textDecoder.decode(headerBytes)) as LuminkaFrame;
  return { header, payload: bytes.slice(4 + headerLength) };
}

export class LuminkaClient {
  private socket: WebSocket | null = null;
  private connectPromise: Promise<void> | null = null;
  private nextRequestId = 1;
  private readonly pending = new Map<string, RequestRecord>();
  private readonly streams = new Map<string, StreamState>();
  private readonly pendingExecStreams = new Map<string, ExecStreamState>();
  private readonly fileListeners = new Set<(path: string) => void>();

  constructor(private readonly options: LuminkaOptions = {}) {}

  async connect(): Promise<void> {
    if (this.socket && this.socket.readyState === WebSocket.OPEN) {
      return;
    }
    if (this.connectPromise) {
      return this.connectPromise;
    }
    const url = this.options.url ?? this.defaultUrl();
    const socket = new WebSocket(url);
    socket.binaryType = "arraybuffer";
    this.socket = socket;
    this.connectPromise = new Promise((resolve, reject) => {
      const cleanup = () => {
        socket.removeEventListener("open", handleOpen);
        socket.removeEventListener("error", handleError);
        socket.removeEventListener("close", handleClose);
      };

      const handleOpen = () => {
        cleanup();
        this.connectPromise = null;
        resolve();
      };

      const handleError = () => {
        cleanup();
        this.connectPromise = null;
        this.socket = null;
        reject(new Error(`failed to connect to Luminka at ${url}`));
      };

      const handleClose = () => {
        cleanup();
        this.connectPromise = null;
        this.socket = null;
        this.failAll(new Error("Luminka connection closed"));
        reject(new Error(`failed to connect to Luminka at ${url}`));
      };

      socket.addEventListener("open", handleOpen);
      socket.addEventListener("error", handleError);
      socket.addEventListener("close", handleClose);
    });
    socket.addEventListener("message", (event) => this.handleMessage(event));
    socket.addEventListener("close", () => {
      if (this.socket === socket) {
        this.socket = null;
      }
      this.failAll(new Error("Luminka connection closed"));
    });
    return this.connectPromise;
  }

  disconnect(): void {
    const socket = this.socket;
    this.socket = null;
    this.connectPromise = null;
    this.failAll(new Error("Luminka connection closed"));
    if (socket && socket.readyState < WebSocket.CLOSING) {
      socket.close();
    }
  }

  async appInfo(): Promise<LuminkaAppInfo> {
    return this.requireAppInfo(await this.request({ event: "app_info" }));
  }

  async readText(path: string): Promise<string> {
    const response = await this.request({ event: "fs_read_text", path });
    return this.requireData(response, "file data");
  }

  async writeText(path: string, data: string): Promise<void> {
    await this.request({ event: "fs_write_text", path, data });
  }

  async readBytes(path: string): Promise<Uint8Array> {
    const stream = await this.createReadStream(path);
    return collectReadableStream(stream);
  }

  async writeBytes(path: string, data: Uint8Array): Promise<void> {
    const writable = await this.createWriteStream(path);
    const writer = writable.getWriter();
    try {
      for (let offset = 0; offset < data.byteLength; offset += DEFAULT_CHUNK_SIZE) {
        await writer.write(data.slice(offset, offset + DEFAULT_CHUNK_SIZE));
      }
      await writer.close();
    } finally {
      writer.releaseLock();
    }
  }

  async createReadStream(path: string): Promise<ReadableStream<Uint8Array>> {
    const response = await this.request({ event: "fs_open_read", path });
    const streamId = this.requireStreamId(response, "fs_open_read");
    const state = this.getOrCreateReadStreamState(streamId);
    return new ReadableStream<Uint8Array>({
      start: (controller) => {
        state.controller = controller;
        this.flushReadStream(state);
      },
      cancel: () => {
        this.streams.delete(streamId);
      },
    });
  }

  async createWriteStream(path: string): Promise<WritableStream<Uint8Array>> {
    const response = await this.request({ event: "fs_open_write", path });
    const streamId = this.requireStreamId(response, "fs_open_write");
    const state = this.getOrCreateWriteStreamState(streamId);
    return new WritableStream<Uint8Array>({
      write: async (chunk) => {
        if (state.closed) {
          throw new Error(`stream ${streamId} is closed`);
        }
        await this.sendFrame({ event: "stream_chunk", stream_id: streamId, seq: state.nextSeq++, eof: false }, toUint8Array(chunk));
      },
      close: async () => {
        if (state.closed) {
          return;
        }
        state.closed = true;
        await this.request({ event: "stream_close", stream_id: streamId });
        this.streams.delete(streamId);
      },
      abort: async () => {
        state.closed = true;
        try {
          await this.request({ event: "stream_close", stream_id: streamId });
        } finally {
          this.streams.delete(streamId);
        }
      },
    });
  }

  async list(path = ""): Promise<string[]> {
    const response = await this.request({ event: "fs_list", path });
    return response.files ?? [];
  }

  async remove(path: string): Promise<void> {
    await this.request({ event: "fs_delete", path });
  }

  async exists(path: string): Promise<boolean> {
    const response = await this.request({ event: "fs_exists", path });
    return response.exists ?? false;
  }

  async watch(path: string): Promise<void> {
    await this.request({ event: "fs_watch", path });
  }

  async unwatch(path: string): Promise<void> {
    await this.request({ event: "fs_unwatch", path });
  }

  async runScript(runner: string, file: string, args: string[] = [], timeout?: number): Promise<{ stdout: string; stderr: string; code: number | null }> {
    const response = await this.request({ event: "script_exec", runner, file, args, timeout });
    return this.requireExecResult(response);
  }

  async runShell(cmd: string, args: string[] = [], timeout?: number): Promise<{ stdout: string; stderr: string; code: number | null }> {
    const response = await this.request({ event: "shell_exec", cmd, args, timeout });
    return this.requireExecResult(response);
  }

  async runScriptStream(runner: string, file: string, args: string[] = [], timeout?: number): Promise<ExecStreamResult> {
    return this.startExecStream({ event: "script_exec_stream", runner, file, args, timeout });
  }

  async runShellStream(cmd: string, args: string[] = [], timeout?: number): Promise<ExecStreamResult> {
    return this.startExecStream({ event: "shell_exec_stream", cmd, args, timeout });
  }

  async read(path: string): Promise<string> {
    return this.readText(path);
  }

  async write(path: string, data: string): Promise<void> {
    await this.writeText(path, data);
  }

  onFileChanged(listener: (path: string) => void): () => void {
    this.fileListeners.add(listener);
    return () => this.fileListeners.delete(listener);
  }

  private async startExecStream(payload: Omit<LuminkaFrame, "id"> & { event: "script_exec_stream" | "shell_exec_stream" }): Promise<ExecStreamResult> {
    await this.connect();
    const socket = this.requireSocket();
    const requestId = this.nextId();
    const state = this.createExecStreamState(requestId, payload.event);
    this.pendingExecStreams.set(requestId, state);
    try {
      socket.send(encodeLuminkaFrame({ ...payload, id: requestId }));
    } catch (error) {
      this.pendingExecStreams.delete(requestId);
      this.streams.delete(requestId);
      state.completed.reject(toError(error, "failed to send Luminka request"));
      throw error;
    }
    return { stdout: state.stdoutStream, stderr: state.stderrStream, completed: state.completed.promise };
  }

  private createExecStreamState(requestId: string, event: "script_exec_stream" | "shell_exec_stream"): ExecStreamState & { stdoutStream: ReadableStream<Uint8Array>; stderrStream: ReadableStream<Uint8Array> } {
    const completed = createDeferred<{ code: number | null; stdout: string; stderr: string }>();
    const state = {
      kind: "exec",
      requestId,
      event,
      streamId: null,
      stdoutChunks: [],
      stderrChunks: [],
      stdoutText: "",
      stderrText: "",
      stdoutController: null,
      stderrController: null,
      completed,
      closed: false,
    } as ExecStreamState;

    const stdoutStream = new ReadableStream<Uint8Array>({
      start: (controller) => {
        state.stdoutController = controller;
        this.flushExecLane(state, "stdout");
      },
      cancel: () => {
        this.streams.delete(requestId);
        this.pendingExecStreams.delete(requestId);
      },
    });

    const stderrStream = new ReadableStream<Uint8Array>({
      start: (controller) => {
        state.stderrController = controller;
        this.flushExecLane(state, "stderr");
      },
      cancel: () => {
        this.streams.delete(requestId);
        this.pendingExecStreams.delete(requestId);
      },
    });

    const execState = Object.assign(state, { stdoutStream, stderrStream }) as ExecStreamState & {
      stdoutStream: ReadableStream<Uint8Array>;
      stderrStream: ReadableStream<Uint8Array>;
    };
    this.streams.set(requestId, execState);
    return execState;
  }

  private async request(payload: Omit<LuminkaFrame, "id">): Promise<LuminkaFrame> {
    await this.connect();
    const socket = this.requireSocket();
    const id = this.nextId();
    const frame = encodeLuminkaFrame({ ...payload, id });
    return new Promise<LuminkaFrame>((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      try {
        socket.send(frame);
      } catch (error) {
        this.pending.delete(id);
        reject(toError(error, "failed to send Luminka request"));
      }
    });
  }

  private async sendFrame(header: LuminkaFrame, payload: Uint8Array = new Uint8Array()): Promise<void> {
    const socket = this.requireSocket();
    socket.send(encodeLuminkaFrame(header, payload));
  }

  private handleMessage(event: MessageEvent): void {
    const raw = event.data;
    const bytes = raw instanceof ArrayBuffer || ArrayBuffer.isView(raw) ? toUint8Array(raw) : null;
    if (!bytes) {
      return;
    }

    let message: { header: LuminkaFrame; payload: Uint8Array };
    try {
      message = decodeLuminkaFrame(bytes);
    } catch {
      return;
    }

    if (message.header.event === "fs_changed" && message.header.path) {
      for (const listener of this.fileListeners) {
        listener(message.header.path);
      }
      return;
    }

    if (message.header.event === "stream_chunk") {
      this.handleStreamChunk(message.header, message.payload);
      return;
    }

    if (message.header.event === "stream_close") {
      this.handleStreamClose(message.header);
    }

    if (message.header.event === "script_response" || message.header.event === "shell_response") {
      this.handleExecResponse(message.header);
    }

    const id = message.header.id;
    if (!id) {
      return;
    }
    const closedStreamId = message.header.event === "stream_close" ? message.header.stream_id ?? null : null;
    const pending = this.pending.get(id);
    if (!pending) {
      return;
    }
    this.pending.delete(id);
    if (message.header.ok === false || message.header.event === "error") {
      if (closedStreamId) {
        this.removeStream(closedStreamId);
      }
      pending.reject(this.responseError(message.header));
      return;
    }

    if (message.header.event === "fs_open_read" && message.header.stream_id) {
      this.getOrCreateReadStreamState(message.header.stream_id);
    }
    if (message.header.event === "fs_open_write" && message.header.stream_id) {
      this.getOrCreateWriteStreamState(message.header.stream_id);
    }
    if (closedStreamId) {
      this.removeStream(closedStreamId);
    }

    pending.resolve(message.header);
  }

  private handleStreamChunk(header: LuminkaFrame, payload: Uint8Array): void {
    if (!header.stream_id) {
      return;
    }
    const stream = this.streams.get(header.stream_id) ?? this.bindPendingExecStream(header.stream_id);
    if (!stream) {
      return;
    }
    if (stream.kind === "read") {
      stream.chunks.push(payload);
      this.flushReadStream(stream);
      return;
    }
    if (stream.kind === "exec") {
      this.appendExecChunk(stream, header.lane === "stderr" ? "stderr" : "stdout", payload);
    }
  }

  private handleStreamClose(header: LuminkaFrame): void {
    if (!header.stream_id) {
      return;
    }
    const stream = this.streams.get(header.stream_id) ?? this.bindPendingExecStream(header.stream_id);
    if (!stream) {
      return;
    }
    if (stream.kind === "read") {
      stream.closed = true;
      if (header.ok === false) {
        stream.error = new Error(header.error || "Luminka stream closed with an error");
      }
      this.flushReadStream(stream);
      return;
    }
    if (stream.kind === "write") {
      stream.closed = true;
      if (header.ok === false) {
        this.streams.delete(stream.streamId);
      } else {
        this.streams.delete(stream.streamId);
      }
      return;
    }
    if (stream.kind === "exec") {
      stream.closed = true;
      this.flushExecLane(stream, "stdout");
      this.flushExecLane(stream, "stderr");
      return;
    }
  }

  private handleExecResponse(header: LuminkaFrame): void {
    if (!header.id) {
      return;
    }
    const stream = this.pendingExecStreams.get(header.id) ?? (header.stream_id ? this.streams.get(header.stream_id) as ExecStreamState | undefined : undefined);
    if (!stream) {
      return;
    }
    if (header.stream_id) {
      this.streams.delete(stream.requestId);
      stream.streamId = header.stream_id;
      this.streams.set(header.stream_id, stream);
      this.pendingExecStreams.delete(header.id);
    }
    stream.closed = true;
    stream.completed.resolve({
      code: typeof header.code === "number" ? header.code : null,
      stdout: stream.stdoutText,
      stderr: stream.stderrText,
    });
    this.flushExecLane(stream, "stdout");
    this.flushExecLane(stream, "stderr");
    this.removeExecStream(stream);
  }

  private appendExecChunk(stream: ExecStreamState, lane: "stdout" | "stderr", payload: Uint8Array): void {
    if (lane === "stdout") {
      stream.stdoutText += textDecoder.decode(payload, { stream: true });
      if (stream.stdoutController) {
        stream.stdoutController.enqueue(payload);
      } else {
        stream.stdoutChunks.push(payload);
      }
      return;
    }
    stream.stderrText += textDecoder.decode(payload, { stream: true });
    if (stream.stderrController) {
      stream.stderrController.enqueue(payload);
    } else {
      stream.stderrChunks.push(payload);
    }
  }

  private flushReadStream(stream: ReadStreamState): void {
    const controller = stream.controller;
    if (!controller) {
      return;
    }
    while (stream.chunks.length > 0) {
      controller.enqueue(stream.chunks.shift() as Uint8Array);
    }
    if (stream.error) {
      controller.error(stream.error);
      this.streams.delete(stream.streamId);
      return;
    }
    if (stream.closed) {
      controller.close();
      this.streams.delete(stream.streamId);
    }
  }

  private flushExecLane(stream: ExecStreamState, lane: "stdout" | "stderr"): void {
    const controller = lane === "stdout" ? stream.stdoutController : stream.stderrController;
    const chunks = lane === "stdout" ? stream.stdoutChunks : stream.stderrChunks;
    if (!controller) {
      return;
    }
    while (chunks.length > 0) {
      controller.enqueue(chunks.shift() as Uint8Array);
    }
    if (stream.closed) {
      controller.close();
    }
  }

  private bindPendingExecStream(streamId: string): ExecStreamState | null {
    if (this.streams.has(streamId)) {
      return this.streams.get(streamId) as ExecStreamState;
    }
    const pending = this.pendingExecStreams.values().next();
    if (pending.done) {
      return null;
    }
    const stream = pending.value;
    if (stream.streamId && stream.streamId !== streamId) {
      return null;
    }
    stream.streamId = streamId;
    this.streams.delete(stream.requestId);
    this.streams.set(streamId, stream);
    return stream;
  }

  private removeStream(streamId: string): void {
    const stream = this.streams.get(streamId);
    if (!stream) {
      return;
    }
    if (stream.kind === "exec") {
      this.removeExecStream(stream);
      return;
    }
    this.streams.delete(streamId);
  }

  private removeExecStream(stream: ExecStreamState): void {
    this.streams.delete(stream.requestId);
    if (stream.streamId) {
      this.streams.delete(stream.streamId);
    } else {
      this.streams.delete(stream.requestId);
    }
    this.pendingExecStreams.delete(stream.requestId);
  }

  private failAll(error: Error): void {
    for (const [id, pending] of this.pending.entries()) {
      this.pending.delete(id);
      pending.reject(error);
    }
    for (const [, stream] of this.streams.entries()) {
      if (stream.kind === "read") {
        if (stream.controller) {
          stream.controller.error(error);
        }
      } else if (stream.kind === "exec") {
        stream.completed.reject(error);
        if (stream.stdoutController) {
          stream.stdoutController.error(error);
        }
        if (stream.stderrController) {
          stream.stderrController.error(error);
        }
      }
    }
    this.streams.clear();
    for (const [id, stream] of this.pendingExecStreams.entries()) {
      this.pendingExecStreams.delete(id);
      stream.completed.reject(error);
    }
  }

  private defaultUrl(): string {
    const host = (globalThis as typeof globalThis & { location?: { host?: string } }).location?.host;
    if (!host) {
      throw new Error('could not infer a WebSocket URL outside a browser host context. Pass an explicit url, for example new LuminkaClient({ url: "ws://127.0.0.1:7777/ws" }).');
    }
    return `ws://${host}${DEFAULT_WS_PATH}`;
  }

  private requireSocket(): WebSocket {
    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) {
      throw new Error("Luminka socket is not open");
    }
    return this.socket;
  }

  private requireAppInfo(response: LuminkaFrame): LuminkaAppInfo {
    const missing: string[] = [];
    if (!response.name) missing.push("name");
    if (!response.mode) missing.push("mode");
    if (!response.root) missing.push("root");
    if (!response.capabilities) missing.push("capabilities");
    if (missing.length > 0) {
      throw new Error(`Luminka app_info response was incomplete: missing ${missing.join(", ")}. Check that the host returns name, mode, root, and capabilities.`);
    }
    return {
      name: response.name!,
      mode: response.mode!,
      root: response.root!,
      capabilities: response.capabilities!,
    };
  }

  private responseError(response: LuminkaFrame): Error {
    return new Error(response.error || "Luminka request failed");
  }

  private requireData(response: LuminkaFrame, label: string): string {
    if (typeof response.data !== "string") {
      throw new Error(`Luminka response did not include ${label}`);
    }
    return response.data;
  }

  private requireExecResult(response: LuminkaFrame): { stdout: string; stderr: string; code: number | null } {
    return {
      stdout: response.stdout ?? "",
      stderr: response.stderr ?? "",
      code: typeof response.code === "number" ? response.code : null,
    };
  }

  private requireStreamId(response: LuminkaFrame, event: string): string {
    if (!response.stream_id) {
      throw new Error(`${event} response did not include a stream_id`);
    }
    return response.stream_id;
  }

  private getOrCreateReadStreamState(streamId: string): ReadStreamState {
    const existing = this.streams.get(streamId);
    if (existing && existing.kind === "read") {
      return existing;
    }
    const state: ReadStreamState = {
      kind: "read",
      streamId,
      chunks: [],
      closed: false,
      error: null,
      controller: null,
    };
    this.streams.set(streamId, state);
    return state;
  }

  private getOrCreateWriteStreamState(streamId: string): WriteStreamState {
    const existing = this.streams.get(streamId);
    if (existing && existing.kind === "write") {
      return existing;
    }
    const state: WriteStreamState = {
      kind: "write",
      streamId,
      nextSeq: 0,
      closed: false,
    };
    this.streams.set(streamId, state);
    return state;
  }

  private nextId(): string {
    return `luminka-${this.nextRequestId++}`;
  }
}

export function createLuminkaClient(options?: LuminkaOptions): LuminkaClient {
  return new LuminkaClient(options);
}

function createDeferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function collectReadableStream(stream: ReadableStream<Uint8Array>): Promise<Uint8Array> {
  const reader = stream.getReader();
  const chunks: Uint8Array[] = [];
  return reader
    .read()
    .then(function pump(result): Promise<Uint8Array> | Uint8Array {
      if (result.done) {
        return concatBytes(chunks);
      }
      chunks.push(result.value);
      return reader.read().then(pump);
    })
    .finally(() => reader.releaseLock());
}

function concatBytes(chunks: Uint8Array[]): Uint8Array {
  const size = chunks.reduce((total, chunk) => total + chunk.byteLength, 0);
  const merged = new Uint8Array(size);
  let offset = 0;
  for (const chunk of chunks) {
    merged.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return merged;
}

function toError(error: unknown, fallback: string): Error {
  return error instanceof Error ? error : new Error(fallback);
}

function toUint8Array(value: ArrayBuffer | ArrayBufferView): Uint8Array {
  if (value instanceof Uint8Array) {
    return value;
  }
  if (ArrayBuffer.isView(value)) {
    return new Uint8Array(value.buffer, value.byteOffset, value.byteLength);
  }
  return new Uint8Array(value);
}
