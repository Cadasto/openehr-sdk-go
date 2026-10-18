*** Settings ***
Documentation   CHECK COMPLETE RETURN OF COMPOSITION C
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FROM_TEST_SUIT.MD#test-complete-return
Resource        ../../../_resources/keywords/aql_keywords.robot
Suite Setup     Set Library Search Order For Tests


*** Test Cases ***
Test Complete Return Of Composition
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition
    ...         - Send AQL 'SELECT c FROM COMPOSITION c'
    ...         - Check if actual response == expected response
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    #[Tags]      not-ready
    [Setup]     Precondition
    ${query}    Set Variable    SELECT c FROM COMPOSITION c
    Set AQL And Execute Ad Hoc Query        ${query}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/from/composition_complete_return.json
    ${exclude_paths}    Create List    root['rows'][0][0]['uid']    root['q']   root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}      exclude_paths=${exclude_paths}
    #Log To Console    \n\n${diff}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!

    ## Get count of compositions
    ${query_count}      Set Variable    SELECT COUNT(*) FROM COMPOSITION c
    Set AQL And Execute Ad Hoc Query        ${query_count}
    Log     ${resp_body_actual}
    Log     ${resp_body_actual['rows'][0][0]}
    [Teardown]      Admin Delete EHR For AQL


*** Keywords ***
Precondition
    Upload OPT For AQL      conformance_ehrbase.de.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
