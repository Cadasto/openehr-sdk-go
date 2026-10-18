*** Settings ***
Documentation   CHECK Primitives: Primitives in simple select
...             - Covers the following:
...             - https://github.com/ehrbase/conformance-testing-documentation/blob/main/SELECT_TEST_SUIT.md#primitives-in-simple-select

Resource        ../../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/select/combinations/primitives_in_simple_select.csv
...     dialect=excel

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Admin Delete EHR For AQL    ${ehr_id}


*** Test Cases ***
SELECT ${primitive} FROM EHR
    #[Tags]      not-ready
    [Template]      Execute Query
    ${primitive}    ${expected_file}    ${nr_of_results}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Create EHR For AQL

Execute Query
    [Arguments]     ${primitive}     ${expected_file}    ${nr_of_results}
    ${query_dict}   Create Dictionary
    ...     tmp_query=SELECT ${primitive} FROM EHR
    Log     ${query_dict["tmp_query"]}
    ${query}    Set Variable    ${query_dict["tmp_query"]}
    Log     ${query}
    Set AQL And Execute Ad Hoc Query    ${query}
    ${expected_result_file}      Set Variable       ${EXPECTED_JSON_DATA_SETS}/select/${expected_file}
    Length Should Be    ${resp_body['rows']}    ${nr_of_results}
    ${exclude_paths}	Create List    root['meta']
    Set Test Variable   ${primitive}    ${primitive}
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!

