*** Settings ***
Documentation   CHECK SELECT PATHS WITHIN SAME HIERARCHY LEVELS
...             - Covers the following:
...             - https://github.com/ehrbase/conformance-testing-documentation/blob/main/SELECT_TEST_SUIT.md#paths-within-same-hierarchy-level-i
...             - https://github.com/ehrbase/conformance-testing-documentation/blob/main/SELECT_TEST_SUIT.md#paths-within-same-hierarchy-level-ii
...             - https://github.com/ehrbase/conformance-testing-documentation/blob/main/SELECT_TEST_SUIT.md#paths-within-same-hierarchy-level-iii
...             - https://github.com/ehrbase/conformance-testing-documentation/blob/main/SELECT_TEST_SUIT.md#subobjects-within-locatable
...             - https://github.com/ehrbase/conformance-testing-documentation/blob/main/SELECT_TEST_SUIT.md#path-to-new-locatable
Resource        ../../../_resources/keywords/aql_keywords.robot
Suite Setup     Set Library Search Order For Tests


*** Test Cases ***
Lvl 1: SELECT p/time/value FROM POINT_EVENT p
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition
    ...         - Send AQL 'SELECT p/time/value FROM POINT_EVENT p'
    ...         - Check that result contains 4 rows.
    ...         - Expected rows items:
    ...         - 1. 2022-02-03T04:05:06
    ...         - 2. 2023-02-03T04:05:06
    ...         - 3. 2024-02-03T04:05:06
    ...         - 4. 2025-02-03T04:05:06
    ...         - Check if actual response == expected response
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    #[Tags]      not-ready
    [Setup]     Precondition1
    ${query}    Set Variable    SELECT p/time/value FROM POINT_EVENT p
    Set AQL And Execute Ad Hoc Query        ${query}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/select/paths_same_hierarchy_lvl1.json
    Length Should Be    ${resp_body['rows']}     5
    ${exclude_paths}	Create List    root['meta']     root['q']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}
    ...     exclude_paths=${exclude_paths}
    #Log To Console    \n\n${diff}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Admin Delete EHR For AQL

Lvl 2: SELECT i/narrative/value FROM INSTRUCTION i
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition
    ...         - Send AQL 'SELECT i/narrative/value FROM INSTRUCTION i'
    ...         - Check that result contains 1 row, with "Human readable instruction narrative"
    ...         - Check if actual response == expected response
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    #[Tags]      not-ready
    [Setup]     Precondition2
    ${query}    Set Variable    SELECT i/narrative/value FROM INSTRUCTION i
    Set AQL And Execute Ad Hoc Query        ${query}
    Length Should Be    ${resp_body['rows']}     1
    Should Be Equal As Strings      ${resp_body['rows'][0][0]}      Human readable instruction narrative
    [Teardown]      Admin Delete EHR For AQL

Lvl 3: SELECT c/start_time/value FROM EVENT_CONTEXT c
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition
    ...         - Send AQL 'SELECT c/start_time/value FROM EVENT_CONTEXT c'
    ...         - Check that result contains 1 row, "2021-12-21T14:19:31.649613+01:00"
    ...         - Check if actual response == expected response
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    #[Tags]      not-ready
    [Setup]     Precondition2
    ${query}    Set Variable    SELECT c/start_time/value FROM EVENT_CONTEXT c
    Set AQL And Execute Ad Hoc Query        ${query}
    Length Should Be    ${resp_body['rows']}     1
    Should Be Equal As Strings      ${resp_body['rows'][0][0]}      2021-12-21T14:19:31.649613+01:00
    [Teardown]      Admin Delete EHR For AQL

Subobjects Within Locatable: SELECT c/setting FROM EVENT_CONTEXT c
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition
    ...         - Send AQL 'SELECT c/setting FROM EVENT_CONTEXT c'
    ...         - Check that result contains 1 row, "{ "_type": "DV_CODED_TEXT", "value": "other care", "defining_code": { "_type": "CODE_PHRASE", "terminology_id": { "_type": "TERMINOLOGY_ID", "value": "openehr" }'"
    ...         - Check if actual response == expected response
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    #[Tags]      not-ready
    [Setup]     Precondition2
    ${query}    Set Variable    SELECT c/setting FROM EVENT_CONTEXT c
    Set AQL And Execute Ad Hoc Query        ${query}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/select/subobjects_within_locatable.json
    Log     Add test data once 200 is returned. File: ${expected_result}    console=yes
    Length Should Be    ${resp_body['rows']}     1
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}
    ...     exclude_paths=${exclude_paths}
    #Log To Console    \n\n${diff}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Admin Delete EHR For AQL

Path To New Locatable: SELECT c/content[openEHR-EHR-SECTION.conformance_section.v0]/items[openEHR-EHR-ACTION.conformance_action_.v0] FROM COMPOSITION c
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition
    ...         - Send AQL 'SELECT c/content[openEHR-EHR-SECTION.conformance_section.v0]/items[openEHR-EHR-ACTION.conformance_action_.v0] FROM COMPOSITION c'
    ...         - Check that result contains 1 row with type ACTION
    ...         - Check if actual response == expected response
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    #[Tags]      not-ready
    [Setup]     Precondition2
    ${query}    Set Variable
    ...     SELECT c/content[openEHR-EHR-SECTION.conformance_section.v0]/items[openEHR-EHR-ACTION.conformance_action_.v0] FROM COMPOSITION c
    Set AQL And Execute Ad Hoc Query        ${query}
    Length Should Be    ${resp_body['rows']}     1
    ##Add checks for row result, to contain type ACTION.
    [Teardown]      Admin Delete EHR For AQL


*** Keywords ***
Precondition1
    Upload OPT For AQL      aql-conformance-ehrbase.org.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      aql-conformance-ehrbase.org.v0_contains.json

Precondition2
    Upload OPT For AQL      conformance_ehrbase.de.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json