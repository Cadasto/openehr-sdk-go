#!/usr/bin/env bash
#
# sync-flat-conformance.sh — ingest / check / sync the upstream EHRbase FLAT
# serialisation conformance corpus into testkit/cassettes/flat-conformance/.
#
# Source: https://github.com/ehrbase/openEHR_SDK
#         test-data/src/main/resources/operationaltemplate/conformance_ehrbase.de.v0.opt
#         test-data/src/main/resources/composition/flat/simSDT/conformance/*.json
#
# These are upstream-authored FLAT (simSDT) composition bodies plus the single
# operational template they all instantiate. They are the PROBE-086 oracle: a
# FLAT corpus this SDK did not produce, so decoding it and re-encoding it
# exercises our REQ-053 codec against someone else's serialiser rather than
# against itself (which is what PROBE-076 already does). EHRbase is the
# reference implementation this SDK locks to per ADR 0014.
#
# Upstream is Apache-2.0; attribution is retained in
# testkit/cassettes/THIRD_PARTY_LICENSES.md.
#
# Subcommands:
#   sync     Download the OPT + every conformance FLAT body at
#            FLAT_CONFORMANCE_REF (default: develop), write them under
#            testkit/cassettes/flat-conformance/, and regenerate MANIFEST.txt
#            (pinned commit + per-file sha256). Removes vendored fixtures no
#            longer present upstream.
#   ingest   Alias for sync (first-time population).
#   check    Verify the vendored copies are intact and current:
#              1. offline — recompute sha256 and compare to MANIFEST
#                 (detects local edits / corruption); fails on mismatch.
#              2. online (best-effort) — compare the pinned commit to the
#                 current upstream HEAD and report if a sync is due.
#
# Environment:
#   FLAT_CONFORMANCE_REF   upstream git ref to pin (branch / tag / sha).
#                          Default: develop.
#   GITHUB_TOKEN           optional; raises the unauthenticated GitHub API
#                          rate limit.
#
# Reproducibility: `sync` resolves the ref to a concrete commit sha and
# downloads every file at that sha, so two syncs of the same ref are
# byte-identical. The resolved sha is recorded in MANIFEST.txt.
set -euo pipefail

readonly REPO="ehrbase/openEHR_SDK"
readonly TESTDATA="test-data/src/main/resources"
readonly FLAT_PATH="$TESTDATA/composition/flat/simSDT/conformance"
readonly OPT_PATH="$TESTDATA/operationaltemplate"
readonly OPT_NAME="conformance_ehrbase.de.v0.opt"
readonly REF="${FLAT_CONFORMANCE_REF:-develop}"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
readonly DEST="$ROOT/testkit/cassettes/flat-conformance"
readonly MANIFEST="$DEST/MANIFEST.txt"

# --- helpers ---------------------------------------------------------------

die() {
  echo "sync-flat-conformance: $*" >&2
  exit 1
}

# api <path> — GET the GitHub REST API, honouring GITHUB_TOKEN when set.
# --retry survives transient network blips / secondary rate limits.
api() {
  local url="https://api.github.com/repos/$REPO/$1"
  if [[ -n "${GITHUB_TOKEN:-}" ]]; then
    curl -fsSL --retry 3 --retry-all-errors -H "Authorization: Bearer $GITHUB_TOKEN" "$url"
  else
    curl -fsSL --retry 3 --retry-all-errors "$url"
  fi
}

# sha256_of <file> — print the bare sha256 hash of a file.
sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

# resolve_commit <ref> — print the concrete commit sha for a ref.
resolve_commit() {
  api "commits/$1" | jq -r '.sha' | grep -E '^[0-9a-f]{40}$' \
    || die "could not resolve ref '$1' in $REPO"
}

# list_flat <commit> — print the conformance FLAT filenames at the given
# commit, one per line, sorted.
list_flat() {
  api "contents/$FLAT_PATH?ref=$1" \
    | jq -r '.[] | select(.type == "file") | .name' \
    | grep -E '\.json$' \
    | sort \
    || die "no '*.json' files found under $FLAT_PATH at $1"
}

# raw_url <commit> <upstream-path> — the raw.githubusercontent URL for a file.
raw_url() {
  echo "https://raw.githubusercontent.com/$REPO/$1/$2"
}

# fetch <commit> <upstream-path> <dest-relative-path>
fetch() {
  local commit="$1" upstream="$2" rel="$3"
  mkdir -p "$(dirname "$DEST/$rel")"
  curl -fsSL --retry 3 --retry-all-errors "$(raw_url "$commit" "$upstream")" -o "$DEST/$rel" \
    || die "download failed: $rel"
  # Guard against a truncated/empty body that still returned HTTP 200 — don't
  # let a corrupt file become the canonical (hashed) copy.
  [[ -s "$DEST/$rel" ]] || die "download produced an empty file: $rel"
}

require_tools() {
  command -v curl >/dev/null || die "curl is required"
  command -v jq >/dev/null || die "jq is required"
}

# --- subcommands -----------------------------------------------------------

cmd_sync() {
  require_tools
  mkdir -p "$DEST/templates" "$DEST/compositions"

  echo "Resolving $REPO@$REF ..."
  local commit
  commit="$(resolve_commit "$REF")"
  echo "Pinned commit: $commit"

  local names
  names="$(list_flat "$commit")"
  [[ -n "$names" ]] || die "upstream returned an empty FLAT fixture list"

  echo "  fetch templates/$OPT_NAME"
  fetch "$commit" "$OPT_PATH/$OPT_NAME" "templates/$OPT_NAME"

  local name
  while IFS= read -r name; do
    echo "  fetch compositions/$name"
    fetch "$commit" "$FLAT_PATH/$name" "compositions/$name"
  done <<<"$names"

  # Drop vendored fixtures no longer present upstream.
  local existing base
  for existing in "$DEST"/compositions/*.json; do
    [[ -e "$existing" ]] || continue
    base="$(basename "$existing")"
    if ! grep -qxF "$base" <<<"$names"; then
      echo "  remove stale compositions/$base"
      rm -f "$existing"
    fi
  done

  # Regenerate the manifest (provenance + per-file integrity hashes). Hash
  # paths are relative to $DEST so `sha256sum -c` runs from there.
  {
    echo "# EHRbase FLAT serialisation conformance corpus — sync manifest"
    echo "# Generated by scripts/sync-flat-conformance.sh — do not edit by hand."
    echo "source_repo: $REPO"
    echo "source_opt: $OPT_PATH/$OPT_NAME"
    echo "source_flat: $FLAT_PATH"
    echo "ref: $REF"
    echo "commit: $commit"
    echo "fetched_utc: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "source_tree: https://github.com/$REPO/tree/$commit/$FLAT_PATH"
    echo "license: Apache-2.0 (see testkit/cassettes/THIRD_PARTY_LICENSES.md)"
    echo "#"
    echo "# sha256  path"
    echo "$(sha256_of "$DEST/templates/$OPT_NAME")  templates/$OPT_NAME"
    while IFS= read -r name; do
      echo "$(sha256_of "$DEST/compositions/$name")  compositions/$name"
    done <<<"$names"
  } >"$MANIFEST"

  local count
  count="$(wc -l <<<"$names")"
  echo "Synced 1 OPT + $count FLAT fixture(s) → testkit/cassettes/flat-conformance/ (commit ${commit:0:12})"
}

cmd_check() {
  [[ -f "$MANIFEST" ]] || die "no MANIFEST.txt — run 'make flat-conformance-sync' first"

  # 1. Offline integrity: local files must match the manifest hashes.
  echo "Checking vendored FLAT corpus integrity against MANIFEST.txt ..."
  local hash_block
  hash_block="$(sed -n '/^# sha256  path/,$p' "$MANIFEST" | tail -n +2)"
  [[ -n "$hash_block" ]] || die "manifest has no hash block"

  local rc=0
  ( cd "$DEST" && sha256sum -c --quiet - <<<"$hash_block" ) || rc=1

  # Detect vendored fixtures absent from the manifest (extras).
  local f name
  for f in "$DEST"/compositions/*.json "$DEST"/templates/*.opt; do
    [[ -e "$f" ]] || continue
    name="$(basename "$(dirname "$f")")/$(basename "$f")"
    if ! grep -qE "  $name\$" <<<"$hash_block"; then
      echo "  UNTRACKED: $name (in flat-conformance/ but not in MANIFEST.txt)"
      rc=1
    fi
  done

  if [[ $rc -ne 0 ]]; then
    die "integrity check failed — run 'make flat-conformance-sync' to refresh"
  fi
  echo "Integrity OK ($(grep -cE '  (compositions|templates)/' <<<"$hash_block") file(s))."

  # 2. Online staleness (best-effort; never fails the command).
  require_tools
  local pinned upstream
  pinned="$(grep -E '^commit: ' "$MANIFEST" | awk '{print $2}')"
  if upstream="$(resolve_commit "$REF" 2>/dev/null)"; then
    if [[ "$pinned" != "$upstream" ]]; then
      echo "Upstream $REPO@$REF advanced: ${pinned:0:12} -> ${upstream:0:12}"
      echo "Run 'make flat-conformance-sync' to update the vendored corpus."
    else
      echo "Up to date with $REPO@$REF (${pinned:0:12})."
    fi
  else
    echo "(offline — skipped upstream staleness check)"
  fi
}

main() {
  case "${1:-}" in
    sync | ingest) cmd_sync ;;
    check) cmd_check ;;
    "" | -h | --help)
      sed -n '2,44p' "$0" | sed 's/^# \{0,1\}//'
      ;;
    *) die "unknown subcommand '$1' (want: sync | ingest | check)" ;;
  esac
}

main "$@"
