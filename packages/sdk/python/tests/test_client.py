"""Sync integration tests — require a live Boxer server at BOXER_URL."""

from __future__ import annotations

from pathlib import Path

import pytest
from conftest import needs_server

from boxer import BoxerClient, BoxerTimeoutError, ResourceLimits

IMAGE = "python:3.12-slim"


@pytest.fixture
def client(boxer_url: str) -> BoxerClient:
    with BoxerClient(base_url=boxer_url) as c:
        yield c


@needs_server
def test_health(client: BoxerClient) -> None:
    assert client.health() is True


@needs_server
def test_run_inline(client: BoxerClient) -> None:
    result = client.run(image=IMAGE, cmd=["python3", "-c", "print(1)"])
    assert result.exit_code == 0
    assert result.stdout.strip() == "1"
    assert result.exec_id


@needs_server
def test_nonzero_exit_code(client: BoxerClient) -> None:
    result = client.run(image=IMAGE, cmd=["python3", "-c", "exit(1)"])
    assert result.exit_code == 1


@needs_server
def test_upload_and_run_with_file(client: BoxerClient) -> None:
    script = b"print('from file')\n"
    remote = "test_input.py"
    client.upload_file(remote, script)
    result = client.run(
        image=IMAGE,
        cmd=["python3", f"/{remote}"],
        files=[remote],
    )
    assert result.exit_code == 0
    assert "from file" in result.stdout


@needs_server
def test_upload_run_and_download_output(client: BoxerClient, tmp_path: Path) -> None:
    script = (
        b"import os; os.makedirs('/output', exist_ok=True); "
        b"open('/output/result.txt', 'w').write('hello output')\n"
    )
    remote = "write_output.py"
    client.upload_file(remote, script)
    result = client.run(
        image=IMAGE,
        cmd=["python3", f"/{remote}"],
        files=[remote],
        persist=True,
    )
    assert result.exit_code == 0

    data = client.download_file(f"output/{result.exec_id}/result.txt")
    assert data == b"hello output"


@needs_server
def test_upload_path_single_file(client: BoxerClient, tmp_path: Path) -> None:
    f = tmp_path / "hello.txt"
    f.write_text("hello from upload_path")

    paths = client.upload_path(f)
    assert paths == ["hello.txt"]

    result = client.run(
        image=IMAGE,
        cmd=["python3", "-c", "import os; print(os.path.exists('/hello.txt'))"],
        files=paths,
    )
    assert result.exit_code == 0
    assert result.stdout.strip() == "True"


@needs_server
def test_upload_path_directory(client: BoxerClient, tmp_path: Path) -> None:
    (tmp_path / "a.txt").write_text("file a")
    (tmp_path / "sub").mkdir()
    (tmp_path / "sub" / "b.txt").write_text("file b")

    paths = client.upload_path(tmp_path, remote_path="mydir")
    assert sorted(paths) == ["mydir/a.txt", "mydir/sub/b.txt"]

    result = client.run(
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
def test_timeout_raises(client: BoxerClient) -> None:
    limits = ResourceLimits(wall_clock_secs=1)
    with pytest.raises(BoxerTimeoutError):
        client.run(
            image=IMAGE,
            cmd=["python3", "-c", "while True: pass"],
            limits=limits,
        )
