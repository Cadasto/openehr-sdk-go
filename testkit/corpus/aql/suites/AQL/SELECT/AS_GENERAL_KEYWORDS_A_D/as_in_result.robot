*** Settings ***
Documentation   CHECK SELECT AS IN RESULT
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/SELECT_TEST_SUIT.md#as-in-result
Resource        ../../../_resources/keywords/aql_keywords.robot
Suite Setup     Set Library Search Order For Tests


*** Test Cases ***
Test As In Result: SELECT c AS full FROM COMPOSITION c
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition
    ...         - Send AQL 'SELECT c AS full FROM COMPOSITION c'
    ...         - Check if actual response == expected response
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    [Setup]     Precondition
    ${query}    Set Variable    SELECT c AS full FROM COMPOSITION c
    Set AQL And Execute Ad Hoc Query        ${query}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/select/composition_complete_return.json
    ${exclude_paths}    Create List    root['meta']     root['q']   root['rows'][0][0]['uid']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}      exclude_paths=${exclude_paths}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Admin Delete EHR For AQL

Check Composition Is Returned For Full Composition Uid
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition
    ...         - Send AQL "SELECT c AS full FROM COMPOSITION c WHERE c/uid/value='{compo_uid}'"
    ...         - Check if actual response == expected response
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    ...         - https://vitagroup-ag.atlassian.net/browse/CDR-1291
    [Setup]     Precondition
    ${query}    Set Variable    SELECT c AS full FROM COMPOSITION c WHERE c/uid/value='${compo_id}::${system_id_with_tenant}::1'
    Set AQL And Execute Ad Hoc Query        ${query}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/select/expected_composition_full_uid.json
    ${exclude_paths}    Create List    root['meta']     root['q']   root['rows'][0][0]['uid']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}      exclude_paths=${exclude_paths}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Admin Delete EHR For AQL

Check Composition Is Returned For Partial Composition Uid - Without System Id
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition
    ...         - Send AQL "SELECT c AS partial_without_system_id FROM COMPOSITION c WHERE c/uid/value='{compo_uid}'"
    ...         - Check if actual response == expected response
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    ...         - https://vitagroup-ag.atlassian.net/browse/CDR-1299
    [Setup]     Precondition
    ${query}    Set Variable    SELECT c AS partial_without_system_id FROM COMPOSITION c WHERE c/uid/value='${compo_id}::::1'
    Set AQL And Execute Ad Hoc Query        ${query}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/select/expected_composition_partial_uid_without_system_id.json
    ${exclude_paths}    Create List    root['meta']     root['q']   root['rows'][0][0]['uid']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}      exclude_paths=${exclude_paths}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Admin Delete EHR For AQL

Check Composition Is Not Returned For Partial Composition Uid - Without Version Id
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition
    ...         - Send AQL "SELECT c AS partial_without_version_id FROM COMPOSITION c WHERE c/uid/value='{compo_uid}'"
    ...         - Check if error is returned with status code *400* and error message containing *is not a valid OBJECT_VERSION_ID/UID*
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    ...         - https://vitagroup-ag.atlassian.net/browse/CDR-1299
    [Setup]     Precondition
    ${query}    Set Variable    SELECT c AS partial_without_version_id FROM COMPOSITION c WHERE c/uid/value='${compo_id}::${system_id_with_tenant}::'
    ${err_msg}  Run Keyword And Expect Error    *
    ...     Set AQL And Execute Ad Hoc Query        ${query}
    Should Contain      ${err_msg}      400 != 200
    Should Contain      ${err_msg}      is not a valid OBJECT_VERSION_ID/UID
    [Teardown]      Admin Delete EHR For AQL

Check Composition Is Not Returned For Full Composition Uid - Non Existing Version Id
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition
    ...         - Send AQL "SELECT c AS full FROM COMPOSITION c WHERE c/uid/value='{compo_uid}'" version id not existing
    ...         - Check if actual response == expected response
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    ...         - https://vitagroup-ag.atlassian.net/browse/CDR-1291
    [Setup]     Precondition
    ${query}    Set Variable    SELECT c AS full FROM COMPOSITION c WHERE c/uid/value='${compo_id}::${system_id_with_tenant}::2'
    Set AQL And Execute Ad Hoc Query        ${query}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/select/expected_composition_full_uid_not_existing_version.json
    ${exclude_paths}    Create List    root['meta']     root['q']   root['rows'][0][0]['uid']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}      exclude_paths=${exclude_paths}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Admin Delete EHR For AQL

Check Composition Is Not Returned For Full Composition Uid - Non Existing System Id
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition
    ...         - Send AQL "SELECT c AS full FROM COMPOSITION c WHERE c/uid/value='{compo_uid}'" system id not existing
    ...         - Check if actual response == expected response
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    ...         - https://vitagroup-ag.atlassian.net/browse/CDR-1291
    [Setup]     Precondition
    ${query}    Set Variable    SELECT c AS full FROM COMPOSITION c WHERE c/uid/value='${compo_id}::non-existing-system::1'
    ${err_msg}  Run Keyword And Expect Error    *
    ...     Set AQL And Execute Ad Hoc Query        ${query}
    Should Contain      ${err_msg}      400 != 200
    Should Contain      ${err_msg}      does not match this server (${system_id_with_tenant})
    [Teardown]      Admin Delete EHR For AQL

Check Composition Is Returned For Full Composition Uid - Complex Query
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition
    ...         - Send AQL "SELECT e/ehr_id/value, c/uid/value, o/uid/value, p/time/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o CONTAINS POINT_EVENT p WHERE c/uid/value='{compo_uid}'"
    ...         - Check if actual response == expected response
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    ...         - https://vitagroup-ag.atlassian.net/browse/CDR-1291
    [Setup]     Precondition
    ${query}    Set Variable    SELECT e/ehr_id/value, c/uid/value, o/uid/value, p/time/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o CONTAINS POINT_EVENT p WHERE c/uid/value='${compo_id}::${system_id_with_tenant}::1'
    Set AQL And Execute Ad Hoc Query        ${query}
    ${expected_result_file}      Set Variable       ${EXPECTED_JSON_DATA_SETS}/select/expected_composition_full_uid_complex_query.json
    ${exclude_paths}    Create List    root['meta']     root['q']   root['rows'][0][0]['uid']
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    #Log     Expected data: ${expected_json}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}    exclude_paths=${exclude_paths}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Admin Delete EHR For AQL

Check Two Composition UIDs Are Returned - AQL With Lower Than Sign
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR;
    ...         3. Create Composition with fixed uid 1; 4. Create Composition with fixed uid 2
    ...         - Send AQL "SELECT c/uid/value AS full FROM COMPOSITION c WHERE c/uid/value<'d037bf7c-0ecb-40fb-aada-fc7d559815ea::::1'"
    ...         - Check if actual response == expected response (2 items in rows)
    ...         - Rows to have 2 items: ["c037bf7c-0ecb-40fb-aada-fc7d559815ea::local.ehrbase.org::1"],["b037bf7c-0ecb-40fb-aada-fc7d559815ea::local.ehrbase.org::1"]
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    ...         - https://vitagroup-ag.atlassian.net/browse/CDR-1299
    [Setup]     Precondition - Add 2 Compositions With Fixed UIDs
    ${query}    Set Variable    SELECT c/uid/value AS full FROM COMPOSITION c WHERE c/uid/value<'d037bf7c-0ecb-40fb-aada-fc7d559815ea::::1'
    Set AQL And Execute Ad Hoc Query        ${query}
    ${expected_result_file}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/select/expected_compositions_with_fixed_uid_lower_than.json
    ${exclude_paths}    Create List    root['meta']     root['q']   root['rows'][0][0]['uid']
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}    exclude_paths=${exclude_paths}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Admin Delete EHR For AQL

Check One Composition UID Is Returned - AQL With Greater Than Sign
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR;
    ...         3. Create Composition with fixed uid 1; 4. Create Composition with fixed uid 2
    ...         - Send AQL "SELECT c/uid/value AS full FROM COMPOSITION c WHERE c/uid/value>'b037bf7c-0ecb-40fb-aada-fc7d559815ea::${system_id_with_tenant}::1'"
    ...         - Check if actual response == expected response (1 item in rows)
    ...         - Rows to have 1 item: ["c037bf7c-0ecb-40fb-aada-fc7d559815ea::local.ehrbase.org::1"]
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    ...         - https://vitagroup-ag.atlassian.net/browse/CDR-1299
    [Setup]     Precondition - Add 2 Compositions With Fixed UIDs
    ${query}    Set Variable    SELECT c/uid/value AS full FROM COMPOSITION c WHERE c/uid/value>'b037bf7c-0ecb-40fb-aada-fc7d559815ea::${system_id_with_tenant}::1'
    Set AQL And Execute Ad Hoc Query        ${query}
    ${expected_result_file}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/select/expected_compositions_with_fixed_uid_greater_than.json
    ${exclude_paths}    Create List    root['meta']     root['q']   root['rows'][0][0]['uid']
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}    exclude_paths=${exclude_paths}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Admin Delete EHR For AQL

Check Two Composition UIDs Are Returned - AQL With Lower Equals Sign
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR;
    ...         3. Create Composition with fixed uid 1; 4. Create Composition with fixed uid 2
    ...         - Send AQL "SELECT c/uid/value AS full FROM COMPOSITION c WHERE c/uid/value<='c037bf7c-0ecb-40fb-aada-fc7d559815ea::${system_id_with_tenant}::1'"
    ...         - Check if actual response == expected response (2 items in rows)
    ...         - Rows to have 2 items: ["c037bf7c-0ecb-40fb-aada-fc7d559815ea::local.ehrbase.org::1"], ["b037bf7c-0ecb-40fb-aada-fc7d559815ea::local.ehrbase.org::1"]
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    ...         - https://vitagroup-ag.atlassian.net/browse/CDR-1299
    [Setup]     Precondition - Add 2 Compositions With Fixed UIDs
    ${query}    Set Variable    SELECT c/uid/value AS full FROM COMPOSITION c WHERE c/uid/value<='c037bf7c-0ecb-40fb-aada-fc7d559815ea::${system_id_with_tenant}::1'
    Set AQL And Execute Ad Hoc Query        ${query}
    ${expected_result_file}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/select/expected_compositions_with_fixed_uid_lower_equals_than.json
    ${exclude_paths}    Create List    root['meta']     root['q']   root['rows'][0][0]['uid']
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}      exclude_paths=${exclude_paths}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Admin Delete EHR For AQL

Check Two Composition UIDs Are Returned - AQL With Greater Equals Sign
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR;
    ...         3. Create Composition with fixed uid 1; 4. Create Composition with fixed uid 2
    ...         - Send AQL "SELECT c/uid/value AS full FROM COMPOSITION c WHERE c/uid/value>='b037bf7c-0ecb-40fb-aada-fc7d559815ea::::1'"
    ...         - Check if actual response == expected response (2 items in rows)
    ...         - Rows to have 2 items: ["c037bf7c-0ecb-40fb-aada-fc7d559815ea::local.ehrbase.org::1"], ["b037bf7c-0ecb-40fb-aada-fc7d559815ea::local.ehrbase.org::1"]
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    ...         - https://vitagroup-ag.atlassian.net/browse/CDR-1299
    [Setup]     Precondition - Add 2 Compositions With Fixed UIDs
    ${query}    Set Variable    SELECT c/uid/value AS full FROM COMPOSITION c WHERE c/uid/value>='b037bf7c-0ecb-40fb-aada-fc7d559815ea::::1'
    Set AQL And Execute Ad Hoc Query        ${query}
    ${expected_result_file}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/select/expected_compositions_with_fixed_uid_greater_equals_than.json
    ${exclude_paths}    Create List    root['meta']     root['q']   root['rows'][0][0]['uid']
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}      exclude_paths=${exclude_paths}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Admin Delete EHR For AQL

Check One Composition UID Is Returned - AQL With Different Than Sign
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR;
    ...         3. Create Composition with fixed uid 1; 4. Create Composition with fixed uid 2
    ...         - Send AQL "SELECT c/uid/value AS full FROM COMPOSITION c WHERE c/uid/value != 'c037bf7c-0ecb-40fb-aada-fc7d559815ea::::1'"
    ...         - Check if actual response == expected response (1 item in rows)
    ...         - Rows to have 1 item: ["b037bf7c-0ecb-40fb-aada-fc7d559815ea::local.ehrbase.org::1"]
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    ...         - https://vitagroup-ag.atlassian.net/browse/CDR-1299
    [Setup]     Precondition - Add 2 Compositions With Fixed UIDs
    ${query}    Set Variable    SELECT c/uid/value AS full FROM COMPOSITION c WHERE c/uid/value!='c037bf7c-0ecb-40fb-aada-fc7d559815ea::::1'
    Set AQL And Execute Ad Hoc Query        ${query}
    ${expected_result_file}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/select/expected_compositions_with_fixed_uid_different_than.json
    ${exclude_paths}    Create List    root['meta']     root['q']   root['rows'][0][0]['uid']
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}      exclude_paths=${exclude_paths}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Admin Delete EHR For AQL

SELECT Date From EHR - AQL Stays The Same In Result
    [Documentation]     Covers https://vitagroup-ag.atlassian.net/browse/CDR-2460
    [Setup]     Create EHR For AQL
    Select Date From EHR And Expect 20260102
    Select Date From EHR And Expect 02012026
    Select Date From EHR And Expect 202601
    Select Date From EHR And Expect 2026
    Select Date From EHR And Expect 02-01-2026
    Select Date From EHR And Expect 2026-01-02T14:30:00+02:00
    Select Date From EHR And Expect 2026-01-02T14:45:00-03:00
    Select Date From EHR And Expect 2026-01-02T14:50
    [Teardown]      Admin Delete EHR For AQL


*** Keywords ***
Precondition
    Upload OPT For AQL      aql-conformance-ehrbase.org.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      aql-conformance-ehrbase.org.v0_contains.json
    Set Test Variable       ${compo_id}     ${composition_short_uid}

Precondition - Add 2 Compositions With Fixed UIDs
    Upload OPT For AQL      aql-conformance-ehrbase.org.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      aql-conformance-ehrbase.org.v0_contains_with_compo_uid_1.json
    Log     ${composition_short_uid}
    Commit Composition For AQL      aql-conformance-ehrbase.org.v0_contains_with_compo_uid_2.json
    Log     ${composition_short_uid}

Select Date From EHR And Expect ${date_format}
    ${query}    Set Variable    SELECT '${date_format}' FROM EHR e LIMIT 1
    Set AQL And Execute Ad Hoc Query    ${query}
    Should Be Equal     ${resp_body_query}      ${query}
    Should Be Equal     ${resp_body['meta']['_executed_aql']}      ${query}
    Should Be Equal     ${resp_body['rows'][0][0]}     ${date_format}