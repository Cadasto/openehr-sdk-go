*** Settings ***
Documentation   ORDER BY - SINGLE ORDER BY - ORDER BY ON EXTRACTED COLUMNS II
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/ORDER_BY_SUIT.md#order-by-on-extracted-columns-ii

Resource        ../../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/order_by/combinations/order_by_extracted_columns_ii.csv
...     dialect=excel

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Admin Delete EHR For AQL    ${ehr_id}


*** Test Cases ***
SELECT o/name/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o ORDER BY ${order}
    #[Tags]      not-ready
    [Template]      Execute Query
    ${order}    ${expected_file}    ${nr_of_results}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      aql-conformance-ehrbase.org.v0.opt
    Upload OPT For AQL      type_repetition_conformance_ehrbase.org.opt
    Create EHR For AQL
    Commit Composition For AQL      aql-conformance-ehrbase.org.v0_contains.json
    Set Suite Variable      ${c_uid1}      ${composition_short_uid}
    Commit Composition For AQL      type_repetition_conformance_ehrbase.org_one_reptation.json
    Set Suite Variable      ${c_uid2}      ${composition_short_uid}

Execute Query
    [Arguments]     ${order}    ${expected_file}    ${nr_of_results}
    ${temp_query}    Set Variable       SELECT o/name/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o ORDER BY ${order}
    ${query}    Replace Variables       ${temp_query}
    Log     ${query}
    Set AQL And Execute Ad Hoc Query    ${query}
    ${expected_result_file}      Set Variable       ${EXPECTED_JSON_DATA_SETS}/order_by/${expected_file}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    ${exclude_paths}	Create List    root['meta']
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    #Log     Expected data: ${expected_json}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${FALSE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!