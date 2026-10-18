*** Settings ***
Documentation   WHERE - LIKE FEATURE LIST - LIKE WITH EXTRACTED COLUMN I
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/WHERE_TEST_SUIT.md#like-with-extracted-column-1

Resource        ../../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/where/combinations/like_with_extracted_column_i.csv
...     dialect=excel

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Run Keywords
...     Admin Delete EHR For AQL    ${ehr_id1}      AND
...     Admin Delete EHR For AQL    ${ehr_id2}


*** Test Cases ***
SELECT c/name/value, c/uid/value FROM EHR e CONTAINS COMPOSITION c WHERE c/name/value LIKE ${like}
    #[Tags]      not-ready
    [Template]      Execute Query
    ${like}    ${expected_file}    ${nr_of_results}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      conformance_ehrbase.de.v0.opt
    Upload OPT For AQL      type_repetition_conformance_ehrbase.org.opt
    Create EHR For AQL
    Set Suite Variable      ${ehr_id1}     ${ehr_id}
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Set Suite Variable      ${c_uid1}      ${composition_short_uid}
    Create EHR For AQL
    Set Suite Variable      ${ehr_id2}     ${ehr_id}
    Set Suite Variable      ${ehr_id}      ${ehr_id2}
    Commit Composition For AQL      type_repetition_conformance_ehrbase.org_one_reptation.json
    Set Suite Variable      ${c_uid2}      ${composition_short_uid}

Execute Query
    [Arguments]     ${like}    ${expected_file}    ${nr_of_results}
    ${temp_query}    Set Variable       SELECT c/name/value, c/uid/value FROM EHR e CONTAINS COMPOSITION c WHERE c/name/value LIKE ${like}
    ${query}    Replace Variables       ${temp_query}
    Log     ${query}
    Set AQL And Execute Ad Hoc Query    ${query}
    ${expected_result_file}      Set Variable       ${EXPECTED_JSON_DATA_SETS}/where/${expected_file}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    ${exclude_paths}	Create List    root['meta']
    Set Test Variable   ${like}    ${like}
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!