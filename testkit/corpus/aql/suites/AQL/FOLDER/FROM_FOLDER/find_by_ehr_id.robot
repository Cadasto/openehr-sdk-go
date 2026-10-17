*** Settings ***
Documentation   CHECK AQL RESPONSE ON FOLDER FROM - FIND BY EHR ID
...         - *Precondition:* 1. Create EHR and store ehr_id1; 2. Create Directory with folder_simple_hierarchy.json;
...         - 3. Create EHR and store ehr_id2; 4. Create Directory with folder_simple.json;
...         - Send AQL query and compare response body with expected file content.
...         - *Postcondition:* Delete EHR using ADMIN endpoint.
Resource        ../../../_resources/keywords/aql_keywords.robot

Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Run Keywords    Admin Delete EHR For AQL    ${ehr_id1}      AND
...                             Admin Delete EHR For AQL    ${ehr_id2}


*** Test Cases ***
Find By EHR Id: SELECT e/ehr_id/value, f/uid/value FROM EHR e CONTAINS FOLDER f WHERE e/ehr_id/value = '${ehr_id1}'
    [Documentation]
    ...     Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FOLDER.md#find-by-ehr-id
    ${query}    Set Variable   SELECT e/ehr_id/value, f/uid/value FROM EHR e CONTAINS FOLDER f WHERE e/ehr_id/value = '${ehr_id1}'
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     3
    ${expected_result_file}      Set Variable       ${EXPECTED_JSON_DATA_SETS}/folder/expected_folder_find_by_ehr_id.json
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
    Create EHR For AQL
    Set Suite Variable      ${ehr_id1}      ${ehr_id}
    Create Directory For AQL    folder_simple_hierarchy.json
    Create EHR For AQL
    Set Suite Variable      ${ehr_id2}      ${ehr_id}
    Create Directory For AQL    folder_simple.json