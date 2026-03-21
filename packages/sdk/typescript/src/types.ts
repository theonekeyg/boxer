export interface ResourceLimits {
  cpu_cores?: number;
  memory_mb?: number;
  pids_limit?: number;
  wall_clock_secs?: number;
  nofile?: number;
}

export interface RunResult {
  exec_id: string;
  exit_code: number;
  stdout: string;
  stderr: string;
  wall_ms: number;
}

export interface RunOptions {
  env?: string[];
  cwd?: string;
  limits?: ResourceLimits;
  files?: string[];
  persist?: boolean;
  network?: "none" | "sandbox" | "host";
}
