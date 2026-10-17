*** Settings ***
Documentation   CHECK AGGREGATE FUNCTIONS IN AQL
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/AGGREGATE_FUNCTIONS.md#count-on-paths
...         - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition.
...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
Resource        ../../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/aggregate_functions/combinations/count_on_paths.csv
...     dialect=excel

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Admin Delete EHR For AQL    #enable this keyword if AQL checks are passing !!!


*** Test Cases ***
SELECT COUNT(${path}) FROM OBSERVATION o[openEHR-EHR-OBSERVATION.conformance_observation.v0]
    #[Tags]      not-ready
    [Template]      Execute Query
    ${path}    ${nr_of_results}     ${expected_value}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      conformance_ehrbase.de.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      conformance_ehrbase.de.v0_max_v3.json

Execute Query
    [Arguments]     ${path}     ${nr_of_results}    ${expected_value}
    ${temp_query}    Set Variable       SELECT COUNT(${path}) FROM OBSERVATION o[openEHR-EHR-OBSERVATION.conformance_observation.v0]
    ${query}    Replace Variables       ${temp_query}
    Log     ${query}
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    Should Be Equal As Strings    ${resp_body['rows'][0][0]}       ${expected_value}