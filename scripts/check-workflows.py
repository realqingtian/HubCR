#!/usr/bin/env python3
"""Reject mutable external GitHub Action references."""

from __future__ import annotations

import re
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
ACTION_USE = re.compile(r"^\s*uses:\s*([^\s#]+)", re.MULTILINE)
COMMIT = re.compile(r"^[0-9a-f]{40}$")


def main() -> int:
    failures: list[str] = []
    for workflow in sorted((ROOT / ".github" / "workflows").glob("*.y*ml")):
        contents = workflow.read_text(encoding="utf-8")
        for reference in ACTION_USE.findall(contents):
            if reference.startswith("./"):
                continue
            action, separator, revision = reference.rpartition("@")
            if not separator or not action or not COMMIT.fullmatch(revision):
                failures.append(f"{workflow.relative_to(ROOT)}: mutable action reference {reference}")
    if failures:
        print("\n".join(failures))
        return 1
    print("GitHub Action references are pinned to full commit SHAs")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
