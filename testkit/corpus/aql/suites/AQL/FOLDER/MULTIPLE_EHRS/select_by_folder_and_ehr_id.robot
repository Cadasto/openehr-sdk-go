*** Settings ***
Documentation   CHECK AQL RESPONSE ON MULTIPLE EHRS - SELECT BY FOLDER AND EHR ID
...         - *Precondition:* 1. Create EHR and store ehr_id1; 2. Create Directory with folder_simple_hierarchy.json;
...         - 3. Create EHR and store ehr_id2; 4. Create Directory with folder_complex_hierarchy2.json;
...         - Send AQL query and compare response body with expected file content.
...         - *Postcondition:* Delete EHR using ADMIN endpoint.
Resource        ../../../_resources/keywords/aql_keywords.robot

Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Run Keywords    Admin Delete EHR For AQL    ${ehr_id1}      AND
...                             Admin Delete EHR For AQL    ${ehr_id2}


*** Test Cases ***
SELECT e/ehr_id/value, f/name/value, f/uid/value FROM EHR e CONTAINS FOLDER f WHERE e/ehr_id/value = '${ehr_id1}' AND f/uid/value = 'd936409e-901f-4994-8d33-ed104d460151'
    [Documentation]
    ...     Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FOLDER.md#select-by-folder-and-ehr-id
    Log     Skipped as returns emtpy rows.     console=yes
    ${query}    Set Variable
    ...     SELECT e/ehr_id/value, f/name/value, f/uid/value FROM EHR e CONTAINS FOLDER f WHERE e/ehr_id/value = '${ehr_id1}' AND f/uid/value = 'd936409e-901f-4994-8d33-ed104d460151'
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     1
    ${temporary_file}     Set Variable  ${EXPECTED_JSON_DATA_SETS}/folder/expected_folder_select_by_folder_and_ehr_id_tmp.json
    ${expected_file}      Set Variable    expected_folder_select_by_folder_and_ehr_id.json
    ${expected_res_tmp}      Set Variable       ${EXPECTED_JSON_DATA_SETS}/folder/${expected_file}
    ${file_without_replaced_vars}   Get File    ${expected_res_tmp}
    ${data_replaced_vars}    Replace Variables  ${file_without_replaced_vars}
    Create File     ${temporary_file}
    ...     ${data_replaced_vars}
    ${exclude_paths}    Create List    root['meta']     root['q']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${temporary_file}      exclude_paths=${exclude_paths}
    ...		ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Remove File     ${temporary_file}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Create EHR For AQL
    Set Suite Variable      ${ehr_id1}      ${ehr_id}
    Create Directory For AQL    folder_simple_hierarchy.json
    Create EHR For AQL
    Set Suite Variable      ${ehr_id2}      ${ehr_id}
    Create Directory For AQL    folder_complex_hierarchy2.json