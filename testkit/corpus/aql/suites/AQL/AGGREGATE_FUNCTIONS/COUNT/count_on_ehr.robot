*** Settings ***
Documentation   CHECK AGGREGATE FUNCTIONS IN AQL
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/AGGREGATE_FUNCTIONS.md#count-on-ehr
...         - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create 2 Compositions; 4. Create second EHR; 5. Create one more Composition.
...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
Resource        ../../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/aggregate_functions/combinations/count_on_ehr.csv
...     dialect=excel

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Run Keywords
...     Admin Delete EHR For AQL    ${ehr_id1}      AND
...     Admin Delete EHR For AQL    ${ehr_id2}


*** Test Cases ***
${statement}
    #[Tags]      not-ready
    [Template]      Execute Query
    ${statement}    ${nr_of_results}    ${expected_value}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      aql-conformance-ehrbase.org.v0.opt
    Create EHR For AQL
    Set Suite Variable      ${ehr_id1}     ${ehr_id}
    Commit Composition For AQL      aql-conformance-ehrbase.org.v0_contains.json
    Set Suite Variable       ${c_uid1}      ${composition_short_uid}
    Commit Composition For AQL      aql-conformance-ehrbase.org.v0_contains.json
    Set Suite Variable       ${c_uid2}      ${composition_short_uid}
    Create EHR For AQL
    Set Suite Variable      ${ehr_id2}     ${ehr_id}
    Commit Composition For AQL      aql-conformance-ehrbase.org.v0_contains.json
    Set Suite Variable       ${c_uid3}      ${composition_short_uid}

Execute Query
    [Arguments]     ${statement}    ${nr_of_results}    ${expected_value}
    Set AQL And Execute Ad Hoc Query    ${statement}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    Should Be Equal As Strings    ${resp_body['rows'][0][0]}       ${expected_value}