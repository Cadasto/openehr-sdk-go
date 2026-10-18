*** Settings ***
Documentation	CHECK AGGREGATE FUNCTIONS AVG NOT IMPLEMENTED ON DV_ORDERED IN AQL
...		- Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/AGGREGATE_FUNCTIONS.md#avg-on-dv-ordered

Resource	../../../_resources/keywords/aql_keywords.robot

Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/aggregate_functions/combinations/avg_on_dv_ordered.csv
...     dialect=excel

Suite Setup     Precondition
Suite Teardown  Admin Delete EHR For AQL


*** Test Cases ***
SELECT AVG(${path}) FROM OBSERVATION o [openEHR-EHR-OBSERVATION.conformance_observation.v0]
    [Template]      Execute Query
    ${path}     ${expected_file}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      	conformance_ehrbase.de.v0.opt
    Create EHR For AQL
    Commit Composition For AQL  conformance_ehrbase.de.v0_max_v3.json

Execute Query
    [Arguments]		${path}     ${expected_file}
   	${query}    Set Variable    SELECT AVG(${path}) FROM OBSERVATION o[openEHR-EHR-OBSERVATION.conformance_observation.v0]
    Set AQL And Execute Ad Hoc Query    ${query}

    ${expected_result}	Set Variable	${EXPECTED_JSON_DATA_SETS}/aggregate_functions/${expected_file}
    ${exclude_paths}    Create List		root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}
    ...		ignore_string_case=${TRUE}

    Should Be Empty    ${diff}    msg=DIFF DETECTED!
