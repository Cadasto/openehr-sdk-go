*** Settings ***
Documentation   Drill down along paths: Drill down OBSERVATION
...             - Covers the following:
...             - https://github.com/ehrbase/conformance-testing-documentation/blob/main/SELECT_TEST_SUIT.md#drill-down-observation

Resource        ../../../_resources/keywords/aql_keywords.robot
Suite Setup     Set Library Search Order For Tests


*** Test Cases ***
SELECT o FROM OBSERVATION o[openEHR-EHR-OBSERVATION.conformance_observation.v0]
    [Setup]     Precondition
    ${query}    Set Variable    SELECT o FROM OBSERVATION o[openEHR-EHR-OBSERVATION.conformance_observation.v0]
    Set AQL And Execute Ad Hoc Query        ${query}
    Length Should Be    ${resp_body['rows']}     1
    Should Be Equal As Strings     ${resp_body['rows'][0][0]['_type']}      OBSERVATION
    [Teardown]      Admin Delete EHR For AQL


*** Keywords ***
Precondition
    Upload OPT For AQL      conformance_ehrbase.de.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Set Suite Variable      ${c_uid1}      ${composition_short_uid}