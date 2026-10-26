#!/usr/bin/env bash
#
# sync-its-xml-schemas.sh: ingest / check / sync the openEHR ITS-XML schemas
# into resources/its-xml/.
#
# Source: https://github.com/openEHR/specifications-ITS-XML
#         components/RM/Release-1.1.0/*.xsd
#         components/BASE/Release-1.1.0/*.xsd
#         components/AM/Release-1.4/Archetype.xsd
#         components/AM/Release-1.4/OpenehrProfile.xsd
#         and every file those reach through xs:include, xs:import or
#         xs:redefine, followed transitively from the files themselves.
#
# The BMM code generator reads these schemas for one thing: the xs:sequence
# of each class, which fixes the order of the children the canonical-XML
# encoders write (ADR 0002 D10). Every file keeps its upstream path under
# resources/its-xml/, so each schemaLocation still resolves in the vendored
# tree. A "*" in a root pattern matches within its own directory only: the
# documents/ subdirectories are not roots.
#
# Subcommands:
#   sync     Resolve ITS_XML_REF (default: master) to a commit sha, download
#            the roots and their include closure at that one sha into a
#            staging directory, then replace the vendored tree with it and
#            regenerate MANIFEST.txt (pinned commit + per-file sha256).
#            Removes vendored files the new closure no longer reaches.
#   ingest   Alias for sync (first-time population).
#   check    Verify the vendored copies are intact and current:
#              1. offline: recompute each sha256 and compare it to the
#                 manifest (detects local edits and corruption); fails on
#                 a mismatch, a missing file or an untracked file.
#              2. online (best-effort): compare the pinned commit with the
#                 current upstream commit of the ref, and say whether any
#                 vendored file differs there. Never fails the command.
#
# Environment:
#   ITS_XML_REF    upstream git ref to pin (branch / tag / sha). Default: master.
#   GITHUB_TOKEN   optional; raises the unauthenticated GitHub API rate limit.
#
# Reproducibility: `sync` resolves ITS_XML_REF to a concrete commit sha and
# downloads every file at that sha, so two syncs of the same ref are
# byte-identical. The resolved sha is recorded in MANIFEST.txt.
set -euo pipefail

readonly REPO="openEHR/specifications-ITS-XML"
readonly REF="${ITS_XML_REF:-master}"
# The root schemas, as patterns over upstream paths from the repository root.
readonly ROOT_GLOBS=(
  "components/RM/Release-1.1.0/*.xsd"
  "components/BASE/Release-1.1.0/*.xsd"
  "components/AM/Release-1.4/Archetype.xsd"
  "components/AM/Release-1.4/OpenehrProfile.xsd"
)

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
readonly DEST="$ROOT/resources/its-xml"
readonly MANIFEST="$DEST/MANIFEST.txt"
# The files in DEST that the script does not vendor.
readonly OWN_FILES=("README.md" "MANIFEST.txt")
readonly HASH_HEADER="# sha256  path"

# --- helpers ---------------------------------------------------------------

die() {
  echo "sync-its-xml-schemas: $*" >&2
  exit 1
}

# api <path>: GET the GitHub REST API, honouring GITHUB_TOKEN when set.
# --retry survives transient network blips and secondary rate limits.
api() {
  local url="https://api.github.com/repos/$REPO/$1"
  if [[ -n "${GITHUB_TOKEN:-}" ]]; then
    curl -fsSL --retry 3 --retry-all-errors -H "Authorization: Bearer $GITHUB_TOKEN" "$url"
  else
    curl -fsSL --retry 3 --retry-all-errors "$url"
  fi
}

# sha256_of <file>: print the bare sha256 hash of a file.
sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

# resolve_commit <ref>: print the concrete commit sha for a ref.
resolve_commit() {
  api "commits/$1" | jq -r '.sha' | grep -E '^[0-9a-f]{40}$' \
    || die "could not resolve ref '$1' in $REPO"
}

# list_tree <commit>: print "<blob sha> <path>" for every file in the
# upstream repository at the commit, one per line.
list_tree() {
  local json
  json="$(api "git/trees/$1?recursive=1")" || die "could not list the tree of $REPO at $1"
  [[ "$(jq -r '.truncated' <<<"$json")" == "false" ]] \
    || die "the tree listing of $REPO at $1 is truncated"
  jq -r '.tree[] | select(.type == "blob") | "\(.sha) \(.path)"' <<<"$json"
}

# matches_root <path> <glob>: true when the path matches the root pattern,
# with the pattern's "*" matching inside the pattern's own directory only.
matches_root() {
  # The right-hand side of == is left unquoted so that it is a pattern.
  # shellcheck disable=SC2053
  [[ "${1%/*}" == "${2%/*}" && "${1##*/}" == ${2##*/} ]]
}

# schema_locations <file>: print the schemaLocation of every xs:include,
# xs:import and xs:redefine in an XML schema, one per line. Comments are
# removed first, so a commented-out include is not followed.
schema_locations() {
  tr '\n\r' '  ' <"$1" \
    | sed -E 's/<!--([^-]|-[^-])*-->//g' \
    | grep -oE '<([A-Za-z_][A-Za-z0-9_.-]*:)?(include|import|redefine)[[:space:]][^>]*>' \
    | grep -oE "schemaLocation[[:space:]]*=[[:space:]]*(\"[^\"]*\"|'[^']*')" \
    | sed -E "s/^schemaLocation[[:space:]]*=[[:space:]]*[\"']//; s/[\"']\$//" \
    || true
}

# normalise <path>: collapse the "." and ".." segments of a relative path.
# Fails when a ".." climbs above the repository root.
normalise() {
  local IFS=/ part
  local -a parts out=()
  read -ra parts <<<"$1"
  for part in "${parts[@]}"; do
    case "$part" in
      "" | .) ;;
      ..)
        ((${#out[@]} > 0)) || return 1
        unset 'out[${#out[@]}-1]'
        ;;
      *) out+=("$part") ;;
    esac
  done
  echo "${out[*]}"
}

# vendored_files: print the path of every file under DEST, relative to it,
# except the files the script does not vendor, sorted.
vendored_files() {
  local f rel own skip
  [[ -d "$DEST" ]] || return 0
  while IFS= read -r f; do
    rel="${f#"$DEST"/}"
    skip=0
    for own in "${OWN_FILES[@]}"; do
      [[ "$rel" == "$own" ]] && skip=1
    done
    ((skip)) || echo "$rel"
  done < <(find "$DEST" -type f) | LC_ALL=C sort
}

# hash_block: print the "<sha256>  <path>" lines of the manifest.
hash_block() {
  sed -n "/^$HASH_HEADER\$/,\$p" "$MANIFEST" | tail -n +2
}

require_tools() {
  command -v curl >/dev/null || die "curl is required"
  command -v jq >/dev/null || die "jq is required"
}

# --- subcommands -----------------------------------------------------------

cmd_sync() {
  require_tools

  echo "Resolving $REPO@$REF ..."
  local commit
  commit="$(resolve_commit "$REF")"
  echo "Pinned commit: $commit"

  local tree paths
  tree="$(list_tree "$commit")"
  paths="$(cut -d' ' -f2- <<<"$tree")"

  # The roots: every upstream path one of ROOT_GLOBS matches. Each pattern
  # must match at least one file, or the upstream layout has changed.
  local -a queue=()
  local glob path found
  for glob in "${ROOT_GLOBS[@]}"; do
    found=0
    while IFS= read -r path; do
      if matches_root "$path" "$glob"; then
        queue+=("$path")
        found=1
      fi
    done <<<"$paths"
    ((found)) || die "no file matches '$glob' in $REPO at $commit"
  done

  local staging
  staging="$(mktemp -d)"
  # shellcheck disable=SC2064
  trap "rm -rf '$staging'" EXIT

  # Download the roots and follow their includes, breadth first, until no
  # new file is reached. Every file comes from the one pinned commit.
  local i=0 seen="" loc dep
  while ((i < ${#queue[@]})); do
    path="${queue[i]}"
    i=$((i + 1))
    grep -qxF "$path" <<<"$seen" && continue
    seen+="$path"$'\n'

    echo "  fetch $path"
    mkdir -p "$staging/${path%/*}"
    curl -fsSL --retry 3 --retry-all-errors \
      "https://raw.githubusercontent.com/$REPO/$commit/$path" -o "$staging/$path" \
      || die "download failed: $path"
    # Guard against a truncated or empty body that still returned HTTP 200:
    # a corrupt file must not become the canonical (hashed) copy.
    [[ -s "$staging/$path" ]] || die "download produced an empty file: $path"

    while IFS= read -r loc; do
      [[ -n "$loc" ]] || continue
      case "$loc" in
        *://* | /*) die "$path names '$loc', which is not a path in $REPO" ;;
      esac
      dep="$(normalise "${path%/*}/$loc")" \
        || die "$path names '$loc', which climbs above the root of $REPO"
      grep -qxF "$dep" <<<"$paths" \
        || die "$path names '$loc' ($dep), which is absent from $REPO at $commit"
      queue+=("$dep")
    done < <(schema_locations "$staging/$path")
  done

  local closure
  closure="$(LC_ALL=C sort <<<"${seen%$'\n'}")"

  # Replace the vendored tree: drop the files the closure no longer
  # reaches, then copy the new closure in.
  mkdir -p "$DEST"
  local existing
  while IFS= read -r existing; do
    [[ -n "$existing" ]] || continue
    if ! grep -qxF "$existing" <<<"$closure"; then
      echo "  remove stale $existing"
      rm -f "$DEST/$existing"
    fi
  done < <(vendored_files)
  find "$DEST" -mindepth 1 -type d -empty -delete
  while IFS= read -r path; do
    mkdir -p "$DEST/${path%/*}"
    cp "$staging/$path" "$DEST/$path"
  done <<<"$closure"

  # Regenerate the manifest (provenance + per-file integrity hashes).
  {
    echo "# openEHR ITS-XML schemas: sync manifest"
    echo "# Generated by scripts/sync-its-xml-schemas.sh; do not edit by hand."
    echo "source_repo: $REPO"
    echo "roots: ${ROOT_GLOBS[*]}"
    echo "closure: transitive xs:include, xs:import and xs:redefine schemaLocation"
    echo "ref: $REF"
    echo "commit: $commit"
    echo "fetched_utc: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "source_tree: https://github.com/$REPO/tree/$commit/components"
    echo "#"
    echo "$HASH_HEADER"
    while IFS= read -r path; do
      echo "$(sha256_of "$DEST/$path")  $path"
    done <<<"$closure"
  } >"$MANIFEST"

  echo "Synced $(wc -l <<<"$closure" | tr -d ' ') schema(s) to resources/its-xml/ (commit ${commit:0:12})"
}

cmd_check() {
  [[ -f "$MANIFEST" ]] || die "no MANIFEST.txt; run 'make its-xml-sync' first"

  # 1. Offline integrity: local files must match the manifest hashes.
  echo "Checking vendored schema integrity against MANIFEST.txt ..."
  local hashes
  hashes="$(hash_block)"
  [[ -n "$hashes" ]] || die "manifest has no hash block"

  local rc=0 want path got
  while read -r want path; do
    [[ -n "$path" ]] || continue
    if [[ ! -f "$DEST/$path" ]]; then
      echo "  MISSING: $path"
      rc=1
      continue
    fi
    got="$(sha256_of "$DEST/$path")"
    if [[ "$got" != "$want" ]]; then
      echo "  MODIFIED: $path"
      rc=1
    fi
  done <<<"$hashes"

  # Files in resources/its-xml/ that the manifest does not list.
  local listed
  listed="$(awk '{ print $2 }' <<<"$hashes")"
  while IFS= read -r path; do
    [[ -n "$path" ]] || continue
    if ! grep -qxF "$path" <<<"$listed"; then
      echo "  UNTRACKED: $path (in resources/its-xml/ but not in MANIFEST.txt)"
      rc=1
    fi
  done < <(vendored_files)

  if [[ $rc -ne 0 ]]; then
    die "integrity check failed; run 'make its-xml-sync' to refresh"
  fi
  echo "Integrity OK ($(wc -l <<<"$listed" | tr -d ' ') schema(s))."

  # 2. Online staleness (best-effort; never fails the command).
  require_tools
  local pinned upstream
  pinned="$(awk '/^commit: /{ print $2; exit }' "$MANIFEST")"
  if ! upstream="$(resolve_commit "$REF" 2>/dev/null)"; then
    echo "(offline: skipped the upstream staleness check)"
    return 0
  fi
  if [[ "$pinned" == "$upstream" ]]; then
    echo "Up to date with $REPO@$REF (${pinned:0:12})."
    return 0
  fi
  echo "Upstream $REPO@$REF advanced: ${pinned:0:12} -> ${upstream:0:12}"

  # Compare each vendored file's git blob id with the one upstream holds at
  # the new commit, so a commit that touched only other files says so.
  local tree changed=0 blob
  if command -v git >/dev/null 2>&1 && tree="$(list_tree "$upstream" 2>/dev/null)"; then
    while IFS= read -r path; do
      [[ -n "$path" ]] || continue
      blob="$(git hash-object --no-filters "$DEST/$path")"
      grep -qxF "$blob $path" <<<"$tree" || changed=$((changed + 1))
    done <<<"$listed"
    if ((changed == 0)); then
      echo "No vendored file changed upstream; a sync would move only the pin."
    else
      echo "$changed vendored file(s) changed or moved upstream."
    fi
  fi
  echo "Run 'make its-xml-sync' to update the vendored schemas."
}

main() {
  case "${1:-}" in
    sync | ingest) cmd_sync ;;
    check) cmd_check ;;
    "" | -h | --help)
      awk 'NR == 1 { next } /^#/ { sub(/^# ?/, ""); print; next } { exit }' "$0"
      ;;
    *) die "unknown subcommand '$1' (want: sync | ingest | check)" ;;
  esac
}

main "$@"
