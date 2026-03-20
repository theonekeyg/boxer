from __future__ import annotations

from dataclasses import dataclass, field
from typing import Optional


@dataclass
class ResourceLimits:
    cpu_cores: Optional[float] = None
    memory_mb: Optional[int] = None
    pids_limit: Optional[int] = None
    wall_clock_secs: Optional[int] = None
    nofile: Optional[int] = None

    def to_dict(self) -> dict:
        result = {}
        if self.cpu_cores is not None:
            result["cpu_cores"] = self.cpu_cores
        if self.memory_mb is not None:
            result["memory_mb"] = self.memory_mb
        if self.pids_limit is not None:
            result["pids_limit"] = self.pids_limit
        if self.wall_clock_secs is not None:
            result["wall_clock_secs"] = self.wall_clock_secs
        if self.nofile is not None:
            result["nofile"] = self.nofile
        return result


@dataclass
class RunResult:
    exec_id: str
    exit_code: int
    stdout: str
    stderr: str
    wall_ms: int
