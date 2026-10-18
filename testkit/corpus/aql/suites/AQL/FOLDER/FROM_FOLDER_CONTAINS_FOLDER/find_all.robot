*** Settings ***
Documentation   CHECK FOLDER CONTAINS FOLDER - Find All
...         - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FOLDER.md#find-all-1
...         - *Precondition:* 1. Create EHR; 2. Create Directory with folder_complex_hierarchy.json;
...         - Send AQL query and compare response body with expected file content.
...         - *Postcondition:* Delete EHR using ADMIN endpoint.
Resource        ../../../_resources/keywords/aql_keywords.robot

Suite Setup     Precondition
Suite Teardown  Admin Delete EHR For AQL


*** Test Cases ***
Find All: SELECT f1/uid/value, f1/name/value, f2/uid/value, f2/name/value FROM FOLDER f1 CONTAINS FOLDER f2
    ${query}    Set Variable    SELECT f1/uid/value, f1/name/value, f2/uid/value, f2/name/value FROM FOLDER f1 CONTAINS FOLDER f2
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     10
    ${expected_result_file}      Set Variable
    ...     ${EXPECTED_JSON_DATA_SETS}/folder/expected_folder_contains_folder_find_all.json
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
    Create EHR For AQL
    Create Directory For AQL    folder_complex_hierarchy.json