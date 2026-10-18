*** Settings ***
Documentation   CHECK AGGREGATE FUNCTIONS IN AQL
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/AGGREGATE_FUNCTIONS.md#max-1
...         - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition.
...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
Resource        ../../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/aggregate_functions/combinations/aggregate_function_and_where_max.csv
...     dialect=excel

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Admin Delete EHR For AQL    #enable this keyword if AQL checks are passing !!!


*** Test Cases ***
SELECT MAX(${path}) FROM OBSERVATION o[openEHR-EHR-OBSERVATION.conformance_observation.v0] WHERE ${path} != ${where}
    #[Tags]      not-ready
    [Template]      Execute Query
    ${path}     ${where}     ${expected_file}       ${nr_of_results}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      conformance_ehrbase.de.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      conformance_ehrbase.de.v0_max_v3.json

Execute Query
    [Arguments]     ${path}     ${where}    ${expected_file}    ${nr_of_results}
    ${query}    Set Variable    SELECT MAX(${path}) FROM OBSERVATION o[openEHR-EHR-OBSERVATION.conformance_observation.v0] WHERE ${path} != ${where}
    Set AQL And Execute Ad Hoc Query    ${query}
    Log     ${expected_file}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/aggregate_functions/${expected_file}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    ${exclude_paths}    Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!