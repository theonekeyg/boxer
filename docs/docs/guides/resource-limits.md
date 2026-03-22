---
sidebar_position: 3
---

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

# Resource Limits

Every execution runs inside a gVisor sandbox with configurable hard limits on CPU, memory, time, and process count. Limits can be set server-side as defaults and overridden per request.

## Available limits

| Field | Unit | Description |
|---|---|---|
| `cpu_cores` | fractional cores | CPU weight (e.g. `0.5` = half a core) |
| `memory_mb` | megabytes | Maximum RSS |
| `wall_clock_secs` | seconds | Hard wall-clock timeout — the container is killed after this |
| `pids_limit` | count | Maximum concurrent processes/threads |
| `nofile` | count | Maximum open file descriptors |

## Setting limits per request

Pass a `limits` object in the run request. Any field you omit falls back to the server default.

<Tabs groupId="sdk-language">
<TabItem value="python" label="Python" default>

```python
from boxer import BoxerClient, ResourceLimits

with BoxerClient("http://localhost:8080") as client:
    result = client.run(
        image="python:3.12-slim",
        cmd=["python3", "-c", "print('done')"],
        limits=ResourceLimits(
            cpu_cores=0.5,
            memory_mb=128,
            wall_clock_secs=10,
        ),
    )
```

</TabItem>
<TabItem value="typescript" label="TypeScript">

```typescript
import type { ResourceLimits } from "boxer-sdk";

const limits: ResourceLimits = {
  cpu_cores: 0.5,
  memory_mb: 128,
  wall_clock_secs: 10,
};

const result = await client.run(
  "python:3.12-slim",
  ["python3", "-c", "print('done')"],
  { limits },
);
```

</TabItem>
</Tabs>

Or with the REST API:

```bash
curl -s http://localhost:8080/run \
  -H 'Content-Type: application/json' \
  -d '{
    "image": "python:3.12-slim",
    "cmd": ["python3", "-c", "print(42)"],
    "limits": {
      "cpu_cores": 0.5,
      "memory_mb": 128,
      "wall_clock_secs": 10
    }
  }'
```

## Server defaults

Set defaults in `config.json` under the `defaults` key. These apply to any request that does not specify a limit:

```json
{
  "defaults": {
    "cpu_cores": 1.0,
    "memory_mb": 256,
    "wall_clock_secs": 30,
    "pids_limit": 64,
    "nofile": 256
  }
}
```

## Timeout behaviour

When `wall_clock_secs` is exceeded, Boxer kills the sandbox and returns HTTP 408 with a `BoxerTimeoutError` on the SDK side. The response includes how long the execution ran.

<Tabs groupId="sdk-language">
<TabItem value="python" label="Python" default>

```python
from boxer import BoxerTimeoutError

try:
    result = client.run(
        image="python:3.12-slim",
        cmd=["python3", "-c", "while True: pass"],
        limits=ResourceLimits(wall_clock_secs=5),
    )
except BoxerTimeoutError:
    print("killed after 5 seconds")
```

</TabItem>
<TabItem value="typescript" label="TypeScript">

```typescript
import { BoxerTimeoutError } from "boxer-sdk";

try {
  const result = await client.run(
    "python:3.12-slim",
    ["python3", "-c", "while True: pass"],
    { limits: { wall_clock_secs: 5 } },
  );
} catch (err) {
  if (err instanceof BoxerTimeoutError) {
    console.error("killed after 5 seconds");
  }
}
```

</TabItem>
</Tabs>

## Output limits

Separately from resource limits, stdout and stderr are each capped at `output_limit_bytes` (configured server-side, default 10 MB). If either stream exceeds the limit, the run returns HTTP 507 and `BoxerOutputLimitError`:

<Tabs groupId="sdk-language">
<TabItem value="python" label="Python" default>

```python
from boxer import BoxerOutputLimitError

try:
    result = client.run(
        image="python:3.12-slim",
        cmd=["python3", "-c", "print('x' * 20_000_000)"],
    )
except BoxerOutputLimitError:
    print("stdout exceeded 10 MB limit")
```

</TabItem>
<TabItem value="typescript" label="TypeScript">

```typescript
import { BoxerOutputLimitError } from "boxer-sdk";

try {
  const result = await client.run(
    "python:3.12-slim",
    ["python3", "-c", "print('x' * 20_000_000)"],
  );
} catch (err) {
  if (err instanceof BoxerOutputLimitError) {
    console.error("stdout exceeded 10 MB limit");
  }
}
```

</TabItem>
</Tabs>

## Choosing limits for LLM workloads

For evaluating generated code (e.g. the [HumanEval example](../examples/humaneval)), a reasonable starting point:

<Tabs groupId="sdk-language">
<TabItem value="python" label="Python" default>

```python
ResourceLimits(
    cpu_cores=1.0,
    memory_mb=256,
    wall_clock_secs=30,
    pids_limit=64,
)
```

</TabItem>
<TabItem value="typescript" label="TypeScript">

```typescript
const limits: ResourceLimits = {
  cpu_cores: 1.0,
  memory_mb: 256,
  wall_clock_secs: 30,
  pids_limit: 64,
};
```

</TabItem>
</Tabs>

Set `wall_clock_secs` conservatively — LLM-generated code can loop infinitely. Most correct solutions finish in under a second; a 30-second limit gives generous headroom while bounding tail latency.
