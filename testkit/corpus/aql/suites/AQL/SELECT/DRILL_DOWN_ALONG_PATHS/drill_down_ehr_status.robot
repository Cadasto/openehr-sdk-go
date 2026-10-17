*** Settings ***
Documentation   Drill down along paths: Drill down EHR_STATUS
...             - Covers the following:
...             - https://github.com/ehrbase/conformance-testing-documentation/blob/main/SELECT_TEST_SUIT.md#drill-down-ehr_status

Resource        ../../../_resources/keywords/aql_keywords.robot
Resource        ../../../_resources/keywords/ehr_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/select/combinations/drill_down_ehr_status.csv
...     dialect=excel

Suite Setup     Precondition
Suite Teardown  Admin Delete EHR For AQL    ${ehr_id}


*** Test Cases ***
${query_nr} SELECT ${path} FROM EHR_STATUS es
    [Template]      Execute Query
    ${path}    ${expected_file}    ${nr_of_results}



*** Keywords ***
Precondition
    Set Library Search Order For Tests
    generate random ehr_id
    Create EHR For AQL      ehr_id=${ehr_id}
	Set Suite Variable 		${resp_json}		${response.json()}
    Set Suite Variable      ${ehr_status_uid}	${resp_json['uid']['value']}
    Set Suite Variable      ${ehr_status_subject_id}
    ...     ${resp_json['subject']['external_ref']['id']['value']}
    Set Suite Variable      ${ehr_status_subject_namespace}
    ...     ${resp_json['subject']['external_ref']['namespace']}

Execute Query
    [Arguments]     ${path}     ${expected_file}    ${nr_of_results}
    ${expected_result_file}     Set Variable
    ...     ${EXPECTED_JSON_DATA_SETS}/select/${expected_file}
    ${query_dict}   Create Dictionary
    ...     tmp_query=SELECT ${path} FROM EHR_STATUS es
    ${query}    Set Variable    ${query_dict["tmp_query"]}
    Log     ${query}
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    ${exclude_paths}	Create List    root['meta']     root['q']
    Set Test Variable   ${path}     ${path}
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!