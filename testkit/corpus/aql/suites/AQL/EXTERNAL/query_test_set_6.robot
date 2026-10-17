*** Settings ***
Documentation   Queries provided by Medblocks Team
...         - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Compositions
...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
Resource        ../../_resources/keywords/aql_keywords.robot

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Admin Delete EHR For AQL    #enable this keyword if AQL checks are passing !!!


*** Variables ***
${EXPECTED_JSON_RESULTS}    ${EXPECTED_JSON_DATA_SETS}/external/test_set_6


*** Test Cases ***
Medication Values: SELECT i/activities[at0001]/description[at0002]/items[at0070]/value/value as medication FROM EHR e CONTAINS COMPOSITION c CONTAINS INSTRUCTION i[openEHR-EHR-INSTRUCTION.medication_order.v3] WHERE (c/archetype_details/template_id/value MATCHES {'vitagroup-medication-list.v0'} AND e/ehr_id/value = '${ehr_id}')
    ${actual_file}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_medication_values.json
    ${tmp_file}         Set Variable    ${EXPECTED_JSON_RESULTS}/expected_medication_values_tmp.json
    ${query}    Set Variable    SELECT i/activities[at0001]/description[at0002]/items[at0070]/value/value as medication FROM EHR e CONTAINS COMPOSITION c CONTAINS INSTRUCTION i[openEHR-EHR-INSTRUCTION.medication_order.v3] WHERE (c/archetype_details/template_id/value MATCHES {'vitagroup-medication-list.v0'} AND e/ehr_id/value = '${ehr_id}')
    ${expected_result}      Set Variable    ${actual_file}
    ${file_without_replaced_vars}       Get File     ${expected_result}
    ${data_replaced_vars}   Replace Variables      ${file_without_replaced_vars}
    Create File     ${tmp_file}
    ...     ${data_replaced_vars}
    Execute Query   query=${query}      expected_rows_nr=2
    ...     expected_file=${tmp_file}
    [Teardown]      Remove File         ${tmp_file}

Medication And Dosage Values: SELECT i/activities[at0001]/description[at0002]/items[at0070]/value/value AS medication, i/activities[at0001]/description[at0002]/items[at0009]/value/value AS dosage FROM EHR e CONTAINS COMPOSITION c CONTAINS INSTRUCTION i[openEHR-EHR-INSTRUCTION.medication_order.v3] WHERE (c/archetype_details/template_id/value MATCHES {'vitagroup-medication-list.v0'} AND e/ehr_id/value = '${ehr_id}')
    ${actual_file}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_medication_and_dosage_values.json
    ${tmp_file}         Set Variable    ${EXPECTED_JSON_RESULTS}/expected_medication_and_dosage_values_tmp.json
    ${query}    Set Variable    SELECT i/activities[at0001]/description[at0002]/items[at0070]/value/value AS medication, i/activities[at0001]/description[at0002]/items[at0009]/value/value AS dosage FROM EHR e CONTAINS COMPOSITION c CONTAINS INSTRUCTION i[openEHR-EHR-INSTRUCTION.medication_order.v3] WHERE (c/archetype_details/template_id/value MATCHES {'vitagroup-medication-list.v0'} AND e/ehr_id/value = '${ehr_id}')
    ${expected_result}      Set Variable    ${actual_file}
    ${file_without_replaced_vars}       Get File     ${expected_result}
    ${data_replaced_vars}   Replace Variables      ${file_without_replaced_vars}
    Create File     ${tmp_file}
    ...     ${data_replaced_vars}
    Execute Query   query=${query}      expected_rows_nr=2
    ...     expected_file=${tmp_file}
    [Teardown]      Remove File         ${tmp_file}

Medication Without Instruction: SELECT c/content[openEHR-EHR-INSTRUCTION.medication_order.v3]/activities[at0001]/description[at0002]/items[at0070]/value/value AS medication FROM EHR e CONTAINS COMPOSITION c WHERE (c/archetype_details/template_id/value MATCHES {'vitagroup-medication-list.v0'} AND e/ehr_id/value = '${ehr_id}')
    ${actual_file}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_medication_without_instruction.json
    ${tmp_file}         Set Variable    ${EXPECTED_JSON_RESULTS}/expected_medication_without_instruction_tmp.json
    ${query}    Set Variable    SELECT c/content[openEHR-EHR-INSTRUCTION.medication_order.v3]/activities[at0001]/description[at0002]/items[at0070]/value/value AS medication FROM EHR e CONTAINS COMPOSITION c WHERE (c/archetype_details/template_id/value MATCHES {'vitagroup-medication-list.v0'} AND e/ehr_id/value = '${ehr_id}')
    ${expected_result}      Set Variable    ${actual_file}
    ${file_without_replaced_vars}       Get File     ${expected_result}
    ${data_replaced_vars}   Replace Variables      ${file_without_replaced_vars}
    Create File     ${tmp_file}
    ...     ${data_replaced_vars}
    Execute Query   query=${query}      expected_rows_nr=2
    ...     expected_file=${tmp_file}
    [Teardown]      Remove File         ${tmp_file}

*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      external/vitagroup-medication-list.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      external/vitagroup-medication-list.v0__composition_with_medication_item_flat.json     format=FLAT
    Set Suite Variable      ${c_uid1}      ${composition_short_uid}

Execute Query
    [Arguments]     ${query}    ${expected_rows_nr}    ${expected_file}
    ...     ${ignore_order}=${TRUE}    ${ignore_string_case}=${TRUE}
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     ${expected_rows_nr}
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_file}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${ignore_order}    ignore_string_case=${ignore_string_case}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!