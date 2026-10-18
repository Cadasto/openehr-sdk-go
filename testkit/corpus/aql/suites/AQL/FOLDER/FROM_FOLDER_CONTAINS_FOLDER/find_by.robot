*** Settings ***
Documentation   CHECK FOLDER CONTAINS FOLDER - Find By
...         - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FOLDER.md#find-by
...         - *Precondition:* 1. Create EHR; 2. Create Directory with folder_complex_hierarchy.json;
...         - Send AQL query and compare response body with expected file content.
...         - *Postcondition:* Delete EHR using ADMIN endpoint.
Resource        ../../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/folder/combinations/folder_contains_folder_find_by.csv
...     dialect=excel

Suite Setup     Precondition
Suite Teardown  Admin Delete EHR For AQL    ${ehr_id}


*** Test Cases ***
${query_nr} SELECT f2/uid/value, f2/name/value FROM FOLDER f1[${predicate1}] CONTAINS FOLDER f2[${predicate2}]
    [Template]      Execute Query
    ${predicate1}    ${predicate2}      ${expected_file}    ${nr_of_results}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Create EHR For AQL
    Create Directory For AQL    folder_complex_hierarchy.json

Execute Query
    [Arguments]     ${predicate1}     ${predicate2}     ${expected_file}    ${nr_of_results}
    ${query}    Set Variable    SELECT f2/uid/value, f2/name/value FROM FOLDER f1[${predicate1}] CONTAINS FOLDER f2[${predicate2}]
    Set AQL And Execute Ad Hoc Query    ${query}
    Log     ${expected_file}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/folder/${expected_file}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!