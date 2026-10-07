#!/usr/bin/env bash
#
# sync-terminology.sh — vendor / verify the openEHR Terminology
# (openehr_terminology.xml and openehr_external_terminologies.xml) into
# resources/terminology/ (REQ-034).
#
# Source: https://github.com/openEHR/specifications-TERM
#         computable/XML/en/openehr_terminology.xml
#         computable/XML/openehr_external_terminologies.xml
#
# These are the openEHR Foundation's computable form of the openEHR
# Terminology. openehr_terminology.xml holds the `openehr` terminology
# *groups* (audit change type, version lifecycle state, setting, participation
# mode, composition category, …) and its own *code sets* (normal statuses,
# compression algorithms, integrity check algorithms) that the RM's own
# invariants reference. openehr_external_terminologies.xml holds the
# Foundation's snapshot of the external code sets the RM names (ISO 639-1
# languages, ISO 3166-1 countries, IANA character sets, IANA media types).
# Both are small, closed, and versioned with the specifications — so the SDK
# pins them in-tree and generates from them the way it does the BMM
# (REQ-034), rather than fetching them at build time or treating them as a
# runtime terminology-service lookup.
#
# Subcommands:
#   sync     Resolve TERMINOLOGY_REF (default: the ref pinned in MANIFEST.txt,
#            else the latest GitHub release tag) to a commit sha, download
#            both files at that one sha, write them byte-identical, regenerate
#            MANIFEST.txt. Then run `make termgen` so the generated tables
#            follow the pin (skipped when TERMINOLOGY_SKIP_GEN=1).
#   verify   Offline: recompute each file's sha256 and compare it to
#            MANIFEST.txt. No network, no curl/jq — run by `make ci`.
#   check    verify, then (best-effort, network) compare the pinned commit
#            with the latest release tag's commit and say if a sync is due.
#            The network step never fails the command.
#
# Environment:
#   TERMINOLOGY_REF        upstream git ref to pin (tag / branch / sha).
#   TERMINOLOGY_SKIP_GEN   set to 1 to skip `make termgen` after a sync.
#   GITHUB_TOKEN           optional; raises the unauthenticated API rate limit.
#
# Reproducibility: `sync` resolves the ref to a concrete commit sha and
# downloads both files at that sha, so two syncs of the same ref are
# byte-identical and the two files always come from the same commit. The
# resolved sha is recorded in MANIFEST.txt.
set -euo pipefail

readonly REPO="openEHR/specifications-TERM"
# The pinned files, as paths from the upstream repository root. Each one is
# vendored under its base name, and all of them come from one resolved commit.
readonly SRC_PATHS=(
  "computable/XML/en/openehr_terminology.xml"
  "computable/XML/openehr_external_terminologies.xml"
)
# The upstream directory both files live under, for the manifest's tree link.
readonly SRC_TREE="computable/XML"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
readonly DEST="$ROOT/resources/terminology"
readonly MANIFEST="$DEST/MANIFEST.txt"

# --- helpers ---------------------------------------------------------------

die() {
  echo "sync-terminology: $*" >&2
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

# pinned_ref — print the ref recorded in MANIFEST.txt (empty when unpinned).
pinned_ref() {
  [[ -f "$MANIFEST" ]] || return 0
  awk '/^ref: /{print $2; exit}' "$MANIFEST"
}

# pinned_commit — print the commit recorded in MANIFEST.txt.
pinned_commit() {
  [[ -f "$MANIFEST" ]] || return 0
  awk '/^commit: /{print $2; exit}' "$MANIFEST"
}

# latest_release_tag — print the tag name of the repo's latest GitHub release.
latest_release_tag() {
  api "releases/latest" | jq -r '.tag_name // empty' | grep -E '.' \
    || die "could not determine the latest release of $REPO"
}

# resolve_commit <ref> — print the concrete commit sha a ref points at.
# Tags go through git/ref/tags/<ref>: a lightweight tag's object.sha is already
# the commit; an annotated tag's object is a tag object, which git/tags/<sha>
# dereferences to the commit. Anything else (branch, sha) goes through
# commits/<ref>, which accepts both.
resolve_commit() {
  local ref="$1" ref_json="" obj_type="" obj_sha=""
  if ref_json="$(api "git/ref/tags/$ref" 2>/dev/null)"; then
    obj_type="$(jq -r '.object.type // empty' <<<"$ref_json")"
    obj_sha="$(jq -r '.object.sha // empty' <<<"$ref_json")"
    case "$obj_type" in
      commit)
        grep -E '^[0-9a-f]{40}$' <<<"$obj_sha" \
          || die "tag '$ref' in $REPO resolved to a malformed sha"
        return 0
        ;;
      tag)
        api "git/tags/$obj_sha" | jq -r '.object.sha // empty' | grep -E '^[0-9a-f]{40}$' \
          || die "could not dereference annotated tag '$ref' in $REPO"
        return 0
        ;;
    esac
  fi
  api "commits/$ref" | jq -r '.sha // empty' | grep -E '^[0-9a-f]{40}$' \
    || die "could not resolve ref '$ref' in $REPO"
}

# raw_url <commit> <path> — the raw.githubusercontent URL for an upstream
# path at a commit.
raw_url() {
  echo "https://raw.githubusercontent.com/$REPO/$1/$2"
}

require_tools() {
  command -v curl >/dev/null || die "curl is required"
  command -v jq >/dev/null || die "jq is required"
}

# --- subcommands -----------------------------------------------------------

cmd_sync() {
  require_tools
  mkdir -p "$DEST"

  local ref
  ref="${TERMINOLOGY_REF:-$(pinned_ref)}"
  if [[ -z "$ref" ]]; then
    echo "No TERMINOLOGY_REF and no pinned ref — using the latest release of $REPO ..."
    ref="$(latest_release_tag)"
  fi

  echo "Resolving $REPO@$ref ..."
  local commit
  commit="$(resolve_commit "$ref")"
  echo "Pinned commit: $commit"

  # Download every file beside the pin first, and move them into place only
  # once all of them pass the guards below. A failed or bogus fetch (a ref
  # whose tree lacks one of the files, say) then leaves the vendored pair and
  # its manifest untouched: the two files only ever move together.
  local src name tmp head_bytes
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
    head_bytes="$(head -c 64 "$tmp")"
    [[ "$head_bytes" == *"<terminology"* ]] \
      || { rm -f "${tmps[@]}"; die "download is not openEHR Terminology XML (no <terminology root): $src"; }
  done
  for src in "${SRC_PATHS[@]}"; do
    name="$(basename "$src")"
    mv -f "$DEST/.$name.tmp" "$DEST/$name"
  done

  # Regenerate the manifest (provenance + integrity hashes). The hash paths
  # are relative to $DEST so the hash block reads as plain `sha256  filename`.
  {
    echo "# openEHR Terminology — sync manifest"
    echo "# Generated by scripts/sync-terminology.sh — do not edit by hand."
    echo "source_repo: $REPO"
    echo "ref: $ref"
    echo "commit: $commit"
    echo "fetched_utc: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "source_tree: https://github.com/$REPO/tree/$commit/$SRC_TREE"
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

  echo "Synced ${#SRC_PATHS[@]} files → resources/terminology/ ($ref, commit ${commit:0:12})"

  # Keep the generated tables in step with the pin (REQ-034 / the REQ-042 rule).
  if [[ "${TERMINOLOGY_SKIP_GEN:-}" == "1" ]]; then
    echo "(TERMINOLOGY_SKIP_GEN=1 — skipped 'make termgen')"
    return 0
  fi
  echo "Regenerating the terminology accessor ('make termgen') ..."
  make -C "$ROOT" termgen
}

# cmd_check [offline] — offline integrity first, then (unless 'offline') a
# best-effort upstream-drift report.
cmd_check() {
  [[ -f "$MANIFEST" ]] || die "no MANIFEST.txt — run 'make terminology-sync' first"

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

  # Every file the pin is made of must have a hash line. A manifest that lost
  # one would otherwise pass this check with that file never hashed.
  local src want_name found
  for src in "${SRC_PATHS[@]}"; do
    want_name="$(basename "$src")"
    found=""
    while IFS= read -r line; do
      if [[ "${line#*  }" == "$want_name" ]]; then
        found=1
        break
      fi
    done <<<"$hash_block"
    if [[ -z "$found" ]]; then
      echo "  NOT PINNED: $want_name (no sha256 line in MANIFEST.txt)"
      rc=1
    fi
  done

  # Detect vendored files absent from the manifest (extras). Compare each
  # file name against the manifest's exact path set — never as a regex, so a
  # file named like `o.*.xml` cannot pass by matching a real manifest row.
  local f name line tracked
  for f in "$DEST"/*.xml; do
    [[ -e "$f" ]] || continue
    name="$(basename "$f")"
    tracked=""
    while IFS= read -r line; do
      [[ -n "$line" ]] || continue
      # Each manifest line is `<sha>  <path>`; strip up to the two-space
      # separator and compare the path literally.
      if [[ "${line#*  }" == "$name" ]]; then
        tracked=1
        break
      fi
    done <<<"$hash_block"
    if [[ -z "$tracked" ]]; then
      echo "  UNTRACKED: $name (in resources/terminology/ but not in MANIFEST.txt)"
      rc=1
    fi
  done

  if [[ $rc -ne 0 ]]; then
    die "integrity check failed — never hand-edit the pin; run 'make terminology-sync' to refresh"
  fi
  echo "terminology-verify: OK"

  # 2. Upstream drift (best-effort; never fails the command). Do NOT
  # require_tools here: the integrity check above is fully offline, so a host
  # without curl/jq must still be able to verify the pin — and `verify`, which
  # `make ci` runs, shares this path.
  if [[ "${1:-}" == "offline" ]]; then
    return 0
  fi
  if ! command -v curl >/dev/null 2>&1 || ! command -v jq >/dev/null 2>&1; then
    echo "(curl/jq unavailable — skipped upstream drift check)"
    return 0
  fi

  local ref pinned latest upstream
  ref="$(pinned_ref)"
  pinned="$(pinned_commit)"
  if ! latest="$(latest_release_tag 2>/dev/null)"; then
    echo "(offline or rate-limited — skipped upstream drift check)"
    return 0
  fi
  if ! upstream="$(resolve_commit "$latest" 2>/dev/null)"; then
    echo "(could not resolve $REPO@$latest — skipped upstream drift check)"
    return 0
  fi
  if [[ "$pinned" != "$upstream" ]]; then
    echo "Latest release of $REPO is $latest (${upstream:0:12}); pinned: $ref (${pinned:0:12})"
    echo "Run 'TERMINOLOGY_REF=$latest make terminology-sync' to update the pin."
  else
    echo "Up to date with the latest release $REPO@$latest (${pinned:0:12})."
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
