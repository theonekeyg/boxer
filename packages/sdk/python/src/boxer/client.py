from __future__ import annotations

from pathlib import Path
from typing import IO

import httpx

from .exceptions import BoxerAPIError, BoxerOutputLimitError, BoxerTimeoutError
from .types import ResourceLimits, RunResult


def _raise_for_status(response: httpx.Response) -> None:
    if response.is_success:
        return
    try:
        detail = response.json().get("error", response.text)
    except Exception:
        detail = response.text
    code = response.status_code
    if code == 408:
        raise BoxerTimeoutError(detail, code)
    if code == 507:
        raise BoxerOutputLimitError(detail, code)
    raise BoxerAPIError(detail, code)


def _build_run_body(
    image: str,
    cmd: list[str],
    env: list[str],
    cwd: str,
    limits: ResourceLimits | None,
    files: list[str],
    persist: bool,
) -> dict:
    body: dict = {"image": image, "cmd": cmd}
    if env:
        body["env"] = env
    if cwd and cwd != "/":
        body["cwd"] = cwd
    if limits is not None:
        body["limits"] = limits.to_dict()
    if files:
        body["files"] = files
    if persist:
        body["persist"] = persist
    return body


def _parse_run_result(data: dict) -> RunResult:
    return RunResult(
        exec_id=data["exec_id"],
        exit_code=data["exit_code"],
        stdout=data["stdout"],
        stderr=data["stderr"],
        wall_ms=data["wall_ms"],
    )


class BoxerClient:
    """Synchronous Boxer API client backed by httpx."""

    def __init__(
        self,
        base_url: str = "http://localhost:8080",
        timeout: float = 120.0,
    ) -> None:
        self._client = httpx.Client(
            base_url=base_url,
            timeout=timeout,
        )

    def __enter__(self) -> BoxerClient:
        self._client.__enter__()
        return self

    def __exit__(self, *args: object) -> None:
        self._client.__exit__(*args)

    def close(self) -> None:
        self._client.close()

    # ------------------------------------------------------------------
    # Core API
    # ------------------------------------------------------------------

    def health(self) -> bool:
        """Return True if the server is healthy."""
        response = self._client.get("/healthz")
        return response.is_success

    def run(
        self,
        image: str,
        cmd: list[str],
        *,
        env: list[str] | None = None,
        cwd: str = "/",
        limits: ResourceLimits | None = None,
        files: list[str] | None = None,
        persist: bool = False,
    ) -> RunResult:
        """Execute a command inside a sandboxed container."""
        body = _build_run_body(
            image=image,
            cmd=cmd,
            env=env or [],
            cwd=cwd,
            limits=limits,
            files=files or [],
            persist=persist,
        )
        response = self._client.post("/run", json=body)
        _raise_for_status(response)
        return _parse_run_result(response.json())

    def upload_file(
        self,
        remote_path: str,
        content: bytes | IO[bytes],
    ) -> None:
        """Upload a file to the Boxer file store."""
        if isinstance(content, bytes):
            file_obj: bytes | IO[bytes] = content
        else:
            file_obj = content
        response = self._client.post(
            "/files",
            data={"path": remote_path},
            files={"file": file_obj},
        )
        _raise_for_status(response)

    def upload_path(
        self,
        local_path: str | Path,
        remote_path: str | None = None,
    ) -> list[str]:
        """Upload a local file or directory to the Boxer file store.

        If *local_path* is a directory, all files inside it are uploaded
        recursively, preserving the directory structure under *remote_path*
        (defaults to the directory name).

        Returns the list of remote paths that were uploaded.
        """
        local = Path(local_path)
        if local.is_dir():
            prefix = remote_path if remote_path is not None else local.name
            uploaded = []
            for file in sorted(local.rglob("*")):
                if not file.is_file():
                    continue
                rel = file.relative_to(local)
                dest = f"{prefix}/{rel}"
                with open(file, "rb") as fh:
                    self.upload_file(dest, fh)
                uploaded.append(dest)
            return uploaded
        else:
            dest = remote_path if remote_path is not None else local.name
            with open(local, "rb") as fh:
                self.upload_file(dest, fh)
            return [dest]

    def download_file(self, path: str) -> bytes:
        """Download a file from the Boxer file store."""
        response = self._client.get("/files", params={"path": path})
        _raise_for_status(response)
        return response.content
