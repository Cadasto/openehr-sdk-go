#!/usr/bin/env bash
# Vendor the EHRbase Robot AQL FROM-combination corpus (sibling clone) into
# testkit/cassettes/aql/conformance/.
#
# Sibling of scripts/ingest-robot-cassettes.sh, but a different job: that one
# hand-curates and renames composition fixtures, this one copies the upstream
# CSVs byte-for-byte. So the commit recorded in AQL_SOURCE.txt is authoritative
# for the vendored content, not merely the tree the ingest happened to read.
#
# ROBOT_ROOT points at the upstream test_data_sets directory (same convention as
# the cassettes ingest). The clone root is derived from it with
# `git rev-parse --show-toplevel`, so a clone laid out elsewhere works as long as
# ROBOT_ROOT points somewhere inside it.
#
# EXPECTED_COMMIT below is a hard guard: the script refuses to vendor from a tree
# whose HEAD is not that commit, or whose AQL subtree has local modifications, so
# vendoring from the wrong tree cannot happen silently. Refreshing the corpus to
# a newer upstream commit means editing EXPECTED_COMMIT here, in the same commit
# as the refreshed CSVs.
#
# Idempotence: re-running at the same pin must leave `git status` clean. The CSVs
# are byte copies and EXCLUDED.txt is derived from the pinned tree, so both are
# stable by construction; recorded_utc would otherwise churn, so it is read back
# from the existing AQL_SOURCE.txt whenever that file's `commit:` still matches
# EXPECTED_COMMIT. Only a re-pin (or a missing pin file) moves the recorded date.
set -euo pipefail

EXPECTED_COMMIT=206ee8c5389e0c0d75cb2366b1dfb0987644a383

ROBOT="${ROBOT_ROOT:-/src/ehrbase/integration-tests/tests/robot/_resources/test_data_sets}"
# Canonicalise: prefix-stripping below compares against
# `git rev-parse --show-toplevel`, which prints a physical path, so a
# ROBOT_ROOT given with a trailing slash or through a symlink would silently
# fail to strip and record wrong provenance paths.
if [[ -d "$ROBOT" ]]; then
  ROBOT="$(cd "$ROBOT" && pwd -P)"
fi
REPO="$(cd "$(dirname "$0")/.." && pwd)"
DEST="$REPO/testkit/cassettes/aql/conformance"

AQL="$ROBOT/aql/fields_and_results"
SRC="$AQL/from/combinations"

if [[ ! -d "$SRC" ]]; then
  echo "AQL corpus not found at $SRC" >&2
  echo "set ROBOT_ROOT to the upstream tests/robot/_resources/test_data_sets directory" >&2
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
if [[ -n "$(git -C "$src_root" status --porcelain -- "$aql_rel")" ]]; then
  echo "$aql_rel has local modifications in $src_root" >&2
  echo "refusing to vendor: the copied bytes would not match the pinned commit" >&2
  exit 1
fi

# EXCLUDED.txt puts a single space between a path and its reason tag, so a path
# containing whitespace would make its line unparsable. None exists at this pin;
# fail loud before writing anything rather than emit an ambiguous list if a
# future re-pin introduces one.
while IFS= read -r -d '' f; do
  rel=${f#"$AQL/"}
  case "$rel" in
  *[[:space:]]*)
    echo "upstream path contains whitespace: $rel" >&2
    echo "EXCLUDED.txt cannot represent it with a single-space separator —" >&2
    echo "switch the separator to a TAB when re-pinning to such a tree" >&2
    exit 1
    ;;
  esac
done < <(find "$AQL" -type f -print0)

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
# tagged with the reason their rows cannot be reconstructed. At this pin:
# from_simple_and_or_ent.csv has zero references anywhere under the upstream
# tests/ tree, so no suite supplies a query template for its rows. A new
# upstream CSV in from/combinations that is neither in CORPUS nor here makes
# the ingest refuse (below) instead of mis-filing it in EXCLUDED.txt —
# classifying it is the re-pin author's decision, not the fallthrough's.
declare -A KNOWN_UNCONSUMED=(
  ["from_simple_and_or_ent.csv"]=unconsumed-by-suite
)

# Clear the family directories first: a re-pin that drops a CSV upstream must not
# leave the old copy behind pretending to still be vendored.
for fam in "${FAMILIES[@]}"; do
  mkdir -p "$DEST/$fam"
  rm -f "${DEST:?}/$fam"/*.csv
done

declare -A VENDORED=()
for entry in "${CORPUS[@]}"; do
  fam=${entry%% *}
  csv=${entry#* }
  if [[ ! -f "$SRC/$csv" ]]; then
    echo "missing upstream CSV at the pin: $SRC/$csv" >&2
    exit 1
  fi
  # Byte-exact copy — CSV content is never rewritten.
  cp "$SRC/$csv" "$DEST/$fam/$csv"
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
  csv=${rel##*/}
  if [[ -n "${VENDORED[$rel]:-}" || -n "${KNOWN_UNCONSUMED[$csv]:-}" ]]; then
    continue
  fi
  echo "new upstream input CSV at the pin: $rel" >&2
  echo "it is neither vendored (CORPUS) nor a named exclusion (KNOWN_UNCONSUMED) —" >&2
  echo "vendor it and teach the reader its template, or record why it cannot be reconstructed" >&2
  exit 1
done < <(find "$SRC" -type f -name '*.csv' -print0)

# --- provenance pin -----------------------------------------------------------

src_rel=${SRC#"$src_root/"}
src_remote=$(git -C "$src_root" config --get remote.origin.url 2>/dev/null \
  | sed -E 's#^(git@github.com:|https://github.com/)##; s#\.git$##' || true)
src_remote=${src_remote:-ehrbase/integration-tests}
src_date=$(git -C "$src_root" show -s --format=%cI "$EXPECTED_COMMIT")

pin_file="$DEST/AQL_SOURCE.txt"
recorded=""
if [[ -f "$pin_file" ]] && grep -qx "commit: $EXPECTED_COMMIT" "$pin_file"; then
  # Same pin as last time: keep the original recording date so a re-run is a
  # no-op in git. A new pin re-stamps it.
  recorded=$(grep -m1 '^recorded_utc: ' "$pin_file" | cut -d' ' -f2- || true)
fi
recorded=${recorded:-$(date -u +%Y-%m-%d)}

{
  echo "# EHRbase Robot AQL FROM-combination corpus — source provenance pin"
  echo "# Written by scripts/ingest-robot-aql.sh. This pin IS authoritative for"
  echo "# the vendored CSVs: they are copied unmodified from the commit below, so"
  echo "# that commit fully determines their bytes. (Contrast ROBOT_SOURCE.txt one"
  echo "# level up, whose cassettes are hand-curated and renamed, making its pin a"
  echo "# record of the tree the ingest read rather than of the files it produced.)"
  echo "source_repo: ${src_remote}"
  echo "source_path: ${src_rel}"
  echo "commit: ${EXPECTED_COMMIT}"
  echo "commit_date: ${src_date}"
  echo "recorded_utc: ${recorded}"
  echo "source_tree: https://github.com/${src_remote}/tree/${EXPECTED_COMMIT}/${src_rel}"
  echo "license: Apache-2.0 (see testkit/cassettes/THIRD_PARTY_LICENSES.md)"
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
      # refused anything not in KNOWN_UNCONSUMED (set -u faults if not).
      tag=${KNOWN_UNCONSUMED[${rel##*/}]}
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
