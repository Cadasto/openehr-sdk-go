#!/usr/bin/env bash
#
# corpus-pins.sh — check the EHRbase corpora under testkit/corpus/ against
# their one pin per upstream repository, testkit/corpus/UPSTREAM_PINS.txt.
#
# Several corpora are vendored from the same upstream repository by different
# scripts. Each records the commit it read in its own provenance file; this
# script holds all of them to the single commit UPSTREAM_PINS.txt names for
# that repository, so two corpora can never silently sit on two commits of
# one upstream.
#
# Provenance files checked:
#   flat-conformance/MANIFEST.txt   source_repo: + commit:
#   crossformat/MANIFEST.txt        every `source` record (repo URL + commit)
#   aql/suites/AQL_SOURCE.txt       source_repo: + commit:
#   ROBOT_SOURCE.txt                source_repo: + commit:
#
# Subcommands:
#   verify   Offline: fail on a malformed or duplicated pin line, a provenance
#            file that is missing, a corpus from a repository with no pin, or
#            a corpus whose commit is not its repository's pin. Run by
#            `make ci`.
#   drift    verify, then (best-effort, network) compare each pin with the
#            head of the branch its line names, via `git ls-remote`. The
#            network step never fails the command.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
readonly CORPUS="$ROOT/testkit/corpus"
readonly PINS="$CORPUS/UPSTREAM_PINS.txt"

die() {
  echo "corpus-pins: $*" >&2
  exit 1
}

# pin_lines — print the pin lines of UPSTREAM_PINS.txt as `repo branch
# commit`, comments and blank lines dropped.
pin_lines() {
  awk '!/^[[:space:]]*(#|$)/ { print $1, $2, $3 }' "$PINS"
}

# pin_of <owner/repo> — print the pinned commit of a repository (empty when
# the repository has no pin line).
pin_of() {
  awk -v r="$1" '!/^[[:space:]]*#/ && $1 == r { print $3; exit }' "$PINS"
}

# field <file> <key> — print the value of a `key: value` line.
field() {
  awk -v k="$2:" '$1 == k { print $2; exit }' "$1"
}

# records — print every corpus provenance record as `file repo commit`, the
# file relative to testkit/corpus/. A missing provenance file prints a
# record with an empty repository, which verify reports.
records() {
  local f
  for f in flat-conformance/MANIFEST.txt aql/suites/AQL_SOURCE.txt ROBOT_SOURCE.txt; do
    if [[ -f "$CORPUS/$f" ]]; then
      echo "$f $(field "$CORPUS/$f" source_repo) $(field "$CORPUS/$f" commit)"
    else
      echo "$f - -"
    fi
  done
  f=crossformat/MANIFEST.txt
  if [[ -f "$CORPUS/$f" ]]; then
    awk -F'\t' -v f="$f" '$1 == "source" { sub("^https://github.com/", "", $3); print f, $3, $4 }' "$CORPUS/$f"
  else
    echo "$f - -"
  fi
}

cmd_verify() {
  [[ -f "$PINS" ]] || die "no testkit/corpus/UPSTREAM_PINS.txt"

  local rc=0 repo branch commit extra
  local -A seen=()
  while read -r repo branch commit extra; do
    if [[ -z "$commit" || -n "$extra" || ! "$commit" =~ ^[0-9a-f]{40}$ ]]; then
      echo "  MALFORMED pin line: $repo $branch $commit $extra"
      rc=1
    elif [[ -n "${seen[$repo]:-}" ]]; then
      echo "  DUPLICATE pin: $repo has more than one line"
      rc=1
    fi
    seen[$repo]=1
  done < <(pin_lines)
  [[ $rc -eq 0 ]] || die "UPSTREAM_PINS.txt is malformed"

  local file pin n=0
  while read -r file repo commit; do
    n=$((n + 1))
    if [[ "$repo" == "-" ]]; then
      echo "  MISSING: $file"
      rc=1
      continue
    fi
    pin="$(pin_of "$repo")"
    if [[ -z "$pin" ]]; then
      echo "  UNPINNED: $file records $repo, which has no line in UPSTREAM_PINS.txt"
      rc=1
    elif [[ "$commit" != "$pin" ]]; then
      echo "  STALE: $file records $repo@${commit:0:12}; the pin is ${pin:0:12}"
      rc=1
    fi
  done < <(records)

  if [[ $rc -ne 0 ]]; then
    die "corpora disagree with their pins — re-run the vendoring scripts UPSTREAM_PINS.txt lists"
  fi
  echo "corpus-pins-check: OK ($n provenance records, $(pin_lines | wc -l) pins)"
}

cmd_drift() {
  cmd_verify
  if ! command -v git >/dev/null 2>&1; then
    echo "(git unavailable — skipped upstream drift check)"
    return 0
  fi
  local repo branch commit head
  while read -r repo branch commit; do
    if ! head="$(git ls-remote "https://github.com/$repo" "refs/heads/$branch" 2>/dev/null | cut -f1)" \
      || [[ -z "$head" ]]; then
      echo "(could not reach $repo@$branch — skipped)"
      continue
    fi
    if [[ "$head" == "$commit" ]]; then
      echo "Up to date: $repo@$branch (${commit:0:12})."
    else
      echo "Behind: $repo@$branch is ${head:0:12}; pinned ${commit:0:12}."
    fi
  done < <(pin_lines)
}

main() {
  case "${1:-}" in
    verify) cmd_verify ;;
    drift) cmd_drift ;;
    "" | -h | --help)
      awk 'NR == 1 { next } /^#/ { sub(/^# ?/, ""); print; next } { exit }' "$0"
      ;;
    *) die "unknown subcommand '$1' (want: verify | drift)" ;;
  esac
}

main "$@"
