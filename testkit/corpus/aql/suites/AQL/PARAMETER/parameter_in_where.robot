*** Settings ***
Documentation   PARAMETER - PARAMETER in where
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/PARAMETER_TEST_SUIT.md#parameter-in-where

Resource        ../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/parameter/combinations/parameter_in_where.csv
...     dialect=excel

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Admin Delete EHR For AQL    ${ehr_id}


*** Test Cases ***
SELECT ${path} FROM COMPOSITION c CONTAINS OBSERVATION o WHERE ${path} = "param": ${value}
    #[Tags]      not-ready
    [Template]      Execute Query
    ${path}     ${value}    ${expected_file}    ${nr_of_results}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      conformance_ehrbase.de.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      conformance_ehrbase.de.v0_where.json
    Set Suite Variable      ${c_uid1}      ${composition_short_uid}

Execute Query
    [Arguments]     ${path}     ${value}    ${expected_file}    ${nr_of_results}
    ${query_dict}   Create Dictionary
    ...     tmp_query=SELECT ${path} FROM COMPOSITION c CONTAINS OBSERVATION o WHERE ${path} = \$param
    Log     ${query_dict["tmp_query"]}
    ${query}    Set Variable    ${query_dict["tmp_query"]}
    Log     ${query}
    ${parameter_obj}    Set Variable    {"param":${value}}
    Set AQL And Execute Ad Hoc Query    ${query}    parameter=${parameter_obj}
    ${expected_result_file}      Set Variable       ${EXPECTED_JSON_DATA_SETS}/parameter/${expected_file}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    ${exclude_paths}	Create List    root['meta']
    Set Test Variable   ${path}     ${path}
    Set Test Variable   ${value}    ${value}
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    #Log     Expected data: ${expected_json}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!