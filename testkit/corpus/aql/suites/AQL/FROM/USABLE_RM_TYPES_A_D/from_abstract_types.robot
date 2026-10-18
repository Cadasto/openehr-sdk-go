*** Settings ***
Documentation   CHECK AQL RESPONSE ON FROM ABSTRACT TYPES
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FROM_TEST_SUIT.MD#test-from-abstract-types
...         - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition
...         - Send AQL 'SELECT t FROM {type} t'
...         - {type} can be:
...         ENTRY, CARE_ENTRY,ITEM_STRUCTURE, DATA_STRUCTURE
...         - Check if actual response == expected response
...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
Resource        ../../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...         file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/from/combinations/from_abstract_types.csv
...         dialect=excel

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Admin Delete EHR For AQL       #enable this keyword if AQL checks are passing !!!


*** Test Cases ***
Test From Abstract Types: SELECT t FROM ${type} t
    #[Tags]      not-ready
    [Template]      Execute Query
    ${type}     ${expected_file}    ${nr_of_results}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      conformance_ehrbase.de.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      conformance_ehrbase.de.v0_max.json

Execute Query
    [Arguments]     ${type}     ${expected_file}    ${nr_of_results}
    Set Test Variable   ${type}     ${type}
    ${query}    Set Variable    SELECT t FROM ${type} t
    Set AQL And Execute Ad Hoc Query    ${query}
    Log     ${expected_file}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/from/${expected_file}
    ${exclude_paths}    Create List    root['meta']     root['q']   root['rows'][0][0]['uid']
    ...     root['rows'][1][0]['uid']   root['rows'][2][0]['uid']   root['rows'][3][0]['uid']
    ...     root['rows'][4][0]['uid']   root['rows'][5][0]['uid']   root['rows'][6][0]['uid']
    ...     root['rows'][7][0]['uid']   root['rows'][8][0]['uid']   root['rows'][9][0]['uid']
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}      exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!