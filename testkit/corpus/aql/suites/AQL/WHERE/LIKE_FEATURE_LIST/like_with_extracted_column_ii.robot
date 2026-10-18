*** Settings ***
Documentation   WHERE - LIKE FEATURE LIST - LIKE WITH EXTRACTED COLUMN II
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/WHERE_TEST_SUIT.md#like-with-extracted-column-ii

Resource        ../../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/where/combinations/like_with_extracted_column_ii.csv
...     dialect=excel

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Admin Delete EHR For AQL    ${ehr_id}


*** Test Cases ***
SELECT s/name/value FROM EHR e CONTAINS COMPOSITION c CONTAINS SECTION s WHERE s/name/value LIKE ${like}
    #[Tags]      not-ready
    [Template]      Execute Query
    ${like}    ${expected_file}    ${nr_of_results}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      type_repetition_conformance_ehrbase.org.opt
    Create EHR For AQL
    Commit Composition For AQL      type_repetition_conformance_ehrbase.org_like.json

Execute Query
    [Arguments]     ${like}    ${expected_file}    ${nr_of_results}
    ####
    ${query_dict}   Create Dictionary   tmp_query=SELECT s/name/value FROM EHR e CONTAINS COMPOSITION c CONTAINS SECTION s WHERE s/name/value LIKE ${like}
    Log     ${query_dict["tmp_query"]}
    ${query}    Set Variable    ${query_dict["tmp_query"]}
    ####
    ###${temp_query}    Set Variable       SELECT s/name/value FROM EHR e CONTAINS COMPOSITION c CONTAINS SECTION s WHERE s/name/value LIKE ${like}
    ###${query}    Replace Variables       ${temp_query}
    Log     ${query}
    Set AQL And Execute Ad Hoc Query    ${query_dict["tmp_query"]}
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