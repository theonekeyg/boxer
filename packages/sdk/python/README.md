# boxer-sdk

Python client SDK for [Boxer](../../packages/core) - a sandboxed container execution service backed by gVisor.

## Installation

```bash
pip install boxer-sdk
```

Requires Python 3.9+ and a running Boxer server.

## Quick start

```python
from boxer import BoxerClient

with BoxerClient("http://localhost:8080") as client:
    result = client.run(
        image="python:3.12-slim",
        cmd=["python3", "-c", "print('hello world')"],
    )
    print(result.stdout)   # hello world
    print(result.exit_code)  # 0
    print(result.wall_ms)    # e.g. 312
```

---

## Hello world in Python, Node.js, and Perl

### Python

```python
result = client.run(
    image="python:3.12-slim",
    cmd=["python3", "-c", "print('hello world')"],
)
print(result.stdout)  # hello world
```

Or use the `run_script` shortcut for inline Python:

```python
result = client.run_script("print('hello world')")
print(result.stdout)  # hello world
```

### Node.js

```python
result = client.run(
    image="node:20-slim",
    cmd=["node", "-e", "console.log('hello world')"],
)
print(result.stdout)  # hello world
```

### Perl

```python
result = client.run(
    image="perl:5.38-slim",
    cmd=["perl", "-e", "print 'hello world\n'"],
)
print(result.stdout)  # hello world
```

---

## Working with files

### Upload a script and run it

Upload a file to the Boxer file store, then reference it by path in `run`.
The file is bind-mounted read-only at `/<remote_path>` inside the container.

```python
with open("script.py", "rb") as f:
    client.upload_file("script.py", f)

result = client.run(
    image="python:3.12-slim",
    cmd=["python3", "/script.py"],
    files=["script.py"],
)
print(result.stdout)
```

The `run_file` helper combines upload + run in one call:

```python
result = client.run_file(
    local_path="script.py",
    image="python:3.12-slim",
)
print(result.stdout)
```

It also works for Node.js and Perl scripts by setting `cmd_prefix`:

```python
# Node.js
result = client.run_file(
    local_path="app.js",
    image="node:20-slim",
    cmd_prefix=["node"],
)

# Perl
result = client.run_file(
    local_path="hello.pl",
    image="perl:5.38-slim",
    cmd_prefix=["perl"],
)
```

### Download output files from the container

Any file the container writes to `/output/` is automatically captured and
retrievable via `download_file` after the run completes.

```python
# Script that writes a file to /output/
code = """
import os, json
os.makedirs('/output', exist_ok=True)
with open('/output/result.json', 'w') as f:
    json.dump({'message': 'hello world', 'value': 42}, f)
"""

with open("compute.py", "rb") as f:
    client.upload_file("compute.py", f)

result = client.run(
    image="python:3.12-slim",
    cmd=["python3", "/compute.py"],
    files=["compute.py"],
)

# Download the file the container wrote
data = client.download_file(f"output/{result.exec_id}/result.json")
print(data)  # b'{"message": "hello world", "value": 42}'
```

The output path pattern is always `output/<exec_id>/<filename>`.

---

## Resource limits

```python
from boxer import ResourceLimits

limits = ResourceLimits(
    cpu_cores=0.5,
    memory_mb=128,
    wall_clock_secs=10,
)

result = client.run(
    image="python:3.12-slim",
    cmd=["python3", "-c", "print('done')"],
    limits=limits,
)
```

---

## Async client

Every method on `BoxerClient` has an `await`-able equivalent on `AsyncBoxerClient`:

```python
import asyncio
from boxer import AsyncBoxerClient

async def main():
    async with AsyncBoxerClient("http://localhost:8080") as client:
        result = await client.run_script("print('hello world')")
        print(result.stdout)

asyncio.run(main())
```

---

## Error handling

```python
from boxer import BoxerAPIError, BoxerTimeoutError, BoxerOutputLimitError

try:
    result = client.run(
        image="python:3.12-slim",
        cmd=["python3", "-c", "while True: pass"],
        limits=ResourceLimits(wall_clock_secs=5),
    )
except BoxerTimeoutError:
    print("execution timed out")
except BoxerOutputLimitError:
    print("output exceeded size limit")
except BoxerAPIError as e:
    print(f"API error {e.status_code}: {e}")
```

---

## Running tests

Tests require a live Boxer server:

```bash
pip install -e ".[dev]"
BOXER_URL=http://localhost:8080 pytest tests/ -v
```

Without `BOXER_URL` all tests are skipped automatically.
