#!/usr/bin/env bash
# Vendor the EHRbase Robot AQL suite corpus, the tree's one AQL corpus, from one
# pinned commit of the upstream integration-tests repository into
# testkit/corpus/aql/suites/: every Robot suite under tests/robot/AQL/ and every
# combination CSV under
# tests/robot/_resources/test_data_sets/aql/fields_and_results/**/combinations/,
# each at its path relative to upstream tests/robot/, so a suite's
# ${PROJECT_ROOT}/tests/robot/<rest> reads suites/<rest>. The corpus carries its
# own AQL_SOURCE.txt and generated EXCLUDED.txt.
#
# Sibling of scripts/ingest-robot-fixtures.sh, but a different job: that one
# hand-curates and renames composition fixtures, this one copies the upstream
# files byte for byte. It reads them from the pinned commit itself, never from
# the clone's working tree: every path it lists comes from `git ls-tree` of
# EXPECTED_COMMIT, and every byte it vendors from `git cat-file blob` of that
# tree's blobs, which applies no attribute, filter or line-ending conversion.
# What is checked out, modified, untracked, ignored or unreadable in the clone
# changes nothing. So the commit recorded in AQL_SOURCE.txt determines the
# vendored content, not merely the tree the ingest happened to read.
#
# ROBOT_ROOT points at the upstream tests/robot/_resources/test_data_sets
# directory of the clone (same convention as the fixtures ingest). It locates
# the clone, and its path inside the clone names the two roots read from the
# commit: tests/robot/AQL, two levels above it, and its
# aql/fields_and_results subtree.
#
# EXPECTED_COMMIT below is the pin. The script refuses unless it names, in full,
# a commit present in the clone; HEAD may point anywhere. Refreshing the corpus
# to a newer upstream commit means editing EXPECTED_COMMIT here, in the same
# commit as the refreshed files.
#
# The pin records source_repo, the GitHub owner/repo of the clone's origin
# remote. Only a github.com origin names one: https://github.com/OWNER/REPO,
# git@github.com:OWNER/REPO or ssh://git@github.com/OWNER/REPO, each with or
# without .git. Any other origin (a local path, another host, none) makes the
# script refuse, unless ROBOT_SOURCE_REPO=OWNER/REPO names the upstream
# explicitly; when set, that value is recorded whatever the origin.
#
# It fails closed: before the first byte is written it refuses a pin with no
# Robot suite under AQL/ or no combination CSV, an entry that is not a regular
# file, a path containing whitespace, a file under AQL/ that is neither a suite
# nor a plain-text note (*.txt, *.md), a CSV it cannot classify as input, and
# any file no rule below knows.
#
# Idempotence: re-running at the same pin must leave `git status` clean. The
# vendored files are byte copies and EXCLUDED.txt is derived from the pinned
# tree, so all are stable by construction; recorded_utc would otherwise churn, so
# it is read back from the existing AQL_SOURCE.txt whenever that file's
# `commit:` still matches EXPECTED_COMMIT. Only a re-pin (or a missing pin file)
# moves the recorded date.
set -euo pipefail

EXPECTED_COMMIT=b4625fc28d6785c05adfd6db26339b4bb2272e06

ROBOT="${ROBOT_ROOT:-/src/ehrbase/integration-tests/tests/robot/_resources/test_data_sets}"
REPO="$(cd "$(dirname "$0")/.." && pwd)"
SUITES_DEST="$REPO/testkit/corpus/aql/suites"

if [[ ! -d "$ROBOT" ]]; then
  echo "ROBOT_ROOT $ROBOT is not a directory" >&2
  echo "set ROBOT_ROOT to the upstream tests/robot/_resources/test_data_sets directory" >&2
  exit 1
fi

src_root=$(git -C "$ROBOT" rev-parse --show-toplevel 2>/dev/null || true)
if [[ -z "$src_root" ]]; then
  echo "$ROBOT is not inside a git checkout — refusing to vendor without provenance" >&2
  exit 1
fi

# ROBOT_ROOT's path inside the clone, as git spells it: relative to the clone
# root, symbolic links resolved, so a trailing slash or a symlinked ROBOT_ROOT
# names the same roots.
robot_rel=$(git -C "$ROBOT" rev-parse --show-prefix)
robot_rel=${robot_rel%/}

# The suite corpus mirrors upstream tests/robot/, two levels above ROBOT_ROOT.
# The suites name their CSVs as ${PROJECT_ROOT}/tests/robot/_resources/
# test_data_sets/..., so a ROBOT_ROOT that is not that directory would vendor
# the CSVs at paths no suite reads.
case "$robot_rel" in
*/_resources/test_data_sets) ;;
*)
  echo "ROBOT_ROOT $ROBOT is not <tests/robot>/_resources/test_data_sets" >&2
  echo "the suite corpus mirrors tests/robot, so its CSVs must sit at that path" >&2
  exit 1
  ;;
esac
tests_robot_rel=${robot_rel%/_resources/test_data_sets}
suites_rel=$tests_robot_rel/AQL
aql_rel=$robot_rel/aql/fields_and_results
# far_rel is aql_rel relative to tests/robot, the spelling the suite corpus
# uses for every path.
far_rel=${aql_rel#"$tests_robot_rel/"}

# --- validate -----------------------------------------------------------------
#
# Every refusal runs before the first byte under $SUITES_DEST moves. Clearing
# the destination first and only then discovering an unclassifiable upstream
# file would empty the vendored corpus and abort, leaving it in a state the pin
# does not describe. So this pass only reads.

# The pin, by its full object name: an abbreviation or a ref name would be
# recorded as the commit, and neither fixes the bytes.
if [[ "$(git -C "$src_root" rev-parse --verify --quiet "$EXPECTED_COMMIT^{commit}" 2>/dev/null || true)" != "$EXPECTED_COMMIT" ]]; then
  echo "EXPECTED_COMMIT $EXPECTED_COMMIT is not a commit in the clone $src_root" >&2
  echo "(by its full object name); fetch it into the clone, or update" >&2
  echo "EXPECTED_COMMIT in $(basename "$0") when re-pinning" >&2
  exit 1
fi
src_date=$(git -C "$src_root" show -s --format=%cI "$EXPECTED_COMMIT")

# source_repo: the GitHub owner/repo the pin records and builds its
# source_tree URL from. A GitHub owner is letters, digits and hyphens; a
# repository name adds dots and underscores, but is never . or ..
owner_repo_re='^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9._-]+$'
is_owner_repo() {
  [[ "$1" =~ $owner_repo_re && "${1#*/}" != . && "${1#*/}" != .. ]]
}
if [[ -n "${ROBOT_SOURCE_REPO:-}" ]]; then
  if ! is_owner_repo "$ROBOT_SOURCE_REPO"; then
    echo "ROBOT_SOURCE_REPO=$ROBOT_SOURCE_REPO is not a GitHub OWNER/REPO" >&2
    exit 1
  fi
  src_remote=$ROBOT_SOURCE_REPO
else
  # The URL itself is not echoed: a remote can carry credentials.
  origin=$(git -C "$src_root" config --get remote.origin.url || true)
  case "$origin" in
  https://github.com/*) src_remote=${origin#https://github.com/} ;;
  git@github.com:*) src_remote=${origin#git@github.com:} ;;
  ssh://git@github.com/*) src_remote=${origin#ssh://git@github.com/} ;;
  *) src_remote= ;;
  esac
  src_remote=${src_remote%.git}
  if ! is_owner_repo "$src_remote"; then
    echo "the origin remote of $src_root is not a github.com repository URL" >&2
    echo "accepted: https://github.com/OWNER/REPO, git@github.com:OWNER/REPO or" >&2
    echo "ssh://git@github.com/OWNER/REPO, each with or without .git; for any other" >&2
    echo "origin, set ROBOT_SOURCE_REPO=OWNER/REPO to name the upstream the pin records" >&2
    exit 1
  fi
fi

# Every entry under the two roots at the pin. The listing goes to a file rather
# than through a process substitution, so a failing git stops the run under
# set -e instead of yielding a short list.
tmp=$(mktemp -d)
trap 'rm -rf -- "$tmp"' EXIT
git -C "$src_root" ls-tree -r -z --full-tree "$EXPECTED_COMMIT" -- "$suites_rel" "$aql_rel" > "$tmp/listing"

# Classify every entry, by its path relative to tests/robot, the spelling a
# suite's DataDriver `file=${PROJECT_ROOT}/tests/robot/<rest>` uses, so a
# carried file lands at suites/<rest>. Each file is carried, excluded with a
# reason tag, or refused: a file no rule knows is the re-pin author's decision,
# not something to file under a guessed tag.
declare -A BLOB=()
SUITES_CARRIED=()
SUITES_EXCLUDED=()
n_suites=0
n_inputs=0
while IFS= read -r -d '' entry; do
  path=${entry#*$'\t'}
  read -r mode _ obj <<<"${entry%%$'\t'*}"
  rel=${path#"$tests_robot_rel/"}

  # Any path under the two roots may be named on an EXCLUDED.txt line, path and
  # reason tag separated by one space, and a re-pin can move a file from carried
  # to excluded by changing a rule. A path with whitespace could not be named
  # unambiguously there, so it is refused wherever it is classified. None exists
  # at this pin.
  case "$rel" in
  *[[:space:]]*)
    echo "upstream path contains whitespace: $rel" >&2
    echo "every path under the two roots must be nameable on an EXCLUDED.txt line," >&2
    echo "where one space separates it from its reason tag, carried or not" >&2
    exit 1
    ;;
  esac

  # A symbolic link or a submodule is neither a byte copy the commit
  # determines nor a file the exclusion list can name.
  case "$mode" in
  100644 | 100755) ;;
  *)
    echo "upstream entry is not a regular file: $rel" >&2
    echo "the AQL corpus carries and lists byte copies of regular files only" >&2
    exit 1
    ;;
  esac
  BLOB[$rel]=$obj

  case "$rel" in
  AQL/*.robot)
    SUITES_CARRIED+=("$rel")
    n_suites=$((n_suites + 1))
    ;;
  AQL/*.txt | AQL/*.md)
    SUITES_EXCLUDED+=("$rel not-a-suite")
    ;;
  AQL/*)
    # A resource or library file beside the suites could send queries of its
    # own; carrying or excluding it is the re-pin author's decision.
    echo "upstream file under AQL/ is neither a Robot suite nor a plain-text note: $rel" >&2
    echo "teach the ingest its rule (carry it, or exclude it with a reason tag)" >&2
    exit 1
    ;;
  "$far_rel"/*.csv)
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
  "$far_rel"/*.json)
    SUITES_EXCLUDED+=("$rel execution-semantics")
    ;;
  *)
    echo "unclassified upstream file under the AQL suite roots: $rel" >&2
    echo "it is neither a suite, a combination CSV nor an expected-result JSON —" >&2
    echo "teach the ingest its rule (carry it, or exclude it with a reason tag)" >&2
    exit 1
    ;;
  esac
done < "$tmp/listing"

# An empty root is a pin this corpus cannot describe, not an empty corpus.
if ((n_suites == 0)); then
  echo "no Robot AQL suite (*.robot) under $suites_rel at the pin" >&2
  echo "refusing to vendor a suite corpus with no suites" >&2
  exit 1
fi
if ((n_inputs == 0)); then
  echo "no combination CSV under $aql_rel/**/combinations/ at the pin" >&2
  echo "refusing to vendor suites without the data they read" >&2
  exit 1
fi

# blob_to OBJECT FILE writes the blob OBJECT of the clone to FILE, byte for
# byte: `git cat-file blob` prints the stored content with no conversion.
blob_to() {
  git -C "$src_root" cat-file blob "$1" > "$2"
}

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

# --- copy ---------------------------------------------------------------------

# Clear both mirrored subtrees first: a re-pin that drops an upstream file must
# not leave the old copy behind pretending to still be vendored.
rm -rf "${SUITES_DEST:?}/AQL" "${SUITES_DEST:?}/_resources"
mkdir -p "$SUITES_DEST"
for rel in "${SUITES_CARRIED[@]}"; do
  mkdir -p "$SUITES_DEST/${rel%/*}"
  blob_to "${BLOB[$rel]}" "$SUITES_DEST/$rel"
done

# --- provenance pin -----------------------------------------------------------

suites_src_rel=$tests_robot_rel
suites_pin="$SUITES_DEST/AQL_SOURCE.txt"
suites_recorded=$(recorded_utc_for "$suites_pin")

{
  echo "# EHRbase Robot AQL suite corpus — source provenance pin"
  echo "# Written by scripts/ingest-robot-aql.sh, which vendors this corpus alone."
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

# --- exclusion list -----------------------------------------------------------

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
