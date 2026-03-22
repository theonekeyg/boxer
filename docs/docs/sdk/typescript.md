---
sidebar_position: 2
---

# TypeScript SDK

TypeScript client SDK for Boxer — a sandboxed container execution service backed by gVisor. Zero dependencies, works in Node.js, Bun, Deno, and browsers.

## Installation

```bash
npm install boxer-sdk
# or
pnpm add boxer-sdk
```

Requires Node.js 18+ (or any runtime with native `fetch`) and a running Boxer server.

## Quick Start

```typescript
import { BoxerClient } from "boxer-sdk";

const client = new BoxerClient({ baseUrl: "http://localhost:8080" });

const result = await client.run(
  "python:3.12-slim",
  ["python3", "-c", "print('hello world')"],
);
console.log(result.stdout);    // hello world
console.log(result.stderr);    // (empty)
console.log(result.exit_code); // 0
console.log(result.wall_ms);   // e.g. 312
```

## Multi-language Examples

```typescript
// Python
const result = await client.run(
  "python:3.12-slim",
  ["python3", "-c", "print('hello world')"],
);

// Node.js
const result = await client.run(
  "node:20-slim",
  ["node", "-e", "console.log('hello world')"],
);

// Perl
const result = await client.run(
  "perl:5.38-slim",
  ["perl", "-e", "print 'hello world\n'"],
);
```

## Working with Files

### Upload a Script and Run It

Upload a file to the Boxer file store, then reference it by path in `run`. The file is bind-mounted read-only at `/<remotePath>` inside the container.

```typescript
import { readFile } from "node:fs/promises";

const script = await readFile("script.py");
await client.uploadFile("script.py", script);

const result = await client.run(
  "python:3.12-slim",
  ["python3", "/script.py"],
  { files: ["script.py"] },
);
```

### Upload a Local Directory

`uploadPath` (from `boxer-sdk/node`) recursively uploads a local file or directory, preserving the directory structure. Returns the list of remote paths, which can be passed directly to `files`.

```typescript
import { uploadPath } from "boxer-sdk/node";

// Upload a single file
const filePaths = await uploadPath(client, "./script.py");
// => ["script.py"]

// Upload an entire directory
const dirPaths = await uploadPath(client, "./myproject", "myproject");
// => ["myproject/main.py", "myproject/utils.py", ...]

const result = await client.run(
  "python:3.12-slim",
  ["python3", "/myproject/main.py"],
  { files: dirPaths },
);
```

### Download Output Files

Any file the container writes to `/output/` is captured at the end of the run. Set `persist: true` to retain them for post-run download — without it, output files are deleted once the response is returned.

```typescript
const code = `
import os, json
os.makedirs('/output', exist_ok=True)
with open('/output/result.json', 'w') as f:
    json.dump({'message': 'hello world', 'value': 42}, f)
`;

const script = new TextEncoder().encode(code);
await client.uploadFile("compute.py", script);

const result = await client.run(
  "python:3.12-slim",
  ["python3", "/compute.py"],
  { files: ["compute.py"], persist: true },
);

const data = await client.downloadFile(`output/${result.exec_id}/result.json`);
console.log(new TextDecoder().decode(data));
// {"message": "hello world", "value": 42}
```

The output path pattern is `output/<exec_id>/<relative_path>`, preserving any subdirectory structure written under `/output/`.

## Resource Limits

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

## Error Handling

```typescript
import { BoxerAPIError, BoxerTimeoutError, BoxerOutputLimitError } from "boxer-sdk";

try {
  const result = await client.run(
    "python:3.12-slim",
    ["python3", "-c", "while True: pass"],
    { limits: { wall_clock_secs: 5 } },
  );
} catch (err) {
  if (err instanceof BoxerTimeoutError) {
    console.error("execution timed out");
  } else if (err instanceof BoxerOutputLimitError) {
    console.error("output exceeded size limit");
  } else if (err instanceof BoxerAPIError) {
    console.error(`API error ${err.statusCode}: ${err.message}`);
  }
}
```

## Runtime Compatibility

The core `boxer-sdk` package uses only web-standard APIs (`fetch`, `FormData`, `Blob`) with zero dependencies:

| Runtime | Core package | `boxer-sdk/node` |
|---|---|---|
| Node.js 18+ | ✓ | ✓ |
| Bun | ✓ | ✓ |
| Deno | ✓ | ✓ |
| Browsers | ✓ | ✗ |

`boxer-sdk/node` (`uploadPath`) requires a Node.js-compatible `fs` module.

## Running Tests

Tests require a live Boxer server:

```bash
pnpm install
BOXER_URL=http://localhost:8080 pnpm test
```

Without `BOXER_URL` all tests are skipped automatically.
