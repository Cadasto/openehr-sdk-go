*** Settings ***
Documentation   CHECK SELECT PATHS IN FOLDER
...         - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FOLDER.md#select-paths-in-folder
...         - *Precondition:*
...         - 1. Create EHR;
...         - 2. Create Directory with folder_details.json;
...         - Send AQL query and compare response body with expected file content.
...         - *Postcondition:* Delete EHR using ADMIN endpoint.
Resource        ../../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/folder/combinations/select_paths_in_folder.csv
...     dialect=excel

Suite Setup     Precondition
Suite Teardown  Admin Delete EHR For AQL


*** Test Cases ***
${query_nr} SELECT ${path} FROM FOLDER f
    [Template]      Execute Query
    ${path}    ${expected_file}    ${nr_of_results}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Create EHR For AQL
    Create Directory For AQL    folder_details.json   has_robot_vars=${FALSE}

Execute Query
    [Arguments]     ${path}     ${expected_file}    ${nr_of_results}
    ${query}    Set Variable    SELECT ${path} FROM FOLDER f
    Set AQL And Execute Ad Hoc Query    ${query}
    Log     ${expected_file}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/folder/${expected_file}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    ${exclude_paths}	Create List    root['meta']     root['q']    root['rows'][0][0]['uid']
    ...     root['rows'][0][0]['folders'][0]['uid']   root['rows'][1][0]['uid']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!