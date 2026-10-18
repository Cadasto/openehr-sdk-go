*** Settings ***
Documentation   CHECK FOLDER CONTAINS COMPOSITION - Select by name
...         - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FOLDER.md#select-by-name
...         - *Precondition:*
...         - 1. Upload OPT; 2. Create EHR;
...         3. Create 4 compositions with conformance_ehrbase.de.v0_max.json and store their compo_ids;
...         - 4. Create Directory with folder_with_compositions.json;
...         - Send AQL query and compare response body with expected file content.
...         - *Postcondition:* Delete EHR using ADMIN endpoint.
Resource        ../../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/folder/combinations/folder_contains_compo_select_by_name.csv
...     dialect=excel

Suite Setup     Precondition
Suite Teardown  Admin Delete EHR For AQL


*** Test Cases ***
${query_nr} SELECT c/uid/value FROM FOLDER f CONTAINS COMPOSITION c WHERE f/name/value = '${name}'
    [Template]      Execute Query
    ${name}    ${expected_file}    ${nr_of_results}



*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      conformance_ehrbase.de.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Set Suite Variable      ${c_uid1}   ${composition_short_uid}
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Set Suite Variable      ${c_uid2}   ${composition_short_uid}
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Set Suite Variable      ${c_uid3}   ${composition_short_uid}
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Set Suite Variable      ${c_uid4}   ${composition_short_uid}
    Create Directory For AQL    folder_with_compositions.json   has_robot_vars=${TRUE}

Execute Query
    [Arguments]     ${name}     ${expected_file}    ${nr_of_results}
    ${query}    Set Variable    SELECT c/uid/value FROM FOLDER f CONTAINS COMPOSITION c WHERE f/name/value = '${name}'
    Set AQL And Execute Ad Hoc Query    ${query}
    ${expected_result_file}     Set Variable    ${EXPECTED_JSON_DATA_SETS}/folder/${expected_file}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    ${exclude_paths}	Create List    root['meta']     root['q']
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!