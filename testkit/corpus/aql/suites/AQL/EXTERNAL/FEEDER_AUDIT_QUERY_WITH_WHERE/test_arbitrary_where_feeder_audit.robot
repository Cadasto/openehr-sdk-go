*** Settings ***
Documentation   Covers:
...             - https://github.com/ehrbase/openEHR_SDK/blob/develop/client/src/test/java/org/ehrbase/openehr/sdk/client/openehrclient/defaultrestclient/systematic/compositionquery/FeederAuditQueryWithWhereIT.java#L42
...             \n*Test testArbitraryWhereClause()*
...             - *Cases 1-7* are skipped due to message: "Not implemented: WHERE: path feeder_audit/feeder_system_audit/other_details[openEHR-EHR-ITEM_TREE.generic.v1]/items[at0001]/name/value contains STRUCTURE_INTERMEDIATE attribute feeder_system_audit"
...             - *Cases 8, 10, 11* should pass, but they are returning 2 'Test all types' instead of 1
Resource        ../../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/external/feeder_audit_query_with_where/combinations/test_arbitrary_where_feeder_audit.csv
...     dialect=excel

Suite Setup     Precondition
Suite Teardown  Admin Delete EHR For AQL


*** Variables ***
${EXPECTED_JSON_RESULTS}    ${EXPECTED_JSON_DATA_SETS}/external/feeder_audit_query_with_where


*** Test Cases ***
${query_nr} SELECT c/name/value FROM EHR e CONTAINS COMPOSITION c WHERE ${where}
    [Template]      Execute Query
    ${query_nr}     ${where}    ${expected_file}    ${nr_of_results}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      external/test_all_types.opt
    Set Suite Variable      ${template_id}      test_all_types.en.v1
    Create EHR For AQL With Custom EHR Status       file_name=status1.json
    Commit Composition For AQL      external/all_types_systematic_tests_feeder_audit.json
    Set Suite Variable      ${c_uid}        ${composition_short_uid}

Execute Query
    [Arguments]     ${query_nr}     ${where}    ${expected_file}    ${nr_of_results}
    ${expected_result_file}      Set Variable    ${EXPECTED_JSON_RESULTS}/${expected_file}
    ${query_dict}   Create Dictionary
    ...     tmp_query=SELECT c/name/value FROM EHR e CONTAINS COMPOSITION c WHERE ${where}
    ${query}    Set Variable    ${query_dict["tmp_query"]}
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    ${exclude_paths}	Create List    root['meta']     root['q']
    Set Test Variable   ${query_nr}     ${query_nr}
    Set Test Variable   ${where}        ${where}
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!