*** Settings ***
Documentation   CHECK SELECT PATH TO TARGET OBJECT
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/SELECT_TEST_SUIT.md#path-to-target-object
Resource        ../../../_resources/keywords/aql_keywords.robot
Suite Setup     Set Library Search Order For Tests


*** Test Cases ***
Test Path To Target Object: SELECT c FROM COMPOSITION c
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition
    ...         - Send AQL 'SELECT c FROM COMPOSITION c'
    ...         - Check that result contains 1 row
    ...         - Check if actual response == expected response
    #[Tags]      not-ready
    [Setup]     Precondition
    ${query1}    Set Variable    SELECT c FROM COMPOSITION c
    Set AQL And Execute Ad Hoc Query        ${query1}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/select/path_to_target_obj_query1.json
    ${exclude_paths}    Create List    root['meta']     root['q']   root['rows'][0][0]['uid']
    Length Should Be    ${resp_body['rows']}     1
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}      exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}
    #Log To Console    \n\n${diff}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!

Test Path To Target Object: SELECT c/uid FROM COMPOSITION c
    [Documentation]     \n
    ...         - Send AQL 'SELECT c/uid FROM COMPOSITION c'
    ...         - Check that result contains 1 row
    ...         - Check if actual response == expected response
    #[Tags]      not-ready
    ${query2}    Set Variable    SELECT c/uid FROM COMPOSITION c
    Set AQL And Execute Ad Hoc Query        ${query2}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/select/path_to_target_obj_query2.json
    Log     Add test data once 200 is returned. File: ${expected_result}    console=yes
    ${exclude_paths}    Create List    root['meta']     root['q']   root['rows'][0][0]['value']
    Length Should Be    ${resp_body['rows']}     1
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}      exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}
    #Log To Console    \n\n${diff}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!

Test Path To Target Object: SELECT c/uid/value FROM COMPOSITION c
    [Documentation]     \n
    ...         - Send AQL 'SELECT c/uid/value FROM COMPOSITION c'
    ...         - Check that result contains 1 row
    ...         - Check if actual response == expected response
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    #[Tags]      not-ready
    ${query3}    Set Variable    SELECT c/uid/value FROM COMPOSITION c
    Set AQL And Execute Ad Hoc Query        ${query3}
    Length Should Be    ${resp_body['rows']}     1
    Should Be Equal As Strings      ${resp_body_columns[0]['path']}       c/uid/value
    Should Be Equal As Strings      ${resp_body_columns[0]['name']}       \#0
    Should Be Equal As Strings      ${composition_uid}      ${resp_body['rows'][0][0]}
    [Teardown]      Admin Delete EHR For AQL


*** Keywords ***
Precondition
    Upload OPT For AQL      aql-conformance-ehrbase.org.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      aql-conformance-ehrbase.org.v0_contains.json