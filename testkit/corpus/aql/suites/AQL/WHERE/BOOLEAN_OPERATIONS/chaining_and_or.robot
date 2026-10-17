*** Settings ***
Documentation   WHERE - BOOLEAN OPERATIONS - CHAINING AND / OR
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/WHERE_TEST_SUIT.md#chaining--and--or
...         - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition; 4. Create Composition; 5. Create EHR; 6. Create Composition; 7. Create Composition
...         - Send AQL 'SELECT e/ehr_id/value, c/uid/value FROM EHR e CONTAINS COMPOSITION c WHERE {where}'
...         - {where} can be:
...         - (e/ehr_id/value = {ehr_id1} and c/uid/value = {c_uid1}) or c/uid/value = {c_uid3},
...         - e/ehr_id/value = {ehr_id1} and (c/uid/value = {c_uid1} or c/uid/value = {c_uid3}),
...         - e/ehr_id/value = {ehr_id1} and c/uid/value = {c_uid1} or c/uid/value = {c_uid3},
...         - (e/ehr_id/value = {ehr_id1} or e/ehr_id/value = {ehr_id2}) and (c/uid/value = {c_uid1} or c/uid/value = {c_uid3}),
...         - (e/ehr_id/value = {ehr_id1} and c/uid/value = {c_uid1}) or (e/ehr_id/value = {ehr_id2} and c/uid/value = {c_uid4})
...         Check if actual response == expected response
...         - *Postcondition:* Delete all EHRs using ADMIN endpoint. This is deleting compositions linked to EHRs.
Resource        ../../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/where/combinations/chaining_and_or.csv
...     dialect=excel

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Run Keywords
...     Admin Delete EHR For AQL    ${ehr_id1}      AND
...     Admin Delete EHR For AQL    ${ehr_id2}


*** Test Cases ***
SELECT e/ehr_id/value, c/uid/value FROM EHR e CONTAINS COMPOSITION c WHERE ${where}
    #[Tags]      not-ready
    [Template]      Execute Query
    ${where}    ${expected_file}    ${nr_of_results}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      conformance_ehrbase.de.v0.opt
    Create EHR For AQL
    Set Suite Variable       ${ehr_id1}    ${ehr_id}
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Set Suite Variable       ${c_uid1}      ${composition_short_uid}
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Set Suite Variable       ${c_uid2}      ${composition_short_uid}
    Create EHR For AQL
    Set Suite Variable       ${ehr_id2}    ${ehr_id}
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Set Suite Variable       ${c_uid3}      ${composition_short_uid}
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Set Suite Variable       ${c_uid4}      ${composition_short_uid}

Execute Query
    [Arguments]     ${where}    ${expected_file}    ${nr_of_results}
    ${temp_query}    Set Variable       SELECT e/ehr_id/value, c/uid/value FROM EHR e CONTAINS COMPOSITION c WHERE ${where}
    ${query}    Replace Variables       ${temp_query}
    Log     ${query}
    Set AQL And Execute Ad Hoc Query    ${query}
    ${expected_result_file}      Set Variable       ${EXPECTED_JSON_DATA_SETS}/where/${expected_file}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    ${exclude_paths}	Create List    root['meta']     root['q']
    Set Test Variable   ${where}    ${where}
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!