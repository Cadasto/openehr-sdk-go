#!/usr/bin/env bash
#
# ingest-ehrbase-service.sh: vendor and verify the operational templates of
# the EHRbase server's tests, under testkit/corpus/ehrbase-service/.
#
# The templates are copied byte for byte from the server's test resources,
# service/src/test/resources/knowledge/, and each is named after its template
# id, as testkit/corpus/templates/ names its files. They are OPT only: no
# composition comes with them. The whole-corpus tests read them through
# fixtures.ListAllOPTs. The table below lists them; `ingest` refuses a file
# whose template id is not the one the table names.
#
# Source (Apache-2.0; attribution in testkit/corpus/THIRD_PARTY_LICENSES.md),
# read from a local clone that `ingest` requires:
#   ehrbase  https://github.com/ehrbase/ehrbase
#            clone: the directory in $EHRBASE_SERVICE_CLONE
#
# The pin, the ehrbase/ehrbase line of testkit/corpus/UPSTREAM_PINS.txt (one
# commit per upstream repository, shared with every other file vendored from
# it), is a hard guard: `ingest` refuses to run when the clone's HEAD is not
# the pin, or when a source file in the clone differs from the blob at the
# pin, so vendoring from the wrong tree cannot happen silently. Re-pinning
# means editing that file, in the same commit as the refreshed files and
# manifest.
#
# Subcommands:
#   ingest   Check the clone against the pin, copy every file in the table
#            byte for byte, check each file's template id, and regenerate
#            testkit/corpus/ehrbase-service/MANIFEST.txt. Everything is staged
#            first, so a refusal leaves the vendored tree untouched. Files no
#            longer in the table are removed.
#   verify   Offline integrity: fail unless MANIFEST.txt holds one source
#            record naming an https repository, a full commit and the
#            Apache-2.0 licence; recompute the sha256 of every `file` record
#            and fail on a mismatch, a missing file, or a file in the subtree
#            that the manifest does not list. Needs no clone and no network
#            (sha256sum or shasum only). The Go tests in
#            testkit/fixtures/ehrbase_service_test.go run it under `make ci`.
#
# MANIFEST.txt is tab-separated, one record per line:
#   source <name> <repo URL> <commit> <licence>
#   file   templates/<template id>.opt <source name> <upstream path> <sha256>
#
# Idempotence: re-running `ingest` at the same pin leaves `git status` clean.
# The files are byte copies, and recorded_utc is read back from the existing
# manifest while the pin is unchanged; only a re-pin moves it.
set -euo pipefail

readonly REPO_URL="https://github.com/ehrbase/ehrbase"
readonly LICENCE="Apache-2.0"
readonly SOURCE="ehrbase"
# Set from UPSTREAM_PINS.txt by cmd_ingest; `verify` needs no pin.
PIN=""

CLONE="${EHRBASE_SERVICE_CLONE:-}"

readonly KNOWLEDGE="service/src/test/resources/knowledge"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
readonly CORPUS="$ROOT/testkit/corpus"
readonly DEST="$CORPUS/ehrbase-service"
readonly MANIFEST="$DEST/MANIFEST.txt"
readonly PINS="$CORPUS/UPSTREAM_PINS.txt"

# FILES: <template id>|<upstream path>, one per line, sorted by template id.
# The file lands at templates/<template id>.opt. Two ids have two spaces
# before the dash, as upstream writes them.
readonly FILES="
DiADeM Assessment.v0|$KNOWLEDGE/operational_templates/DiADeM Assessment.v0.opt
IDCR  - Generic MDT Output Report.v0|$KNOWLEDGE/opt/IDCR  - Generic MDT Output Report.opt
IDCR  - Laboratory Order.v0|$KNOWLEDGE/operational_templates/IDCR  - Laboratory Order.opt
IDCR - End of Life Patient Preferences.v0|$KNOWLEDGE/opt/IDCR - End of Life Patient Preferences.v0.opt
IDCR - Immunisation summary.v0|$KNOWLEDGE/IDCR - Immunisation summary.v0.opt
IDCR - Procedures List.v1|$KNOWLEDGE/opt/IDCR - Procedures List.v1.opt
IDCR - Relevant contacts.v0|$KNOWLEDGE/operational_templates/IDCR - Relevant contacts.v0.opt
IDCR - Service Request.v0|$KNOWLEDGE/operational_templates/IDCR - Service Request.v0.opt
careplan.v1|$KNOWLEDGE/operational_templates/careplan.v1.opt
"

# --- helpers ---------------------------------------------------------------

die() {
  echo "ingest-ehrbase-service: $*" >&2
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

# sha256_stdin: print the bare sha256 hash of standard input.
sha256_stdin() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | cut -d' ' -f1
  else
    shasum -a 256 | cut -d' ' -f1
  fi
}

require_hasher() {
  command -v sha256sum >/dev/null 2>&1 || command -v shasum >/dev/null 2>&1 \
    || die "sha256sum or shasum is required"
}

# upstream_pin: print the commit UPSTREAM_PINS.txt pins for ehrbase/ehrbase,
# or fail when it has no full commit.
upstream_pin() {
  local repo="${REPO_URL#https://github.com/}" pin
  pin="$(awk -v r="$repo" '!/^[[:space:]]*#/ && $1 == r { print $3; exit }' "$PINS" 2>/dev/null || true)"
  [[ "$pin" =~ ^[0-9a-f]{40}$ ]] || die "no full commit for $repo in testkit/corpus/UPSTREAM_PINS.txt"
  echo "$pin"
}

# check_clone: refuse unless the clone exists and its HEAD is the pin.
check_clone() {
  local head
  [[ -n "$CLONE" ]] || die "set EHRBASE_SERVICE_CLONE to a local clone of $REPO_URL"
  [[ -d "$CLONE" ]] || die "clone not found at $CLONE (set EHRBASE_SERVICE_CLONE)"
  head="$(git -C "$CLONE" rev-parse HEAD 2>/dev/null)" \
    || die "clone $CLONE is not a git checkout; refusing to vendor without provenance"
  if [[ "$head" != "$PIN" ]]; then
    echo "ingest-ehrbase-service: clone $CLONE is at $head" >&2
    echo "ingest-ehrbase-service: expected pin $PIN; refusing to vendor from an unpinned tree" >&2
    die "check out the pin, or update testkit/corpus/UPSTREAM_PINS.txt when re-pinning"
  fi
}

# pinned_sha <upstream path>: print the sha256 of the blob at the pin, or
# fail when the path does not exist there.
pinned_sha() {
  git -C "$CLONE" cat-file -e "$PIN:$1" 2>/dev/null \
    || die "$1 does not exist at pin ${PIN:0:12}"
  git -C "$CLONE" cat-file blob "$PIN:$1" | sha256_stdin
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

  local stage
  stage="$(mktemp -d "$CORPUS/.ehrbase-service.XXXXXX")"
  # shellcheck disable=SC2064 # expand $stage now, while it is set.
  trap "rm -rf -- '$stage'" EXIT
  mkdir -p -- "$stage/templates"

  local body="" line id upath rel want got found
  while IFS='|' read -r id upath; do
    [[ -n "$id" ]] || continue
    rel="templates/$id.opt"
    [[ -f "$CLONE/$upath" ]] || die "$upath not found in $CLONE"
    want="$(pinned_sha "$upath")"
    cp -- "$CLONE/$upath" "$stage/$rel"
    got="$(sha256_of "$stage/$rel")"
    [[ "$got" == "$want" ]] \
      || die "$upath in $CLONE differs from the blob at its pin (local edit?)"
    found="$(template_id_of "$stage/$rel")"
    [[ "$found" == "$id" ]] \
      || die "$upath holds template id $found; the table names it $id"
    printf -v line 'file\t%s\t%s\t%s\t%s\n' "$rel" "$SOURCE" "$upath" "$got"
    body+="$line"
    echo "  file $rel <- $upath"
  done <<<"$FILES"

  # Keep recorded_utc stable while the pin is unchanged, so a re-run at the
  # same pin rewrites a byte-identical manifest.
  local recorded=""
  if [[ -f "$MANIFEST" ]] \
    && grep -qF "$(printf 'source\t%s\t%s\t%s\t' "$SOURCE" "$REPO_URL" "$PIN")" "$MANIFEST"; then
    recorded="$(sed -n 's/^# recorded_utc: //p' "$MANIFEST" | head -n 1)"
  fi
  [[ -n "$recorded" ]] || recorded="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  {
    echo "# Operational templates of the EHRbase server's tests."
    echo "# generated by scripts/ingest-ehrbase-service.sh — do not edit"
    echo "# recorded_utc: $recorded"
    echo "# Tab-separated records:"
    echo "#   source <name> <repo URL> <commit> <licence>"
    echo "#   file   templates/<template id>.opt <source name> <upstream path> <sha256>"
    printf 'source\t%s\t%s\t%s\t%s\n' "$SOURCE" "$REPO_URL" "$PIN" "$LICENCE"
    printf '%s' "$body"
  } >"$stage/MANIFEST.txt"

  # Land the stage in place of the vendored tree. Move the previous tree
  # aside first: if the mv fails, put it back and die, so a refusal leaves
  # the previous tree in place. Drop the aside copy only after the stage
  # has landed and the EXIT trap no longer points at the stage.
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

  local nfiles
  # grep -c exits 1 when the count is zero; under set -e that would abort
  # after the tree is already replaced.
  nfiles="$(grep -c $'^file\t' "$MANIFEST" || true)"
  echo "Ingested $nfiles file(s) into testkit/corpus/ehrbase-service/ (ehrbase ${PIN:0:12})"
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
          echo "  MISSING: ehrbase-service/$a"
          rc=1
        elif [[ "$(sha256_of "$path")" != "$d" ]]; then
          echo "  FAILED: ehrbase-service/$a (sha256 mismatch)"
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
      echo "  UNTRACKED: ehrbase-service/$rel (not listed in MANIFEST.txt)"
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
