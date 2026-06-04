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
  # ~1.2MB bulk payload; smaller contribution fixtures cover multi-version cases.
  if [[ "$base" == "contribution.create_multiple_compositions.json" ]]; then
    continue
  fi
  rel=${f#"$ROBOT/"}
  safe=$(echo "$rel" | tr '/' '_')
  cp_sub "$rel" "$safe"
done < <(find "$ROBOT/contributions" -name '*.json' -print0)

echo "ingested robot cassettes into $CAS"
