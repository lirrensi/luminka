// FILE: luminka/sdk/luminka.ts
// PURPOSE: Provide the browser-first Luminka WebSocket SDK for frontend apps.
// OWNS: WebSocket lifecycle, request/response helpers, and file/process capability wrappers.
// EXPORTS: LuminkaCapabilities, LuminkaAppInfo, LuminkaOptions, LuminkaClient, createLuminkaClient
// DOCS: docs/spec.md, docs/arch.md
const DEFAULT_WS_PATH = "/ws";
export class LuminkaClient {
    constructor(options = {}) {
        this.socket = null;
        this.connectPromise = null;
        this.nextRequestId = 1;
        this.pending = new Map();
        this.fileListeners = new Set();
        this.options = options;
    }
    async connect() {
        if (this.socket && this.socket.readyState === WebSocket.OPEN) {
            return;
        }
        if (this.connectPromise) {
            return this.connectPromise;
        }
        const url = this.options.url ?? this.defaultUrl();
        const socket = new WebSocket(url);
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
                reject(new Error(`failed to connect to Luminka at ${url}`));
            };
            const handleClose = () => {
                cleanup();
                this.connectPromise = null;
                this.socket = null;
                this.rejectPending(new Error("Luminka connection closed"));
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
            this.rejectPending(new Error("Luminka connection closed"));
        });
        return this.connectPromise;
    }
    disconnect() {
        const socket = this.socket;
        this.socket = null;
        this.connectPromise = null;
        this.rejectPending(new Error("Luminka connection closed"));
        if (socket && socket.readyState < WebSocket.CLOSING) {
            socket.close();
        }
    }
    async appInfo() {
        const response = await this.request({ event: "app_info" });
        return this.requireAppInfo(response);
    }
    async read(path) {
        const response = await this.request({ event: "fs_read", path });
        return this.requireData(response);
    }
    async write(path, data) {
        await this.request({ event: "fs_write", path, data });
    }
    async list(path = "") {
        const response = await this.request({ event: "fs_list", path });
        return response.files ?? [];
    }
    async remove(path) {
        await this.request({ event: "fs_delete", path });
    }
    async exists(path) {
        const response = await this.request({ event: "fs_exists", path });
        return response.exists ?? false;
    }
    async watch(path) {
        await this.request({ event: "fs_watch", path });
    }
    async unwatch(path) {
        await this.request({ event: "fs_unwatch", path });
    }
    async runScript(runner, file, args = [], timeout) {
        const response = await this.request({ event: "script_exec", runner, file, args, timeout });
        return this.requireExecResult(response);
    }
    async runShell(cmd, args = [], timeout) {
        const response = await this.request({ event: "shell_exec", cmd, args, timeout });
        return this.requireExecResult(response);
    }
    onFileChanged(listener) {
        this.fileListeners.add(listener);
        return () => this.fileListeners.delete(listener);
    }
    defaultUrl() {
        const host = globalThis.location?.host;
        if (!host) {
            throw new Error("LuminkaClient requires an explicit url outside the browser");
        }
        return `ws://${host}${DEFAULT_WS_PATH}`;
    }
    async request(payload) {
        await this.connect();
        const socket = this.socket;
        if (!socket || socket.readyState !== WebSocket.OPEN) {
            throw new Error("Luminka socket is not open");
        }
        const id = this.nextId();
        const message = { ...payload, id };
        return new Promise((resolve, reject) => {
            this.pending.set(id, { resolve, reject });
            try {
                socket.send(JSON.stringify(message));
            }
            catch (error) {
                this.pending.delete(id);
                reject(error instanceof Error ? error : new Error("failed to send Luminka request"));
            }
        }).then((response) => {
            if (response.event === "error" || response.ok === false) {
                throw this.responseError(response);
            }
            return response;
        });
    }
    handleMessage(event) {
        let message;
        try {
            message = JSON.parse(event.data);
        }
        catch {
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
    rejectPending(error) {
        for (const [id, pending] of this.pending.entries()) {
            this.pending.delete(id);
            pending.reject(error);
        }
    }
    responseError(response) {
        return new Error(response.error || "Luminka request failed");
    }
    requireAppInfo(response) {
        if (!response.name || !response.mode || !response.root || !response.capabilities) {
            throw new Error("Luminka app_info response was incomplete");
        }
        return {
            name: response.name,
            mode: response.mode,
            root: response.root,
            capabilities: response.capabilities,
        };
    }
    requireData(response) {
        if (typeof response.data !== "string") {
            throw new Error("Luminka response did not include file data");
        }
        return response.data;
    }
    requireExecResult(response) {
        return {
            stdout: response.stdout ?? "",
            stderr: response.stderr ?? "",
            code: response.code ?? null,
        };
    }
    nextId() {
        return `luminka-${this.nextRequestId++}`;
    }
}
export function createLuminkaClient(options) {
    return new LuminkaClient(options);
}
