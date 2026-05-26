#!/usr/bin/env bash
# Build GitHub Release notes for vX.Y.Z.
#
# Extracts the matching ## [X.Y.Z] section from CHANGELOG.md and appends a
# compatibility metadata block (SDK tag, Go minimum, openEHR REST, BMM pins,
# git revision) per docs/releases.md § Compatibility metadata.
#
# Usage:
#   scripts/release-notes.sh 0.1.0          # writes to stdout
#   scripts/release-notes.sh 0.1.0 out.md   # writes to file
#
# Inputs the workflow expects:
#   - CHANGELOG.md         contains a "## [<version>] - YYYY-MM-DD" section
#   - go.mod               first `go <ver>` directive supplies Go minimum
#   - resources/bmm/*.bmm.json  BMM corpus filenames feed the BMM row
#
# Exits non-zero if the CHANGELOG section is missing — the workflow uses
# that to fail-fast before drafting a release.
set -euo pipefail

version="${1:?usage: $0 <version-without-v> [out-file]}"
out="${2:-/dev/stdout}"
changelog="${CHANGELOG_FILE:-CHANGELOG.md}"

if [ ! -f "$changelog" ]; then
  echo "error: $changelog not found" >&2
  exit 1
fi

section=$(awk -v v="$version" '
  $0 ~ "^## \\[" v "\\]" { capture=1; print; next }
  capture && /^## \[/   { capture=0 }
  capture               { print }
' "$changelog")

if [ -z "$section" ]; then
  echo "error: no CHANGELOG section found for [${version}] in ${changelog}" >&2
  exit 2
fi

go_min=$(awk '$1=="go" {print $2; exit}' go.mod)
if [ -z "$go_min" ]; then
  echo "error: could not read 'go <version>' from go.mod" >&2
  exit 3
fi

short_sha=$(git rev-parse --short HEAD)

bmm_pins=$(ls resources/bmm/*.bmm.json 2>/dev/null \
  | sed -E 's#.*/##; s/\.bmm\.json$//' \
  | sort \
  | awk 'BEGIN{ORS=""} {if(NR>1) printf ", "; printf "`%s`", $0}')
if [ -z "$bmm_pins" ]; then
  bmm_pins="(none)"
fi

{
  printf '%s\n\n' "$section"
  printf '### Compatibility (auto-generated)\n\n'
  printf '| Concept | Value |\n'
  printf '|---|---|\n'
  printf '| SDK semver | `v%s` |\n' "$version"
  printf '| Go toolchain (minimum) | `%s` |\n' "$go_min"
  printf '| openEHR REST | `1.1.0-development` |\n'
  printf '| BMM corpus | %s |\n' "$bmm_pins"
  printf '| Git revision | `%s` |\n' "$short_sha"
} > "$out"
