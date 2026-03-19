"""HumanEval evaluation using boxer sandbox for safe code execution."""

import argparse
import asyncio
import json
import shutil
import textwrap
import time
from datetime import datetime, timezone
from pathlib import Path

import black
import httpx
import litellm
from datasets import load_dataset
from dotenv import load_dotenv

load_dotenv()


SYSTEM_PROMPT = (
    "Complete the Python function. Output only the function body "
    "(the code that follows the signature), no markdown fences, no extra text."
)


async def generate_completion(model: str, prompt: str) -> str:
    response = await litellm.acompletion(
        model=model,
        messages=[
            {"role": "system", "content": SYSTEM_PROMPT},
            {"role": "user", "content": prompt},
        ],
    )
    return response.choices[0].message.content or ""


async def run_in_boxer(
    http: httpx.AsyncClient,
    boxer_url: str,
    task_id: str,
    code: str,
) -> dict:
    filename = f"run/{task_id}.py"
    code_bytes = code.encode()

    upload_resp = await http.post(
        f"{boxer_url}/files",
        files={"file": (filename, code_bytes, "text/x-python")},
        data={"path": filename},
    )
    upload_resp.raise_for_status()

    run_resp = await http.post(
        f"{boxer_url}/run",
        json={
            "image": "python:3.12-slim",
            "cmd": ["python3", f"/{filename}"],
            "files": [filename],
            "limits": {"wall_clock_secs": 30, "memory_mb": 256},
        },
    )
    run_resp.raise_for_status()
    return run_resp.json()


def _write_problem_dir(problem_dir: Path, *, completion: str, code: str, stdout: str, stderr: str, result_data: dict) -> None:
    problem_dir.mkdir(parents=True, exist_ok=True)
    (problem_dir / "completion.txt").write_text(completion)
    (problem_dir / "code.py").write_text(code)
    (problem_dir / "stdout.txt").write_text(stdout)
    (problem_dir / "stderr.txt").write_text(stderr)
    (problem_dir / "result.json").write_text(json.dumps(result_data, indent=2))


async def evaluate_problem(
    sem: asyncio.Semaphore,
    http: httpx.AsyncClient,
    boxer_url: str,
    model: str,
    problem: dict,
    # counter is a single-element list rather than a plain int because Python
    # integers are immutable — rebinding a local variable wouldn't be visible
    # to the caller. A list is a mutable object shared by reference, so
    # counter[0] += 1 is visible across all coroutines.
    counter: list,
    counter_lock: asyncio.Lock,
    total: int,
    output_dir: Path,
) -> dict:
    """Evaluate a single HumanEval problem end-to-end:
      1. Call the LLM to generate a function body completion.
      2. Assemble the full test harness (prompt + completion + tests + check call).
      3. Upload the file to the boxer server and run it inside a sandbox container.
      4. Write all artifacts (code, completion, stdout/stderr, result) to output_dir.
    Returns a result dict with task_id, passed, exit_code, wall_ms, stdout, stderr.
    """
    task_id = problem["task_id"].replace("/", "_")
    label = problem["task_id"]
    problem_dir = output_dir / "problems" / task_id

    # sem limits the number of problems evaluated concurrently.
    async with sem:
        t0 = time.monotonic()

        # Step 1: ask the LLM to complete the function body.
        try:
            completion = await generate_completion(model, problem["prompt"])
        except Exception as exc:
            elapsed_ms = int((time.monotonic() - t0) * 1000)
            async with counter_lock:
                counter[0] += 1
                n = counter[0]
            print(f"[{n:3d}/{total}] {label:<20} FAIL  (openai error: {exc})")
            result_data = {
                "task_id": label,
                "passed": False,
                "exit_code": None,
                "wall_ms": elapsed_ms,
                "error": f"openai: {exc}",
            }
            _write_problem_dir(problem_dir, completion="", code="", stdout="", stderr=str(exc), result_data=result_data)
            return {**result_data, "stdout": "", "stderr": str(exc)}

        # Step 2: assemble the test harness.
        # textwrap.dedent normalizes whatever indentation the model used (models
        # are inconsistent), then re-indents with exactly 4 spaces so the
        # completion sits correctly inside the function body. black then
        # formats the whole file uniformly; if it can't parse (rare), we fall
        # back to the textwrap-normalized version.
        indented = textwrap.indent(textwrap.dedent(completion).strip(), "    ")
        code = f"{problem['prompt']}{indented}\n\n{problem['test']}\n\ncheck({problem['entry_point']})\n"
        try:
            code = black.format_str(code, mode=black.Mode())
        except black.InvalidInput:
            pass

        # Step 3: upload to boxer and execute inside a sandboxed container.
        # boxer runs the code in an isolated python:3.12-slim container with
        # resource limits; a zero exit code means all assertions passed.
        try:
            result = await run_in_boxer(http, boxer_url, task_id, code)
        except httpx.HTTPStatusError as exc:
            elapsed_ms = int((time.monotonic() - t0) * 1000)
            note = "timeout" if exc.response.status_code == 408 else str(exc)
            async with counter_lock:
                counter[0] += 1
                n = counter[0]
            print(f"[{n:3d}/{total}] {label:<20} FAIL  (boxer error: {note})")
            result_data = {
                "task_id": label,
                "passed": False,
                "exit_code": None,
                "wall_ms": elapsed_ms,
                "error": f"boxer: {note}",
            }
            _write_problem_dir(problem_dir, completion=completion, code=code, stdout="", stderr=note, result_data=result_data)
            return {**result_data, "stdout": "", "stderr": note}
        except Exception as exc:
            elapsed_ms = int((time.monotonic() - t0) * 1000)
            async with counter_lock:
                counter[0] += 1
                n = counter[0]
            print(f"[{n:3d}/{total}] {label:<20} FAIL  (boxer error: {exc})")
            result_data = {
                "task_id": label,
                "passed": False,
                "exit_code": None,
                "wall_ms": elapsed_ms,
                "error": f"boxer: {exc}",
            }
            _write_problem_dir(problem_dir, completion=completion, code=code, stdout="", stderr=str(exc), result_data=result_data)
            return {**result_data, "stdout": "", "stderr": str(exc)}

        elapsed_ms = int((time.monotonic() - t0) * 1000)
        exit_code = result.get("exit_code", -1)
        passed = exit_code == 0
        stdout = result.get("stdout", "")
        stderr = result.get("stderr", "")

        # Step 4: log completion in arrival order (counter increments as each
        # task finishes, not in submission order).
        async with counter_lock:
            counter[0] += 1
            n = counter[0]
        status = "PASS" if passed else f"FAIL  exit={exit_code}"
        print(f"[{n:3d}/{total}] {label:<20} {status}  {elapsed_ms}ms")

        result_data = {
            "task_id": label,
            "passed": passed,
            "exit_code": exit_code,
            "wall_ms": elapsed_ms,
        }
        _write_problem_dir(problem_dir, completion=completion, code=code, stdout=stdout, stderr=stderr, result_data=result_data)

        return {**result_data, "stdout": stdout, "stderr": stderr}


async def main() -> None:
    parser = argparse.ArgumentParser(description="Evaluate o3-mini on HumanEval via boxer")
    parser.add_argument("--boxer-url", default="http://localhost:8080", help="Boxer server base URL")
    parser.add_argument("--model", default="o3-mini", help="OpenAI model ID")
    parser.add_argument("--max-problems", type=int, default=None, help="Limit number of problems")
    parser.add_argument("--workers", type=int, default=8, help="Concurrent async tasks")
    args = parser.parse_args()

    output_dir = Path(__file__).parent / "results"
    shutil.rmtree(output_dir, ignore_errors=True)
    (output_dir / "problems").mkdir(parents=True)

    print("Loading HumanEval dataset…")
    dataset = load_dataset("openai_humaneval", split="test")
    problems = list(dataset)
    if args.max_problems is not None:
        problems = problems[: args.max_problems]

    total = len(problems)
    print(f"Evaluating {total} problem(s) with model={args.model}, workers={args.workers}\n")

    sem = asyncio.Semaphore(args.workers)
    counter = [0]
    counter_lock = asyncio.Lock()

    async with httpx.AsyncClient(timeout=120.0) as http:
        tasks = [
            evaluate_problem(
                sem, http, args.boxer_url, args.model,
                problem, counter, counter_lock, total, output_dir,
            )
            for problem in problems
        ]
        results = await asyncio.gather(*tasks)

    passed_count = sum(1 for r in results if r["passed"])
    pct = passed_count / total * 100 if total else 0.0

    print()
    print("─" * 40)
    print(f"pass@1:  {passed_count}/{total}  ({pct:.1f}%)")

    summary = {
        "model": args.model,
        "pass_at_1": pct / 100,
        "passed": passed_count,
        "total": total,
        "run_at": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
    }
    (output_dir / "summary.json").write_text(json.dumps(summary, indent=2))
    print(f"Results saved to {output_dir}/")


if __name__ == "__main__":
    asyncio.run(main())
