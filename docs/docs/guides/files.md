---
sidebar_position: 1
---

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

# Files

Boxer has a server-side file store for passing data in and out of containers. The flow is:

1. **Upload** input files before the run (`POST /files`)
2. **Reference** them by path in the run request — Boxer bind-mounts each one read-only at `/<path>` inside the container
3. **Write** output to `/output/` inside the container
4. **Download** captured output files after the run (`GET /files?path=output/<exec_id>/...`)

## Setup

<Tabs groupId="sdk-language">
<TabItem value="python" label="Python" default>

```python
from boxer import BoxerClient

client = BoxerClient("http://localhost:8080")
```

</TabItem>
<TabItem value="typescript" label="TypeScript">

```typescript
import { BoxerClient } from "boxer-sdk";

const client = new BoxerClient({ baseUrl: "http://localhost:8080" });
```

</TabItem>
</Tabs>

## Uploading input files

<Tabs groupId="sdk-language">
<TabItem value="python" label="Python" default>

```python
with open("script.py", "rb") as f:
    client.upload_file("workspace/script.py", f)
```

</TabItem>
<TabItem value="typescript" label="TypeScript">

```typescript
import { readFile } from "node:fs/promises";

const script = await readFile("script.py");
await client.uploadFile("workspace/script.py", script);
```

</TabItem>
</Tabs>

The path you provide (`workspace/script.py`) is the key in the store **and** the mount destination inside the container (`/workspace/script.py`).

You can upload a whole directory by iterating over it:

<Tabs groupId="sdk-language">
<TabItem value="python" label="Python" default>

```python
import pathlib

root = pathlib.Path("project/")
for p in root.rglob("*"):
    if p.is_file():
        rel = str(p)
        with open(p, "rb") as f:
            client.upload_file(rel, f)
```

</TabItem>
<TabItem value="typescript" label="TypeScript">

```typescript
import { uploadPath } from "boxer-sdk/node";

// Upload a single file
const filePaths = await uploadPath(client, "./script.py");
// => ["script.py"]

// Upload an entire directory
const dirPaths = await uploadPath(client, "./project", "project");
// => ["project/main.py", "project/utils.py", ...]
```

</TabItem>
</Tabs>

## Referencing files in a run

Pass the paths you uploaded in the `files` array. Boxer checks that all listed files exist before spawning the container — the request fails with 400 if any are missing.

<Tabs groupId="sdk-language">
<TabItem value="python" label="Python" default>

```python
result = client.run(
    image="python:3.12-slim",
    cmd=["python3", "/workspace/script.py"],
    files=["workspace/script.py"],
)
```

</TabItem>
<TabItem value="typescript" label="TypeScript">

```typescript
const result = await client.run(
  "python:3.12-slim",
  ["python3", "/workspace/script.py"],
  { files: ["workspace/script.py"] },
);
```

</TabItem>
</Tabs>

Each file is mounted read-only. The container cannot modify uploaded files — if the workload needs to write, have it write to `/output/` instead.

## Capturing output

Any path written under `/output/` inside the container is captured to the file store automatically at the end of the run, keyed as `output/<exec_id>/<relative-path>`.

<Tabs groupId="sdk-language">
<TabItem value="python" label="Python" default>

```python
# Container writes /output/result.json
result = client.run(
    image="python:3.12-slim",
    cmd=["python3", "-c", """
import json, os
os.makedirs('/output', exist_ok=True)
with open('/output/result.json', 'w') as f:
    json.dump({'answer': 42}, f)
"""],
    persist=True,
)

data = client.download_file(f"output/{result.exec_id}/result.json")
print(data)  # b'{"answer": 42}'
```

</TabItem>
<TabItem value="typescript" label="TypeScript">

```typescript
// Container writes /output/result.json
const result = await client.run(
  "python:3.12-slim",
  ["python3", "-c", `
import json, os
os.makedirs('/output', exist_ok=True)
with open('/output/result.json', 'w') as f:
    json.dump({'answer': 42}, f)
`],
  { persist: true },
);

const data = await client.downloadFile(`output/${result.exec_id}/result.json`);
console.log(new TextDecoder().decode(data)); // {"answer": 42}
```

</TabItem>
</Tabs>

The `/output/` directory itself exists inside the container — you do not need to create it, though `os.makedirs('/output', exist_ok=True)` is harmless.

## Persistence

By default, **both input files and output files are deleted** once the response is returned. Set `persist=True` to keep them:

<Tabs groupId="sdk-language">
<TabItem value="python" label="Python" default>

```python
result = client.run(
    image="python:3.12-slim",
    cmd=["python3", "/workspace/script.py"],
    files=["workspace/script.py"],
    persist=True,
)

# Files are still available after the run:
out = client.download_file(f"output/{result.exec_id}/result.json")
inp = client.download_file("workspace/script.py")
```

</TabItem>
<TabItem value="typescript" label="TypeScript">

```typescript
const result = await client.run(
  "python:3.12-slim",
  ["python3", "/workspace/script.py"],
  { files: ["workspace/script.py"], persist: true },
);

// Files are still available after the run:
const out = await client.downloadFile(`output/${result.exec_id}/result.json`);
const inp = await client.downloadFile("workspace/script.py");
```

</TabItem>
</Tabs>

Use `persist=True` when:
- You want to download output files separately after inspecting `stdout`/`stderr`
- You are running a batch of tasks and want to collect all outputs at the end

## Sharing files across runs

Because files are stored by path, you can upload once and reuse across multiple runs:

<Tabs groupId="sdk-language">
<TabItem value="python" label="Python" default>

```python
with open("model_weights.bin", "rb") as f:
    client.upload_file("shared/weights.bin", f)
with open("run.py", "rb") as f:
    client.upload_file("run.py", f)

for prompt in prompts:
    result = client.run(
        image="myimage:latest",
        cmd=["python3", "/run.py", prompt],
        files=["shared/weights.bin", "run.py"],
        persist=True,  # keep weights for next iteration
    )
```

</TabItem>
<TabItem value="typescript" label="TypeScript">

```typescript
import { readFile } from "node:fs/promises";

const weights = await readFile("model_weights.bin");
await client.uploadFile("shared/weights.bin", weights);
const runScript = await readFile("run.py");
await client.uploadFile("run.py", runScript);

for (const prompt of prompts) {
  const result = await client.run(
    "myimage:latest",
    ["python3", "/run.py", prompt],
    { files: ["shared/weights.bin", "run.py"], persist: true },
  );
}
```

</TabItem>
</Tabs>

## Full example

See the [upload-and-run example](../examples/upload-and-run) for a complete walkthrough: uploading a Python project, running its test suite inside the sandbox, and reading the results.
