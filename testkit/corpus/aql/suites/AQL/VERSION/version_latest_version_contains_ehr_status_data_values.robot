*** Settings ***
Documentation   VERSION[LATEST_VERSION] CONTAINS EHR_STATUS - DATA VALUES
...             - Covers the following:
...             - https://github.com/ehrbase/conformance-testing-documentation/blob/main/VERSION_TEST_SUIT.md#data-values

Resource        ../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/version/combinations/version_latest_version_contains_ehr_status_data_values.csv
...     dialect=excel

Suite Setup     Precondition
Suite Teardown  Run Keywords    Admin Delete EHR For AQL    ${ehr_id1}  AND
                ...             Admin Delete EHR For AQL    ${ehr_id3}


*** Test Cases ***
${query_nr} SELECT ${path} FROM VERSION cv[LATEST_VERSION] CONTAINS EHR_STATUS s ${where} ${order_by}
    [Template]      Execute Query
    ${path}     ${where}    ${order_by}     ${expected_file}    ${nr_of_results}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Create EHR For AQL
    Set Suite Variable      ${ehr_id1}  ${ehr_id}
    Set Suite Variable      ${ehr_status_id1}   ${resp_json['uid']['value']}
    Set Suite Variable      ${ehr_status_id1_subject_external_ref_id}
    ...     ${response.json()['subject']['external_ref']['id']['value']}
    Create EHR For AQL
    Set Suite Variable      ${ehr_id2}  ${ehr_id}
    Set Suite Variable      ${ehr_status_id2}   ${resp_json['uid']['value']}
    Admin Delete EHR For AQL    ${ehr_id2}
    Create EHR For AQL
    Set Suite Variable      ${ehr_id3}  ${ehr_id}
    Set Suite Variable      ${ehr_status_id3}   ${resp_json['uid']['value']}
    Update EHR Status For EHR
    ...     ehr_id=${ehr_id3}   ehrstatus_uid=${ehr_status_id3}     ehr_status_file=status3.json
    Set Suite Variable      ${ehr_status_id3_version2}  ${response.json()['uid']['value']}
    Set Suite Variable      ${ehr_status_id3_subject_external_ref_id}
    ...     ${response.json()['subject']['external_ref']['id']['value']}

Execute Query
    [Arguments]     ${path}     ${where}    ${order_by}     ${expected_file}    ${nr_of_results}
    ${expected_result_file}     Set Variable
    ...     ${EXPECTED_JSON_DATA_SETS}/version/${expected_file}
    ${query_dict}   Create Dictionary
    ...     tmp_query=SELECT ${path} FROM VERSION cv[LATEST_VERSION] CONTAINS EHR_STATUS s ${where} ${order_by}
    ${query}    Set Variable    ${query_dict["tmp_query"]}
    Log     ${query}
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    ${exclude_paths}	Create List    root['meta']     root['q']
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!