#!/usr/bin/env bash
#
# ingest-reference-content.sh: vendor and verify the reference operational
# templates of Cadasto/openehr-reference-content, under
# testkit/corpus/reference/.
#
# Every operational template of that repository, tech/templates/<id>/opt/
# <id>.opt, is copied byte for byte to templates/<id>.opt: the generic
# EHR_STATUS and FOLDER templates and the demographic ones. They are OPT
# only; the archetypes they are built from stay in their repository. The
# whole-corpus tests read them through fixtures.ListAllOPTs. `ingest`
# refuses a file whose template id is not its folder's name.
#
# Source (CC-BY-SA-4.0; attribution in testkit/corpus/THIRD_PARTY_LICENSES.md),
# read from a local clone that `ingest` requires:
#   reference  https://github.com/Cadasto/openehr-reference-content
#              clone: the directory in $REFERENCE_CONTENT_CLONE
#
# The pin, the Cadasto/openehr-reference-content line of
# testkit/corpus/UPSTREAM_PINS.txt, is a released tag's commit. `ingest`
# reads every file from the clone's objects at the pin, whatever the clone
# has checked out, so vendoring from another commit cannot happen silently.
# Re-pinning means editing that line, in the same commit as the refreshed
# files and manifest.
#
# Subcommands:
#   ingest   Copy every template at the pin, check each one's template id,
#            and regenerate testkit/corpus/reference/MANIFEST.txt. Everything
#            is staged first, so a refusal leaves the vendored tree untouched.
#            Files no longer at the pin are removed.
#   verify   Offline integrity: fail unless MANIFEST.txt holds one source
#            record naming an https repository, a full commit and the
#            CC-BY-SA-4.0 licence; recompute the sha256 of every `file` record
#            and fail on a mismatch, a missing file, or a file in the subtree
#            that the manifest does not list. Needs no clone and no network
#            (sha256sum or shasum only). The Go tests in
#            testkit/fixtures/reference_content_test.go run it under `make test`.
#
# MANIFEST.txt is tab-separated, one record per line:
#   source <name> <repo URL> <commit> <licence>
#   file   templates/<template id>.opt <source name> <upstream path> <sha256>
#
# Idempotence: re-running `ingest` at the same pin leaves `git status` clean:
# the files are byte copies, and the manifest records the pin's tag, not a
# clock reading.
set -euo pipefail

readonly REPO_URL="https://github.com/Cadasto/openehr-reference-content"
readonly LICENCE="CC-BY-SA-4.0"
readonly SOURCE="reference"
# Set from UPSTREAM_PINS.txt by cmd_ingest; `verify` needs no pin.
PIN=""

CLONE="${REFERENCE_CONTENT_CLONE:-}"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
readonly CORPUS="$ROOT/testkit/corpus"
readonly DEST="$CORPUS/reference"
readonly MANIFEST="$DEST/MANIFEST.txt"
readonly PINS="$CORPUS/UPSTREAM_PINS.txt"

# --- helpers ---------------------------------------------------------------

die() {
  echo "ingest-reference-content: $*" >&2
  exit 1
}

# sha256_of <file>: print the bare sha256 hash of a file.
sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -- "$1" | cut -d' ' -f1
  else
    shasum -a 256 -- "$1" | cut -d' ' -f1
  fi
}

require_hasher() {
  command -v sha256sum >/dev/null 2>&1 || command -v shasum >/dev/null 2>&1 \
    || die "sha256sum or shasum is required"
}

# upstream_pin: print the commit UPSTREAM_PINS.txt pins for the repository,
# or fail when it has no full commit.
upstream_pin() {
  local repo="${REPO_URL#https://github.com/}" pin
  pin="$(awk -v r="$repo" '!/^[[:space:]]*#/ && $1 == r { print $3; exit }' "$PINS" 2>/dev/null || true)"
  [[ "$pin" =~ ^[0-9a-f]{40}$ ]] || die "no full commit for $repo in testkit/corpus/UPSTREAM_PINS.txt"
  echo "$pin"
}

# check_clone: refuse unless the clone exists and holds the pinned commit.
check_clone() {
  [[ -n "$CLONE" ]] || die "set REFERENCE_CONTENT_CLONE to a local clone of $REPO_URL"
  [[ -d "$CLONE" ]] || die "clone not found at $CLONE (set REFERENCE_CONTENT_CLONE)"
  git -C "$CLONE" rev-parse --git-dir >/dev/null 2>&1 \
    || die "clone $CLONE is not a git checkout; refusing to vendor without provenance"
  git -C "$CLONE" cat-file -e "$PIN^{commit}" 2>/dev/null \
    || die "clone $CLONE does not hold the pin ${PIN:0:12}; fetch it, or update UPSTREAM_PINS.txt when re-pinning"
}

# template_id_of <file>: print the value of the first <template_id> in an
# operational template, line breaks set aside, or nothing when it has none.
template_id_of() {
  tr -d '\r\n' <"$1" \
    | { grep -o '<template_id>[[:space:]]*<value>[^<]*</value>' || true; } \
    | sed -n '1{s|^.*<value>||;s|</value>$||;p;}'
}

# --- subcommands -----------------------------------------------------------

cmd_ingest() {
  require_hasher
  PIN="$(upstream_pin)"
  check_clone

  local tag
  tag="$(git -C "$CLONE" tag --points-at "$PIN" | sort | tail -n 1)"
  [[ -n "$tag" ]] || die "the pin ${PIN:0:12} is not a released tag's commit"

  local stage
  stage="$(mktemp -d "$CORPUS/.reference.XXXXXX")"
  # shellcheck disable=SC2064 # expand $stage now, while it is set.
  trap "rm -rf -- '$stage'" EXIT
  mkdir -p -- "$stage/templates"

  local body="" line upath id rel got found n=0
  while IFS= read -r upath; do
    [[ "$upath" =~ ^tech/templates/([^/]+)/opt/([^/]+)\.opt$ ]] || continue
    id="${BASH_REMATCH[1]}"
    [[ "${BASH_REMATCH[2]}" == "$id" ]] || die "$upath is not named after its folder"
    rel="templates/$id.opt"
    git -C "$CLONE" cat-file blob "$PIN:$upath" >"$stage/$rel"
    got="$(sha256_of "$stage/$rel")"
    found="$(template_id_of "$stage/$rel")"
    [[ "$found" == "$id" ]] || die "$upath holds template id $found, not its folder's name $id"
    printf -v line 'file\t%s\t%s\t%s\t%s\n' "$rel" "$SOURCE" "$upath" "$got"
    body+="$line"
    n=$((n + 1))
    echo "  file $rel <- $upath"
  done < <(git -C "$CLONE" ls-tree -r --name-only "$PIN" -- tech/templates | LC_ALL=C sort)
  [[ $n -gt 0 ]] || die "no operational template under tech/templates at ${PIN:0:12}"

  {
    echo "# Reference operational templates of Cadasto/openehr-reference-content."
    echo "# generated by scripts/ingest-reference-content.sh — do not edit"
    echo "# tag: $tag"
    echo "# Tab-separated records:"
    echo "#   source <name> <repo URL> <commit> <licence>"
    echo "#   file   templates/<template id>.opt <source name> <upstream path> <sha256>"
    printf 'source\t%s\t%s\t%s\t%s\n' "$SOURCE" "$REPO_URL" "$PIN" "$LICENCE"
    printf '%s' "$body"
  } >"$stage/MANIFEST.txt"

  # Land the stage in place of the vendored tree. Move the previous tree
  # aside first: if the mv fails, put it back and die, so a refusal leaves
  # the previous tree in place.
  local aside=""
  if [[ -e "$DEST" ]]; then
    aside="${DEST}.aside.$$"
    mv -- "$DEST" "$aside"
  fi
  if ! mv -- "$stage" "$DEST"; then
    if [[ -n "$aside" ]]; then
      mv -- "$aside" "$DEST" || die "replacing $DEST failed and restoring the previous tree failed"
      die "replacing $DEST failed; the previous tree is in place"
    fi
    die "replacing $DEST failed"
  fi
  trap - EXIT
  if [[ -n "$aside" ]]; then
    rm -rf -- "$aside"
  fi
  chmod 0755 "$DEST" "$DEST/templates"
  echo "Ingested $n file(s) into testkit/corpus/reference/ ($tag, ${PIN:0:12})"
}

cmd_verify() {
  require_hasher
  [[ -f "$MANIFEST" ]] || die "no MANIFEST.txt; run '$(basename "$0") ingest' first"

  local rc=0 kind a b c d path listed=""
  local nfiles=0 nsources=0
  # A last record without its final newline still counts: read fails at EOF
  # but has filled the fields.
  while IFS=$'\t' read -r kind a b c d || [[ -n "$kind" ]]; do
    case "$kind" in
      source)
        nsources=$((nsources + 1))
        if [[ "$b" != https://* ]]; then
          echo "  PROVENANCE: source $a repository $b is not an https URL"
          rc=1
        fi
        if [[ ! "$c" =~ ^[0-9a-f]{40}$ ]]; then
          echo "  PROVENANCE: source $a commit $c is not a full commit"
          rc=1
        fi
        if [[ "$d" != "$LICENCE" ]]; then
          echo "  PROVENANCE: source $a licence $d, want $LICENCE"
          rc=1
        fi
        ;;
      file)
        nfiles=$((nfiles + 1))
        # A record names one template directly under templates/, so it can
        # point at nothing outside the subtree.
        if [[ ! "$a" =~ ^templates/[^/]+\.opt$ ]]; then
          echo "  BAD RECORD: $a is not templates/<template id>.opt"
          rc=1
          continue
        fi
        listed+="$a"$'\n'
        path="$DEST/$a"
        if [[ ! -f "$path" ]]; then
          echo "  MISSING: reference/$a"
          rc=1
        elif [[ "$(sha256_of "$path")" != "$d" ]]; then
          echo "  FAILED: reference/$a (sha256 mismatch)"
          rc=1
        fi
        ;;
      *) continue ;;
    esac
  done <"$MANIFEST"

  if [[ $nsources -ne 1 ]]; then
    echo "  PROVENANCE: $nsources source records, want exactly one source record"
    rc=1
  fi

  # Every regular file in the subtree, other than the manifest, must be listed.
  local f rel
  while IFS= read -r -d '' f; do
    rel="${f#"$DEST/"}"
    [[ "$rel" == "MANIFEST.txt" ]] && continue
    if ! grep -qxF -- "$rel" <<<"$listed"; then
      echo "  UNTRACKED: reference/$rel (not listed in MANIFEST.txt)"
      rc=1
    fi
  done < <(find "$DEST" -type f -print0)

  [[ $nfiles -gt 0 ]] || die "MANIFEST.txt lists no files"
  [[ $rc -eq 0 ]] || die "integrity check failed; re-run '$(basename "$0") ingest' at the pin"
  echo "Integrity OK ($nfiles file(s))."
}

main() {
  case "${1:-}" in
    ingest) cmd_ingest ;;
    verify) cmd_verify ;;
    "" | -h | --help)
      awk 'NR == 1 { next } /^#/ { sub(/^# ?/, ""); print; next } { exit }' "$0"
      ;;
    *) die "unknown subcommand '$1' (want: ingest | verify)" ;;
  esac
}

main "$@"
