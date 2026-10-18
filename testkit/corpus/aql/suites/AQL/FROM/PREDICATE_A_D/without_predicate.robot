*** Settings ***
Documentation   CHECK FROM WITHOUT PREDICATE
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FROM_TEST_SUIT.MD#without-predicate
Resource        ../../../_resources/keywords/aql_keywords.robot
Suite Setup     Set Library Search Order For Tests

Test Setup      Precondition
Test Teardown   Admin Delete EHR For AQL


*** Test Cases ***
1. Test From Without Predicate
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition
    ...         - Send AQL 'SELECT o FROM OBSERVATION o'
    ...         - Check query from response == query from script
    ...         - Check response to have *5 items in rows*
    ...         - Check that all items from rows are of _type=OBSERVATION
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    #[Tags]      not-ready
    ${query}    Set Variable    SELECT o FROM OBSERVATION o
    Set AQL And Execute Ad Hoc Query        ${query}
    Should Be Equal As Strings      ${resp_body_query}      ${query}
    ${rows_length}      Get Length      ${resp_body['rows']}
    Length Should Be    ${resp_body['rows']}     5
    FOR     ${INDEX}   IN RANGE     0   ${rows_length}
        #Log     ${resp_body['rows'][${INDEX}]}      #enable to see the rows item at specific index
        IF  '''${resp_body['rows'][${INDEX}][0]}''' == '''${NONE}'''
           Fail     None is present within rows results.
        ELSE
            Should Be Equal As Strings     ${resp_body['rows'][${INDEX}][0]["_type"]}       OBSERVATION
        END
    END

2. SELECT c0/uid/value FROM COMPOSITION c0[uid/value='${c_uid}'] CONTAINS COMPOSITION c1
    [Documentation]     - Covers https://vitagroup-ag.atlassian.net/browse/CDR-2248
    ${query}    Set Variable    SELECT c0/uid/value FROM COMPOSITION c0[uid/value='${c_uid}'] CONTAINS COMPOSITION c1
    Set AQL And Execute Ad Hoc Query        ${query}
    Should Be Empty     ${resp_body['rows']}


*** Keywords ***
Precondition
    Upload OPT For AQL      aql-conformance-ehrbase.org.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      aql-conformance-ehrbase.org.v0_contains.json
    Set Suite Variable       ${c_uid}       ${composition_short_uid}