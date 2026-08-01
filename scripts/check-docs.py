#!/usr/bin/env python3

from __future__ import annotations

import re
import subprocess
import sys
from pathlib import Path
from urllib.parse import unquote


ROOT = Path(__file__).resolve().parent.parent
PAIR_EXEMPT_BASENAMES = {"CLAUDE.md", "GEMINI.md", "copilot-instructions.md"}
LINK_PATTERN = re.compile(r"!?\[[^\]]*\]\(([^)]+)\)")


def tracked_markdown() -> list[Path]:
    result = subprocess.run(
        ["git", "ls-files", "--cached", "--others", "--exclude-standard", "-z", "*.md"],
        cwd=ROOT,
        check=True,
        capture_output=True,
    )
    return [ROOT / value.decode() for value in result.stdout.split(b"\0") if value]


def counterpart(path: Path) -> Path:
    if path.name.endswith(".zh-CN.md"):
        return path.with_name(path.name.removesuffix(".zh-CN.md") + ".md")
    return path.with_name(path.stem + ".zh-CN.md")


def check_link(path: Path, raw_target: str) -> str | None:
    target = raw_target.strip()
    if target.startswith("<") and target.endswith(">"):
        target = target[1:-1]
    elif " " in target:
        target = target.split(" ", 1)[0]
    target = target.split("#", 1)[0]
    if not target or target.startswith(("http://", "https://", "mailto:", "/")):
        return None
    resolved = (path.parent / unquote(target)).resolve()
    try:
        resolved.relative_to(ROOT)
    except ValueError:
        return f"link escapes repository: {raw_target}"
    if not resolved.exists():
        return f"missing relative link target: {raw_target}"
    return None


def main() -> int:
    errors: list[str] = []
    markdown = tracked_markdown()
    tracked = {path.resolve() for path in markdown}

    for path in markdown:
        contents = path.read_text(encoding="utf-8")
        relative = path.relative_to(ROOT)
        if contents and not contents.endswith("\n"):
            errors.append(f"{relative}: missing final newline")
        for number, line in enumerate(contents.splitlines(), 1):
            if line.rstrip() != line:
                errors.append(f"{relative}:{number}: trailing whitespace")
        for match in LINK_PATTERN.finditer(contents):
            error = check_link(path, match.group(1))
            if error:
                line = contents.count("\n", 0, match.start()) + 1
                errors.append(f"{relative}:{line}: {error}")

        if path.name in PAIR_EXEMPT_BASENAMES:
            continue
        pair = counterpart(path)
        if pair.resolve() not in tracked:
            errors.append(f"{relative}: missing bilingual counterpart {pair.name}")
            continue
        if pair.name not in contents:
            errors.append(f"{relative}: language switch does not link to {pair.name}")

    if errors:
        print("documentation checks failed:", file=sys.stderr)
        for error in errors:
            print(f"- {error}", file=sys.stderr)
        return 1
    print(f"documentation checks passed for {len(markdown)} Markdown files")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
