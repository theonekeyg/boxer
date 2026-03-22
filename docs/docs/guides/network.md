---
sidebar_position: 2
---

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

# Network

Every execution has an explicit network mode. The default is `none` — no network at all. You opt in to connectivity explicitly per request.

```json
{
  "image": "python:3.12-slim",
  "cmd": ["python3", "fetch.py"],
  "network": "sandbox"
}
```

## Modes

### `none` (default)

The container has no network interface beyond loopback. Any attempt to open a socket to an external address fails immediately. This is the most secure mode and the right choice whenever the workload does not need the internet.

<Tabs groupId="sdk-language">
<TabItem value="python" label="Python" default>

```python
result = client.run(
    image="python:3.12-slim",
    cmd=["python3", "-c", "import socket; socket.create_connection(('8.8.8.8', 53))"],
    # network defaults to "none"
)
# exit_code != 0 — external connection fails
```

</TabItem>
<TabItem value="typescript" label="TypeScript">

```typescript
const result = await client.run(
  "python:3.12-slim",
  ["python3", "-c", "import socket; socket.create_connection(('8.8.8.8', 53))"],
  // network defaults to "none"
);
// result.exit_code != 0 — external connection fails
```

</TabItem>
</Tabs>

### `sandbox`

Boxer creates a dedicated network namespace for the container and connects it to the host via a veth pair and NAT. The container gets a private IP, a default route through the host, and DNS resolvers (8.8.8.8 / 8.8.4.4 by default). Traffic is fully isolated from other running containers.

<Tabs groupId="sdk-language">
<TabItem value="python" label="Python" default>

```python
result = client.run(
    image="python:3.12-slim",
    cmd=["python3", "-c", "import urllib.request; print(urllib.request.urlopen('http://example.com').status)"],
    network="sandbox",
)
print(result.stdout)  # 200
```

</TabItem>
<TabItem value="typescript" label="TypeScript">

```typescript
const result = await client.run(
  "python:3.12-slim",
  ["python3", "-c", "import urllib.request; print(urllib.request.urlopen('http://example.com').status)"],
  { network: "sandbox" },
);
console.log(result.stdout); // 200
```

</TabItem>
</Tabs>

Use `sandbox` when:
- The workload needs to fetch data from the internet (pip install, API calls, dataset downloads)
- You still want network isolation between concurrent executions

Requires Boxer to run as root (`CAP_NET_ADMIN`) for veth and bridge setup.

### `host`

The container shares the host's network stack. There is no isolation — the container can reach anything the host can, and can bind ports on the host's interfaces.

Use `host` only when you need to reach services on the host itself (e.g. a local database or another process listening on `localhost`) and you trust the workload.

Requires root for the same reasons as `sandbox`.

## DNS

In `sandbox` mode, DNS is handled with explicit public resolvers. The default resolvers are `8.8.8.8` and `8.8.4.4`. You can override them in the server config:

```json
{
  "dns_servers": ["1.1.1.1", "1.0.0.1"]
}
```

## Example: pip install at runtime

<Tabs groupId="sdk-language">
<TabItem value="python" label="Python" default>

```python
result = client.run(
    image="python:3.12-slim",
    cmd=["sh", "-c", "pip install httpx -q && python3 -c \"import httpx; print(httpx.get('http://example.com').status_code)\""],
    network="sandbox",
    limits={"wall_clock_secs": 60},
)
print(result.stdout)  # 200
```

</TabItem>
<TabItem value="typescript" label="TypeScript">

```typescript
const result = await client.run(
  "python:3.12-slim",
  ["sh", "-c", "pip install httpx -q && python3 -c \"import httpx; print(httpx.get('http://example.com').status_code)\""],
  { network: "sandbox", limits: { wall_clock_secs: 60 } },
);
console.log(result.stdout); // 200
```

</TabItem>
</Tabs>

See also: [upload-and-run example](../examples/upload-and-run) for the pattern of uploading code before execution instead of installing dependencies at runtime.
