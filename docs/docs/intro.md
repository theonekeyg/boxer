---
sidebar_position: 1
---

# Getting Started

Boxer is a sandboxed container execution service backed by [gVisor](https://gvisor.dev/). It exposes a simple HTTP API for running arbitrary commands inside any container image, with strong isolation guarantees and configurable resource limits.

## Why Boxer

Running untrusted code is a hard problem. Docker alone provides namespace isolation but shares the host kernel — a compromised container can exploit kernel vulnerabilities and escape. Boxer wraps every execution in gVisor's user-space kernel (`runsc`), which intercepts and validates all system calls before they reach the host. The attack surface is dramatically reduced.

This makes Boxer a good fit for:

- **LLM training and inference pipelines** — execute model-generated code safely without exposing your host to arbitrary syscalls
- **Decentralized oracle evaluations** — run untrusted verification scripts submitted by network participants
- **Prediction markets and agent frameworks** — evaluate outcomes by executing code from unknown sources
- **Code execution as a service** — any scenario where you need to run unstructured, user-supplied, or LLM-generated code at scale

## How It Works

1. A client sends a `POST /run` request with a container image, command, optional files, and resource limits.
2. Boxer pulls and caches the image rootfs locally (shared read-only across executions).
3. It constructs a hardened OCI bundle and spawns `runsc` (gVisor) to execute the command.
4. Stdout, stderr, wall time, and exit code are returned in the response.

Files can be uploaded before a run and bind-mounted read-only inside the container. Output files written to `/output/` inside the container are captured and retrievable after the run.

## Prerequisites

- [gVisor `runsc`](https://gvisor.dev/docs/user_guide/install/) installed and in `PATH`
- Go 1.22+

## Run the Server

```bash
cd packages/core
go run . --config config.dev.json
```

The server listens on `:8080` by default. Configuration can also be set via `$BOXER_CONFIG` or `~/.boxer/config.json`.

## Quick Test

```bash
curl -s http://localhost:8080/run \
  -H 'Content-Type: application/json' \
  -d '{"image":"python:3.12-slim","cmd":["python3","-c","print(42)"]}'
```

Swagger UI is available at `http://localhost:8080/swagger`.

## Configuration

Key fields in `config.json`:

| Field | Default | Description |
|---|---|---|
| `home` | `~/.boxer` | Base directory for all boxer data |
| `runsc_path` | *(PATH lookup)* | Path to `runsc` binary |
| `platform` | `systrap` | gVisor platform: `systrap`, `ptrace`, or `kvm` |
| `listen_addr` | `:8080` | HTTP listen address |
| `ignore_cgroups` | `false` | Skip cgroup setup (useful for rootless/dev) |
| `output_limit_bytes` | `10485760` | Maximum bytes captured per stream (stdout/stderr) |
| `upload_limit_bytes` | `10485760` | Maximum multipart upload size buffered in RAM |
| `defaults.cpu_cores` | `1.0` | Default CPU limit per execution |
| `defaults.memory_mb` | `256` | Default memory limit (MB) |
| `defaults.wall_clock_secs` | `30` | Default execution timeout |
| `defaults.pids_limit` | `64` | Default max processes per execution |
| `defaults.nofile` | `256` | Default max open file descriptors per execution |

Per-request limits in `POST /run` override the configured defaults.
