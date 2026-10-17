*** Settings ***
Documentation	CHECK AGGREGATE FUNCTIONS MAX ON DV_TEXT AND DV_CODED_TEXT ON DV_ORDERED IN AQL
...		- Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/AGGREGATE_FUNCTIONS.md

Resource	../../../_resources/keywords/aql_keywords.robot

Suite Setup     Precondition
Suite Teardown  Admin Delete EHR For AQL


*** Variables ***
${query_max_1}      SELECT MAX(o/data[at0001]/events[at0002]/data[at0003]/items[at0004]/value), o/data[at0001]/events[at0002]/data[at0003]/items[at0004]/value FROM OBSERVATION o[openEHR-EHR-OBSERVATION.conformance_observation.v0]
${query_max_2}      SELECT MAX(o/data[at0001]/events[at0002]/data[at0003]/items[at0005]/value), o/data[at0001]/events[at0002]/data[at0003]/items[at0005]/value FROM OBSERVATION o[openEHR-EHR-OBSERVATION.conformance_observation.v0]


*** Test Cases ***
${query_max_1}
    Set Test Variable   ${query}    ${query_max_1}
    Execute Query   expected_max_dv_text_dv_coded_text_at4.json

${query_max_2}
    Set Test Variable   ${query}    ${query_max_2}
    Execute Query   expected_max_dv_text_dv_coded_text_at5.json


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      	conformance_ehrbase.de.v0.opt
    Create EHR For AQL
    Commit Composition For AQL  conformance_ehrbase.de.v0_max_v3.json

Execute Query
    [Arguments]		${expected_file}
    Set AQL And Execute Ad Hoc Query    ${query}

    ${expected_result}	Set Variable	${EXPECTED_JSON_DATA_SETS}/aggregate_functions/${expected_file}
    ${exclude_paths}    Create List		root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}
    ...		ignore_string_case=${TRUE}

    Should Be Empty    ${diff}    msg=DIFF DETECTED!