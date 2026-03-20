# upload-and-run

Uploads a local Python project to Boxer and runs its pytest suite inside a sandboxed container.

## Project layout

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
