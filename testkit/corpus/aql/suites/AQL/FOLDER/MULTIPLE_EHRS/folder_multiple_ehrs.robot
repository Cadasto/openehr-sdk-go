*** Settings ***
Documentation   CHECK FOLDER MULTIPLE EHRS
...         - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FOLDER.md#multiple-ehrs
...         - *Precondition:*
...         - 1. Create EHR; 2. Create Directory with folder_complex_hierarchy.json;
...         - 3. Create Directory with folder_complex_hierarchy2.json;
...         - Send AQL query and compare response body with expected file content.
...         - *Postcondition:* Delete EHR using ADMIN endpoint.
Resource        ../../../_resources/keywords/aql_keywords.robot

Suite Setup     Precondition
Suite Teardown  Run Keywords    Admin Delete EHR For AQL    ${ehr_id1}  AND
...                             Admin Delete EHR For AQL    ${ehr_id2}


*** Test Cases ***
Multiple EHRs: SELECT e/ehr_id/value, f/uid/value FROM EHR e CONTAINS FOLDER f[openEHR-EHR-FOLDER.episode_of_care.v1,'subsubfolder1']
    ${query}    Set Variable
    ...     SELECT e/ehr_id/value, f/uid/value FROM EHR e CONTAINS FOLDER f[openEHR-EHR-FOLDER.episode_of_care.v1,'subsubfolder1']
    ${temporary_file}     Set Variable  ${EXPECTED_JSON_DATA_SETS}/folder/expected_folder_multiple_ehrs_tmp.json
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     4
    ${expected_file}      Set Variable  expected_folder_multiple_ehrs.json
    ${temporary_file}     Set Variable  ${EXPECTED_JSON_DATA_SETS}/folder/expected_folder_multiple_ehrs_tmp.json
    ${expected_res_tmp}      Set Variable       ${EXPECTED_JSON_DATA_SETS}/folder/${expected_file}
    ${file_without_replaced_vars}   Get File    ${expected_res_tmp}
    ${data_replaced_vars}    Replace Variables  ${file_without_replaced_vars}
    Create File     ${temporary_file}
    ...     ${data_replaced_vars}
    ${exclude_paths}    Create List    root['meta']     root['q']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${temporary_file}      exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}
    ...		ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Remove File     ${temporary_file}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Create EHR For AQL
    Set Suite Variable      ${ehr_id1}     ${ehr_id}
    Create Directory For AQL    folder_complex_hierarchy.json
    Create EHR For AQL
    Set Suite Variable      ${ehr_id2}     ${ehr_id}
    Create Directory For AQL    folder_complex_hierarchy2.json