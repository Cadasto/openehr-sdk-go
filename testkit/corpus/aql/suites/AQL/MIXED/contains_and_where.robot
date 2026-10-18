*** Settings ***
Documentation   MIXED - Contains and where cases
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/MIXED_TEST_SUIT.md#contains-and-where
Resource        ../../_resources/keywords/aql_keywords.robot

Suite Setup     Precondition
Suite Teardown      Admin Delete EHR For AQL    ${ehr_id}


*** Test Cases ***
1. SELECT s1/feeder_audit/originating_system_item_ids/id, s2/feeder_audit/originating_system_item_ids/id, o/feeder_audit/originating_system_item_ids/id, v/time/value, c1/items[at0001]/value/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS SECTION s1 CONTAINS SECTION s2 CONTAINS OBSERVATION o CONTAINS EVENT v CONTAINS CLUSTER c1 CONTAINS CLUSTER c2 CONTAINS ELEMENT l
    ${query}    Set Variable    SELECT s1/feeder_audit/originating_system_item_ids/id, s2/feeder_audit/originating_system_item_ids/id, o/feeder_audit/originating_system_item_ids/id, v/time/value, c1/items[at0001]/value/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS SECTION s1 CONTAINS SECTION s2 CONTAINS OBSERVATION o CONTAINS EVENT v CONTAINS CLUSTER c1 CONTAINS CLUSTER c2 CONTAINS ELEMENT l
    Set AQL And Execute Ad Hoc Query        ${query}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/mixed/mixed_contains_and_where_q1.json
    Length Should Be    ${resp_body['rows']}     16
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!

2. SELECT s1/feeder_audit/originating_system_item_ids/id, s2/feeder_audit/originating_system_item_ids/id, o/feeder_audit/originating_system_item_ids/id, v/time/value, c1/items[at0001]/value/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS SECTION s1 CONTAINS SECTION s2 CONTAINS OBSERVATION o CONTAINS EVENT v CONTAINS CLUSTER c1 CONTAINS CLUSTER c2 CONTAINS ELEMENT l WHERE s1/feeder_audit/originating_system_item_ids/id = 'ad_hoc_heading 2'
    ${query}    Set Variable    SELECT s1/feeder_audit/originating_system_item_ids/id, s2/feeder_audit/originating_system_item_ids/id, o/feeder_audit/originating_system_item_ids/id, v/time/value, c1/items[at0001]/value/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS SECTION s1 CONTAINS SECTION s2 CONTAINS OBSERVATION o CONTAINS EVENT v CONTAINS CLUSTER c1 CONTAINS CLUSTER c2 CONTAINS ELEMENT l WHERE s1/feeder_audit/originating_system_item_ids/id = 'ad_hoc_heading 2'
    Set AQL And Execute Ad Hoc Query        ${query}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/mixed/mixed_contains_and_where_q2.json
    Length Should Be    ${resp_body['rows']}     8
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!

3. SELECT s1/feeder_audit/originating_system_item_ids/id, s2/feeder_audit/originating_system_item_ids/id, o/feeder_audit/originating_system_item_ids/id, v/time/value, c1/items[at0001]/value/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS SECTION s1 CONTAINS SECTION s2 CONTAINS OBSERVATION o CONTAINS EVENT v CONTAINS CLUSTER c1 CONTAINS CLUSTER c2 CONTAINS ELEMENT l WHERE (s1/feeder_audit/originating_system_item_ids/id = 'ad_hoc_heading 2' AND s2/feeder_audit/originating_system_item_ids/id = 'conformance_section 4' AND v/time/value = '2024-02-03T04:05:06')
    ${query}    Set Variable    SELECT s1/feeder_audit/originating_system_item_ids/id, s2/feeder_audit/originating_system_item_ids/id, o/feeder_audit/originating_system_item_ids/id, v/time/value, c1/items[at0001]/value/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS SECTION s1 CONTAINS SECTION s2 CONTAINS OBSERVATION o CONTAINS EVENT v CONTAINS CLUSTER c1 CONTAINS CLUSTER c2 CONTAINS ELEMENT l WHERE (s1/feeder_audit/originating_system_item_ids/id = 'ad_hoc_heading 2' AND s2/feeder_audit/originating_system_item_ids/id = 'conformance_section 4' AND v/time/value = '2024-02-03T04:05:06')
    Set AQL And Execute Ad Hoc Query        ${query}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/mixed/mixed_contains_and_where_q3.json
    Length Should Be    ${resp_body['rows']}     2
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!

4. SELECT s1/feeder_audit/originating_system_item_ids/id, s2/feeder_audit/originating_system_item_ids/id, o/feeder_audit/originating_system_item_ids/id, v/time/value, c1/items[at0001]/value/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS SECTION s1 CONTAINS SECTION s2 CONTAINS OBSERVATION o CONTAINS EVENT v CONTAINS CLUSTER c1 CONTAINS CLUSTER c2 CONTAINS ELEMENT l WHERE (s1/feeder_audit/originating_system_item_ids/id = 'ad_hoc_heading 2' AND s2/feeder_audit/originating_system_item_ids/id = 'conformance_section 4' AND v/time/value = '2024-02-03T04:05:06' AND l/value/value = 'cluster outer text16')
    ${query}    Set Variable    SELECT s1/feeder_audit/originating_system_item_ids/id, s2/feeder_audit/originating_system_item_ids/id, o/feeder_audit/originating_system_item_ids/id, v/time/value, c1/items[at0001]/value/value, l/value/value FROM EHR e CONTAINS COMPOSITION c CONTAINS SECTION s1 CONTAINS SECTION s2 CONTAINS OBSERVATION o CONTAINS EVENT v CONTAINS CLUSTER c1 CONTAINS CLUSTER c2 CONTAINS ELEMENT l WHERE (s1/feeder_audit/originating_system_item_ids/id = 'ad_hoc_heading 2' AND s2/feeder_audit/originating_system_item_ids/id = 'conformance_section 4' AND v/time/value = '2024-02-03T04:05:06' AND l/value/value = 'cluster outer text16')
    Set AQL And Execute Ad Hoc Query        ${query}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/mixed/mixed_contains_and_where_q4.json
    Length Should Be    ${resp_body['rows']}     1
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      type_repetition_conformance_ehrbase.org.opt
    Create EHR For AQL
    Commit Composition For AQL      type_repetition_conformance_ehrbase.org_where1.json
    Set Suite Variable      ${c_uid1}      ${composition_short_uid}