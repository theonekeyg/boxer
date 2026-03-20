"""Upload a local Python project to Boxer and run its pytest suite inside the sandbox."""

import sys
from pathlib import Path

from boxer import BoxerClient

PROJECT_DIR = Path(__file__).parent / "project"


with BoxerClient() as client:
    paths = client.upload_path(PROJECT_DIR, remote_path="project")
    print(f"Uploaded {len(paths)} file(s)")

    result = client.run(
        image="python:3.12-slim",
        cmd=[
            "bash", "-c",
            "pip install pytest -q && pytest /project/ -v",
        ],
        files=paths,
    )

    print(result.stdout)
    if result.stderr:
        print(result.stderr, file=sys.stderr)

    sys.exit(result.exit_code)
