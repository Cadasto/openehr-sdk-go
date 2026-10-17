*** Settings ***
Documentation   CHECK FROM EHR AQL RESULT
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FROM_TEST_SUIT.MD#test-from-ehr
...             - Covers: https://github.com/ehrbase/openEHR_SDK/blob/develop/client/src/test/java/org/ehrbase/openehr/sdk/client/openehrclient/defaultrestclient/systematic/ehrquery/CanonicalEhrQuery1IT.java
Resource        ../../../_resources/keywords/aql_keywords.robot

Suite Setup     Run Keywords    Set Library Search Order For Tests      AND
                ...     Create EHR For AQL With Custom EHR Status       file_name=status1.json
Suite Teardown  Admin Delete EHR For AQL


*** Test Cases ***
Test From EHR: SELECT e/ehr_id/value FROM EHR e
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition
    ...         - Send AQL 'SELECT e/ehr_id/value FROM EHR e'
    ...         - Check query from response == query from script
    ...         - Check response rows to have the same ehr_id value as it was created in Precondition
    ...         - Check length of response rows to be 1
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    ${query}    Set Variable    SELECT e/ehr_id/value FROM EHR e
    Set AQL And Execute Ad Hoc Query        ${query}
    Should Be Equal As Strings      ${resp_body_query}      ${query}
    Should Be Equal As Strings      ${resp_body['rows'][0][0]}      ${ehr_id}
    Should Be Equal As Strings      ${resp_body_columns[0]['path']}       e/ehr_id/value
    Should Be Equal As Strings      ${resp_body_columns[0]['name']}       \#0
    Length Should Be    ${resp_body_columns}        1
    Length Should Be    ${resp_body['rows'][0]}     1

SELECT e/ehr_status/name FROM EHR e [ehr_id/value = '${ehr_id}']
    [Documentation]     openEHR_SDK test: testEhrAttributesDrillDown1()
    ${query}    Set Variable
    ...     SELECT e/ehr_status/name FROM EHR e [ehr_id/value = '${ehr_id}']
    ${expected_returned_query}      Set Variable
    ...     SELECT e/ehr_status/name FROM EHR e [ehr_id/value = '${ehr_id}']
    Set AQL And Execute Ad Hoc Query        ${query}
    Should Be Equal As Strings      ${resp_body_query}      ${expected_returned_query}
    ${response_rows}    Set Variable     ${resp_body_actual['rows'][0][0]}
    Should Be Equal As Strings      ${response_rows['_type']}   DV_TEXT
    Should Be Equal As Strings      ${response_rows['value']}   EHR Status

SELECT e/ehr_status/archetype_node_id FROM EHR e[ehr_id/value = '${ehr_id}']
    [Documentation]     openEHR_SDK test: testEhrAttributesDrillDown11()
    ${query}    Set Variable
    ...     SELECT e/ehr_status/archetype_node_id FROM EHR e[ehr_id/value = '${ehr_id}']
    ${expected_returned_query}      Set Variable
    ...     SELECT e/ehr_status/archetype_node_id FROM EHR e[ehr_id/value = '${ehr_id}']
    Set AQL And Execute Ad Hoc Query        ${query}
    Should Be Equal As Strings      ${resp_body_query}      ${expected_returned_query}
    Should Be Equal As Strings      ${resp_body_actual['rows'][0][0]}   openEHR-EHR-EHR_STATUS.generic.v1

SELECT e/ehr_status/name/value FROM EHR e[ehr_id/value = '${ehr_id}']
    [Documentation]     openEHR_SDK test: testEhrAttributesDrillDown2()
    ${query}    Set Variable
    ...     SELECT e/ehr_status/name/value FROM EHR e[ehr_id/value = '${ehr_id}']
    ${expected_returned_query}      Set Variable
    ...     SELECT e/ehr_status/name/value FROM EHR e[ehr_id/value = '${ehr_id}']
    Set AQL And Execute Ad Hoc Query        ${query}
    Should Be Equal As Strings      ${resp_body_query}      ${expected_returned_query}
    Should Be Equal As Strings      ${resp_body_actual['rows'][0][0]}   EHR Status