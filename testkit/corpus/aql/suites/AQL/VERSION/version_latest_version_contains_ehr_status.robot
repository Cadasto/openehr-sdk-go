*** Settings ***
Documentation   VERSION[LATEST_VERSION] CONTAINS EHR_STATUS
...             - Covers the following:
...             - https://github.com/ehrbase/conformance-testing-documentation/blob/main/VERSION_TEST_SUIT.md#versionlatest_version-contains-ehr_status

Resource        ../../_resources/keywords/aql_keywords.robot
Resource        ../../_resources/keywords/contribution_keywords.robot

Suite Setup     Precondition
Suite Teardown  Run Keywords    Admin Delete EHR For AQL    ${ehr_id1}  AND
                ...             Admin Delete EHR For AQL    ${ehr_id3}


*** Variables ***
${query1}   SELECT cv/uid/value, cv/commit_audit/time_committed/value, cv/commit_audit/change_type/value, cv/commit_audit/change_type/defining_code/code_string, cv/commit_audit/change_type/defining_code/preferred_term, cv/commit_audit/change_type/defining_code/terminology_id/value, s/subject/external_ref/id/value FROM VERSION cv[LATEST_VERSION] CONTAINS EHR_STATUS s ORDER BY cv/commit_audit/time_committed ASC
${query2}   SELECT cv/uid/value, cv/contribution/id/value, cv/commit_audit/time_committed/value FROM VERSION cv[LATEST_VERSION] CONTAINS EHR_STATUS s ORDER BY cv/commit_audit/time_committed ASC


*** Test Cases ***
1. ${query1}
    Log     ${ehr_id1}, ${ehr_status_id1} \n ${ehr_id3}, ${ehr_status_id3}
    ${expected_result_file}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/version/latest_version_contains_ehr_status_sanity_check.json
    ${time_committed_ehr_status1}   Get EHR Status Time Committed   ${ehr_status_id1}
    ${time_committed_ehr_status2}   Get EHR Status Time Committed   ${ehr_status_id3_version2}
                        Set Suite Variable  ${time_committed_ehr_status1}   ${time_committed_ehr_status1}
                        Set Suite Variable  ${time_committed_ehr_status2}   ${time_committed_ehr_status2}
    Set AQL And Execute Ad Hoc Query    ${query1}
    Length Should Be    ${resp_body['rows']}     2
    ${exclude_paths}    Create List    root['meta']     root['q']   root['rows'][0][0]['uid']
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}     exclude_paths=${exclude_paths}
    Should Be Empty     ${diff}    msg=DIFF DETECTED!

2. ${query2}
    ${expected_result_file}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/version/latest_version_contains_ehr_status_contribution_check.json
    Set AQL And Execute Ad Hoc Query    ${query2}
    Set Suite Variable   ${contribution_id1}     ${resp_body['rows'][0][1]}
    Set Suite Variable   ${contribution_id2}     ${resp_body['rows'][1][1]}
    Length Should Be    ${resp_body['rows']}     2
    ${exclude_paths}    Create List    root['meta']     root['q']   root['rows'][0][0]['uid']
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}     exclude_paths=${exclude_paths}
    Should Be Empty     ${diff}    msg=DIFF DETECTED!

3. Check Contributions Exists And Valid - Latest Version EHR Status
    Set Test Variable   ${contribution_uid}    ${contribution_id1}
    Set Test Variable   ${ehr_id}    ${ehr_id1}
    GET /ehr/ehr_id/contribution/contribution_uid   format=JSON
    Should Be Equal     ${response.status_code}     ${200}
    Should Be Equal     ${response.json()['versions'][0]['id']['value']}    ${ehr_status_id1}
    Set Test Variable   ${contribution_uid}    ${contribution_id2}
    Set Test Variable   ${ehr_id}    ${ehr_id3}
    GET /ehr/ehr_id/contribution/contribution_uid   format=JSON
    Should Be Equal     ${response.status_code}     ${200}
    Should Be Equal     ${response.json()['versions'][0]['id']['value']}    ${ehr_status_id3_version2}


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
