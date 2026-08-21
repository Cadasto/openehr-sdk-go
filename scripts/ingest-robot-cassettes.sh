#!/usr/bin/env bash
# One-off ingest from ehrbase Robot integration test data (sibling clone).
set -euo pipefail

ROBOT="${ROBOT_ROOT:-/src/ehrbase/integration-tests/tests/robot/_resources/test_data_sets}"
CAS="$(cd "$(dirname "$0")/.." && pwd)/testkit/cassettes"

if [[ ! -d "$ROBOT" ]]; then
  echo "robot data not found at $ROBOT" >&2
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

# Record the upstream source commit so the curated cassettes carry provenance.
# Best-effort: the Robot data is copied and renamed by hand, so unlike
# flat-conformance/MANIFEST.txt this pins ONLY the source commit the ingest
# read from — it is not a per-file sha256 lock.
src_root=$(git -C "$ROBOT" rev-parse --show-toplevel 2>/dev/null || true)
if [[ -n "$src_root" ]]; then
  src_sha=$(git -C "$src_root" rev-parse HEAD 2>/dev/null || echo unknown)
  src_date=$(git -C "$src_root" log -1 --format=%cI 2>/dev/null || echo unknown)
  src_remote=$(git -C "$src_root" config --get remote.origin.url 2>/dev/null \
    | sed -E 's#^(git@github.com:|https://github.com/)##; s#\.git$##' || true)
  src_rel=${ROBOT#"$src_root/"}
  {
    echo "# EHRbase Robot integration-test data — source provenance pin"
    echo "# Written by scripts/ingest-robot-cassettes.sh. Best-effort: the curated"
    echo "# cassettes are copied and renamed by hand, so this pins only the upstream"
    echo "# commit the ingest read from — NOT a per-file sha256 lock."
    echo "source_repo: ${src_remote:-unknown}"
    echo "source_path: ${src_rel:-unknown}"
    echo "commit: ${src_sha}"
    echo "commit_date: ${src_date}"
    echo "recorded_utc: $(date -u +%Y-%m-%d)"
    echo "source_tree: https://github.com/${src_remote:-ehrbase/integration-tests}/tree/${src_sha}/${src_rel}"
    echo "license: Apache-2.0 (see THIRD_PARTY_LICENSES.md)"
  } > "$CAS/ROBOT_SOURCE.txt"
  echo "recorded source pin ${src_remote:-?}@${src_sha:0:12} -> $CAS/ROBOT_SOURCE.txt"
else
  echo "WARNING: $ROBOT is not a git checkout; ROBOT_SOURCE.txt not updated" >&2
fi

echo "ingested robot cassettes into $CAS"
