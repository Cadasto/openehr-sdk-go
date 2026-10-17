*** Settings ***
Documentation   CHECK FOLDER CONTAINS COMPOSITION - Multiple Compositions in a Folder
...         - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FOLDER.md#multi-comp-in-a-folder
...         - *Precondition:*
...         - 1. Upload OPT; 2. Create EHR and store ehr_id1;
...         - 3. Create 2 compositions with conformance_ehrbase.de.v0_max.json and store their compo_ids;
...         - 4. Create Directory with folder_multi_compositions.json;
...         - 5. Create 1 composition with conformance_ehrbase.de.v0_max.json;
...         - Send AQL query and compare response body with expected file content.
...         - *Postcondition:* Delete EHR using ADMIN endpoint.
Resource        ../../../_resources/keywords/aql_keywords.robot

Suite Setup     Precondition
Suite Teardown  Admin Delete EHR For AQL

*** Variables ***
${q}   SELECT c/uid/value FROM FOLDER f CONTAINS COMPOSITION c


*** Test Cases ***
Multi Comp In A Folder: ${q}
    Set Test Variable   ${query}    ${q}
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     2
    ${expected_result_file}      Set Variable
    ...     ${EXPECTED_JSON_DATA_SETS}/folder/expected_contains_compo_multi_compo_in_folder.json
    ${exclude_paths}    Create List    root['meta']     root['q']
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}      exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}
    ...		ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      conformance_ehrbase.de.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Set Suite Variable      ${c_uid1}   ${composition_short_uid}
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Set Suite Variable      ${c_uid2}   ${composition_short_uid}
    Create Directory For AQL    folder_multi_compositions.json   has_robot_vars=${TRUE}
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json