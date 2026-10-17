#!/usr/bin/env bash
# Vendor both EHRbase Robot AQL corpora from one pinned clone of the upstream
# integration-tests repository:
#
#   testkit/corpus/aql/conformance/  the FROM-combination corpus: the FROM-family
#                                    combination CSVs, filed under the Robot
#                                    suite family that consumes each one.
#   testkit/corpus/aql/suites/       the AQL suite corpus: every Robot suite
#                                    under tests/robot/AQL/ and every combination
#                                    CSV under
#                                    tests/robot/_resources/test_data_sets/aql/fields_and_results/**/combinations/,
#                                    each at its path relative to upstream
#                                    tests/robot/, so a suite's
#                                    ${PROJECT_ROOT}/tests/robot/<rest> reads
#                                    suites/<rest>.
#
# Each corpus carries its own AQL_SOURCE.txt and generated EXCLUDED.txt. One run
# writes both from the same clone, so the two pins cannot name different commits.
#
# Sibling of scripts/ingest-robot-fixtures.sh, but a different job: that one
# hand-curates and renames composition fixtures, this one copies the upstream
# files byte-for-byte. So the commit recorded in each AQL_SOURCE.txt is
# authoritative for the vendored content, not merely the tree the ingest
# happened to read.
#
# ROBOT_ROOT points at the upstream tests/robot/_resources/test_data_sets
# directory (same convention as the fixtures ingest); the suites are read from
# tests/robot, two levels above it. The clone root is derived from it with
# `git rev-parse --show-toplevel`, so a clone laid out elsewhere works as long as
# ROBOT_ROOT points at that directory inside it.
#
# EXPECTED_COMMIT below is a hard guard: the script refuses to vendor from a tree
# whose HEAD is not that commit, or whose AQL subtrees have local modifications,
# untracked or ignored files, so vendoring from the wrong tree cannot happen
# silently. Refreshing the corpora to a newer upstream commit means editing
# EXPECTED_COMMIT here, in the same commit as the refreshed files.
#
# Idempotence: re-running at the same pin must leave `git status` clean. The
# vendored files are byte copies and each EXCLUDED.txt is derived from the pinned
# tree, so all are stable by construction; recorded_utc would otherwise churn, so
# it is read back from each existing AQL_SOURCE.txt whenever that file's
# `commit:` still matches EXPECTED_COMMIT. Only a re-pin (or a missing pin file)
# moves the recorded date.
set -euo pipefail

EXPECTED_COMMIT=b4625fc28d6785c05adfd6db26339b4bb2272e06

ROBOT="${ROBOT_ROOT:-/src/ehrbase/integration-tests/tests/robot/_resources/test_data_sets}"
# Canonicalise: prefix-stripping below compares against
# `git rev-parse --show-toplevel`, which prints a physical path, so a
# ROBOT_ROOT given with a trailing slash or through a symlink would silently
# fail to strip and record wrong provenance paths.
if [[ -d "$ROBOT" ]]; then
  ROBOT="$(cd "$ROBOT" && pwd -P)"
fi
REPO="$(cd "$(dirname "$0")/.." && pwd)"
DEST="$REPO/testkit/corpus/aql/conformance"
SUITES_DEST="$REPO/testkit/corpus/aql/suites"

AQL="$ROBOT/aql/fields_and_results"
SRC="$AQL/from/combinations"

if [[ ! -d "$SRC" ]]; then
  echo "AQL corpus not found at $SRC" >&2
  echo "set ROBOT_ROOT to the upstream tests/robot/_resources/test_data_sets directory" >&2
  exit 1
fi

# The suite corpus mirrors upstream tests/robot/, two levels above ROBOT_ROOT.
# The suites name their CSVs as ${PROJECT_ROOT}/tests/robot/_resources/
# test_data_sets/..., so a ROBOT_ROOT that is not that directory would vendor
# the CSVs at paths no suite reads.
TESTS_ROBOT="$(cd "$ROBOT/../.." && pwd -P)"
if [[ "$ROBOT" != "$TESTS_ROBOT/_resources/test_data_sets" ]]; then
  echo "ROBOT_ROOT $ROBOT is not <tests/robot>/_resources/test_data_sets" >&2
  echo "the suite corpus mirrors tests/robot, so its CSVs must sit at that path" >&2
  exit 1
fi
SUITES="$TESTS_ROBOT/AQL"
if [[ ! -d "$SUITES" ]]; then
  echo "AQL suites not found at $SUITES" >&2
  exit 1
fi

src_root=$(git -C "$ROBOT" rev-parse --show-toplevel 2>/dev/null || true)
if [[ -z "$src_root" ]]; then
  echo "$ROBOT is not inside a git checkout — refusing to vendor without provenance" >&2
  exit 1
fi

head_sha=$(git -C "$src_root" rev-parse HEAD)
if [[ "$head_sha" != "$EXPECTED_COMMIT"* ]]; then
  echo "clone $src_root is at $head_sha" >&2
  echo "expected pin $EXPECTED_COMMIT — refusing to vendor from an unpinned tree" >&2
  echo "check out the pin, or update EXPECTED_COMMIT in $(basename "$0") when re-pinning" >&2
  exit 1
fi

aql_rel=${AQL#"$src_root/"}
suites_rel=${SUITES#"$src_root/"}
# --ignored as well: find(1) below reads the working tree, so an ignored file
# under either root would be copied or listed as if the pinned commit held it.
for rel in "$aql_rel" "$suites_rel"; do
  if [[ -n "$(git -C "$src_root" status --porcelain --ignored -- "$rel")" ]]; then
    echo "$rel has local modifications, untracked or ignored files in $src_root" >&2
    echo "refusing to vendor: the copied bytes would not match the pinned commit" >&2
    exit 1
  fi
done

# Each EXCLUDED.txt puts a single space between a path and its reason tag, so a
# path containing whitespace would make its line unparsable. None exists at this
# pin; fail loud before writing anything rather than emit an ambiguous list if a
# future re-pin introduces one. Both corpora's roots are walked, and the path is
# shown relative to tests/robot.
while IFS= read -r -d '' f; do
  rel=${f#"$TESTS_ROBOT/"}
  case "$rel" in
  *[[:space:]]*)
    echo "upstream path contains whitespace: $rel" >&2
    echo "EXCLUDED.txt cannot represent it with a single-space separator —" >&2
    echo "switch the separator to a TAB when re-pinning to such a tree" >&2
    exit 1
    ;;
  esac
done < <(find "$AQL" "$SUITES" -type f -print0)

# CSV -> the Robot suite family that consumes it, under upstream
# tests/robot/AQL_TESTS/FROM/<FAMILY>/. The family is the vendored directory name
# so a reader can find the suite that supplies the query template for each row.
CORPUS=(
  "AND_OR from_simple_and_or.csv"
  "CONTAINS_A_D from_contains_plus_contain_chaining.csv"
  "CONTAINS_A_D from_contains_with_repeating_types.csv"
  "EHR_STATUS contains.csv"
  "EHR_STATUS from_single_ehr.csv"
  "EHR_STATUS via_part.csv"
  "PREDICATE_A_D from_predicate_on_extracted_column.csv"
  "USABLE_RM_TYPES_A_D from_abstract_types.csv"
  "USABLE_RM_TYPES_A_D from_common_types.csv"
  "USABLE_RM_TYPES_A_D from_composition.csv"
  "USABLE_RM_TYPES_A_D from_item_structure_and_element_in_composition.csv"
  "USABLE_RM_TYPES_A_D from_item_structure_composition.csv"
)

FAMILIES=(AND_OR CONTAINS_A_D EHR_STATUS PREDICATE_A_D USABLE_RM_TYPES_A_D)

# from/combinations CSVs that are input data yet deliberately NOT vendored,
# tagged with the reason their rows cannot be reconstructed. Keyed by the
# path relative to the corpus root — the same spelling EXCLUDED.txt records —
# and never by basename: a nested CSV that merely shares an excluded basename
# (say from/combinations/sub/from_simple_and_or_ent.csv) is a different file,
# and must reach the refusal below rather than inherit an exclusion written for
# its namesake. At this pin: from/combinations/from_simple_and_or_ent.csv has
# zero references anywhere under the upstream tests/ tree, so no suite supplies
# a query template for its rows. A new upstream CSV in from/combinations that is
# neither in CORPUS nor here makes the ingest refuse (below) instead of
# mis-filing it in EXCLUDED.txt — classifying it is the re-pin author's
# decision, not the fallthrough's.
declare -A KNOWN_UNCONSUMED=(
  ["from/combinations/from_simple_and_or_ent.csv"]=unconsumed-by-suite
)

# --- validate -----------------------------------------------------------------
#
# Every refusal, for either corpus, runs before the first byte under $DEST or
# $SUITES_DEST moves. Clearing the family directories first and only then
# discovering an unclassifiable upstream CSV would empty the vendored corpus and
# abort — leaving the destination in a state the pin does not describe, which is
# the opposite of the fail-closed promise the guards above make. So this pass
# only reads.

declare -A VENDORED=()
for entry in "${CORPUS[@]}"; do
  csv=${entry#* }
  if [[ ! -f "$SRC/$csv" ]]; then
    echo "missing upstream CSV at the pin: $SRC/$csv" >&2
    exit 1
  fi
  VENDORED["from/combinations/$csv"]=1
done

# Input-CSV completeness: every from/combinations CSV at the pin must be either
# vendored or a named exclusion. Anything else is new input data the corpus
# does not know — refuse loudly rather than let the classifier below file it
# under a wrong tag. The walk is recursive because the classifier's
# `from/combinations/*.csv` case glob is too (a bash case `*` crosses `/`), so a
# CSV in a new subdirectory must reach this refusal rather than fall through to
# the classifier's unbound KNOWN_UNCONSUMED key and die as a bare `set -u` abort.
while IFS= read -r -d '' f; do
  rel=${f#"$AQL/"}
  if [[ -n "${VENDORED[$rel]:-}" || -n "${KNOWN_UNCONSUMED[$rel]:-}" ]]; then
    continue
  fi
  echo "new upstream input CSV at the pin: $rel" >&2
  echo "it is neither vendored (CORPUS) nor a named exclusion (KNOWN_UNCONSUMED) —" >&2
  echo "vendor it and teach the reader its template, or record why it cannot be reconstructed" >&2
  exit 1
done < <(find "$SRC" -type f -name '*.csv' -print0)

# Suite corpus: classify every upstream file under its two roots, still before
# anything moves. Paths are relative to tests/robot, the spelling a suite's
# DataDriver `file=${PROJECT_ROOT}/tests/robot/<rest>` uses, so a carried file
# lands at suites/<rest>. Each file is carried, excluded with a reason tag, or
# refused: a file no rule knows is the re-pin author's decision, not something
# to file under a guessed tag.
SUITES_CARRIED=()
SUITES_EXCLUDED=()
n_suites=0
n_inputs=0
while IFS= read -r -d '' f; do
  rel=${f#"$TESTS_ROBOT/"}
  # A symbolic link or any other non-regular file is neither a byte copy the
  # commit determines nor a file the exclusion list can name.
  if [[ -L "$f" || ! -f "$f" ]]; then
    echo "upstream entry is not a regular file: $rel" >&2
    echo "the suite corpus carries byte copies of regular files only" >&2
    exit 1
  fi
  case "$rel" in
  AQL/*.robot)
    SUITES_CARRIED+=("$rel")
    n_suites=$((n_suites + 1))
    ;;
  AQL/*)
    SUITES_EXCLUDED+=("$rel not-a-suite")
    ;;
  *.csv)
    # Input data sits directly in a combinations/ directory. A CSV anywhere
    # else is not a shape the suites read at this pin; refuse rather than
    # guess whether it is input or a result artefact.
    parent=${rel%/*}
    if [[ "${parent##*/}" != combinations ]]; then
      echo "upstream CSV outside a combinations/ directory: $rel" >&2
      echo "classify it (input the suites read, or a result artefact) before re-pinning" >&2
      exit 1
    fi
    SUITES_CARRIED+=("$rel")
    n_inputs=$((n_inputs + 1))
    ;;
  *.json)
    SUITES_EXCLUDED+=("$rel execution-semantics")
    ;;
  *)
    echo "unclassified upstream file under the AQL suite roots: $rel" >&2
    echo "it is neither a suite, a combination CSV nor an expected-result JSON —" >&2
    echo "teach the ingest its rule (carry it, or exclude it with a reason tag)" >&2
    exit 1
    ;;
  esac
done < <(find "$SUITES" "$AQL" ! -type d -print0)

# --- copy ---------------------------------------------------------------------

# Clear the family directories first: a re-pin that drops a CSV upstream must not
# leave the old copy behind pretending to still be vendored.
for fam in "${FAMILIES[@]}"; do
  mkdir -p "$DEST/$fam"
  rm -f "${DEST:?}/$fam"/*.csv
done

for entry in "${CORPUS[@]}"; do
  fam=${entry%% *}
  csv=${entry#* }
  # Byte-exact copy — CSV content is never rewritten.
  cp "$SRC/$csv" "$DEST/$fam/$csv"
done

# --- provenance pin -----------------------------------------------------------

src_rel=${SRC#"$src_root/"}
src_remote=$(git -C "$src_root" config --get remote.origin.url 2>/dev/null \
  | sed -E 's#^(git@github.com:|https://github.com/)##; s#\.git$##' || true)
# A remote that is not a GitHub owner/repo (none, a local path such as a clone
# of a clone, another host) cannot form the github.com tree URL below, so the
# pins name the canonical upstream instead. The commit, which EXPECTED_COMMIT
# guards, is what fixes the vendored bytes either way.
if [[ ! "$src_remote" =~ ^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$ ]]; then
  src_remote=ehrbase/integration-tests
fi
src_date=$(git -C "$src_root" show -s --format=%cI "$EXPECTED_COMMIT")

# recorded_utc_for PIN_FILE prints the recording date to write into PIN_FILE.
# Same pin as last time: keep the original recording date so a re-run is a
# no-op in git. A new pin (or a missing pin file) re-stamps it.
recorded_utc_for() {
  local pin=$1 kept=""
  if [[ -f "$pin" ]] && grep -qx "commit: $EXPECTED_COMMIT" "$pin"; then
    kept=$(grep -m1 '^recorded_utc: ' "$pin" | cut -d' ' -f2- || true)
  fi
  echo "${kept:-$(date -u +%Y-%m-%d)}"
}

pin_file="$DEST/AQL_SOURCE.txt"
recorded=$(recorded_utc_for "$pin_file")

{
  echo "# EHRbase Robot AQL FROM-combination corpus — source provenance pin"
  echo "# Written by scripts/ingest-robot-aql.sh. This pin IS authoritative for"
  echo "# the vendored CSVs: they are copied unmodified from the commit below, so"
  echo "# that commit fully determines their bytes. (Contrast ROBOT_SOURCE.txt one"
  echo "# level up, whose fixtures are hand-curated and renamed, making its pin a"
  echo "# record of the tree the ingest read rather than of the files it produced.)"
  echo "source_repo: ${src_remote}"
  echo "source_path: ${src_rel}"
  echo "commit: ${EXPECTED_COMMIT}"
  echo "commit_date: ${src_date}"
  echo "recorded_utc: ${recorded}"
  echo "source_tree: https://github.com/${src_remote}/tree/${EXPECTED_COMMIT}/${src_rel}"
  echo "license: Apache-2.0 (see ../../THIRD_PARTY_LICENSES.md)"
} > "$pin_file"

# --- exclusion list -----------------------------------------------------------

excl_file="$DEST/EXCLUDED.txt"
{
  echo "# Generated by scripts/ingest-robot-aql.sh — DO NOT HAND-EDIT."
  echo "# Regenerate by re-running the ingest; hand edits are overwritten."
  echo "#"
  echo "# Every file under the upstream AQL tree that this corpus does NOT carry,"
  echo "# one per line, as:"
  echo "#   <path relative to ${aql_rel}> <reason-tag>"
  echo "# Sorted with LC_ALL=C so the list has a stable diff across machines."
  echo "#"
  echo "# Reason tags:"
  echo "#   execution-semantics  an expected-result artefact — what a query"
  echo "#                        returned against a loaded EHR. This corpus"
  echo "#                        asserts admissibility (the query parses and"
  echo "#                        survives the semantic lint), never result"
  echo "#                        equality, so no result artefact is vendored."
  echo "#   non-from-family      a combination CSV for another clause family"
  echo "#                        (where/, select/, order_by/, …). This corpus is"
  echo "#                        the FROM family only."
  echo "#   unconsumed-by-suite  a from/combinations CSV that no Robot suite"
  echo "#                        references under the upstream tests/ tree at this"
  echo "#                        pin, so no query template exists to reconstruct"
  echo "#                        its rows into queries."
  echo "#"
  echo "# The pin these paths are relative to: AQL_SOURCE.txt, beside this file."
} > "$excl_file"

{
  while IFS= read -r -d '' f; do
    rel=${f#"$AQL/"}
    if [[ -n "${VENDORED[$rel]:-}" ]]; then
      continue
    fi
    case "$rel" in
    from/combinations/*.csv)
      # Always classified: the input-CSV completeness check above already
      # refused anything not in KNOWN_UNCONSUMED (set -u faults if not). Both
      # sides key on the corpus-relative path, so this glob crossing `/` is
      # correct rather than a hole — a nested CSV needs its own entry.
      tag=${KNOWN_UNCONSUMED[$rel]}
      ;;
    *.csv)
      case "$rel" in
      # A FROM-family CSV that is not in the vendored set is a result artefact,
      # not input data. Everything else is another clause family.
      from/*) tag=execution-semantics ;;
      *) tag=non-from-family ;;
      esac
      ;;
    *)
      tag=execution-semantics
      ;;
    esac
    printf '%s %s\n' "$rel" "$tag"
  done < <(find "$AQL" -type f -print0)
} | LC_ALL=C sort >> "$excl_file"

echo "vendored ${#CORPUS[@]} CSVs across ${#FAMILIES[@]} families into $DEST"
echo "pinned ${src_remote}@${EXPECTED_COMMIT:0:12} -> $pin_file"
echo "excluded $(grep -cv '^#' "$excl_file") upstream files -> $excl_file"

# --- suite corpus: copy -------------------------------------------------------

# Clear both mirrored subtrees first: a re-pin that drops an upstream file must
# not leave the old copy behind pretending to still be vendored.
rm -rf "${SUITES_DEST:?}/AQL" "${SUITES_DEST:?}/_resources"
mkdir -p "$SUITES_DEST"
for rel in "${SUITES_CARRIED[@]}"; do
  mkdir -p "$SUITES_DEST/${rel%/*}"
  # Byte-exact copy — neither a suite nor a CSV is ever rewritten.
  cp "$TESTS_ROBOT/$rel" "$SUITES_DEST/$rel"
done

# --- suite corpus: provenance pin ---------------------------------------------

suites_src_rel=${TESTS_ROBOT#"$src_root/"}
suites_pin="$SUITES_DEST/AQL_SOURCE.txt"
suites_recorded=$(recorded_utc_for "$suites_pin")

{
  echo "# EHRbase Robot AQL suite corpus — source provenance pin"
  echo "# Written by scripts/ingest-robot-aql.sh, in the same run and from the same"
  echo "# clone as ../conformance/AQL_SOURCE.txt, so both pins name one commit."
  echo "# This pin IS authoritative for the vendored files: the Robot AQL suites"
  echo "# under AQL/ and the combination CSVs they read under _resources/ are"
  echo "# copied unmodified from the commit below, each at its path relative to"
  echo "# source_path, so that commit fully determines their bytes."
  echo "source_repo: ${src_remote}"
  echo "source_path: ${suites_src_rel}"
  echo "commit: ${EXPECTED_COMMIT}"
  echo "commit_date: ${src_date}"
  echo "recorded_utc: ${suites_recorded}"
  echo "source_tree: https://github.com/${src_remote}/tree/${EXPECTED_COMMIT}/${suites_src_rel}"
  echo "license: Apache-2.0 (see ../../THIRD_PARTY_LICENSES.md)"
} > "$suites_pin"

# --- suite corpus: exclusion list ---------------------------------------------

suites_excl="$SUITES_DEST/EXCLUDED.txt"
{
  echo "# Generated by scripts/ingest-robot-aql.sh — DO NOT HAND-EDIT."
  echo "# Regenerate by re-running the ingest; hand edits are overwritten."
  echo "#"
  echo "# Every file under the two upstream roots this corpus mirrors that it does"
  echo "# NOT carry, one per line, as:"
  echo "#   <path relative to ${suites_src_rel}> <reason-tag>"
  echo "# The roots are ${suites_rel} and"
  echo "# ${aql_rel}."
  echo "# Sorted with LC_ALL=C so the list has a stable diff across machines."
  echo "#"
  echo "# Reason tags:"
  echo "#   execution-semantics  an expected-result artefact — what a query"
  echo "#                        returned against a loaded EHR. This corpus"
  echo "#                        asserts admissibility (the query parses and"
  echo "#                        survives the semantic lint), never result"
  echo "#                        equality, so no result artefact is vendored."
  echo "#   not-a-suite          a file under AQL/ that is not a Robot suite"
  echo "#                        (*.robot), so it sends no query."
  echo "#"
  echo "# The pin these paths are relative to: AQL_SOURCE.txt, beside this file."
} > "$suites_excl"

# printf with no arguments would still print one empty line.
if ((${#SUITES_EXCLUDED[@]})); then
  printf '%s\n' "${SUITES_EXCLUDED[@]}" | LC_ALL=C sort >> "$suites_excl"
fi

echo "vendored ${#SUITES_CARRIED[@]} files (${n_suites} suites, ${n_inputs} CSVs) into $SUITES_DEST"
echo "pinned ${src_remote}@${EXPECTED_COMMIT:0:12} -> $suites_pin"
echo "excluded ${#SUITES_EXCLUDED[@]} upstream files -> $suites_excl"
