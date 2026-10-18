*** Settings ***
Documentation   CHECK GET ehr_status FROM EHR WITH WHERE AQL RESULT
...             \nCovers:
...             - https://github.com/ehrbase/openEHR_SDK/blob/develop/client/src/test/java/org/ehrbase/openehr/sdk/client/openehrclient/defaultrestclient/systematic/ehrquery/CanonicalEhrQuery3IT.java
...             \n*Test testEhrAutoWhere()*
Resource        ../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/external/drill_down_ehr/combinations/ehr_paths_where.csv
...     dialect=excel

Suite Setup     Precondition
Suite Teardown  Admin Delete EHR For AQL


*** Variables ***
${EXPECTED_JSON_RESULTS}    ${EXPECTED_JSON_DATA_SETS}/external/drill_down_ehr


*** Test Cases ***
SELECT e/ehr_status/${path} FROM EHR e WHERE e/ehr_status/${where}
    [Template]      Execute Query
    ${path}     ${where}     ${expected_file}    ${nr_of_results}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Create EHR For AQL With Custom EHR Status       file_name=status1.json

Execute Query
    [Arguments]     ${path}     ${where}    ${expected_file}    ${nr_of_results}
    ${expected_result_file}      Set Variable    ${EXPECTED_JSON_RESULTS}/${expected_file}
    ${query_dict}   Create Dictionary
    ...     tmp_query=SELECT e/ehr_status/${path} FROM EHR e WHERE e/ehr_status/${where}
    ${query}    Set Variable    ${query_dict["tmp_query"]}
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    ${exclude_paths}	Create List    root['meta']     root['q']
    Set Test Variable   ${path}     ${path}
    Set Test Variable   ${where}    ${where}
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!