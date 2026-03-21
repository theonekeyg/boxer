# boxer-sdk

TypeScript client SDK for [Boxer](https://github.com/theonekeyg/boxer) - a sandboxed container execution service backed by gVisor.

## Installation

```bash
npm install boxer-sdk
```

Requires Node.js 18+ (or any runtime with native `fetch`) and a running Boxer server.

## Quick start

```typescript
import { BoxerClient } from "boxer-sdk";

const client = new BoxerClient({ baseUrl: "http://localhost:8080" });

const result = await client.run(
  "python:3.12-slim",
  ["python3", "-c", "print('hello world')"],
);
console.log(result.stdout);    // hello world
console.log(result.exit_code);  // 0
console.log(result.wall_ms);   // e.g. 312
```

---

## Hello world in Python, Node.js, and Perl

### Python

```typescript
const result = await client.run(
  "python:3.12-slim",
  ["python3", "-c", "print('hello world')"],
);
console.log(result.stdout); // hello world
```

### Node.js

```typescript
const result = await client.run(
  "node:20-slim",
  ["node", "-e", "console.log('hello world')"],
);
console.log(result.stdout); // hello world
```

### Perl

```typescript
const result = await client.run(
  "perl:5.38-slim",
  ["perl", "-e", "print 'hello world\n'"],
);
console.log(result.stdout); // hello world
```

---

## Working with files

### Upload a script and run it

Upload a file to the Boxer file store, then reference it by path in `run`.
The file is bind-mounted read-only at `/<remotePath>` inside the container.

```typescript
import { readFile } from "node:fs/promises";

// Python
const pyScript = await readFile("script.py");
await client.uploadFile("script.py", pyScript);

const pyResult = await client.run(
  "python:3.12-slim",
  ["python3", "/script.py"],
  { files: ["script.py"] },
);

// Node.js
const jsScript = await readFile("app.js");
await client.uploadFile("app.js", jsScript);

const jsResult = await client.run(
  "node:20-slim",
  ["node", "/app.js"],
  { files: ["app.js"] },
);

// Perl
const plScript = await readFile("hello.pl");
await client.uploadFile("hello.pl", plScript);

const plResult = await client.run(
  "perl:5.38-slim",
  ["perl", "/hello.pl"],
  { files: ["hello.pl"] },
);
```

### Upload a local file or directory

`uploadPath` (available from `boxer-sdk/node`) recursively uploads a local file
or directory, preserving the directory structure.

```typescript
import { uploadPath } from "boxer-sdk/node";

// Upload a single file
const paths = await uploadPath(client, "./script.py");
// => ["script.py"]

// Upload an entire directory
const paths = await uploadPath(client, "./myproject", "myproject");
// => ["myproject/main.py", "myproject/utils.py", ...]

const result = await client.run(
  "python:3.12-slim",
  ["python3", "/myproject/main.py"],
  { files: paths },
);
```

### Download output files from the container

Any file the container writes to `/output/` is automatically captured and
retrievable via `downloadFile` after the run completes.

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

// Download the file the container wrote
const data = await client.downloadFile(`output/${result.exec_id}/result.json`);
console.log(new TextDecoder().decode(data)); // {"message": "hello world", "value": 42}
```

The output path pattern is always `output/<exec_id>/<filename>`.

---

## Resource limits

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

---

## Error handling

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

---

## Runtime compatibility

The core `boxer-sdk` package uses only web-standard APIs (`fetch`, `FormData`,
`Blob`) and has zero dependencies, making it compatible with:

- Node.js 18+
- Bun
- Deno
- Browsers

The `boxer-sdk/node` subpath export (`uploadPath`) requires a Node.js-compatible
`fs` module and is available in Node, Bun, and Deno.

---

## Running tests

Tests require a live Boxer server:

```bash
pnpm install
BOXER_URL=http://localhost:8080 pnpm test
```

Without `BOXER_URL` all tests are skipped automatically.
