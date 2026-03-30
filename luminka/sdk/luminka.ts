// FILE: luminka/sdk/luminka.ts
// PURPOSE: Provide the browser-first Luminka WebSocket SDK for frontend apps.
// OWNS: WebSocket lifecycle, request/response helpers, and file/process capability wrappers.
// EXPORTS: LuminkaCapabilities, LuminkaAppInfo, LuminkaOptions, LuminkaClient, createLuminkaClient
// DOCS: docs/spec.md, docs/arch.md

export type LuminkaCapabilities = {
  fs: boolean;
  scripts: boolean;
  shell: boolean;
};

export type LuminkaAppInfo = {
  name: string;
  mode: "browser" | "webview";
  root: string;
  capabilities: LuminkaCapabilities;
};

export type LuminkaOptions = {
  url?: string;
};

type WireRequest = {
  event: string;
  id?: string;
  path?: string;
  data?: string;
  runner?: string;
  file?: string;
  cmd?: string;
  args?: string[];
  timeout?: number;
};

type WireResponse = {
  event?: string;
  id?: string;
  ok?: boolean;
  error?: string;
  path?: string;
  data?: string;
  files?: string[];
  exists?: boolean;
  stdout?: string;
  stderr?: string;
  code?: number;
  name?: string;
  mode?: "browser" | "webview";
  root?: string;
  capabilities?: LuminkaCapabilities;
};

type PendingRequest = {
  resolve: (value: WireResponse) => void;
  reject: (reason: Error) => void;
};

const DEFAULT_WS_PATH = "/ws";

const REQUEST_PENDING_CLOSED_MESSAGE =
  "Luminka connection closed while waiting for a response. Reconnect and retry the request.";

export class LuminkaClient {
  private readonly options: LuminkaOptions;
  private socket: WebSocket | null = null;
  private connectPromise: Promise<void> | null = null;
  private nextRequestId = 1;
  private pending = new Map<string, PendingRequest>();
  private fileListeners = new Set<(path: string) => void>();

  constructor(options: LuminkaOptions = {}) {
    this.options = options;
  }

  async connect(): Promise<void> {
    if (this.socket && this.socket.readyState === WebSocket.OPEN) {
      return;
    }
    if (this.connectPromise) {
      return this.connectPromise;
    }

    const url = this.options.url ?? this.defaultUrl();
    const socket = new WebSocket(url);
    this.socket = socket;

    this.connectPromise = new Promise<void>((resolve, reject) => {
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
        reject(this.connectionFailedError(url));
      };

      const handleClose = () => {
        cleanup();
        this.connectPromise = null;
        this.socket = null;
        this.rejectPending(this.pendingClosedError());
        reject(this.connectionFailedError(url));
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
      this.rejectPending(this.pendingClosedError());
    });

    return this.connectPromise;
  }

  disconnect(): void {
    const socket = this.socket;
    this.socket = null;
    this.connectPromise = null;
    this.rejectPending(this.pendingClosedError());
    if (socket && socket.readyState < WebSocket.CLOSING) {
      socket.close();
    }
  }

  async appInfo(): Promise<LuminkaAppInfo> {
    const response = await this.request({ event: "app_info" });
    return this.requireAppInfo(response);
  }

  async read(path: string): Promise<string> {
    const response = await this.request({ event: "fs_read", path });
    return this.requireData(response);
  }

  async write(path: string, data: string): Promise<void> {
    await this.request({ event: "fs_write", path, data });
  }

  async list(path: string = ""): Promise<string[]> {
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

  async runScript(
    runner: string,
    file: string,
    args: string[] = [],
    timeout?: number,
  ): Promise<{ stdout: string; stderr: string; code: number | null }> {
    const response = await this.request({ event: "script_exec", runner, file, args, timeout });
    return this.requireExecResult(response);
  }

  async runShell(
    cmd: string,
    args: string[] = [],
    timeout?: number,
  ): Promise<{ stdout: string; stderr: string; code: number | null }> {
    const response = await this.request({ event: "shell_exec", cmd, args, timeout });
    return this.requireExecResult(response);
  }

  onFileChanged(listener: (path: string) => void): () => void {
    this.fileListeners.add(listener);
    return () => this.fileListeners.delete(listener);
  }

  private defaultUrl(): string {
    const host = globalThis.location?.host;
    if (!host) {
      throw new Error(
        "LuminkaClient could not infer a WebSocket URL outside a browser host context. Pass an explicit url, for example new LuminkaClient({ url: \"ws://127.0.0.1:7777/ws\" }).",
      );
    }
    return `ws://${host}${DEFAULT_WS_PATH}`;
  }

  private async request(payload: WireRequest): Promise<WireResponse> {
    await this.connect();
    const socket = this.socket;
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      throw new Error("Luminka socket is not open");
    }

    const id = this.nextId();
    const message = { ...payload, id };

    return new Promise<WireResponse>((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      try {
        socket.send(JSON.stringify(message));
      } catch (error) {
        this.pending.delete(id);
        reject(error instanceof Error ? error : new Error("failed to send Luminka request"));
      }
    }).then((response) => {
      if (response.event === "error" || response.ok === false) {
        throw this.responseError(payload, response);
      }
      return response;
    });
  }

  private handleMessage(event: MessageEvent<string>): void {
    let message: WireResponse;
    try {
      message = JSON.parse(event.data) as WireResponse;
    } catch {
      return;
    }

    if (message.event === "fs_changed" && message.path) {
      for (const listener of this.fileListeners) {
        listener(message.path);
      }
      return;
    }

    const id = message.id;
    if (!id) {
      return;
    }

    const pending = this.pending.get(id);
    if (!pending) {
      return;
    }

    this.pending.delete(id);
    pending.resolve(message);
  }

  private rejectPending(error: Error): void {
    for (const [id, pending] of this.pending.entries()) {
      this.pending.delete(id);
      pending.reject(error);
    }
  }

  private responseError(request: WireRequest, response: WireResponse): Error {
    const message = response.error || "Luminka request failed.";
    const capabilityError = this.capabilityDisabledError(request, message);
    if (capabilityError) {
      return capabilityError;
    }
    return new Error(message);
  }

  private requireAppInfo(response: WireResponse): LuminkaAppInfo {
    const missingFields: string[] = [];
    if (!response.name) {
      missingFields.push("name");
    }
    if (!response.mode) {
      missingFields.push("mode");
    }
    if (!response.root) {
      missingFields.push("root");
    }
    if (!response.capabilities) {
      missingFields.push("capabilities");
    }

    if (missingFields.length > 0 || !response.name || !response.mode || !response.root || !response.capabilities) {
      throw new Error(
        `Luminka app_info response was incomplete: missing ${missingFields.join(", ")}. Check that the host returns name, mode, root, and capabilities.`,
      );
    }
    return {
      name: response.name,
      mode: response.mode,
      root: response.root,
      capabilities: response.capabilities,
    };
  }

  private requireData(response: WireResponse): string {
    if (typeof response.data !== "string") {
      throw new Error("Luminka response did not include file data");
    }
    return response.data;
  }

  private requireExecResult(response: WireResponse): { stdout: string; stderr: string; code: number | null } {
    return {
      stdout: response.stdout ?? "",
      stderr: response.stderr ?? "",
      code: response.code ?? null,
    };
  }

  private nextId(): string {
    return `luminka-${this.nextRequestId++}`;
  }

  private connectionFailedError(url: string): Error {
    return new Error(
      `Failed to connect to Luminka at ${url}. Confirm the host app is running and serving the SDK WebSocket endpoint.`,
    );
  }

  private pendingClosedError(): Error {
    return new Error(REQUEST_PENDING_CLOSED_MESSAGE);
  }

  private capabilityDisabledError(request: WireRequest, message: string): Error | null {
    if (!/capability is disabled/i.test(message)) {
      return null;
    }

    const capability = this.requestCapability(request.event);
    if (!capability) {
      return new Error(message);
    }

    return new Error(
      `${message}. Enable the ${capability} capability in the Luminka host app, or avoid calling this SDK method when that capability is unavailable.`,
    );
  }

  private requestCapability(event: string): string | null {
    if (event.startsWith("fs_")) {
      return "filesystem";
    }
    if (event.startsWith("script_")) {
      return "scripts";
    }
    if (event.startsWith("shell_")) {
      return "shell";
    }
    return null;
  }
}

export function createLuminkaClient(options?: LuminkaOptions): LuminkaClient {
  return new LuminkaClient(options);
}
