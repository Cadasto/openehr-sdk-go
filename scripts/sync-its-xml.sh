#!/usr/bin/env bash
#
# sync-its-xml.sh — vendor / verify the openEHR ITS-XML schemas (XSD) into
# resources/its-xml/.
#
# Source: https://github.com/openEHR/specifications-ITS-XML
#         components/<COMPONENT>/<release>/**/*.xsd
#
# These are the openEHR Foundation's XML Schemas for the RM (canonical XML)
# and for the ADL 1.4 operational template. The SDK pins them as a reference
# contract, the way it pins the ITS-REST OpenAPI documents: no code is
# generated from them and no test reads them. The canonical XML codec and the
# OPT parser are written against them, so a reviewer checks an element,
# attribute or type here rather than guessing.
#
# Each file is vendored at its path below components/, so the relative
# xs:include locations between the schemas resolve inside resources/its-xml/.
# The set in SRC_PATHS is closed under those includes; `sync` refuses to write
# a set that is not.
#
# Subcommands:
#   sync     Fetch every file in SRC_PATHS at one commit, check the set is
#            closed under xs:include / xs:import, write the files
#            byte-identical, remove any *.xsd that SRC_PATHS no longer lists,
#            and regenerate MANIFEST.txt. The commit is ITS_XML_REF resolved
#            when set; otherwise the commit MANIFEST.txt already pins, so a
#            bare sync restores the pinned bytes and never moves the pin; with
#            no MANIFEST.txt either, the tip of master.
#   verify   Offline: recompute each file's sha256 and compare it to
#            MANIFEST.txt; fail on a mismatch, a file with no hash line, or an
#            *.xsd in resources/its-xml/ the manifest does not list. No
#            network, no curl/jq — run by `make ci`.
#   check    verify, then (best-effort, network) compare each pinned file with
#            the tip of master, and list any Release-* directory newer than a
#            pinned one of the same component. The network step never fails
#            the command.
#
# Environment:
#   ITS_XML_REF    upstream git ref to pin (branch / tag / sha). The release
#                  tags predate the current AM 1.4 schemas, so a re-pin is
#                  normally ITS_XML_REF=master.
#   GITHUB_TOKEN   optional; raises the unauthenticated API rate limit.
#
# Reproducibility: `sync` resolves the ref to a concrete commit sha and
# downloads every file at that one sha, so two syncs of the same ref are
# byte-identical. The resolved sha is recorded in MANIFEST.txt.
set -euo pipefail

readonly REPO="openEHR/specifications-ITS-XML"
readonly DEFAULT_BRANCH="master"
readonly SRC_ROOT="components"
# The pinned files, as paths below components/. RM Release-1.1.0 is the newest
# RM release upstream publishes schemas for (RM/latest differs from it only in
# line endings); AM Release-1.4 is the ADL 1.4 OPT schema and includes it;
# BASE/latest/BaseTypes.xsd is what the RM schemas include.
readonly SRC_PATHS=(
  "AM/Release-1.4/Archetype.xsd"
  "AM/Release-1.4/OpenehrProfile.xsd"
  "AM/Release-1.4/Resource.xsd"
  "AM/Release-1.4/Template.xsd"
  "BASE/latest/BaseTypes.xsd"
  "RM/Release-1.1.0/Common.xsd"
  "RM/Release-1.1.0/DataStructures.xsd"
  "RM/Release-1.1.0/DataTypes.xsd"
  "RM/Release-1.1.0/Demographic.xsd"
  "RM/Release-1.1.0/Ehr.xsd"
  "RM/Release-1.1.0/EhrExtract.xsd"
  "RM/Release-1.1.0/documents/Composition.xsd"
  "RM/Release-1.1.0/documents/Extract.xsd"
  "RM/Release-1.1.0/documents/Structure.xsd"
  "RM/Release-1.1.0/documents/Version.xsd"
)
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
readonly DEST="$ROOT/resources/its-xml"
readonly MANIFEST="$DEST/MANIFEST.txt"

# --- helpers ---------------------------------------------------------------

die() {
  echo "sync-its-xml: $*" >&2
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

# raw_url <commit> <path> — the raw.githubusercontent URL for a path below
# components/ at a commit.
raw_url() {
  echo "https://raw.githubusercontent.com/$REPO/$1/$SRC_ROOT/$2"
}

# normpath <path> — print a relative path with its `.` and `..` segments
# resolved, or fail when it climbs above its root.
normpath() {
  local -a out=()
  local seg
  IFS='/' read -r -a segs <<<"$1"
  for seg in "${segs[@]}"; do
    case "$seg" in
      "" | .) ;;
      ..)
        ((${#out[@]})) || return 1
        unset 'out[${#out[@]}-1]'
        ;;
      *) out+=("$seg") ;;
    esac
  done
  (IFS=/; echo "${out[*]}")
}

# release_of <dir name> — print x.y.z of Release-x.y.z, else nothing.
release_of() {
  sed -nE 's/^Release-([0-9]+(\.[0-9]+)*)$/\1/p' <<<"$1"
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
  if [[ -n "${ITS_XML_REF:-}" ]]; then
    ref="$ITS_XML_REF"
    echo "Resolving $REPO@$ref ..."
    commit="$(resolve_commit "$ref")"
  elif [[ -n "$(manifest_field commit)" ]]; then
    ref="$(manifest_field ref)"
    commit="$(manifest_field commit)"
    echo "No ITS_XML_REF — re-fetching the pinned commit (set ITS_XML_REF to move the pin) ..."
  else
    ref="$DEFAULT_BRANCH"
    echo "No ITS_XML_REF and no pin — resolving $REPO@$ref ..."
    commit="$(resolve_commit "$ref")"
  fi
  echo "Pinned commit: $commit"

  # Download every file into a stage first, and move them into place only
  # once all of them pass the guards below, so a failed or bogus fetch leaves
  # the vendored files and their manifest untouched.
  local stage
  stage="$(mktemp -d "$DEST/.stage.XXXXXX")"
  # shellcheck disable=SC2064 # expand $stage now, while it is set.
  trap "rm -rf -- '$stage'" EXIT

  local src
  for src in "${SRC_PATHS[@]}"; do
    echo "  fetch $SRC_ROOT/$src"
    mkdir -p "$stage/$(dirname "$src")"
    curl -fsSL --retry 3 --retry-all-errors "$(raw_url "$commit" "$src")" -o "$stage/$src" \
      || die "download failed: $src"
    # Guard against a truncated body or an error page that still returned
    # HTTP 200 — don't let a corrupt file become the canonical (hashed) copy.
    [[ -s "$stage/$src" ]] || die "download produced an empty file: $src"
    grep -q '<xs:schema' "$stage/$src" || die "download is not an XML Schema (no xs:schema): $src"
  done

  # The set must be closed under its includes, or a reader of one schema
  # meets a reference this pin does not carry.
  local loc target
  for src in "${SRC_PATHS[@]}"; do
    while IFS= read -r loc; do
      [[ -n "$loc" ]] || continue
      [[ "$loc" != *://* ]] || die "$src includes the remote $loc; pin only local includes"
      target="$(normpath "$(dirname "$src")/$loc")" || die "$src includes $loc, outside components/"
      [[ -f "$stage/$target" ]] \
        || die "$src includes $target, which SRC_PATHS does not list; add it so the set stays closed"
    done < <(grep -o -E '<xs:(include|import)[^>]*schemaLocation="[^"]*"' "$stage/$src" \
      | sed -E 's/.*schemaLocation="([^"]*)"/\1/')
  done

  for src in "${SRC_PATHS[@]}"; do
    mkdir -p "$DEST/$(dirname "$src")"
    mv -f "$stage/$src" "$DEST/$src"
  done

  # Drop schemas no longer in SRC_PATHS, then any directory they leave empty.
  local f rel keep
  while IFS= read -r -d '' f; do
    rel="${f#"$DEST/"}"
    keep=""
    for src in "${SRC_PATHS[@]}"; do
      [[ "$src" == "$rel" ]] && keep=1
    done
    if [[ -z "$keep" ]]; then
      echo "  remove $rel (not in SRC_PATHS)"
      rm -f "$f"
    fi
  done < <(find "$DEST" -name '*.xsd' -type f -not -path "$stage/*" -print0)
  find "$DEST" -mindepth 1 -type d -empty -not -path "$stage" -delete

  # Keep fetched_utc while the commit is unchanged, so a re-sync at the pin
  # rewrites a byte-identical manifest; only a re-pin moves it.
  local fetched=""
  if [[ "$(manifest_field commit)" == "$commit" ]]; then
    fetched="$(manifest_field fetched_utc)"
  fi
  [[ -n "$fetched" ]] || fetched="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  # Regenerate the manifest (provenance + integrity hashes). Hash paths are
  # relative to $DEST, which mirrors components/.
  {
    echo "# openEHR ITS-XML schemas — sync manifest"
    echo "# Generated by scripts/sync-its-xml.sh — do not edit by hand."
    echo "source_repo: $REPO"
    echo "ref: $ref"
    echo "commit: $commit"
    echo "fetched_utc: $fetched"
    echo "source_tree: https://github.com/$REPO/tree/$commit/$SRC_ROOT"
    echo "source_root: $SRC_ROOT"
    echo "#"
    echo "# sha256  path"
    for src in "${SRC_PATHS[@]}"; do
      echo "$(sha256_of "$DEST/$src")  $src"
    done
  } >"$MANIFEST"

  echo "Synced ${#SRC_PATHS[@]} files → resources/its-xml/ ($ref, commit ${commit:0:12})"
}

# cmd_check [offline] — offline integrity first, then (unless 'offline') a
# best-effort upstream-drift report.
cmd_check() {
  [[ -f "$MANIFEST" ]] || die "no MANIFEST.txt — run 'make its-xml-sync' first"

  # 1. Offline integrity: every vendored file must match its manifest hash.
  local hash_block
  hash_block="$(sed -n '/^# sha256  path/,$p' "$MANIFEST" | tail -n +2)"
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
  # resources/its-xml/ must be one of them. Paths are compared literally,
  # never as a regex.
  local src f rel found
  for src in "${SRC_PATHS[@]}"; do
    found=""
    while IFS= read -r line; do
      if [[ "${line#*  }" == "$src" ]]; then
        found=1
        break
      fi
    done <<<"$hash_block"
    if [[ -z "$found" ]]; then
      echo "  NOT PINNED: $src (no sha256 line in MANIFEST.txt)"
      rc=1
    fi
  done
  while IFS= read -r -d '' f; do
    rel="${f#"$DEST/"}"
    found=""
    while IFS= read -r line; do
      if [[ "${line#*  }" == "$rel" ]]; then
        found=1
        break
      fi
    done <<<"$hash_block"
    if [[ -z "$found" ]]; then
      echo "  UNTRACKED: $rel (in resources/its-xml/ but not in MANIFEST.txt)"
      rc=1
    fi
  done < <(find "$DEST" -name '*.xsd' -type f -print0)

  if [[ $rc -ne 0 ]]; then
    die "integrity check failed — never hand-edit the pin; run 'make its-xml-sync' to restore it"
  fi
  echo "its-xml-check: OK"

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

  # Compare the files themselves, not the commits: the git blob sha the
  # contents API lists for each name against `git hash-object` of the
  # vendored copy, one listing per pinned directory.
  local dir listing name ours theirs changed=0 newer=0
  local -A seen=()
  for src in "${SRC_PATHS[@]}"; do
    dir="$(dirname "$src")"
    [[ -z "${seen[$dir]:-}" ]] || continue
    seen[$dir]=1
    if ! listing="$(api "contents/$SRC_ROOT/$dir?ref=$head" 2>/dev/null)"; then
      echo "  GONE upstream: $SRC_ROOT/$dir"
      changed=1
      continue
    fi
    for f in "${SRC_PATHS[@]}"; do
      [[ "$(dirname "$f")" == "$dir" ]] || continue
      name="$(basename "$f")"
      ours="$(git hash-object --no-filters "$DEST/$f")"
      theirs="$(jq -r --arg n "$name" '.[] | select(.name == $n) | .sha' <<<"$listing")"
      if [[ -z "$theirs" ]]; then
        echo "  GONE upstream: $SRC_ROOT/$f"
        changed=1
      elif [[ "$theirs" != "$ours" ]]; then
        echo "  CHANGED upstream: $SRC_ROOT/$f"
        changed=1
      fi
    done
  done

  # A newer release is a Release-* directory of a component above the one
  # pinned, the signal that, say, RM schemas for a later release exist.
  local comp rel_dir pinned_v v
  local -A comps=()
  for src in "${SRC_PATHS[@]}"; do
    comp="${src%%/*}"
    rel_dir="${src#*/}"
    rel_dir="${rel_dir%%/*}"
    v="$(release_of "$rel_dir")"
    [[ -n "$v" ]] || continue
    comps[$comp]="$v"
  done
  for comp in "${!comps[@]}"; do
    pinned_v="${comps[$comp]}"
    listing="$(api "contents/$SRC_ROOT/$comp?ref=$head" 2>/dev/null)" || continue
    while IFS= read -r name; do
      v="$(release_of "$name")"
      [[ -n "$v" && "$v" != "$pinned_v" ]] || continue
      if [[ "$(printf '%s\n%s\n' "$pinned_v" "$v" | sort -V | tail -n 1)" == "$v" ]]; then
        echo "  NEWER $comp release upstream: $SRC_ROOT/$comp/$name (pinned: Release-$pinned_v)"
        newer=1
      fi
    done < <(jq -r '.[] | select(.type == "dir") | .name' <<<"$listing")
  done

  if [[ $changed -eq 0 && $newer -eq 0 ]]; then
    echo "Every pinned schema is byte-identical at $REPO@$DEFAULT_BRANCH; no newer release."
  else
    [[ $changed -eq 0 ]] || echo "Run 'ITS_XML_REF=$DEFAULT_BRANCH make its-xml-sync' to take the changed files."
    [[ $newer -eq 0 ]] || echo "To move to a newer release, edit SRC_PATHS in scripts/sync-its-xml.sh."
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
