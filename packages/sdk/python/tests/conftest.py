from __future__ import annotations

import os

import pytest

BOXER_URL = os.environ.get("BOXER_URL", "")

needs_server = pytest.mark.skipif(
    not BOXER_URL,
    reason="BOXER_URL not set — skipping live-server tests",
)


@pytest.fixture
def boxer_url() -> str:
    return BOXER_URL
