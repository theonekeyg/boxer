import { BoxerAPIError, BoxerOutputLimitError, BoxerTimeoutError } from "./errors.js";
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

  private async fetch(path: string, init: RequestInit = {}): Promise<Response> {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeout);

    try {
      return await fetch(`${this.baseUrl}${path}`, {
        ...init,
        signal: controller.signal,
      });
    } catch (err) {
      if (controller.signal.aborted || (err instanceof Error && err.name === "AbortError")) {
        throw new BoxerTimeoutError(`Request to ${path} timed out after ${this.timeout}ms`, 0);
      }
      throw err;
    } finally {
      clearTimeout(timer);
    }
  }

  async health(): Promise<boolean> {
    const res = await this.fetch("/healthz");
    return res.ok;
  }

  async run(image: string, cmd: string[], options: RunOptions = {}): Promise<RunResult> {
    if (!cmd.length) throw new BoxerAPIError("cmd must be a non-empty array", 0);
    const body = buildRunBody(image, cmd, options);
    const res = await this.fetch("/run", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    await raiseForStatus(res);
    return parseRunResult(await res.json());
  }

  async uploadFile(remotePath: string, content: Blob | Uint8Array | ArrayBuffer): Promise<void> {
    const form = new FormData();
    form.append("path", remotePath);
    let blob: Blob;
    if (content instanceof Blob) {
      blob = content;
    } else if (content instanceof Uint8Array) {
      // Copy via ArrayLike<number> to ensure ArrayBuffer (not SharedArrayBuffer)
      blob = new Blob([new Uint8Array(content)], { type: "application/octet-stream" });
    } else {
      blob = new Blob([content], { type: "application/octet-stream" });
    }
    form.append("file", blob, remotePath.split("/").pop() ?? "file");

    const res = await this.fetch("/files", { method: "POST", body: form });
    await raiseForStatus(res);
  }

  async downloadFile(path: string): Promise<Uint8Array> {
    const res = await this.fetch(`/files?${new URLSearchParams({ path }).toString()}`);
    await raiseForStatus(res);
    return new Uint8Array(await res.arrayBuffer());
  }
}
