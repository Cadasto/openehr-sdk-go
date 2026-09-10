#!/usr/bin/env python3
"""Fetch the Cadasto brand layer (and any pinned `sources` entries).

This repository authors its site pages in `pages/`. `sources.json` `sources`
is empty; only the docs-theme files are fetched. Brand files from
`Cadasto/docs-theme` land on their live paths (`extra_css`, `custom_dir`,
`docs_dir/assets`). Copying them here would guarantee drift, so they are
pulled at build time from a pinned ref — `sources.json` is the single place a
version is named.

Rewriting of install Markdown happens here rather than in a MkDocs hook on
purpose: a hook sees the `--8<--` include line, not the included text, so it
could never fix links inside fetched content. Doing it at fetch time is also
deterministic and testable.

Usage:
    python3 scripts/sync_sources.py            # fetch, fail if unreachable
    python3 scripts/sync_sources.py --offline  # reuse cached copies
    # Makefile wraps these as `docs-sync` / `docs-sync-offline`.
"""

from __future__ import annotations

import argparse
import http.client
import json
import posixpath
import re
import sys
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
CONFIG = ROOT / "sources.json"
OUT_DIR = ROOT / ".fetched"
#: Records `repo@ref` per fetched brand file. `--offline` needs it to tell a
#: copy fetched at the pinned ref from one an earlier ref left behind.
THEME_LOCK = OUT_DIR / "theme.lock"

RAW = "https://raw.githubusercontent.com/{repo}/{ref}/{path}"
BLOB = "https://github.com/{repo}/blob/{ref}/{path}"

#: A relative Markdown link: not absolute, not an anchor, not a mail link.
RELATIVE_LINK = re.compile(
    r"\]\("
    r"(?!https?:|mailto:|#|/)"
    r"([^)\s#]+)"
    r"(#[^)\s]*)?"
    r"\)"
)

HEADING = re.compile(r"^(#{1,6})(\s+)", re.MULTILINE)
FENCE = re.compile(r"^(```|~~~)")


class FetchError(Exception):
    """A source responded, but with something we cannot use."""


#: Everything a fetch can plausibly raise. `urllib` turns socket errors on the
#: *request* into `URLError`, but `getresponse()` and `read()` sit outside that
#: conversion, so a mid-transfer drop arrives as a bare `OSError` or an
#: `http.client.HTTPException` — a truncated read is exactly the transient
#: failure `--offline` exists for, so both must reach its fallback rather than
#: escaping as a traceback. `URLError` and `TimeoutError` are `OSError`
#: subclasses, so this covers them too.
FETCH_ERRORS = (OSError, http.client.HTTPException, FetchError)


def fetch_bytes(url: str, timeout: int = 20) -> bytes:
    with urllib.request.urlopen(url, timeout=timeout) as response:
        if response.status != 200:
            raise FetchError(f"{url} returned HTTP {response.status}")
        data = response.read()
    if not data:
        raise FetchError(f"{url} returned an empty body")
    return data


def fetch(url: str, timeout: int = 20) -> str:
    return fetch_bytes(url, timeout=timeout).decode("utf-8")


def write_atomic(dest: Path, data: bytes) -> None:
    """Put `data` at `dest` in one step, so a failed write cannot truncate it.

    The fetched brand files are untracked, so a half-written one is the only
    copy there is — and `--offline` would go on reusing it.
    """
    dest.parent.mkdir(parents=True, exist_ok=True)
    tmp = dest.with_name(dest.name + ".tmp")
    try:
        tmp.write_bytes(data)
        tmp.replace(dest)
    finally:
        tmp.unlink(missing_ok=True)


def _fail_fetch(
    url: str, error: Exception, offline: bool, dest: Path | None = None
) -> int:
    """Report an unusable source on stderr and return the process exit code."""
    print(f"sync: cannot fetch {url}\n      {error}", file=sys.stderr)
    if offline and dest is not None:
        print(
            f"      --offline needs a cached {dest.relative_to(ROOT)}, "
            f"and there is none.",
            file=sys.stderr,
        )
    elif not offline:
        print(
            "      run `make docs-sync-offline` to build from the cached copies.",
            file=sys.stderr,
        )
    return 1


def _theme_dest(raw_dest: str) -> Path:
    """Resolve a `theme.files` dest, refusing anything outside the repository."""
    dest = (ROOT / raw_dest).resolve()
    if not dest.is_relative_to(ROOT):
        raise SystemExit(
            f"sync: theme dest {raw_dest!r} must stay under the repository root."
        )
    return dest


def _read_lock() -> dict[str, str]:
    try:
        return json.loads(THEME_LOCK.read_text())
    except (OSError, ValueError):
        return {}


def _write_lock(lock: dict[str, str]) -> None:
    THEME_LOCK.parent.mkdir(parents=True, exist_ok=True)
    THEME_LOCK.write_text(json.dumps(lock, indent=2, sort_keys=True) + "\n")


def sync_theme(theme: dict, offline: bool) -> int:
    """Write each brand file onto the path MkDocs actually reads.

    Returns a process exit code — 0 on success — the same convention `main`
    uses, because `__main__` passes it straight to `SystemExit`.

    Fetched as bytes, not text, because the company mark is a PNG; reusing the
    `fetch()` path above would corrupt it. Every write records `repo@ref` in
    `THEME_LOCK`, which is what lets `--offline` refuse a copy left behind by an
    earlier ref. Without it the fallback can only ask whether the file exists,
    and a bumped `theme.ref` plus a flaky network publishes a site built from a
    mix of two brands, with nothing non-zero anywhere in the pipeline.
    """
    for key in ("repo", "ref", "files"):
        if key not in theme:
            raise SystemExit(f'sync: sources.json "theme" is missing {key!r}.')

    repo, ref = theme["repo"], theme["ref"]
    stamp = f"{repo}@{ref}"
    lock = _read_lock()
    status = 0

    for item in theme["files"]:
        if "path" not in item or "dest" not in item:
            raise SystemExit(
                f'sync: every "theme.files" entry needs "path" and "dest"; '
                f"got {item!r}."
            )
        name = item["dest"]
        dest = _theme_dest(name)
        url = RAW.format(repo=repo, ref=ref, path=item["path"])

        try:
            data = fetch_bytes(url)
        except FETCH_ERRORS as error:
            if offline and dest.exists() and lock.get(name) == stamp:
                print(f"  ! {name}: unreachable, reusing the {stamp} copy ({error})")
                continue
            if offline and dest.exists():
                cached = lock.get(name) or "an unrecorded ref"
                print(
                    f"sync: cached {name} came from {cached}, but sources.json "
                    f"pins {stamp}.\n      refusing to build a mixed brand "
                    f"layer — fetch it online.",
                    file=sys.stderr,
                )
                status = 1
                break
            status = _fail_fetch(url, error, offline, dest)
            break

        write_atomic(dest, data)
        lock[name] = stamp
        print(f"  ✓ {name}  ←  {stamp}:{item['path']}")

    _write_lock(lock)
    return status


def absolutise_links(text: str, repo: str, ref: str, doc_path: str) -> str:
    """Point every relative link at the source repository on GitHub.

    Relative targets resolve against the document's own directory in that repo,
    not against this site, so they must be rewritten or they 404 once published.
    """
    doc_dir = posixpath.dirname(doc_path)

    def rewrite(match: re.Match) -> str:
        target, anchor = match.group(1), match.group(2) or ""
        resolved = posixpath.normpath(posixpath.join(doc_dir, target))
        if resolved.startswith(".."):
            raise SystemExit(
                f"sync: {repo}:{doc_path} links to {target!r}, which escapes the "
                f"repository root — cannot rewrite to a GitHub URL."
            )
        return "](" + BLOB.format(repo=repo, ref=ref, path=resolved) + anchor + ")"

    return RELATIVE_LINK.sub(rewrite, text)


def shift_headings(text: str, levels: int) -> str:
    """Demote headings so fetched content nests under this site's own sections.

    Fenced code is skipped — a `#` comment inside a shell block is not a heading.
    """
    if levels <= 0:
        return text
    out, in_fence = [], False
    for line in text.splitlines(keepends=True):
        if FENCE.match(line):
            in_fence = not in_fence
        if not in_fence:
            line = HEADING.sub(
                lambda m: "#" * min(len(m.group(1)) + levels, 6) + m.group(2), line
            )
        out.append(line)
    return "".join(out)


def drop_leading_heading(text: str) -> str:
    """Remove the source document's own title; this site supplies the framing."""
    lines = text.splitlines(keepends=True)
    for i, line in enumerate(lines):
        if line.strip():
            if line.lstrip().startswith("#"):
                return "".join(lines[i + 1 :]).lstrip("\n")
            break
    return text


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--offline",
        action="store_true",
        help=(
            "reuse the cached .fetched/ docs and the last fetched brand files "
            "instead of failing when offline"
        ),
    )
    args = parser.parse_args()

    config = json.loads(CONFIG.read_text())
    OUT_DIR.mkdir(exist_ok=True)

    for source in config["sources"]:
        name, repo, ref, path = (
            source["name"], source["repo"], source["ref"], source["path"],
        )
        destination = OUT_DIR / f"{name}.md"
        url = RAW.format(repo=repo, ref=ref, path=path)

        try:
            text = fetch(url)
        except FETCH_ERRORS as error:
            if args.offline and destination.exists():
                print(f"  ! {name}: unreachable, reusing cached copy ({error})")
                continue
            return _fail_fetch(url, error, args.offline, destination)

        if source.get("drop_first_heading"):
            text = drop_leading_heading(text)
        text = absolutise_links(text, repo, ref, path)
        text = shift_headings(text, source.get("heading_shift", 0))

        destination.write_text(
            f"<!-- Fetched from {repo}@{ref}:{path} by scripts/sync_sources.py."
            f" Do not edit; edit it in that repository. -->\n\n{text}"
        )
        print(f"  ✓ {name}  ←  {repo}@{ref}:{path}")

    theme = config.get("theme")
    if theme:
        result = sync_theme(theme, args.offline)
        if result != 0:
            return result

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
