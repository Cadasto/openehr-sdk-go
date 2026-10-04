#!/usr/bin/env bash
#
# ingest-crossformat.sh: vendor and verify the upstream cross-format corpus
# under testkit/corpus/crossformat/.
#
# A cross-format set is one upstream composition given in two or more of
# canonical JSON, canonical XML, FLAT and STRUCTURED, together with the OPT of
# its template. Each set is one directory with fixed file names (template.opt,
# canonical.json, canonical.xml, flat.json, structured.json). A set whose OPT is
# already vendored elsewhere in testkit/corpus/ carries no template.opt; its
# manifest records an `opt` pointer to that file instead, and the ingest checks
# that the pointed-to file is byte-identical to the upstream OPT. The sets are
# the input to PROBE-105 (REQ-080).
#
# Only files that describe the same instance form a set. Upstream often has
# several files for one template whose values disagree (another instance of the
# same template); those are not a cross-format pair and are not vendored. The
# table below is curated with that rule in mind: extend it only with files that
# were checked to carry the same composition.
#
# Sources (both Apache-2.0; attribution in testkit/corpus/THIRD_PARTY_LICENSES.md),
# read from local sibling clones:
#   sdk    https://github.com/ehrbase/openEHR_SDK
#          clone: $CROSSFORMAT_SDK_CLONE (default /src/ehrbase/openEHR_SDK)
#   robot  https://github.com/ehrbase/integration-tests
#          clone: $CROSSFORMAT_ROBOT_CLONE (default /src/ehrbase/integration-tests)
#
# The pins below are a hard guard: `ingest` refuses to run when a clone's HEAD
# is not its pin, or when a source file in the clone differs from the blob at
# that pin, so vendoring from the wrong tree cannot happen silently. Re-pinning
# means editing SDK_PIN / ROBOT_PIN here, in the same commit as the refreshed
# files and MANIFEST.txt.
#
# Subcommands:
#   ingest   Check both clones against their pins, copy every file in the table
#            byte for byte, check every OPT pointer, and regenerate
#            testkit/corpus/crossformat/MANIFEST.txt. Everything is staged
#            first, so a refusal leaves the vendored tree untouched. Files no
#            longer in the table are removed.
#   verify   Offline integrity: recompute the sha256 of every `file` and `opt`
#            line in MANIFEST.txt and fail on a mismatch, a missing file, or a
#            file in the subtree that the manifest does not list. Needs no clone
#            and no network (sha256sum or shasum only). The Go tests in
#            testkit/fixtures/crossformat_test.go check the same properties
#            under `make ci`.
#
# MANIFEST.txt is tab-separated, one record per line:
#   source <name> <repo URL> <commit> <licence>
#   file   <set>/<local name> <source name> <upstream path> <sha256>
#   opt    <set> <path relative to testkit/corpus/> <source name> <upstream path> <sha256>
#
# Idempotence: re-running `ingest` at the same pins leaves `git status` clean.
# The files are byte copies, and recorded_utc is read back from the existing
# manifest while both pins are unchanged; only a re-pin moves it.
set -euo pipefail

readonly SDK_REPO="https://github.com/ehrbase/openEHR_SDK"
readonly SDK_PIN="8a5dae6fd82a5f0aa23b25681a13df79bfc21d2b"
readonly SDK_LICENCE="Apache-2.0"
readonly ROBOT_REPO="https://github.com/ehrbase/integration-tests"
readonly ROBOT_PIN="fcb3ac4b0a47e51bf23c03c13f86bea8fb0bfb67"
readonly ROBOT_LICENCE="Apache-2.0"

SDK_CLONE="${CROSSFORMAT_SDK_CLONE:-/src/ehrbase/openEHR_SDK}"
ROBOT_CLONE="${CROSSFORMAT_ROBOT_CLONE:-/src/ehrbase/integration-tests}"

readonly SDKTD="test-data/src/main/resources"
readonly ROBOT="tests/robot/_resources/test_data_sets"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
readonly CORPUS="$ROOT/testkit/corpus"
readonly DEST="$CORPUS/crossformat"
readonly MANIFEST="$DEST/MANIFEST.txt"

# FILES: <set>/<local name> <source name> <upstream path>, one per line.
# Local names are fixed: template.opt, canonical.json, canonical.xml,
# flat.json, structured.json.
readonly FILES="
alternative_events/template.opt sdk $SDKTD/operationaltemplate/AlternativeEvents.opt
alternative_events/canonical.json sdk $SDKTD/composition/canonical_json/alternative_events.json
alternative_events/flat.json sdk $SDKTD/composition/flat/simSDT/AlternativeEvents2.json
consult_record/template.opt robot $ROBOT/valid_templates/all_types/EHRN-ABDM-OPConsultRecord.v2.0.opt
consult_record/canonical.json robot $ROBOT/compositions/CANONICAL_JSON/consult_record__.json
consult_record/canonical.xml robot $ROBOT/compositions/CANONICAL_XML/consult_record__.xml
consult_record/flat.json robot $ROBOT/compositions/FLAT/consult_record__.json
consult_record/structured.json robot $ROBOT/compositions/STRUCTURED/consult_record__.json
corona/canonical.json sdk $SDKTD/composition/canonical_json/compo_corona.json
corona/flat.json sdk $SDKTD/composition/flat/simSDT/corona.json
corona/structured.json sdk $SDKTD/composition/flat/structured/corona.json
ehrn_abdm/template.opt sdk $SDKTD/operationaltemplate/EHRN-ABDM-OPConsultRecord.v2.0.opt
ehrn_abdm/canonical.json sdk $SDKTD/composition/canonical_json/ehrb_adbm_op_consult_record.json
ehrn_abdm/flat.json sdk $SDKTD/composition/flat/simSDT/ehrb_adbm_op_consult_record.json
family_history/template.opt robot $ROBOT/valid_templates/all_types/family_history.opt
family_history/canonical.json robot $ROBOT/compositions/CANONICAL_JSON/family_history.v2__.json
family_history/canonical.xml robot $ROBOT/compositions/CANONICAL_XML/family_history.v2__.xml
multi_list/template.opt sdk $SDKTD/operationaltemplate/Multi_list.opt
multi_list/flat.json sdk $SDKTD/composition/flat/simSDT/multi_list.json
multi_list/structured.json sdk $SDKTD/composition/flat/structured/multi_list.json
multi_occurrence/template.opt sdk $SDKTD/operationaltemplate/ehrbase_multi_occurrence.de.opt
multi_occurrence/canonical.json sdk $SDKTD/composition/canonical_json/multi_occurrence.json
multi_occurrence/flat.json sdk $SDKTD/composition/flat/simSDT/multi_occurrence.json
nested/canonical.xml robot $ROBOT/compositions/CANONICAL_XML/nested.en.v1__full_without_links.xml
nested/flat.json robot $ROBOT/compositions/FLAT/nested.en.v1__full_without_links.xml.flat.json
persistent_minimal/canonical.xml robot $ROBOT/compositions/CANONICAL_XML/persistent_minimal.en.v1__full_without_links.xml
persistent_minimal/flat.json robot $ROBOT/compositions/FLAT/persistent_minimal.en.v1__full_without_links.xml.flat.json
test_all_types/template.opt sdk $SDKTD/operationaltemplate/Test_all_types.opt
test_all_types/canonical.json sdk $SDKTD/composition/canonical_json/all_types_no_multimedia.json
test_all_types/flat.json sdk $SDKTD/composition/flat/simSDT/test_all_types.json
"

# OPT_POINTERS: <set> <path relative to testkit/corpus/> <source name>
# <upstream path of the byte-identical upstream OPT>, one per line.
readonly OPT_POINTERS="
corona webtemplate/Corona_Anamnese.opt sdk $SDKTD/operationaltemplate/corona_anamnese.opt
nested templates/nested.en.v1.opt robot $ROBOT/valid_templates/nested/nested.opt
persistent_minimal templates/persistent_minimal.en.v1.opt robot $ROBOT/valid_templates/minimal_persistent/persistent_minimal.opt
"

# --- helpers ---------------------------------------------------------------

die() {
  echo "ingest-crossformat: $*" >&2
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

# clone_of <source name>: print the clone directory of a source.
clone_of() {
  case "$1" in
    sdk) echo "$SDK_CLONE" ;;
    robot) echo "$ROBOT_CLONE" ;;
    *) die "unknown source '$1' in the table" ;;
  esac
}

# pin_of <source name>: print the pinned commit of a source.
pin_of() {
  case "$1" in
    sdk) echo "$SDK_PIN" ;;
    robot) echo "$ROBOT_PIN" ;;
    *) die "unknown source '$1' in the table" ;;
  esac
}

# check_clone <source name>: refuse unless the clone exists and its HEAD is
# the pin.
check_clone() {
  local name="$1" clone pin head
  clone="$(clone_of "$name")"
  pin="$(pin_of "$name")"
  [[ -d "$clone" ]] || die "$name clone not found at $clone (set CROSSFORMAT_$(tr '[:lower:]' '[:upper:]' <<<"$name")_CLONE)"
  head="$(git -C "$clone" rev-parse HEAD 2>/dev/null)" \
    || die "$name clone $clone is not a git checkout; refusing to vendor without provenance"
  if [[ "$head" != "$pin" ]]; then
    echo "ingest-crossformat: $name clone $clone is at $head" >&2
    echo "ingest-crossformat: expected pin $pin; refusing to vendor from an unpinned tree" >&2
    die "check out the pin, or update the pin in $(basename "$0") when re-pinning"
  fi
}

# pinned_sha <source name> <upstream path>: print the sha256 of the blob at
# the source's pin, or fail when the path does not exist there.
pinned_sha() {
  local clone pin
  clone="$(clone_of "$1")"
  pin="$(pin_of "$1")"
  git -C "$clone" cat-file -e "$pin:$2" 2>/dev/null \
    || die "$1: $2 does not exist at pin ${pin:0:12}"
  git -C "$clone" cat-file blob "$pin:$2" | sha256_stdin
}

# --- subcommands -----------------------------------------------------------

cmd_ingest() {
  require_hasher
  check_clone sdk
  check_clone robot

  local stage
  stage="$(mktemp -d "$CORPUS/.crossformat.XXXXXX")"
  # shellcheck disable=SC2064 # expand $stage now, while it is set.
  trap "rm -rf -- '$stage'" EXIT

  local body="" line rel src upath clone want got
  while IFS=' ' read -r rel src upath; do
    [[ -n "$rel" ]] || continue
    clone="$(clone_of "$src")"
    [[ -f "$clone/$upath" ]] || die "$src: $upath not found in $clone"
    want="$(pinned_sha "$src" "$upath")"
    mkdir -p -- "$stage/$(dirname "$rel")"
    cp -- "$clone/$upath" "$stage/$rel"
    got="$(sha256_of "$stage/$rel")"
    [[ "$got" == "$want" ]] \
      || die "$src: $upath in $clone differs from the blob at its pin (local edit?)"
    printf -v line 'file\t%s\t%s\t%s\t%s\n' "$rel" "$src" "$upath" "$got"
    body+="$line"
    echo "  file $rel <- $src:$upath"
  done <<<"$FILES"

  local set target
  while IFS=' ' read -r set target src upath; do
    [[ -n "$set" ]] || continue
    [[ -d "$stage/$set" ]] || die "opt pointer for unknown set '$set'"
    [[ ! -e "$stage/$set/template.opt" ]] \
      || die "set '$set' has both a template.opt and an opt pointer"
    [[ -f "$CORPUS/$target" ]] || die "opt pointer target $target not found under testkit/corpus/"
    want="$(pinned_sha "$src" "$upath")"
    got="$(sha256_of "$CORPUS/$target")"
    [[ "$got" == "$want" ]] \
      || die "opt pointer $target is not byte-identical to $src:$upath at its pin"
    printf -v line 'opt\t%s\t%s\t%s\t%s\t%s\n' "$set" "$target" "$src" "$upath" "$got"
    body+="$line"
    echo "  opt  $set -> $target (= $src:$upath)"
  done <<<"$OPT_POINTERS"

  # Keep recorded_utc stable while both pins are unchanged, so a re-run at the
  # same pins rewrites a byte-identical manifest.
  local recorded=""
  if [[ -f "$MANIFEST" ]] \
    && grep -qF "$(printf 'source\tsdk\t%s\t%s\t' "$SDK_REPO" "$SDK_PIN")" "$MANIFEST" \
    && grep -qF "$(printf 'source\trobot\t%s\t%s\t' "$ROBOT_REPO" "$ROBOT_PIN")" "$MANIFEST"; then
    recorded="$(sed -n 's/^# recorded_utc: //p' "$MANIFEST" | head -n 1)"
  fi
  [[ -n "$recorded" ]] || recorded="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  {
    echo "# Upstream cross-format corpus (PROBE-105, REQ-080)."
    echo "# generated by scripts/ingest-crossformat.sh — do not edit"
    echo "# recorded_utc: $recorded"
    echo "# Tab-separated records:"
    echo "#   source <name> <repo URL> <commit> <licence>"
    echo "#   file   <set>/<local name> <source name> <upstream path> <sha256>"
    echo "#   opt    <set> <path relative to testkit/corpus/> <source name> <upstream path> <sha256>"
    printf 'source\tsdk\t%s\t%s\t%s\n' "$SDK_REPO" "$SDK_PIN" "$SDK_LICENCE"
    printf 'source\trobot\t%s\t%s\t%s\n' "$ROBOT_REPO" "$ROBOT_PIN" "$ROBOT_LICENCE"
    printf '%s' "$body"
  } >"$stage/MANIFEST.txt"

  rm -rf -- "$DEST"
  mv -- "$stage" "$DEST"
  chmod 0755 "$DEST"
  trap - EXIT

  local nfiles nopts
  nfiles="$(grep -c $'^file\t' "$MANIFEST")"
  nopts="$(grep -c $'^opt\t' "$MANIFEST")"
  echo "Ingested $nfiles file(s) and $nopts OPT pointer(s) into testkit/corpus/crossformat/ (sdk ${SDK_PIN:0:12}, robot ${ROBOT_PIN:0:12})"
}

cmd_verify() {
  require_hasher
  [[ -f "$MANIFEST" ]] || die "no MANIFEST.txt; run '$(basename "$0") ingest' first"

  local rc=0 kind a b c d e path want listed=""
  local nfiles=0 nopts=0
  while IFS=$'\t' read -r kind a b c d e; do
    case "$kind" in
      file)
        path="$DEST/$a"
        want="$d"
        listed+="$a"$'\n'
        nfiles=$((nfiles + 1))
        ;;
      opt)
        path="$CORPUS/$b"
        want="$e"
        nopts=$((nopts + 1))
        ;;
      *) continue ;;
    esac
    if [[ ! -f "$path" ]]; then
      echo "  MISSING: ${path#"$CORPUS/"}"
      rc=1
    elif [[ "$(sha256_of "$path")" != "$want" ]]; then
      echo "  FAILED: ${path#"$CORPUS/"} (sha256 mismatch)"
      rc=1
    fi
  done <"$MANIFEST"

  # Every regular file in the subtree, other than the manifest, must be listed.
  local f rel
  while IFS= read -r -d '' f; do
    rel="${f#"$DEST/"}"
    [[ "$rel" == "MANIFEST.txt" ]] && continue
    if ! grep -qxF -- "$rel" <<<"$listed"; then
      echo "  UNTRACKED: crossformat/$rel (not listed in MANIFEST.txt)"
      rc=1
    fi
  done < <(find "$DEST" -type f -print0)

  [[ $nfiles -gt 0 ]] || die "MANIFEST.txt lists no files"
  [[ $rc -eq 0 ]] || die "integrity check failed; re-run '$(basename "$0") ingest' at the pins"
  echo "Integrity OK ($nfiles file(s), $nopts OPT pointer(s))."
}

main() {
  case "${1:-}" in
    ingest) cmd_ingest ;;
    verify) cmd_verify ;;
    "" | -h | --help)
      sed -n '3,54p' "$0" | sed 's/^# \{0,1\}//'
      ;;
    *) die "unknown subcommand '$1' (want: ingest | verify)" ;;
  esac
}

main "$@"
