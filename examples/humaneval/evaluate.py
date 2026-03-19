"""HumanEval evaluation using boxer sandbox for safe code execution."""

import argparse
import asyncio
import json
import sys
import time

import httpx
from datasets import load_dataset
from dotenv import load_dotenv
from openai import AsyncOpenAI

load_dotenv()


SYSTEM_PROMPT = (
    "Complete the Python function. Output only the function body "
    "(the code that follows the signature), no markdown fences, no extra text."
)


async def generate_completion(client: AsyncOpenAI, model: str, prompt: str) -> str:
    response = await client.chat.completions.create(
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


async def evaluate_problem(
    sem: asyncio.Semaphore,
    openai_client: AsyncOpenAI,
    http: httpx.AsyncClient,
    boxer_url: str,
    model: str,
    problem: dict,
    index: int,
    total: int,
) -> dict:
    task_id = problem["task_id"].replace("/", "_")
    label = problem["task_id"]

    async with sem:
        t0 = time.monotonic()

        # Generate completion
        try:
            completion = await generate_completion(
                openai_client, model, problem["prompt"]
            )
        except Exception as exc:
            elapsed_ms = int((time.monotonic() - t0) * 1000)
            print(f"[{index:3d}/{total}] {label:<20} FAIL  (openai error: {exc})")
            return {
                "task_id": label,
                "passed": False,
                "exit_code": None,
                "stdout": "",
                "stderr": str(exc),
                "wall_ms": elapsed_ms,
                "error": f"openai: {exc}",
            }

        # Build test harness
        code = f"{problem['prompt']}{completion}\n\n{problem['test']}\n\ncheck({problem['entry_point']})\n"

        # Execute in boxer
        try:
            result = await run_in_boxer(http, boxer_url, task_id, code)
        except httpx.HTTPStatusError as exc:
            elapsed_ms = int((time.monotonic() - t0) * 1000)
            note = "timeout" if exc.response.status_code == 408 else str(exc)
            print(f"[{index:3d}/{total}] {label:<20} FAIL  (boxer error: {note})")
            return {
                "task_id": label,
                "passed": False,
                "exit_code": None,
                "stdout": "",
                "stderr": note,
                "wall_ms": elapsed_ms,
                "error": f"boxer: {note}",
            }
        except Exception as exc:
            elapsed_ms = int((time.monotonic() - t0) * 1000)
            print(f"[{index:3d}/{total}] {label:<20} FAIL  (boxer error: {exc})")
            return {
                "task_id": label,
                "passed": False,
                "exit_code": None,
                "stdout": "",
                "stderr": str(exc),
                "wall_ms": elapsed_ms,
                "error": f"boxer: {exc}",
            }

        elapsed_ms = int((time.monotonic() - t0) * 1000)
        exit_code = result.get("exit_code", -1)
        passed = exit_code == 0

        status = "PASS" if passed else f"FAIL  exit={exit_code}"
        print(f"[{index:3d}/{total}] {label:<20} {status}  {elapsed_ms}ms")

        return {
            "task_id": label,
            "passed": passed,
            "exit_code": exit_code,
            "stdout": result.get("stdout", ""),
            "stderr": result.get("stderr", ""),
            "wall_ms": elapsed_ms,
        }


async def main() -> None:
    parser = argparse.ArgumentParser(description="Evaluate o3-mini on HumanEval via boxer")
    parser.add_argument("--boxer-url", default="http://localhost:8080", help="Boxer server base URL")
    parser.add_argument("--model", default="o3-mini", help="OpenAI model ID")
    parser.add_argument("--max-problems", type=int, default=None, help="Limit number of problems")
    parser.add_argument("--workers", type=int, default=8, help="Concurrent async tasks")
    parser.add_argument("--output", default=None, help="Optional path to save JSON results")
    args = parser.parse_args()

    print("Loading HumanEval dataset…")
    dataset = load_dataset("openai_humaneval", split="test")
    problems = list(dataset)
    if args.max_problems is not None:
        problems = problems[: args.max_problems]

    total = len(problems)
    print(f"Evaluating {total} problem(s) with model={args.model}, workers={args.workers}\n")

    openai_client = AsyncOpenAI()
    sem = asyncio.Semaphore(args.workers)

    async with httpx.AsyncClient(timeout=120.0) as http:
        tasks = [
            evaluate_problem(
                sem, openai_client, http, args.boxer_url, args.model,
                problem, i + 1, total,
            )
            for i, problem in enumerate(problems)
        ]
        results = await asyncio.gather(*tasks)

    passed_count = sum(1 for r in results if r["passed"])
    pct = passed_count / total * 100 if total else 0.0

    print()
    print("─" * 40)
    print(f"pass@1:  {passed_count}/{total}  ({pct:.1f}%)")

    if args.output:
        with open(args.output, "w") as f:
            json.dump(
                {"pass_at_1": pct / 100, "passed": passed_count, "total": total, "results": results},
                f,
                indent=2,
            )
        print(f"Results saved to {args.output}")


if __name__ == "__main__":
    asyncio.run(main())
