#!/usr/bin/env python3
"""Assert the three hand-maintained lists of fetched brand paths agree.

`sources.json` `theme.files[].dest` is the source of truth. The same six paths
are repeated in `.gitignore` (so fetched content cannot be committed) and in the
Makefile's `THEME_FETCHED` (so `make clean` reaps them). Nothing in the build
notices when they drift, and each drift fails silently in its own way: a path
missing from `.gitignore` makes fetched content committable, reintroducing the
second copy this repository exists to avoid; a path missing from
`THEME_FETCHED` survives `make clean` and is then reused by `--offline`
indefinitely.

Run from the repository root. Exits non-zero with the difference on drift.
"""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent


def sources_dests() -> set[str]:
    theme = json.loads((ROOT / "sources.json").read_text())["theme"]
    return {item["dest"] for item in theme["files"]}


def makefile_dests() -> set[str]:
    text = (ROOT / "Makefile").read_text()
    block = re.search(r"^THEME_FETCHED :=(.*?)(?=\n[^\t\n])", text, re.S | re.M)
    if not block:
        sys.exit("check: could not find THEME_FETCHED in the Makefile")
    # Line continuations leave bare backslashes among the tokens.
    return {tok for tok in block.group(1).split() if tok != "\\"}


def gitignore_dests() -> set[str]:
    return {
        line.strip().lstrip("/")
        for line in (ROOT / ".gitignore").read_text().splitlines()
        if re.match(r"^/(pages|overrides)/", line.strip())
    }


def main() -> int:
    expected = sources_dests()
    problems = []
    for name, actual in (
        ("Makefile THEME_FETCHED", makefile_dests()),
        (".gitignore", gitignore_dests()),
    ):
        if actual != expected:
            missing = sorted(expected - actual)
            extra = sorted(actual - expected)
            detail = ", ".join(
                part
                for part in (
                    f"missing {missing}" if missing else "",
                    f"unexpected {extra}" if extra else "",
                )
                if part
            )
            problems.append(f"  {name}: {detail}")

    if problems:
        print(
            "check: fetched brand paths have drifted from sources.json "
            "theme.files:\n" + "\n".join(problems),
            file=sys.stderr,
        )
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
