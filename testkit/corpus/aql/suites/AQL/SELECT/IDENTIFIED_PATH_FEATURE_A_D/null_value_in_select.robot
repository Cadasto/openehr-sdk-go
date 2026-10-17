*** Settings ***
Documentation   CHECK SELECT WITH NULL VALUE IN SELECT
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/SELECT_TEST_SUIT.md#null-value-in--select
Resource        ../../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/select/combinations/null_value_in_select.csv
...     dialect=excel

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Admin Delete EHR For AQL    ${ehr_id}


*** Test Cases ***
${statement}
    #[Tags]      not-ready
    [Template]      Execute Query
    ${statement}    ${expected_file}    ${nr_of_results}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      conformance_ehrbase.de.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      conformance_ehrbase.de.v0_max_v3.json
    Set Suite Variable      ${c_uid1}      ${composition_short_uid}

Execute Query
    [Arguments]     ${statement}     ${expected_file}    ${nr_of_results}
    Log     ${statement}
    ${query}    Set Variable    ${statement}
    Log     ${query}
    Set AQL And Execute Ad Hoc Query    ${query}
    ${expected_res}      Set Variable       ${EXPECTED_JSON_DATA_SETS}/select/${expected_file}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_res}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!