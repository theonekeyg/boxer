import { BoxerAPIError, BoxerOutputLimitError, BoxerTimeoutError, BoxerValidationError } from "./errors.js";
import type { ResourceLimits, RunOptions, RunResult } from "./types.js";

export interface BoxerClientOptions {
  baseUrl?: string;
  timeout?: number;
}

async function raiseForStatus(res: Response): Promise<void> {
  if (res.ok) return;

  let detail: string;
  try {
    const body = (await res.json()) as Record<string, unknown>;
    detail = typeof body.error === "string" ? body.error : res.statusText;
  } catch {
    detail = res.statusText;
  }

  const code = res.status;
  if (code === 408) throw new BoxerTimeoutError(detail, code);
  if (code === 507) throw new BoxerOutputLimitError(detail, code);
  throw new BoxerAPIError(detail, code);
}

function buildRunBody(image: string, cmd: string[], options: RunOptions): Record<string, unknown> {
  const body: Record<string, unknown> = { image, cmd };
  if (options.env?.length) body.env = options.env;
  if (options.cwd && options.cwd !== "/") body.cwd = options.cwd;
  if (options.limits) body.limits = limitsToObject(options.limits);
  if (options.files?.length) body.files = options.files;
  if (options.persist) body.persist = options.persist;
  if (options.network != null) body.network = options.network;
  return body;
}

function limitsToObject(limits: ResourceLimits): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  if (limits.cpu_cores != null) out.cpu_cores = limits.cpu_cores;
  if (limits.memory_mb != null) out.memory_mb = limits.memory_mb;
  if (limits.pids_limit != null) out.pids_limit = limits.pids_limit;
  if (limits.wall_clock_secs != null) out.wall_clock_secs = limits.wall_clock_secs;
  if (limits.nofile != null) out.nofile = limits.nofile;
  return out;
}

function parseRunResult(data: unknown): RunResult {
  if (data == null || typeof data !== "object" || Array.isArray(data)) {
    throw new BoxerAPIError("Unexpected response from Boxer API: expected an object", 200);
  }

  const d = data as Record<string, unknown>;

  if (typeof d.exec_id !== "string") {
    throw new BoxerAPIError("Unexpected response from Boxer API: exec_id must be a string", 200);
  }
  if (typeof d.exit_code !== "number" || !Number.isFinite(d.exit_code)) {
    throw new BoxerAPIError(
      "Unexpected response from Boxer API: exit_code must be a finite number",
      200,
    );
  }
  if (typeof d.stdout !== "string") {
    throw new BoxerAPIError("Unexpected response from Boxer API: stdout must be a string", 200);
  }
  if (typeof d.stderr !== "string") {
    throw new BoxerAPIError("Unexpected response from Boxer API: stderr must be a string", 200);
  }
  if (typeof d.wall_ms !== "number" || !Number.isFinite(d.wall_ms)) {
    throw new BoxerAPIError(
      "Unexpected response from Boxer API: wall_ms must be a finite number",
      200,
    );
  }

  return {
    exec_id: d.exec_id,
    exit_code: d.exit_code,
    stdout: d.stdout,
    stderr: d.stderr,
    wall_ms: d.wall_ms,
  };
}

export class BoxerClient {
  private readonly baseUrl: string;
  private readonly timeout: number;

  constructor(options: BoxerClientOptions = {}) {
    this.baseUrl = (options.baseUrl ?? "http://localhost:8080").replace(/\/$/, "");
    this.timeout = options.timeout ?? 120_000;
  }

  private async fetch(
    path: string,
    init: RequestInit = {},
  ): Promise<[Response, AbortController]> {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeout);
    // biome-ignore lint/suspicious/noExplicitAny: unref is Node-only
    (timer as any).unref?.();

    try {
      const res = await fetch(`${this.baseUrl}${path}`, {
        ...init,
        signal: controller.signal,
      });
      return [res, controller];
    } catch (err) {
      clearTimeout(timer);
      if (controller.signal.aborted) {
        throw new BoxerTimeoutError(`Request to ${path} timed out after ${this.timeout}ms`, 0);
      }
      throw err;
    }
    // timer intentionally not cleared on success — it keeps running through the
    // caller's body read and aborts a stalled stream if the deadline is exceeded
  }

  private timeoutError(path: string): BoxerTimeoutError {
    return new BoxerTimeoutError(`Request to ${path} timed out after ${this.timeout}ms`, 0);
  }

  /**
   * Returns `true` if the server responds with a 2xx status, `false` for non-OK responses.
   * Network-level failures (connection refused, DNS error, timeout) propagate as exceptions
   * so callers can distinguish between "server is up but unhealthy" and "server is unreachable".
   */
  async health(): Promise<boolean> {
    const [res] = await this.fetch("/healthz");
    // Discard the body so the underlying TCP connection is released back to the
    // keep-alive pool immediately. cancel() signals the stream to abort without
    // reading — it returns instantly and does not wait for data to drain.
    await res.body?.cancel();
    return res.ok;
  }

  async run(image: string, cmd: string[], options: RunOptions = {}): Promise<RunResult> {
    if (!image) throw new BoxerValidationError("image must be a non-empty string");
    if (!cmd.length) throw new BoxerValidationError("cmd must be a non-empty array");
    const body = buildRunBody(image, cmd, options);
    const [res, controller] = await this.fetch("/run", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    try {
      await raiseForStatus(res);
      return parseRunResult(await res.json());
    } catch (err) {
      if (controller.signal.aborted) throw this.timeoutError("/run");
      throw err;
    }
  }

  async uploadFile(remotePath: string, content: Blob | Uint8Array | ArrayBuffer): Promise<void> {
    if (!remotePath) throw new BoxerValidationError("remotePath must be a non-empty string");
    // Normalise once: strip trailing slash so both the path field and the
    // Content-Disposition filename are consistent (e.g. "output/" → "output").
    const normalisedPath = remotePath.replace(/\/$/, "");
    const form = new FormData();
    form.append("path", normalisedPath);
    let blob: Blob;
    if (content instanceof Blob) {
      blob = content;
    } else if (content instanceof Uint8Array) {
      // Copy via ArrayLike<number> to ensure ArrayBuffer (not SharedArrayBuffer)
      blob = new Blob([new Uint8Array(content)], { type: "application/octet-stream" });
    } else {
      blob = new Blob([content], { type: "application/octet-stream" });
    }
    form.append("file", blob, normalisedPath.split("/").pop() || "file");
    const [res, controller] = await this.fetch("/files", { method: "POST", body: form });
    try {
      await raiseForStatus(res);
      // Discard the success response body to release the connection back to the
      // keep-alive pool — raiseForStatus returns without consuming it on 2xx.
      await res.body?.cancel();
    } catch (err) {
      if (controller.signal.aborted) throw this.timeoutError("/files");
      throw err;
    }
  }

  async downloadFile(path: string): Promise<Uint8Array> {
    if (!path) throw new BoxerValidationError("path must be a non-empty string");
    const [res, controller] = await this.fetch(
      `/files?${new URLSearchParams({ path }).toString()}`,
    );
    try {
      await raiseForStatus(res);
      return new Uint8Array(await res.arrayBuffer());
    } catch (err) {
      if (controller.signal.aborted) throw this.timeoutError("/files");
      throw err;
    }
  }
}
