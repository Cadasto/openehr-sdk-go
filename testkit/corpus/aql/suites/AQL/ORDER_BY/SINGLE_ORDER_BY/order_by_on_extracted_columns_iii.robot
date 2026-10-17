*** Settings ***
Documentation   ORDER BY - SINGLE ORDER BY - ORDER BY ON EXTRACTED COLUMNS III
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/ORDER_BY_SUIT.md#order-by-on-extracted-columns-iii

Resource        ../../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/order_by/combinations/order_by_extracted_columns_iii.csv
...     dialect=excel

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Run Keywords
...     Admin Delete EHR For AQL    ${ehr_id1}      AND
...     Admin Delete EHR For AQL    ${ehr_id2}


*** Test Cases ***
SELECT ${path} FROM EHR e CONTAINS COMPOSITION c ORDER BY ${path} ${order}
    #[Tags]      not-ready
    [Template]      Execute Query
    ${path}     ${order}    ${expected_file}    ${nr_of_results}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      aql-conformance-ehrbase.org.v0.opt
    Upload OPT For AQL      type_repetition_conformance_ehrbase.org.opt
    Create EHR For AQL      ehr_id=4a7ee122-7579-4bb1-9dbb-6703e7590a54
    Set Suite Variable      ${ehr_id1}     ${ehr_id}
    Commit Composition For AQL      aql-conformance-ehrbase.org.v0_contains_with_compo_uid.json
    Set Suite Variable      ${c_uid1}      ${composition_short_uid}
    Create EHR For AQL      ehr_id=fb86c5c2-e24d-4614-ba8d-259b9a09dd04
    Set Suite Variable      ${ehr_id2}     ${ehr_id}
    Set Suite Variable      ${ehr_id}      ${ehr_id2}
    Commit Composition For AQL      type_repetition_conformance_ehrbase.org_one_reptation_with_compo_uid.json
    Set Suite Variable      ${c_uid2}      ${composition_short_uid}

Execute Query
    [Arguments]     ${path}     ${order}    ${expected_file}    ${nr_of_results}
    ${temp_query}    Set Variable       SELECT ${path} FROM EHR e CONTAINS COMPOSITION c ORDER BY ${path} ${order}
    ${query}    Replace Variables       ${temp_query}
    Log     ${query}
    Set AQL And Execute Ad Hoc Query    ${query}
    ${expected_result_file}      Set Variable       ${EXPECTED_JSON_DATA_SETS}/order_by/${expected_file}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    ${exclude_paths}	Create List    root['meta']
    Set Test Variable   ${path}     ${path}
    Set Test Variable   ${order}    ${order}
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    #Log     Expected data: ${expected_json}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${FALSE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!