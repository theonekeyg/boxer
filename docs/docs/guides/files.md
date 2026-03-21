---
sidebar_position: 1
---

# Files

Boxer has a server-side file store for passing data in and out of containers. The flow is:

1. **Upload** input files before the run (`POST /files`)
2. **Reference** them by path in the run request — Boxer bind-mounts each one read-only at `/<path>` inside the container
3. **Write** output to `/output/` inside the container
4. **Download** captured output files after the run (`GET /files?path=output/<exec_id>/...`)

## Uploading input files

```python
with open("script.py", "rb") as f:
    client.upload_file("workspace/script.py", f)
```

The path you provide (`workspace/script.py`) is the key in the store **and** the mount destination inside the container (`/workspace/script.py`).

You can upload a whole directory by iterating over it:

```python
import pathlib

root = pathlib.Path("project/")
for p in root.rglob("*"):
    if p.is_file():
        rel = str(p)
        with open(p, "rb") as f:
            client.upload_file(rel, f)
```

## Referencing files in a run

Pass the paths you uploaded in the `files` array. Boxer checks that all listed files exist before spawning the container — the request fails with 400 if any are missing.

```python
result = client.run(
    image="python:3.12-slim",
    cmd=["python3", "/workspace/script.py"],
    files=["workspace/script.py"],
)
```

Each file is mounted read-only. The container cannot modify uploaded files — if the workload needs to write, have it write to `/output/` instead.

## Capturing output

Any path written under `/output/` inside the container is captured to the file store automatically at the end of the run, keyed as `output/<exec_id>/<relative-path>`.

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
)

data = client.download_file(f"output/{result.exec_id}/result.json")
print(data)  # b'{"answer": 42}'
```

The `/output/` directory itself exists inside the container — you do not need to create it, though `os.makedirs('/output', exist_ok=True)` is harmless.

## Persistence

By default, **both input files and output files are deleted** once the response is returned. Set `persist=True` to keep them:

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

Use `persist=True` when:
- You want to download output files separately after inspecting `stdout`/`stderr`
- You are running a batch of tasks and want to collect all outputs at the end

## Sharing files across runs

Because files are stored by path, you can upload once and reuse across multiple runs:

```python
with open("model_weights.bin", "rb") as f:
    client.upload_file("shared/weights.bin", f)

for prompt in prompts:
    result = client.run(
        image="myimage:latest",
        cmd=["python3", "/run.py", prompt],
        files=["shared/weights.bin", "run.py"],
        persist=True,  # keep weights for next iteration
    )
```

## Full example

See the [upload-and-run example](../examples/upload-and-run) for a complete walkthrough: uploading a Python project, running its test suite inside the sandbox, and reading the results.
