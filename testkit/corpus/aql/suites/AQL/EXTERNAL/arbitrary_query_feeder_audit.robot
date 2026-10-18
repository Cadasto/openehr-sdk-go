*** Settings ***
Documentation   Covers:
...             - https://github.com/ehrbase/openEHR_SDK/blob/develop/client/src/test/java/org/ehrbase/openehr/sdk/client/openehrclient/defaultrestclient/systematic/compositionquery/ArbitraryQueryFeederAuditIT.java
...             \n*Test testArbitraryFeederAudit()*
Resource        ../../_resources/keywords/aql_keywords.robot

Suite Setup     Precondition
Suite Teardown  Admin Delete EHR For AQL


*** Variables ***
${EXPECTED_JSON_RESULTS}    ${EXPECTED_JSON_DATA_SETS}/external/arbitrary_query_feeder_audit
${query1}       SELECT c/feeder_audit FROM EHR e CONTAINS COMPOSITION c[openEHR-EHR-COMPOSITION.test_all_types.v1]

*** Test Cases ***
SELECT c/feeder_audit FROM EHR e CONTAINS COMPOSITION c[openEHR-EHR-COMPOSITION.test_all_types.v1]
    ${expected_file}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_arbitrary_query_feeder_audit.json
    ${query_dict}   Create Dictionary
    ...     tmp_query=${query1}
    ${query}    Set Variable    ${query_dict["tmp_query"]}
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     1
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_file}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      external/test_all_types.opt
    Set Suite Variable      ${template_id}      test_all_types.en.v1
    Create EHR For AQL With Custom EHR Status       file_name=status1.json
    Set Suite Variable      ${ehr_id1}      ${ehr_id}
    Commit Composition For AQL      external/all_types_systematic_tests_feeder_audit.json
    Set Suite Variable      ${c_uid1}       ${composition_short_uid}