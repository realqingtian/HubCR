#!/usr/bin/env python3

from __future__ import annotations

import re
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
SAFE_LOCAL_VALUES = ("hubcr-dev-only", "hubcr-test-only", "do-not-log-this-password")
PATTERNS = {
    "private key": re.compile(r"-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----"),
    "AWS access key": re.compile(r"\b(?:AKIA|ASIA)[A-Z0-9]{16}\b"),
    "GitHub token": re.compile(r"\b(?:gh[pousr]_[A-Za-z0-9]{36,}|github_pat_[A-Za-z0-9_]{40,})\b"),
    "Slack token": re.compile(r"\bxox[baprs]-[A-Za-z0-9-]{20,}\b"),
    "Stripe live secret": re.compile(r"\bsk_live_[A-Za-z0-9]{16,}\b"),
    "authorization header": re.compile(r"(?i)authorization\s*:\s*(?:bearer|basic)\s+[A-Za-z0-9+/._=-]{12,}"),
    "credential URL": re.compile(
        r"[a-z][a-z0-9+.-]*://[^\s/:@]+:[A-Za-z0-9._~!$&'()*+,;=:%-]+@",
        re.IGNORECASE,
    ),
}


def tracked_files() -> list[Path]:
    result = subprocess.run(
        ["git", "ls-files", "--cached", "--others", "--exclude-standard", "-z"],
        cwd=ROOT,
        check=True,
        capture_output=True,
    )
    return [ROOT / value.decode() for value in result.stdout.split(b"\0") if value]


def main() -> int:
    findings: list[str] = []
    scanned = 0
    for path in tracked_files():
        if not path.is_file() or path.stat().st_size > 2 * 1024 * 1024:
            continue
        data = path.read_bytes()
        if b"\0" in data:
            continue
        contents = data.decode("utf-8", errors="replace")
        scanned += 1
        for number, line in enumerate(contents.splitlines(), 1):
            for name, pattern in PATTERNS.items():
                match = pattern.search(line)
                if not match:
                    continue
                safe_local_url = name == "credential URL" and any(
                    value in match.group(0) for value in SAFE_LOCAL_VALUES
                )
                if not safe_local_url:
                    findings.append(f"{path.relative_to(ROOT)}:{number}: possible {name}")

    if findings:
        print("secret checks failed:", file=sys.stderr)
        for finding in findings:
            print(f"- {finding}", file=sys.stderr)
        return 1
    print(f"secret checks passed for {scanned} tracked text files")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
