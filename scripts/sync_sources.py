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
import hashlib
import http.client
import json
import posixpath
import re
import sys
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
CONFIG = ROOT / "sources.json"
OUT_DIR = ROOT / ".fetched"
#: Records `repo@ref:path[:sha256]` per fetched file — brand files and
#: `sources` entries alike. `--offline` needs it to tell a copy fetched at the
#: pinned ref from one an earlier ref, or a different upstream path, left
#: behind.
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
            "      run `make docs-check DOCS_SYNC=docs-sync-offline` to build "
            "from the cached copies.",
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


def _out_dest(name: str) -> Path:
    """Resolve a `sources` entry's destination, refusing anything outside `.fetched/`.

    `name` comes from `sources.json` and is pasted straight into a filename, so
    a `../` in it would write outside the cache the same way a bad `theme.dest`
    would — so the `_theme_dest` guard applies here too.
    """
    dest = (OUT_DIR / f"{name}.md").resolve()
    if not dest.is_relative_to(OUT_DIR):
        raise SystemExit(
            f"sync: source name {name!r} must stay under .fetched/."
        )
    return dest


def _digest(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def _is_client_error(error: Exception) -> bool:
    """Is this a 4xx — a mis-configured source rather than an unreachable one?

    A 404 means the upstream file moved or was removed, or `ref` is wrong. That
    is a `sources.json` defect: serving yesterday's cached copy in its place
    would hide the very drift the weekly rebuild exists to surface, so it must
    never reach the `--offline` fallback.
    """
    return isinstance(error, urllib.error.HTTPError) and 400 <= error.code < 500


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
    `fetch()` path above would corrupt it. Three things have to line up before
    a file is accepted:

    * `theme.ref` is a full commit sha — a tag can be moved, and the weekly
      rebuild would then publish a different brand with nothing here recording
      it (`sources.json` says the same, at length);
    * the bytes hash to the `sha256` recorded beside the entry, so an upstream
      edit at a *different* path, or a corrupted transfer, cannot pass;
    * `THEME_LOCK` records `repo@ref:path:sha256` per destination, which is
      what lets `--offline` refuse a copy left behind by an earlier ref or by
      a file that has since moved upstream.

    Without those, a bumped `theme.ref` plus a flaky network publishes a site
    built from a mix of two brands, with nothing non-zero anywhere in the
    pipeline.
    """
    for key in ("repo", "ref", "files"):
        if key not in theme:
            raise SystemExit(f'sync: sources.json "theme" is missing {key!r}.')

    repo, ref = theme["repo"], theme["ref"]
    if not re.fullmatch(r"[0-9a-f]{40}", ref):
        raise SystemExit(
            f'sync: sources.json "theme.ref" is {ref!r}, which is not a commit. '
            f"sources.json's own instruction: `ref` is the commit a release tag "
            f"points at rather than the tag itself — \"resolve its tag to a "
            f'commit and replace `ref`: gh api repos/{repo}/commits/<tag> '
            f'--jq .sha".'
        )

    lock = _read_lock()
    status = 0
    settled = 0

    for item in theme["files"]:
        missing = [key for key in ("path", "dest", "sha256") if key not in item]
        if missing:
            raise SystemExit(
                f'sync: every "theme.files" entry needs "path", "dest" and '
                f"\"sha256\"; {item!r} is missing {missing}."
            )
        name, expected = item["dest"], item["sha256"]
        dest = _theme_dest(name)
        url = RAW.format(repo=repo, ref=ref, path=item["path"])
        stamp = f"{repo}@{ref}:{item['path']}:{expected}"

        try:
            data = fetch_bytes(url)
        except FETCH_ERRORS as error:
            if _is_client_error(error):
                print(
                    f"sync: {url}\n      {error}\n      that is a sources.json "
                    f"problem — the file moved, was removed, or the ref is "
                    f"wrong — not an unreachable network, so no cached copy "
                    f"stands in for it.",
                    file=sys.stderr,
                )
                status = 1
                break
            if offline and dest.exists():
                if lock.get(name) != stamp:
                    cached = lock.get(name) or "an unrecorded ref"
                    print(
                        f"sync: cached {name} came from {cached}, but "
                        f"sources.json pins {stamp}.\n      refusing to build a "
                        f"mixed brand layer — fetch it online.",
                        file=sys.stderr,
                    )
                    status = 1
                    break
                actual = _digest(dest.read_bytes())
                if actual != expected:
                    print(
                        f"sync: cached {name} hashes {actual}, but sources.json "
                        f"records {expected}.\n      refusing to reuse it — "
                        f"fetch it online.",
                        file=sys.stderr,
                    )
                    status = 1
                    break
                settled += 1
                print(f"  ! {name}: unreachable, reusing the {stamp} copy ({error})")
                continue
            status = _fail_fetch(url, error, offline, dest)
            break

        actual = _digest(data)
        if actual != expected:
            print(
                f"sync: {name} fetched from {repo}@{ref}:{item['path']}\n"
                f"      hashes {actual},\n"
                f"      but sources.json records {expected}.\n"
                f"      refusing to write it — if the change upstream is "
                f"intended, update the sha256 beside that entry.",
                file=sys.stderr,
            )
            status = 1
            break

        write_atomic(dest, data)
        lock[name] = stamp
        settled += 1
        print(f"  ✓ {name}  ←  {repo}@{ref}:{item['path']}")

    _write_lock(lock)
    if status == 0 and settled == 0:
        print(
            'sync: sources.json "theme.files" is empty — the site would build '
            "without the brand layer at all.",
            file=sys.stderr,
        )
        return 1
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

    lock = _read_lock()
    for source in config["sources"]:
        name, repo, ref, path = (
            source["name"], source["repo"], source["ref"], source["path"],
        )
        destination = _out_dest(name)
        key = str(destination.relative_to(ROOT))
        stamp = f"{repo}@{ref}:{path}"
        url = RAW.format(repo=repo, ref=ref, path=path)

        try:
            text = fetch(url)
        except FETCH_ERRORS as error:
            if _is_client_error(error):
                print(
                    f"sync: {url}\n      {error}\n      that is a sources.json "
                    f"problem — the file moved, was removed, or the ref is "
                    f"wrong — not an unreachable network, so no cached copy "
                    f"stands in for it.",
                    file=sys.stderr,
                )
                return 1
            if args.offline and destination.exists():
                if lock.get(key) != stamp:
                    cached = lock.get(key) or "an unrecorded ref"
                    print(
                        f"sync: cached {name} came from {cached}, but "
                        f"sources.json pins {stamp}.\n      refusing to build "
                        f"from a stale copy — fetch it online.",
                        file=sys.stderr,
                    )
                    return 1
                print(f"  ! {name}: unreachable, reusing the {stamp} copy ({error})")
                continue
            return _fail_fetch(url, error, args.offline, destination)

        if source.get("drop_first_heading"):
            text = drop_leading_heading(text)
        text = absolutise_links(text, repo, ref, path)
        text = shift_headings(text, source.get("heading_shift", 0))

        write_atomic(
            destination,
            (
                f"<!-- Fetched from {stamp} by scripts/sync_sources.py."
                f" Do not edit; edit it in that repository. -->\n\n{text}"
            ).encode("utf-8"),
        )
        lock[key] = stamp
        print(f"  ✓ {name}  ←  {stamp}")
    if config["sources"]:
        _write_lock(lock)

    if "theme" not in config:
        raise SystemExit(
            'sync: sources.json has no "theme" block. The brand layer is not '
            "optional — without it the site builds unstyled and unbranded, and "
            "nothing downstream notices."
        )
    return sync_theme(config["theme"], args.offline)


if __name__ == "__main__":
    raise SystemExit(main())
