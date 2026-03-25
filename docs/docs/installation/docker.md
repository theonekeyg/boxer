---
sidebar_position: 1
---

# Docker

The easiest way to run Boxer is via the pre-built image published on DockerHub.

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) installed and running
- The host must support nested cgroup management (most Linux distributions with cgroup v2 do)

## Run with Docker Compose (recommended)

Download the production compose file and start the service:

```bash
# Replace "main" with the version tag you want to deploy, e.g. refs/tags/v1.0.0
curl -fsSL https://raw.githubusercontent.com/theonekeyg/boxer/main/docker-compose.prod.yml -o docker-compose.prod.yml
docker compose -f docker-compose.prod.yml up -d
```

The server will be available at `http://localhost:8080`.

## Run with Docker

```bash
docker run -d \
  --privileged \
  -p 8080:8080 \
  theonekeyg/boxer
```

:::note Why `--privileged`?
Boxer manages cgroups, network namespaces, and spawns gVisor (`runsc`) to sandbox each execution. These operations require elevated privileges on the host. Without `--privileged`, container creation will fail with a cgroup permission error.
:::

## Verify

```bash
curl http://localhost:8080/healthz
```

## Available Tags

| Tag | Description |
|---|---|
| `latest` | Latest build from `main` |
| `1.2.3` | Specific release version |
| `1.2` | Latest patch of a minor version |

## Configuration

The image ships with a Docker-optimised config (see `docker/config.json` in the repository). To override it, mount your own config file over the default path:

```bash
docker run -d \
  --privileged \
  -p 8080:8080 \
  -v /path/to/your/config.json:/etc/boxer/config.json:ro \
  theonekeyg/boxer
```

To mount to a different path, also set `BOXER_CONFIG`:

```bash
docker run -d \
  --privileged \
  -p 8080:8080 \
  -v /path/to/your/config.json:/my/config.json:ro \
  -e BOXER_CONFIG=/my/config.json \
  theonekeyg/boxer
```

See the [Getting Started](../intro.md#configuration) page for all available configuration fields.
