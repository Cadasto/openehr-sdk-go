#!/usr/bin/env bash
# One-off ingest from ehrbase Robot integration test data. ROBOT_ROOT is
# required and has no default: it points at the upstream
# tests/robot/_resources/test_data_sets directory of an ehrbase/integration-tests
# clone.
#
# The files are copied from the clone's working tree, so the clone must be
# checked out at the ehrbase/integration-tests commit that
# testkit/corpus/UPSTREAM_PINS.txt pins, with no local change under ROBOT_ROOT;
# the script refuses before it copies anything otherwise. That commit is the
# one every corpus vendored from the repository shares, and the script records
# it in testkit/corpus/ROBOT_SOURCE.txt. It is a provenance pin, not a per-file
# sha256 lock: the fixtures are curated and renamed on the way in.
set -euo pipefail

if [[ -z "${ROBOT_ROOT:-}" ]]; then
  echo "ROBOT_ROOT is unset or empty" >&2
  echo "set ROBOT_ROOT to the upstream tests/robot/_resources/test_data_sets directory of an ehrbase/integration-tests clone" >&2
  exit 1
fi
ROBOT=$ROBOT_ROOT
CAS="$(cd "$(dirname "$0")/.." && pwd)/testkit/corpus"

if [[ ! -d "$ROBOT" ]]; then
  echo "robot data not found at $ROBOT" >&2
  exit 1
fi

readonly SRC_REPO="ehrbase/integration-tests"
PIN=$(awk -v r="$SRC_REPO" '!/^[[:space:]]*#/ && $1 == r { print $3; exit }' "$CAS/UPSTREAM_PINS.txt" 2>/dev/null || true)
if [[ ! "$PIN" =~ ^[0-9a-f]{40}$ ]]; then
  echo "no full commit for $SRC_REPO in $CAS/UPSTREAM_PINS.txt" >&2
  exit 1
fi
src_root=$(git -C "$ROBOT" rev-parse --show-toplevel 2>/dev/null || true)
if [[ -z "$src_root" ]]; then
  echo "$ROBOT is not inside a git checkout — refusing to vendor without provenance" >&2
  exit 1
fi
src_sha=$(git -C "$src_root" rev-parse HEAD)
if [[ "$src_sha" != "$PIN" ]]; then
  echo "the clone $src_root is at $src_sha; the pin is $PIN" >&2
  echo "check out the pin, or update testkit/corpus/UPSTREAM_PINS.txt when re-pinning" >&2
  exit 1
fi
if [[ -n "$(git -C "$ROBOT" status --porcelain -- .)" ]]; then
  echo "$ROBOT has local changes; the copies would not be the pinned bytes" >&2
  exit 1
fi

cp_opt() { cp "$ROBOT/$1" "$CAS/templates/$2"; }
cp_comp_json() { cp "$ROBOT/$1" "$CAS/compositions/$2"; }
cp_comp_xml() { cp "$ROBOT/$1" "$CAS/compositions/$2"; }
cp_rm_json() { cp "$ROBOT/$1" "$CAS/rm/$2"; }
cp_sub() { cp "$ROBOT/$1" "$CAS/submissions/$2"; }

mkdir -p "$CAS/templates" "$CAS/compositions" "$CAS/rm" "$CAS/submissions"

# 1 — minimal entry suite
cp_opt valid_templates/minimal/minimal_evaluation.opt 'minimal_evaluation.en.v1.opt'
cp_comp_json compositions/CANONICAL_JSON/minimal_evaluation.en.v1__.json 'minimal_evaluation.en.v1.json'
cp_comp_xml xml_compositions/minimal_evaluation.en.v1.instance_xml_input_1.xml 'minimal_evaluation.en.v1.xml'

cp_opt valid_templates/minimal/minimal_observation.opt 'minimal_observation.en.v1.opt'
cp_comp_xml xml_compositions/minimal_observation.en.v1.instance_xml_input_1.xml 'minimal_observation.en.v1.xml'

cp_opt valid_templates/minimal/minimal_admin.opt 'minimal_admin.en.v1.opt'
cp_comp_xml xml_compositions/minimal_admin.en.v1.instance_xml_input_1.xml 'minimal_admin.en.v1.xml'

cp_opt valid_templates/minimal/minimal_instruction.opt 'minimal_instruction.en.v1.opt'
cp_comp_json compositions/CANONICAL_JSON/minimal_instruction_1.composition.json 'minimal_instruction.en.v1.json'
cp_comp_xml xml_compositions/minimal_instruction.en.v1.instance_xml_input_1.xml 'minimal_instruction.en.v1.xml'

# minimal_action.en.v1 OPT does not compile (duplicate AQL); use minimal_action_2 instead.
cp_opt valid_templates/minimal/minimal_action_2.opt 'minimal_action_2.opt'
cp_comp_json valid_templates/minimal/minimal_action_2.instance.composition.json 'minimal_action_2.json'
cp_comp_xml valid_templates/minimal/minimal_action_2.instance.composition.xml 'minimal_action_2.xml'

# 4 — validation (compile-passing) + Test_dv_* OPT+JSON
cp_opt valid_templates/validation/clinical_content_validation.opt 'clinical_content_validation.opt'
# Upstream constrains ITEM_TABLE.rotated, an attribute RM 1.2.0's ITEM_TABLE
# does not define; drop that constraint (testkit/corpus/README.md).
python3 - "$CAS/templates/clinical_content_validation.opt" <<'PY'
import re, sys
p = sys.argv[1]
s = open(p, encoding='utf-8', newline='').read()
s, n = re.subn(r'[ \t]*<attributes xsi:type="C_SINGLE_ATTRIBUTE">\s*<rm_attribute_name>rotated</rm_attribute_name>.*?</attributes>\r?\n', '', s, flags=re.S)
if n != 1:
    sys.exit(f'expected one ITEM_TABLE.rotated block in {p}, found {n}')
open(p, 'w', encoding='utf-8', newline='').write(s)
PY
cp_comp_json compositions/CANONICAL_JSON/clinical_content_validation__full.json 'clinical_content_validation.json'

for opt in "$ROBOT"/valid_templates/all_types/Test_dv_*.opt; do
  [[ -f "$opt" ]] || continue
  base=$(basename "$opt" .opt)
  json="$ROBOT/compositions/CANONICAL_JSON/${base}.json"
  [[ -f "$json" ]] || json="$ROBOT/compositions/CANONICAL_JSON/${base}__.json"
  [[ -f "$json" ]] || continue
	cp "$opt" "$CAS/templates/${base}.opt"
	cp "$json" "$CAS/compositions/${base}.json"
done

# all_types OPT+JSON pairs added after the 206ee8c pin (not Test_dv_*).
# Valid canonical JSON only — invalid/update variants stay upstream.
for base in \
  family_history.v.1.2.3 \
  my_spanish_template_v0 \
  terminology_test.ehrbase.org.v1 \
  terminology_test2.ehrbase.org.v1
do
  opt="$ROBOT/valid_templates/all_types/${base}.opt"
  [[ -f "$opt" ]] || { echo "missing OPT: $opt" >&2; exit 1; }
  json="$ROBOT/compositions/CANONICAL_JSON/${base}.json"
  [[ -f "$json" ]] || json="$ROBOT/compositions/CANONICAL_JSON/${base}__.json"
  [[ -f "$json" ]] || { echo "missing composition JSON for $base" >&2; exit 1; }
  cp "$opt" "$CAS/templates/${base}.opt"
  cp "$json" "$CAS/compositions/${base}.json"
done

# 5 — persistent_minimal
cp_opt valid_templates/minimal_persistent/persistent_minimal.opt 'persistent_minimal.en.v1.opt'
cp_comp_json compositions/CANONICAL_JSON/persistent_minimal.en.v1__full.json 'persistent_minimal.en.v1.json'
cp_comp_xml valid_templates/minimal_persistent/persistent_minimal.composition.xml 'persistent_minimal.en.v1.xml'

# 2 — EHR_STATUS (flat rm/ names)
for f in "$ROBOT"/ehr/valid/*.json "$ROBOT"/ehr/invalid/*.json; do
  [[ -f "$f" ]] || continue
  name=$(basename "$f" .json)
  if [[ "$f" == *'/valid/'* ]]; then
    cp_rm_json "ehr/valid/$(basename "$f")" "ehr_status_valid_${name}.json"
  else
    cp_rm_json "ehr/invalid/$(basename "$f")" "ehr_status_invalid_${name}.json"
  fi
done

# 3 — FOLDER / directory
for f in "$ROBOT"/directory/*.json; do
  [[ -f "$f" ]] || continue
  cp_rm_json "directory/$(basename "$f")" "folder_$(basename "$f" .json).json"
done
for f in "$ROBOT"/directory/update/*.json; do
  [[ -f "$f" ]] || continue
  cp_rm_json "directory/update/$(basename "$f")" "folder_update_$(basename "$f" .json).json"
done

# 6 — contribution submission wire (CONTRIBUTION + inline ORIGINAL_VERSION)
while IFS= read -r -d '' f; do
  base=$(basename "$f")
  # Reject filenames carrying shell-special or path-traversal characters
  # before they flow into copy targets (the Robot source is external).
  if [[ ! "$base" =~ ^[A-Za-z0-9._-]+$ ]]; then
    echo "skip (unsafe filename): $base" >&2
    continue
  fi
  # ~1.2MB bulk payload; smaller contribution fixtures cover multi-version cases.
  if [[ "$base" == "contribution.create_multiple_compositions.json" ]]; then
    continue
  fi
  rel=${f#"$ROBOT/"}
  safe=$(printf '%s' "$rel" | tr '/' '_')
  cp_sub "$rel" "$safe"
done < <(find "$ROBOT/contributions" -name '*.json' -print0)

# Record the upstream source commit so the curated fixtures carry provenance.
# The Robot data is copied and renamed by hand, so unlike
# flat-conformance/MANIFEST.txt this pins ONLY the source commit the ingest
# read from — it is not a per-file sha256 lock. recorded_utc is kept while the
# commit is unchanged, so a re-run at the pin rewrites an identical file.
src_date=$(git -C "$src_root" show -s --format=%cI "$PIN")
src_rel=$(git -C "$ROBOT" rev-parse --show-prefix)
src_rel=${src_rel%/}
recorded=""
if [[ -f "$CAS/ROBOT_SOURCE.txt" ]] && grep -qx "commit: $PIN" "$CAS/ROBOT_SOURCE.txt"; then
  recorded=$(sed -n 's/^recorded_utc: //p' "$CAS/ROBOT_SOURCE.txt" | head -n 1)
fi
{
  echo "# EHRbase Robot integration-test data — source provenance pin"
  echo "# Written by scripts/ingest-robot-fixtures.sh from the commit"
  echo "# UPSTREAM_PINS.txt pins. The curated fixtures are copied and renamed by"
  echo "# hand, so this pins only the upstream commit — NOT a per-file sha256 lock."
  echo "source_repo: ${SRC_REPO}"
  echo "source_path: ${src_rel}"
  echo "commit: ${PIN}"
  echo "commit_date: ${src_date}"
  echo "recorded_utc: ${recorded:-$(date -u +%Y-%m-%d)}"
  echo "source_tree: https://github.com/${SRC_REPO}/tree/${PIN}/${src_rel}"
  echo "license: Apache-2.0 (see THIRD_PARTY_LICENSES.md)"
} > "$CAS/ROBOT_SOURCE.txt"
echo "recorded source pin ${SRC_REPO}@${PIN:0:12} -> $CAS/ROBOT_SOURCE.txt"

echo "ingested robot fixtures into $CAS"
