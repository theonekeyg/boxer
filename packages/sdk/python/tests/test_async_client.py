"""Async integration tests — require a live Boxer server at BOXER_URL."""

from __future__ import annotations

from pathlib import Path

import pytest
from conftest import needs_server

from boxer import AsyncBoxerClient, BoxerTimeoutError, ResourceLimits

IMAGE = "python:3.12-slim"


@pytest.fixture
async def client(boxer_url: str) -> AsyncBoxerClient:
    async with AsyncBoxerClient(base_url=boxer_url) as c:
        yield c


@needs_server
async def test_health(client: AsyncBoxerClient) -> None:
    assert await client.health() is True


@needs_server
async def test_run_inline(client: AsyncBoxerClient) -> None:
    result = await client.run(image=IMAGE, cmd=["python3", "-c", "print(1)"])
    assert result.exit_code == 0
    assert result.stdout.strip() == "1"
    assert result.exec_id


@needs_server
async def test_nonzero_exit_code(client: AsyncBoxerClient) -> None:
    result = await client.run(image=IMAGE, cmd=["python3", "-c", "exit(1)"])
    assert result.exit_code == 1


@needs_server
async def test_upload_and_run_with_file(client: AsyncBoxerClient) -> None:
    script = b"print('from file async')\n"
    remote = "test_input_async.py"
    await client.upload_file(remote, script)
    result = await client.run(
        image=IMAGE,
        cmd=["python3", f"/{remote}"],
        files=[remote],
    )
    assert result.exit_code == 0
    assert "from file async" in result.stdout


@needs_server
async def test_upload_run_and_download_output(
    client: AsyncBoxerClient, tmp_path: Path
) -> None:
    script = (
        b"import os; os.makedirs('/output', exist_ok=True); "
        b"open('/output/result.txt', 'w').write('hello async output')\n"
    )
    remote = "write_output_async.py"
    await client.upload_file(remote, script)
    result = await client.run(
        image=IMAGE,
        cmd=["python3", f"/{remote}"],
        files=[remote],
    )
    assert result.exit_code == 0

    data = await client.download_file(f"output/{result.exec_id}/result.txt")
    assert data == b"hello async output"


@needs_server
async def test_upload_path_single_file(
    client: AsyncBoxerClient, tmp_path: Path
) -> None:
    f = tmp_path / "hello.txt"
    f.write_text("hello from upload_path")

    paths = await client.upload_path(f)
    assert paths == ["hello.txt"]

    result = await client.run(
        image=IMAGE,
        cmd=["python3", "-c", "import os; print(os.path.exists('/hello.txt'))"],
        files=paths,
    )
    assert result.exit_code == 0
    assert result.stdout.strip() == "True"


@needs_server
async def test_upload_path_directory(client: AsyncBoxerClient, tmp_path: Path) -> None:
    (tmp_path / "a.txt").write_text("file a")
    (tmp_path / "sub").mkdir()
    (tmp_path / "sub" / "b.txt").write_text("file b")

    paths = await client.upload_path(tmp_path, remote_path="mydir")
    assert sorted(paths) == ["mydir/a.txt", "mydir/sub/b.txt"]

    result = await client.run(
        image=IMAGE,
        cmd=[
            "python3",
            "-c",
            "import os; "
            "print(os.path.exists('/mydir/a.txt')"
            " and os.path.exists('/mydir/sub/b.txt'))",
        ],
        files=paths,
    )
    assert result.exit_code == 0
    assert result.stdout.strip() == "True"


@needs_server
async def test_timeout_raises(client: AsyncBoxerClient) -> None:
    limits = ResourceLimits(wall_clock_secs=1)
    with pytest.raises(BoxerTimeoutError):
        await client.run(
            image=IMAGE,
            cmd=["python3", "-c", "while True: pass"],
            limits=limits,
        )
