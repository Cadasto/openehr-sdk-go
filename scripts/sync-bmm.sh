#!/usr/bin/env bash
#
# sync-bmm.sh — vendor / verify the openEHR BMM schemas (P_BMM JSON) into
# resources/bmm/ (REQ-041).
#
# Source: https://github.com/openEHR/specifications-ITS-BMM
#         components/<COMPONENT>/json/<schema id>.bmm.json
#
# These are the openEHR Foundation's computable form of the BASE, RM, AM, LANG
# and TERM specifications, the inputs `make codegen` generates openehr/rm/,
# openehr/aom/aom14/ and the type registry from. The SDK pins exactly the files
# listed in SRC_PATHS below, byte-identical to one upstream commit, so a build
# never fetches them and a change to any of them is a reviewable commit.
#
# A version bump (ADR 0001) is an edit to SRC_PATHS: add the new path, run
# `BMM_REF=master make bmm-sync`, regenerate, and drop the old path and file
# in the same commit.
#
# Subcommands:
#   sync     Fetch every file in SRC_PATHS at one commit, write them
#            byte-identical, remove any *.bmm.json in resources/bmm/ that
#            SRC_PATHS no longer lists, and regenerate MANIFEST.txt. The commit is
#            BMM_REF resolved when set; otherwise the commit MANIFEST.txt
#            already pins, so a bare sync restores the pinned bytes and never
#            moves the pin; with no MANIFEST.txt either, the tip of master.
#            Then run `make codegen` so the generated code follows the pin
#            (skipped when BMM_SKIP_GEN=1).
#   verify   Offline: recompute each file's sha256 and compare it to
#            MANIFEST.txt; fail on a mismatch, a file with no hash line, or a
#            *.bmm.json in resources/bmm/ the manifest does not list. No
#            network, no curl/jq — run by `make ci`.
#   check    verify, then (best-effort, network) compare each pinned file with
#            the tip of master, and list any newer schema version upstream
#            for a pinned component. The network step never fails the command.
#
# Environment:
#   BMM_REF        upstream git ref to pin (branch / tag / sha). ITS-BMM has no
#                  release tags, so a re-pin is normally BMM_REF=master.
#   BMM_SKIP_GEN   set to 1 to skip `make codegen` after a sync.
#   GITHUB_TOKEN   optional; raises the unauthenticated API rate limit.
#
# Reproducibility: `sync` resolves the ref to a concrete commit sha and
# downloads every file at that one sha, so two syncs of the same ref are
# byte-identical. The resolved sha is recorded in MANIFEST.txt.
set -euo pipefail

readonly REPO="openEHR/specifications-ITS-BMM"
readonly DEFAULT_BRANCH="master"
# The pinned files, as paths from the upstream repository root. Each one is
# vendored under its base name, and all of them come from one resolved commit.
readonly SRC_PATHS=(
  "components/BASE/json/openehr_base_1.3.0.bmm.json"
  "components/RM/json/openehr_rm_1.2.0.bmm.json"
  "components/AM/json/openehr_am_1.4.0.bmm.json"
  "components/AM/json/openehr_am_2.4.0.bmm.json"
  "components/LANG/json/openehr_lang_1.1.0.bmm.json"
  "components/TERM/json/openehr_term_3.1.0.bmm.json"
)
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
readonly DEST="$ROOT/resources/bmm"
readonly MANIFEST="$DEST/MANIFEST.txt"

# --- helpers ---------------------------------------------------------------

die() {
  echo "sync-bmm: $*" >&2
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

# manifest_field <key> — print a `key: value` field of MANIFEST.txt (empty
# when there is no manifest or no such field).
manifest_field() {
  [[ -f "$MANIFEST" ]] || return 0
  awk -v k="$1:" '$1 == k { print $2; exit }' "$MANIFEST"
}

# resolve_commit <ref> — print the concrete commit sha a ref points at.
# commits/<ref> accepts a branch, a lightweight or annotated tag, or a sha.
resolve_commit() {
  api "commits/$1" | jq -r '.sha // empty' | grep -E '^[0-9a-f]{40}$' \
    || die "could not resolve ref '$1' in $REPO"
}

# raw_url <commit> <path> — the raw.githubusercontent URL for an upstream
# path at a commit.
raw_url() {
  echo "https://raw.githubusercontent.com/$REPO/$1/$2"
}

# version_of <file name> — print the x.y.z of openehr_<x>_<x.y.z>.bmm.json,
# or nothing for a name with any other shape (a suffixed variant, say).
version_of() {
  sed -nE 's/^openehr_[a-z]+_([0-9]+\.[0-9]+\.[0-9]+)\.bmm\.json$/\1/p' <<<"$1"
}

require_tools() {
  command -v curl >/dev/null || die "curl is required"
  command -v jq >/dev/null || die "jq is required"
}

# --- subcommands -----------------------------------------------------------

cmd_sync() {
  require_tools
  mkdir -p "$DEST"

  local ref commit
  if [[ -n "${BMM_REF:-}" ]]; then
    ref="$BMM_REF"
    echo "Resolving $REPO@$ref ..."
    commit="$(resolve_commit "$ref")"
  elif [[ -n "$(manifest_field commit)" ]]; then
    ref="$(manifest_field ref)"
    commit="$(manifest_field commit)"
    echo "No BMM_REF — re-fetching the pinned commit (set BMM_REF to move the pin) ..."
  else
    ref="$DEFAULT_BRANCH"
    echo "No BMM_REF and no pin — resolving $REPO@$ref ..."
    commit="$(resolve_commit "$ref")"
  fi
  echo "Pinned commit: $commit"

  # Download every file beside the pin first, and move them into place only
  # once all of them pass the guards below. A failed or bogus fetch (a ref
  # whose tree lacks one of the files, say) then leaves the vendored files and
  # their manifest untouched: the files only ever move together.
  local src name tmp
  local -a tmps=()
  for src in "${SRC_PATHS[@]}"; do
    name="$(basename "$src")"
    tmp="$DEST/.$name.tmp"
    tmps+=("$tmp")
    echo "  fetch $src"
    curl -fsSL --retry 3 --retry-all-errors "$(raw_url "$commit" "$src")" -o "$tmp" \
      || { rm -f "${tmps[@]}"; die "download failed: $src"; }

    # Guard against a truncated body or an error page that still returned
    # HTTP 200 — don't let a corrupt file become the canonical (hashed) copy.
    [[ -s "$tmp" ]] || { rm -f "${tmps[@]}"; die "download produced an empty file: $src"; }
    [[ "$(head -c 256 "$tmp")" == *'"bmm_version"'* ]] \
      || { rm -f "${tmps[@]}"; die "download is not a P_BMM schema (no bmm_version near the top): $src"; }
  done
  for src in "${SRC_PATHS[@]}"; do
    name="$(basename "$src")"
    mv -f "$DEST/.$name.tmp" "$DEST/$name"
  done

  # Drop schemas no longer in SRC_PATHS: the SDK pins one version per schema
  # id, so the bump that adds a version removes the old one (ADR 0001).
  local f keep
  for f in "$DEST"/*.bmm.json; do
    [[ -e "$f" ]] || continue
    keep=""
    for src in "${SRC_PATHS[@]}"; do
      [[ "$(basename "$src")" == "$(basename "$f")" ]] && keep=1
    done
    if [[ -z "$keep" ]]; then
      echo "  remove $(basename "$f") (not in SRC_PATHS)"
      rm -f "$f"
    fi
  done

  # Keep fetched_utc while the commit is unchanged, so a re-sync at the pin
  # rewrites a byte-identical manifest; only a re-pin moves it.
  local fetched=""
  if [[ "$(manifest_field commit)" == "$commit" ]]; then
    fetched="$(manifest_field fetched_utc)"
  fi
  [[ -n "$fetched" ]] || fetched="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  # Regenerate the manifest (provenance + integrity hashes). The hash paths
  # are relative to $DEST so the hash block reads as plain `sha256  filename`.
  {
    echo "# openEHR BMM schemas — sync manifest"
    echo "# Generated by scripts/sync-bmm.sh — do not edit by hand."
    echo "source_repo: $REPO"
    echo "ref: $ref"
    echo "commit: $commit"
    echo "fetched_utc: $fetched"
    echo "source_tree: https://github.com/$REPO/tree/$commit/components"
    for src in "${SRC_PATHS[@]}"; do
      echo "source_path: $src"
    done
    echo "#"
    echo "# sha256  filename"
    for src in "${SRC_PATHS[@]}"; do
      name="$(basename "$src")"
      echo "$(sha256_of "$DEST/$name")  $name"
    done
  } >"$MANIFEST"

  echo "Synced ${#SRC_PATHS[@]} files → resources/bmm/ ($ref, commit ${commit:0:12})"

  # Keep the generated code in step with the pin (REQ-042).
  if [[ "${BMM_SKIP_GEN:-}" == "1" ]]; then
    echo "(BMM_SKIP_GEN=1 — skipped 'make codegen')"
    return 0
  fi
  echo "Regenerating the RM and AOM ('make codegen') ..."
  make -C "$ROOT" codegen
}

# cmd_check [offline] — offline integrity first, then (unless 'offline') a
# best-effort upstream-drift report.
cmd_check() {
  [[ -f "$MANIFEST" ]] || die "no MANIFEST.txt — run 'make bmm-sync' first"

  # 1. Offline integrity: every vendored file must match its manifest hash.
  local hash_block
  hash_block="$(sed -n '/^# sha256  filename/,$p' "$MANIFEST" | tail -n +2)"
  [[ -n "$hash_block" ]] || die "manifest has no hash block"

  # Recompute through sha256_of rather than `sha256sum -c` so hosts that only
  # have Perl's shasum (stock macOS) can still run the `make ci` gate.
  command -v sha256sum >/dev/null 2>&1 || command -v shasum >/dev/null 2>&1 \
    || die "sha256sum or shasum is required"

  local rc=0 line want path
  while IFS= read -r line; do
    want="${line%% *}"
    path="${line#*  }"
    if [[ ! -f "$DEST/$path" ]]; then
      echo "  MISSING: $path"
      rc=1
    elif [[ "$(sha256_of "$DEST/$path")" != "$want" ]]; then
      echo "  FAILED: $path (sha256 mismatch)"
      rc=1
    fi
  done <<<"$hash_block"

  # Every file the pin is made of must have a hash line, and every schema in
  # resources/bmm/ must be one of them. Names are compared literally, never
  # as a regex.
  local src f name found
  for src in "${SRC_PATHS[@]}"; do
    name="$(basename "$src")"
    found=""
    while IFS= read -r line; do
      if [[ "${line#*  }" == "$name" ]]; then
        found=1
        break
      fi
    done <<<"$hash_block"
    if [[ -z "$found" ]]; then
      echo "  NOT PINNED: $name (no sha256 line in MANIFEST.txt)"
      rc=1
    fi
  done
  for f in "$DEST"/*.bmm.json; do
    [[ -e "$f" ]] || continue
    name="$(basename "$f")"
    found=""
    while IFS= read -r line; do
      if [[ "${line#*  }" == "$name" ]]; then
        found=1
        break
      fi
    done <<<"$hash_block"
    if [[ -z "$found" ]]; then
      echo "  UNTRACKED: $name (in resources/bmm/ but not in MANIFEST.txt)"
      rc=1
    fi
  done

  if [[ $rc -ne 0 ]]; then
    die "integrity check failed — never hand-edit the pin; run 'make bmm-sync' to restore it"
  fi
  echo "bmm-check: OK"

  # 2. Upstream drift (best-effort; never fails the command). Do NOT
  # require_tools here: the integrity check above is fully offline, so a host
  # without curl/jq must still be able to verify the pin — and `verify`, which
  # `make ci` runs, shares this path.
  if [[ "${1:-}" == "offline" ]]; then
    return 0
  fi
  if ! command -v curl >/dev/null 2>&1 || ! command -v jq >/dev/null 2>&1 \
    || ! command -v git >/dev/null 2>&1; then
    echo "(curl/jq/git unavailable — skipped upstream drift check)"
    return 0
  fi

  local pinned head
  pinned="$(manifest_field commit)"
  if ! head="$(resolve_commit "$DEFAULT_BRANCH" 2>/dev/null)"; then
    echo "(offline or rate-limited — skipped upstream drift check)"
    return 0
  fi
  echo "Pinned ${pinned:0:12}; $REPO@$DEFAULT_BRANCH is ${head:0:12}."

  # A commit that moves master need not touch a pinned file, so compare the
  # files themselves: the git blob sha the contents API lists for each name
  # against `git hash-object` of the vendored copy. One listing per component
  # directory also names every schema version published there.
  local dir listing ours theirs changed=0 newer=0 comp max v
  local -A seen=()
  for src in "${SRC_PATHS[@]}"; do
    dir="$(dirname "$src")"
    [[ -z "${seen[$dir]:-}" ]] || continue
    seen[$dir]=1
    if ! listing="$(api "contents/$dir?ref=$head" 2>/dev/null)"; then
      echo "(could not list $dir at ${head:0:12} — skipped)"
      continue
    fi

    # Each pinned file of this component: unchanged, changed or gone.
    for f in "${SRC_PATHS[@]}"; do
      [[ "$(dirname "$f")" == "$dir" ]] || continue
      name="$(basename "$f")"
      ours="$(git hash-object --no-filters "$DEST/$name")"
      theirs="$(jq -r --arg n "$name" '.[] | select(.name == $n) | .sha' <<<"$listing")"
      if [[ -z "$theirs" ]]; then
        echo "  GONE upstream: $f"
        changed=1
      elif [[ "$theirs" != "$ours" ]]; then
        echo "  CHANGED upstream: $f"
        changed=1
      fi
    done

    # A newer version is one above every version pinned for the component,
    # so pinning AM 1.4 and AM 2.4 side by side does not report AM 2.2.
    comp="$(basename "$(dirname "$dir")")"
    max=""
    for f in "${SRC_PATHS[@]}"; do
      [[ "$(dirname "$f")" == "$dir" ]] || continue
      v="$(version_of "$(basename "$f")")"
      max="$(printf '%s\n%s\n' "$max" "$v" | sed '/^$/d' | sort -V | tail -n 1)"
    done
    while IFS= read -r name; do
      v="$(version_of "$name")"
      [[ -n "$v" && -n "$max" && "$v" != "$max" ]] || continue
      if [[ "$(printf '%s\n%s\n' "$max" "$v" | sort -V | tail -n 1)" == "$v" ]]; then
        echo "  NEWER $comp version upstream: $dir/$name (pinned: $max)"
        newer=1
      fi
    done < <(jq -r '.[] | select(.type == "file") | .name' <<<"$listing")
  done

  if [[ $changed -eq 0 && $newer -eq 0 ]]; then
    echo "Every pinned schema is byte-identical at $REPO@$DEFAULT_BRANCH; no newer version."
  else
    [[ $changed -eq 0 ]] || echo "Run 'BMM_REF=$DEFAULT_BRANCH make bmm-sync' to take the changed files."
    [[ $newer -eq 0 ]] || echo "A version bump follows ADR 0001: add the new path to SRC_PATHS in scripts/sync-bmm.sh."
  fi
}

main() {
  case "${1:-}" in
    sync) cmd_sync ;;
    check) cmd_check ;;
    verify) cmd_check offline ;;
    "" | -h | --help)
      # Print the header comment: every line after the shebang up to the
      # first line that is not a comment.
      awk 'NR == 1 { next } /^#/ { sub(/^# ?/, ""); print; next } { exit }' "$0"
      ;;
    *) die "unknown subcommand '$1' (want: sync | check | verify)" ;;
  esac
}

main "$@"
