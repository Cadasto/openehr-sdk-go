*** Settings ***
Documentation   CHECK AQL RESPONSE ON FROM EHR_STATUS, test via part
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FROM_TEST_SUIT.MD#test-via-part
...         - *Precondition:* 1. Create EHR with Status from status1.json; 2. Save {ehr_id};
...         - Send AQL 'SELECT e/ehr_status/{path} FROM EHR e WHERE e/ehr_id/value = '{ehr_id}''
...         - {path} can be:
...         is_modifiable, subject, subject/external_ref/id/value,
...         other_details/items[at0001]/value/id.
...         - Check if actual response == expected response
...         - *Postcondition:* Delete EHR using ADMIN endpoint.
Resource        ../../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/from/combinations/via_part.csv
...     dialect=excel

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Admin Delete EHR For AQL       #enable this keyword if AQL checks are passing !!!


*** Test Cases ***
SELECT e/ehr_status/${path} FROM EHR e WHERE e/ehr_id/value = '${ehr_id}'
    #[Tags]      not-ready
    [Template]      Execute Query
    ${path}     ${expected_file}    ${nr_of_results}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Create EHR For AQL With Custom EHR Status       file_name=status1.json

Execute Query
    [Arguments]     ${path}     ${expected_file}    ${nr_of_results}
    ${query}    Set Variable    SELECT e/ehr_status/${path} FROM EHR e WHERE e/ehr_id/value = '${ehr_id}'
    ${expected_result_file}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/from/ehr_status_via_part_tmp.json
    Set AQL And Execute Ad Hoc Query    ${query}
    Log     ${expected_file}
    ${expected_res_tmp}      Set Variable       ${EXPECTED_JSON_DATA_SETS}/from/${expected_file}
    ${file_without_replaced_vars}   Get File    ${expected_res_tmp}
    ${data_replaced_vars}    Replace Variables  ${file_without_replaced_vars}
    #Log     Expected data: ${data_replaced_vars}
    Create File     ${expected_result_file}
    ...     ${data_replaced_vars}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    ${exclude_paths}	Create List    root['meta']     root['q']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result_file}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Run Keyword And Return Status   Remove File     ${expected_result_file}