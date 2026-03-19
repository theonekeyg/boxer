# HumanEval Evaluation with Boxer

Evaluates OpenAI's `o3-mini` model on the [HumanEval](https://github.com/openai/human-eval)
benchmark (164 code-generation problems). Generated code is executed inside the boxer sandbox,
keeping untrusted LLM output completely isolated. Reports a pass@1 score.

## Prerequisites

- [uv](https://docs.astral.sh/uv/) installed
- Boxer server running locally
- `OPENAI_API_KEY` set in your environment

## Setup

```bash
# Start boxer (from the repo root)
cd packages/core && go run . --config config.dev.json

# In a separate terminal, install dependencies
cd examples/humaneval
uv sync
```

## Usage

### Quick smoke test (3 problems)

```bash
OPENAI_API_KEY=sk-... uv run python evaluate.py --max-problems 3
```

Expected output:

```
Loading HumanEval dataset…
Evaluating 3 problem(s) with model=o3-mini, workers=8

[  1/  3] HumanEval/0          PASS  412ms
[  2/  3] HumanEval/1          PASS  389ms
[  3/  3] HumanEval/2          PASS  501ms

────────────────────────────────────────
pass@1:  3/3  (100.0%)
```

### Full evaluation

```bash
OPENAI_API_KEY=sk-... uv run python evaluate.py --output results.json
```

`results.json` contains the pass@1 score and per-problem details (exit code, stdout/stderr, wall time).

## Options

| Flag | Default | Description |
|---|---|---|
| `--boxer-url` | `http://localhost:8080` | Boxer server base URL |
| `--model` | `o3-mini` | OpenAI model ID |
| `--max-problems` | *(all 164)* | Limit number of problems |
| `--workers` | `8` | Concurrent async tasks |
| `--output` | *(stdout only)* | Path to save JSON results |

## How it works

1. Loads the `openai_humaneval` dataset from HuggingFace
2. For each problem, calls `o3-mini` to complete the function body
3. Assembles the test harness: `prompt + completion + tests + check(entry_point)`
4. Uploads the `.py` file to boxer and executes it in a `python:3.12-slim` container
5. A zero exit code means the tests passed (pass@1)
