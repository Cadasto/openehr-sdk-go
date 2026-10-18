*** Settings ***
Documentation   CHECK FOLDER CONTAINS COMPOSITION - MULTIPLE EHRS AND COMPOSITIONS SELECT BY IDS
...         - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FOLDER.md#multiple-ehr-and-compositions-select-by-ids
...         - *Precondition:*
...         - 1. Upload OPT; 2. Create EHR and store ehr_id1;
...         - 3. Create 4 compositions with conformance_ehrbase.de.v0_max.json and store their compo_ids;
...         - 4. Create Directory with folder_with_compositions.json replacing compo_ids vars;
...         - 5. Create 1 composition with conformance_ehrbase.de.v0_max.json;
...         - 6. Create EHR and store ehr_id2;
...         - 7. Create 4 compositions with conformance_ehrbase.de.v0_max.json and store their compo_ids;
...         - 8. Create Directory with folder_with_compositions1.json replacing compo_ids vars generated at step 7;
...         - 9. Create 1 composition with conformance_ehrbase.de.v0_max.json;
...         - Send AQL query and compare response body with expected file content.
...         - *Postcondition:* Delete EHR using ADMIN endpoint.
Resource        ../../../_resources/keywords/aql_keywords.robot


Suite Setup     Precondition
Suite Teardown  Run Keywords    Admin Delete EHR For AQL    ${ehr_id1}      AND
...                             Admin Delete EHR For AQL    ${ehr_id2}


*** Test Cases ***
SELECT e/ehr_id/value, f/uid/value, c/uid/value FROM EHR e CONTAINS FOLDER f CONTAINS COMPOSITION c WHERE e/ehr_id/value = '${ehr_id2}' AND f/uid/value = '70939d97-8add-4419-b27c-516e64b1c744' AND c/uid/value = '${c_uid4b}'
    ${query}    Set Variable
    ...     SELECT e/ehr_id/value, f/uid/value, c/uid/value FROM EHR e CONTAINS FOLDER f CONTAINS COMPOSITION c WHERE e/ehr_id/value = '${ehr_id2}' AND f/uid/value = '70939d97-8add-4419-b27c-516e64b1c744' AND c/uid/value = '${c_uid4b}'
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     1
    ${expected_result_file}      Set Variable
    ...     ${EXPECTED_JSON_DATA_SETS}/folder/expected_multiple_ehrs_and_compo_select_by_ids.json
    ${exclude_paths}    Create List    root['meta']     root['q']
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}      exclude_paths=${exclude_paths}
    ...		ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      conformance_ehrbase.de.v0.opt
    Create EHR For AQL
    Set Suite Variable      ${ehr_id1}      ${ehr_id}
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Set Suite Variable      ${c_uid1}   ${composition_short_uid}
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Set Suite Variable      ${c_uid2}   ${composition_short_uid}
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Set Suite Variable      ${c_uid3}   ${composition_short_uid}
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Set Suite Variable      ${c_uid4}   ${composition_short_uid}
    Create Directory For AQL    folder_with_compositions.json   has_robot_vars=${TRUE}
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Create EHR For AQL
    Set Suite Variable      ${ehr_id2}      ${ehr_id}
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Set Suite Variable      ${c_uid1b}   ${composition_short_uid}
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Set Suite Variable      ${c_uid2b}   ${composition_short_uid}
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Set Suite Variable      ${c_uid3b}   ${composition_short_uid}
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Set Suite Variable      ${c_uid4b}   ${composition_short_uid}
    Create Directory For AQL    folder_with_compositions1.json   has_robot_vars=${TRUE}
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json