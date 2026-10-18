*** Settings ***
Documentation   CHECK SELECT DISTINCT
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/SELECT_TEST_SUIT.md#distinct--general-keywords-a-d
Resource        ../../../_resources/keywords/aql_keywords.robot
Suite Setup     Set Library Search Order For Tests


*** Test Cases ***
Test Distinct: SELECT e/ehr_id/value AS full FROM EHR e CONTAINS COMPOSITION C
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create 2x Compositions
    ...         - Send AQL 'SELECT e/ehr_id/value AS full FROM EHR e CONTAINS COMPOSITION C'
    ...         - Check that result contains 2 rows
    #[Tags]      not-ready
    [Setup]     Precondition
    ${query1}    Set Variable    SELECT e/ehr_id/value AS full FROM EHR e CONTAINS COMPOSITION C
    Set AQL And Execute Ad Hoc Query        ${query1}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/select/distinct_2_rows.json
    Length Should Be    ${resp_body['rows']}     2

Test Distinct: SELECT DISTINCT e/ehr_id/value AS full FROM EHR e CONTAINS COMPOSITION C
    [Documentation]     \n
    ...         - Send AQL 'SELECT DISTINCT e/ehr_id/value AS full FROM EHR e CONTAINS COMPOSITION C'
    ...         - Check that result contains 1 row
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    #[Tags]      not-ready
    ${query2}    Set Variable    SELECT DISTINCT e/ehr_id/value AS full FROM EHR e CONTAINS COMPOSITION C
    Set AQL And Execute Ad Hoc Query        ${query2}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/select/distinct_1_row.json
    Log     Add test data once 200 is returned. File: ${expected_result}    console=yes
    Length Should Be    ${resp_body['rows']}     1
    [Teardown]      Admin Delete EHR For AQL


*** Keywords ***
Precondition
    Upload OPT For AQL      aql-conformance-ehrbase.org.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      aql-conformance-ehrbase.org.v0_contains.json
    Set Test Variable       ${compo_uid_1}      ${composition_short_uid}
    Commit Composition For AQL      aql-conformance-ehrbase.org.v0_contains.json
    Set Test Variable       ${compo_uid_2}      ${composition_short_uid}