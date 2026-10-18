*** Settings ***
Documentation   CHECK LIMIT WITHOUT OFFSET WITHOUT ORDER BY
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/LIMIT_TEST_SUIT.md#without-order-by
...         - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create 4x Compositions
...         - Send AQL 'SELECT c FROM COMPOSITION c LIMIT \{limit}'
...         - *\{limit}* can be:
...         - 10
...         - 4
...         - 3
...         - 2
...         - 1
...         - Check if result count corresponds to each case, based on documentation.
...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
Resource        ../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/limit/combinations/without_offset_without_order_by.csv
...     dialect=excel

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Admin Delete EHR For AQL    ${ehr_id1}    #enable this keyword if AQL checks are passing !!!


*** Test Cases ***
SELECT c FROM COMPOSITION c LIMIT ${limit}
    #[Tags]      not-ready
    [Template]      Execute Query
    ${limit}    ${nr_of_results}


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
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Set Suite Variable       ${c_uid3}      ${composition_short_uid}
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json
    Set Suite Variable       ${c_uid4}      ${composition_short_uid}

Execute Query
    [Arguments]     ${limit}     ${nr_of_results}
    ${query}    Set Variable    SELECT c FROM COMPOSITION c LIMIT ${limit}
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}