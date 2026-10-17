*** Settings ***
Documentation   MIXED - Predicated and where cases
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/MIXED_TEST_SUIT.md#predicated-and-where
Resource        ../../_resources/keywords/aql_keywords.robot

Suite Setup     Precondition
Suite Teardown  Run Keywords
...     Admin Delete EHR For AQL    ${ehr_id1}      AND
...     Admin Delete EHR For AQL    ${ehr_id2}


*** Test Cases ***
1. SELECT e/ehr_id/value, c/uid/value, o/name/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0, 'Husten'] CONTAINS ELEMENT l[at0005, 'Vorhanden?'] WHERE (c/archetype_details/template_id/value MATCHES {'Corona_Anamnese', 'Corona_Anamnese2'} AND e/ehr_id/value MATCHES {'${ehr_id1}', '${ehr_id2}'})
    ${query}    Set Variable    SELECT e/ehr_id/value, c/uid/value, o/name/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0, 'Husten'] CONTAINS ELEMENT l[at0005, 'Vorhanden?'] WHERE (c/archetype_details/template_id/value MATCHES {'Corona_Anamnese', 'Corona_Anamnese2'} AND e/ehr_id/value MATCHES {'${ehr_id1}', '${ehr_id2}'})
    Create Temp File With Expected JSON For AQL     file_name=mixed_predicated_and_where_q1
    Set AQL And Execute Ad Hoc Query        ${query}
    #Should Be Equal As Strings      ${resp_body_query}      ${query}
    Length Should Be    ${resp_body['rows']}     3
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${EXPECTED_JSON_DATA_SETS}/mixed/${expected_file_tmp}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Remove File     ${EXPECTED_JSON_DATA_SETS}/mixed/${expected_file_tmp}

2. SELECT e/ehr_id/value, c/uid/value, o/name/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0, 'Husten'] CONTAINS ELEMENT l[at0005] WHERE (c/archetype_details/template_id/value MATCHES {'Corona_Anamnese', 'Corona_Anamnese2'} AND e/ehr_id/value MATCHES {'${ehr_id1}', '${ehr_id2}'})
    ${query}    Set Variable    SELECT e/ehr_id/value, c/uid/value, o/name/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0, 'Husten'] CONTAINS ELEMENT l[at0005] WHERE (c/archetype_details/template_id/value MATCHES {'Corona_Anamnese', 'Corona_Anamnese2'} AND e/ehr_id/value MATCHES {'${ehr_id1}', '${ehr_id2}'})
    Create Temp File With Expected JSON For AQL     file_name=mixed_predicated_and_where_q2
    Set AQL And Execute Ad Hoc Query        ${query}
    #Should Be Equal As Strings      ${resp_body_query}      ${query}
    Length Should Be    ${resp_body['rows']}     3
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${EXPECTED_JSON_DATA_SETS}/mixed/${expected_file_tmp}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Remove File     ${EXPECTED_JSON_DATA_SETS}/mixed/${expected_file_tmp}

3. SELECT e/ehr_id/value, c/uid/value, o/name/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0, 'Husten'] CONTAINS ELEMENT l[name/value='Vorhanden?'] WHERE (c/archetype_details/template_id/value MATCHES {'Corona_Anamnese', 'Corona_Anamnese2'} AND e/ehr_id/value MATCHES {'${ehr_id1}', '${ehr_id2}'})
    ${query}    Set Variable    SELECT e/ehr_id/value, c/uid/value, o/name/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0, 'Husten'] CONTAINS ELEMENT l[name/value='Vorhanden?'] WHERE (c/archetype_details/template_id/value MATCHES {'Corona_Anamnese', 'Corona_Anamnese2'} AND e/ehr_id/value MATCHES {'${ehr_id1}', '${ehr_id2}'})
    Create Temp File With Expected JSON For AQL     file_name=mixed_predicated_and_where_q3
    Set AQL And Execute Ad Hoc Query        ${query}
    #Should Be Equal As Strings      ${resp_body_query}      ${query}
    Length Should Be    ${resp_body['rows']}     3
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${EXPECTED_JSON_DATA_SETS}/mixed/${expected_file_tmp}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Remove File     ${EXPECTED_JSON_DATA_SETS}/mixed/${expected_file_tmp}

4. SELECT e/ehr_id/value, c/uid/value, o/name/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0, 'Husten'] CONTAINS ELEMENT l[at0005, 'Vorhanden?'] WHERE (c/archetype_details/template_id/value MATCHES {'Corona_Anamnese'} AND e/ehr_id/value MATCHES {'${ehr_id1}'})
    ${query}    Set Variable    SELECT e/ehr_id/value, c/uid/value, o/name/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0, 'Husten'] CONTAINS ELEMENT l[at0005, 'Vorhanden?'] WHERE (c/archetype_details/template_id/value MATCHES {'Corona_Anamnese'} AND e/ehr_id/value MATCHES {'${ehr_id1}'})
    Create Temp File With Expected JSON For AQL     file_name=mixed_predicated_and_where_q4
    Set AQL And Execute Ad Hoc Query        ${query}
    #Should Be Equal As Strings      ${resp_body_query}      ${query}
    Length Should Be    ${resp_body['rows']}     1
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${EXPECTED_JSON_DATA_SETS}/mixed/${expected_file_tmp}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Remove File     ${EXPECTED_JSON_DATA_SETS}/mixed/${expected_file_tmp}

5. SELECT o/name/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0] CONTAINS ELEMENT l[at0005, 'Vorhanden?'] WHERE (c/archetype_details/template_id/value MATCHES {'Corona_Anamnese'} AND e/ehr_id/value MATCHES {'${ehr_id1}'})
    ${query}    Set Variable    SELECT o/name/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0] CONTAINS ELEMENT l[at0005, 'Vorhanden?'] WHERE (c/archetype_details/template_id/value MATCHES {'Corona_Anamnese'} AND e/ehr_id/value MATCHES {'${ehr_id1}'})
    Create Temp File With Expected JSON For AQL     file_name=mixed_predicated_and_where_q5
    Set AQL And Execute Ad Hoc Query        ${query}
    #Should Be Equal As Strings      ${resp_body_query}      ${query}
    Length Should Be    ${resp_body['rows']}     7
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${EXPECTED_JSON_DATA_SETS}/mixed/${expected_file_tmp}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Remove File     ${EXPECTED_JSON_DATA_SETS}/mixed/${expected_file_tmp}

6. SELECT n/value/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0] CONTAINS (ELEMENT l[at0005] AND ELEMENT n[at0004]) WHERE (c/archetype_details/template_id/value MATCHES {'Corona_Anamnese'} AND e/ehr_id/value MATCHES {'${ehr_id1}'})
    ${query}    Set Variable    SELECT n/value/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0] CONTAINS (ELEMENT l[at0005] AND ELEMENT n[at0004]) WHERE (c/archetype_details/template_id/value MATCHES {'Corona_Anamnese'} AND e/ehr_id/value MATCHES {'${ehr_id1}'})
    Create Temp File With Expected JSON For AQL     file_name=mixed_predicated_and_where_q6
    Set AQL And Execute Ad Hoc Query        ${query}
    #Should Be Equal As Strings      ${resp_body_query}      ${query}
    Length Should Be    ${resp_body['rows']}     7
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${EXPECTED_JSON_DATA_SETS}/mixed/${expected_file_tmp}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Remove File     ${EXPECTED_JSON_DATA_SETS}/mixed/${expected_file_tmp}

7. SELECT n/value/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0] CONTAINS (ELEMENT l[at0005] AND ELEMENT n[at0004]) WHERE (c/archetype_details/template_id/value MATCHES {'Corona_Anamnese'} AND l/value/value = 'Vorhanden' AND e/ehr_id/value MATCHES {'${ehr_id1}'})
    ${query}    Set Variable    SELECT n/value/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0] CONTAINS (ELEMENT l[at0005] AND ELEMENT n[at0004]) WHERE (c/archetype_details/template_id/value MATCHES {'Corona_Anamnese'} AND l/value/value = 'Vorhanden' AND e/ehr_id/value MATCHES {'${ehr_id1}'})
    Create Temp File With Expected JSON For AQL     file_name=mixed_predicated_and_where_q7
    Set AQL And Execute Ad Hoc Query        ${query}
    #Should Be Equal As Strings      ${resp_body_query}      ${query}
    Length Should Be    ${resp_body['rows']}     3
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${EXPECTED_JSON_DATA_SETS}/mixed/${expected_file_tmp}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Remove File     ${EXPECTED_JSON_DATA_SETS}/mixed/${expected_file_tmp}

8. SELECT n/value/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0] CONTAINS (ELEMENT l[at0005] and ELEMENT n[at0004]) WHERE (c/archetype_details/template_id/value MATCHES {'Corona_Anamnese'} AND l/value/value = 'Vorhanden' AND n/value/value = 'Husten' AND e/ehr_id/value MATCHES {'${ehr_id1}'})
    ${query}    Set Variable    SELECT n/value/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0] CONTAINS (ELEMENT l[at0005] and ELEMENT n[at0004]) WHERE (c/archetype_details/template_id/value MATCHES {'Corona_Anamnese'} AND l/value/value = 'Vorhanden' AND n/value/value = 'Husten' AND e/ehr_id/value MATCHES {'${ehr_id1}'})
    Create Temp File With Expected JSON For AQL     file_name=mixed_predicated_and_where_q8
    Set AQL And Execute Ad Hoc Query        ${query}
    #Should Be Equal As Strings      ${resp_body_query}      ${query}
    Length Should Be    ${resp_body['rows']}     1
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${EXPECTED_JSON_DATA_SETS}/mixed/${expected_file_tmp}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Remove File     ${EXPECTED_JSON_DATA_SETS}/mixed/${expected_file_tmp}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      corona_anamnese1.opt
    Upload OPT For AQL      corona_anamnese2.opt
    Create EHR For AQL
    Set Suite Variable       ${ehr_id1}     ${ehr_id}
    Commit Composition For AQL      corona_anamnese1.json
    Set Suite Variable       ${c_uid1}      ${composition_short_uid}
    Create EHR For AQL
    Set Suite Variable       ${ehr_id2}     ${ehr_id}
    Commit Composition For AQL      corona_anamnese2.json
    Set Suite Variable       ${c_uid2}      ${composition_short_uid}
    Commit Composition For AQL      corona_anamnese3.json
    Set Suite Variable       ${c_uid3}      ${composition_short_uid}

Create Temp File With Expected JSON For AQL
    [Documentation]     Takes 1 mandatory arg {file_name}, pointing to filename with expected result.
    ...                 Example = mixed_predicated_and_where_q1
    ...                 Already existing files are not used directly, as they might contain Robot vars to be replaced.
    [Arguments]     ${file_name}
    ${expected_file}    Set Variable    ${file_name}.json
    ${expected_file_tmp}    Set Variable    ${file_name}_tmp.json
    Set Test Variable       ${expected_file_tmp}    ${expected_file_tmp}
    ${expected_res_tmp}      Set Variable       ${EXPECTED_JSON_DATA_SETS}/mixed/${expected_file}
    ${file_without_replaced_vars}   Get File    ${expected_res_tmp}
    ${data_replaced_vars}    Replace Variables  ${file_without_replaced_vars}
    #Log     Expected data: ${data_replaced_vars}
    Create File     ${EXPECTED_JSON_DATA_SETS}/mixed/${expected_file_tmp}
    ...     ${data_replaced_vars}