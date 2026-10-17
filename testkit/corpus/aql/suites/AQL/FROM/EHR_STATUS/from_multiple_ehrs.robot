*** Settings ***
Documentation   CHECK AQL RESPONSE ON FROM EHR_STATUS, test from multiple ehrs
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FROM_TEST_SUIT.MD#test-from-multiple-ehrs
...         - *Precondition:* 1. Create EHRs with Status from status1.json and status2.json; 2. Save {ehr_id1}, {ehr_id2};
...         - Send AQL 'SELECT s/subject/external_ref/id/value, s/other_details/items[at0001]/value/id FROM EHR e CONTAINS EHR_STATUS s'
...         - Check AQL result to be {ins1920,55175056}, {ins1921,55175057}
...         - *Postcondition:* Delete EHR using ADMIN endpoint.
Resource        ../../../_resources/keywords/aql_keywords.robot

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Run Keywords
...     Admin Delete EHR For AQL    ${ehr_id1}      AND
...     Admin Delete EHR For AQL    ${ehr_id2}


*** Test Cases ***
1. SELECT s/subject/external_ref/id/value, s/other_details/items[at0001]/value/id FROM EHR e CONTAINS EHR_STATUS s
    ${query}    Set Variable    SELECT s/subject/external_ref/id/value, s/other_details/items[at0001]/value/id FROM EHR e CONTAINS EHR_STATUS s
    Set AQL And Execute Ad Hoc Query        ${query}
    ${expected_result_file}         Set Variable    ${EXPECTED_JSON_DATA_SETS}/from/expected_ehr_status_from_multiple_ehrs.json
    ${expected_result_file_tmp}     Set Variable    ${EXPECTED_JSON_DATA_SETS}/from/expected_ehr_status_from_multiple_ehrs_tmp.json
    ${file_without_replaced_vars}   Get File    ${expected_result_file}
    ${data_replaced_vars}    Replace Variables  ${file_without_replaced_vars}
    #Log     Expected data: ${data_replaced_vars}
    Create File     ${expected_result_file_tmp}
    ...     ${data_replaced_vars}
    Length Should Be    ${resp_body['rows']}     2
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result_file_tmp}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Run Keyword And Return Status   Remove File     ${expected_result_file_tmp}

2. SELECT eq/subject/external_ref/id/value FROM EHR_STATUS eq CONTAINS EHR_STATUS eq
    [Documentation]     - Covers https://vitagroup-ag.atlassian.net/browse/CDR-2248
    ${query}    Set Variable    SELECT eq/subject/external_ref/id/value FROM EHR_STATUS eq CONTAINS EHR_STATUS eq
    Set AQL And Execute Ad Hoc Query        ${query}
    Should Be Empty     ${resp_body['rows']}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Create EHR For AQL With Custom EHR Status       file_name=status1.json
    Set Suite Variable      ${ehr_id1}      ${ehr_id}
    Set Suite Variable      ${subject_external_ref_value1}      ${subject_external_ref_value}
    Set Suite Variable      ${at1_value_id1}      55175056
    Create EHR For AQL With Custom EHR Status       file_name=status2.json
    Set Suite Variable      ${ehr_id2}      ${ehr_id}
    Set Suite Variable      ${subject_external_ref_value2}      ${subject_external_ref_value}
    Set Suite Variable      ${at1_value_id2}      55175057