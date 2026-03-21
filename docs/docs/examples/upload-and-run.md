---
sidebar_position: 2
---

# Upload and Run

Uploads a local Python project to Boxer and runs its pytest suite inside a sandboxed container.

Source: [`examples/upload-and-run/`](https://github.com/theonekeyg/boxer/tree/main/examples/upload-and-run)

## Project Layout

```
project/
  foo/
    __init__.py
    calculator.py     # simple arithmetic implementation
  tests/
    test_calculator.py
```

## Usage

Start a Boxer server, then:

```bash
uv run run_tests.py
```

The script uploads the entire `project/` directory, installs pytest inside the container, and runs the test suite. Exit code mirrors pytest's own exit code.

## What It Demonstrates

- Uploading multiple files before execution
- Bind-mounting uploaded files into the container at known paths
- Running a test suite (`pytest`) inside a sandboxed container
- Capturing the exit code to report pass/fail
