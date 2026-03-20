from __future__ import annotations

from pathlib import Path
from typing import IO, List, Optional, Union

import httpx

from .client import _build_run_body, _parse_run_result, _raise_for_status
from .types import ResourceLimits, RunResult


class AsyncBoxerClient:
    """Asynchronous Boxer API client backed by httpx."""

    def __init__(
        self,
        base_url: str = "http://localhost:8080",
        timeout: float = 120.0,
    ) -> None:
        self._client = httpx.AsyncClient(
            base_url=base_url,
            timeout=timeout,
        )

    async def __aenter__(self) -> "AsyncBoxerClient":
        await self._client.__aenter__()
        return self

    async def __aexit__(self, *args: object) -> None:
        await self._client.__aexit__(*args)

    async def aclose(self) -> None:
        await self._client.aclose()

    # ------------------------------------------------------------------
    # Core API
    # ------------------------------------------------------------------

    async def health(self) -> bool:
        """Return True if the server is healthy."""
        response = await self._client.get("/healthz")
        return response.is_success

    async def run(
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
        response = await self._client.post("/run", json=body)
        _raise_for_status(response)
        return _parse_run_result(response.json())

    async def upload_file(
        self,
        remote_path: str,
        content: Union[bytes, IO[bytes]],
    ) -> None:
        """Upload a file to the Boxer file store."""
        response = await self._client.post(
            "/files",
            data={"path": remote_path},
            files={"file": content},
        )
        _raise_for_status(response)

    async def download_file(self, path: str) -> bytes:
        """Download a file from the Boxer file store."""
        response = await self._client.get("/files", params={"path": path})
        _raise_for_status(response)
        return response.content

    # ------------------------------------------------------------------
    # Convenience helpers
    # ------------------------------------------------------------------

    async def run_script(
        self,
        code: str,
        image: str = "python:3.12-slim",
        *,
        interpreter: Optional[List[str]] = None,
        limits: Optional[ResourceLimits] = None,
    ) -> RunResult:
        """Run inline code without manual file management."""
        if interpreter is None:
            interpreter = ["python3", "-c"]
        cmd = interpreter + [code]
        return await self.run(image=image, cmd=cmd, limits=limits)

    async def run_file(
        self,
        local_path: Union[str, Path],
        image: str,
        *,
        remote_path: Optional[str] = None,
        cmd_prefix: Optional[List[str]] = None,
        limits: Optional[ResourceLimits] = None,
        persist: bool = False,
    ) -> RunResult:
        """Upload a local file and run it inside the sandbox."""
        local = Path(local_path)
        if remote_path is None:
            remote_path = local.name
        if cmd_prefix is None:
            cmd_prefix = ["python3"]

        with open(local, "rb") as fh:
            await self.upload_file(remote_path, fh)

        container_path = f"/{remote_path}"
        return await self.run(
            image=image,
            cmd=cmd_prefix + [container_path],
            files=[remote_path],
            limits=limits,
            persist=persist,
        )
