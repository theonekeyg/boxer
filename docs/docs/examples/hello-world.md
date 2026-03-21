---
sidebar_position: 1
---

# Hello World

Minimal example of running a sandboxed Python script via the Boxer SDK.

Source: [`examples/hello-world/`](https://github.com/theonekeyg/boxer/tree/main/examples/hello-world)

## Usage

Start a Boxer server, then:

```bash
uv run hello.py
```

By default the client connects to `http://localhost:8080`.

## What It Does

This example demonstrates the most basic Boxer usage: run a one-liner inside a `python:3.12-slim` container and print the result. It's the best starting point for understanding the client flow before moving to more complex examples.
