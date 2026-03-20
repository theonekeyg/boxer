from __future__ import annotations

from pathlib import Path
from typing import IO

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

    async def __aenter__(self) -> AsyncBoxerClient:
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
        response = await self._client.post("/run", json=body)
        _raise_for_status(response)
        return _parse_run_result(response.json())

    async def upload_file(
        self,
        remote_path: str,
        content: bytes | IO[bytes],
    ) -> None:
        """Upload a file to the Boxer file store."""
        response = await self._client.post(
            "/files",
            data={"path": remote_path},
            files={"file": content},
        )
        _raise_for_status(response)

    async def upload_path(
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
                    await self.upload_file(dest, fh)
                uploaded.append(dest)
            return uploaded
        else:
            dest = remote_path if remote_path is not None else local.name
            with open(local, "rb") as fh:
                await self.upload_file(dest, fh)
            return [dest]

    async def download_file(self, path: str) -> bytes:
        """Download a file from the Boxer file store."""
        response = await self._client.get("/files", params={"path": path})
        _raise_for_status(response)
        return response.content
