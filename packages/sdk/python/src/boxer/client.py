from __future__ import annotations

from typing import IO, List, Optional, Union

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
    cmd: List[str],
    env: List[str],
    cwd: str,
    limits: Optional[ResourceLimits],
    files: List[str],
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

    def __enter__(self) -> "BoxerClient":
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
        cmd: List[str],
        *,
        env: Optional[List[str]] = None,
        cwd: str = "/",
        limits: Optional[ResourceLimits] = None,
        files: Optional[List[str]] = None,
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
        content: Union[bytes, IO[bytes]],
    ) -> None:
        """Upload a file to the Boxer file store."""
        if isinstance(content, bytes):
            file_obj: Union[bytes, IO[bytes]] = content
        else:
            file_obj = content
        response = self._client.post(
            "/files",
            data={"path": remote_path},
            files={"file": file_obj},
        )
        _raise_for_status(response)

    def download_file(self, path: str) -> bytes:
        """Download a file from the Boxer file store."""
        response = self._client.get("/files", params={"path": path})
        _raise_for_status(response)
        return response.content

