---
sidebar_position: 3
---

# HumanEval Benchmark

Evaluates OpenAI's `o3-mini` model on the [HumanEval](https://github.com/openai/human-eval) benchmark (164 code-generation problems). Generated code is executed inside the Boxer sandbox, keeping untrusted LLM output completely isolated. Reports a pass@1 score.

Source: [`examples/humaneval/`](https://github.com/theonekeyg/boxer/tree/main/examples/humaneval)

## Prerequisites

- [uv](https://docs.astral.sh/uv/) installed
- Boxer server running locally
- `OPENAI_API_KEY` set in your environment (or via `.env`)

## Setup

```bash
# Start boxer (from the repo root)
cd packages/core && go run . --config config.dev.json

# In a separate terminal, set up credentials and install dependencies
cd examples/humaneval
cp .env.example .env        # then fill in your OPENAI_API_KEY
uv sync
```

## Usage

### Quick Smoke Test (3 problems)

```bash
OPENAI_API_KEY=sk-... uv run python evaluate.py --max-problems 3
```

Expected output:

```text
Loading HumanEval dataset…
Evaluating 3 problem(s) with model=o3-mini, workers=8

[  1/  3] HumanEval/0          PASS  412ms
[  2/  3] HumanEval/1          PASS  389ms
[  3/  3] HumanEval/2          PASS  501ms

────────────────────────────────────────
pass@1:  3/3  (100.0%)
```

### Full Evaluation

```bash
OPENAI_API_KEY=sk-... uv run python evaluate.py
```

Artifacts are written to `examples/humaneval/results/` (wiped and recreated on each run):

```text
results/
├── summary.json          ← aggregate: pass@1, total, passed, run metadata
└── problems/
    ├── HumanEval_0/
    │   ├── code.py       ← full test harness (prompt + completion + tests + check)
    │   ├── completion.txt ← raw LLM output
    │   ├── stdout.txt    ← captured stdout from sandbox
    │   ├── stderr.txt    ← captured stderr from sandbox
    │   └── result.json   ← {task_id, passed, exit_code, wall_ms, error?}
    └── ...
```

## Options

| Flag | Default | Description |
|---|---|---|
| `--boxer-url` | `http://localhost:8080` | Boxer server base URL |
| `--model` | `o3-mini` | OpenAI model ID |
| `--max-problems` | *(all 164)* | Limit number of problems |
| `--workers` | `8` | Concurrent async tasks |

## How It Works

1. Loads the `openai_humaneval` dataset from HuggingFace
2. For each problem, calls `o3-mini` to complete the function body
3. Assembles the test harness: `prompt + completion + tests + check(entry_point)`
4. Uploads the `.py` file to Boxer and executes it in a `python:3.12-slim` container
5. A zero exit code means the tests passed (pass@1)
