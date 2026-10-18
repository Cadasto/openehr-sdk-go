*** Settings ***
Documentation   MIXED - Distinct
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/MIXED_TEST_SUIT.md#distinct
Resource        ../../_resources/keywords/aql_keywords.robot

Suite Setup     Precondition
Suite Teardown  Run Keywords
...     Admin Delete EHR For AQL    ${ehr_id1}      AND
...     Admin Delete EHR For AQL    ${ehr_id2}


*** Test Cases ***
1. SELECT DISTINCT o/data[at0001]/events[at0002]/data[at0003]/items[at0022]/items[at0004]/value/value, o/data[at0001]/events[at0002]/data[at0003]/items[at0022]/items[at0005]/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0] ORDER BY o/data[at0001]/events[at0002]/data[at0003]/items[at0022]/items[at0004]/value/value ASC, o/data[at0001]/events[at0002]/data[at0003]/items[at0022]/items[at0005]/value/value ASC
    ${query}    Set Variable    SELECT DISTINCT o/data[at0001]/events[at0002]/data[at0003]/items[at0022]/items[at0004]/value/value, o/data[at0001]/events[at0002]/data[at0003]/items[at0022]/items[at0005]/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0] ORDER BY o/data[at0001]/events[at0002]/data[at0003]/items[at0022]/items[at0004]/value/value ASC, o/data[at0001]/events[at0002]/data[at0003]/items[at0022]/items[at0005]/value/value ASC
    Set AQL And Execute Ad Hoc Query        ${query}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/mixed/mixed_distinct_q1.json
    Length Should Be    ${resp_body['rows']}     8
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${FALSE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!

2. SELECT DISTINCT o/data[at0001]/events[at0002]/data[at0003]/items[at0022]/items[at0004]/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0] WHERE o/data[at0001]/events[at0002]/data[at0003]/items[at0022]/items[at0005]/value/value = 'Vorhanden' ORDER BY o/data[at0001]/events[at0002]/data[at0003]/items[at0022]/items[at0004]/value/value
    ${query}    Set Variable    SELECT DISTINCT o/data[at0001]/events[at0002]/data[at0003]/items[at0022]/items[at0004]/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0] WHERE o/data[at0001]/events[at0002]/data[at0003]/items[at0022]/items[at0005]/value/value = 'Vorhanden' ORDER BY o/data[at0001]/events[at0002]/data[at0003]/items[at0022]/items[at0004]/value/value
    Set AQL And Execute Ad Hoc Query        ${query}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/mixed/mixed_distinct_q2.json
    Length Should Be    ${resp_body['rows']}     3
    ${exclude_paths}	Create List    root['meta']     root['q']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${FALSE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!

3. SELECT DISTINCT o/data[at0001]/events[at0002]/data[at0003]/items[at0022]/items[at0004]/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0] WHERE o/data[at0001]/events[at0002]/data[at0003]/items[at0022]/items[at0005]/value/value = 'Vorhanden' ORDER BY o/data[at0001]/events[at0002]/data[at0003]/items[at0022]/items[at0004]/value/value ASC LIMIT 1 OFFSET 1
    ${query}    Set Variable    SELECT DISTINCT o/data[at0001]/events[at0002]/data[at0003]/items[at0022]/items[at0004]/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0] WHERE o/data[at0001]/events[at0002]/data[at0003]/items[at0022]/items[at0005]/value/value = 'Vorhanden' ORDER BY o/data[at0001]/events[at0002]/data[at0003]/items[at0022]/items[at0004]/value/value ASC LIMIT 1 OFFSET 1
    Set AQL And Execute Ad Hoc Query        ${query}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/mixed/mixed_distinct_q3.json
    Length Should Be    ${resp_body['rows']}     1
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${FALSE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!


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