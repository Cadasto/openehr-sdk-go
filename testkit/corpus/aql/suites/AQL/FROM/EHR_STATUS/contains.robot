*** Settings ***
Documentation   CHECK AQL RESPONSE ON FROM EHR_STATUS, contains
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FROM_TEST_SUIT.MD#contains
...         - *Precondition:* 1. Upload OPT; 2. Create EHR with Status from status1.json; 3. Save {ehr_id};
...         4. Commit Composition.
...         - Send AQL 'SELECT l/name/value FROM EHR e CONTAINS {path}'
...         - {path} can be:
...         EHR_STATUS contains ELEMENT l, ELEMENT l [at0001], ELEMENT l [at0013],
...         ELEMENT l [at0013].
...         - Check if actual response == expected response
...         - *Postcondition:* Delete EHR using ADMIN endpoint.
Resource        ../../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/from/combinations/contains.csv
...     dialect=excel

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Admin Delete EHR For AQL       #enable this keyword if AQL checks are passing !!!


*** Test Cases ***
SELECT l/name/value FROM EHR e CONTAINS ${path}
    #[Tags]      not-ready
    [Template]      Execute Query
    ${path}     ${expected_file}    ${nr_of_results}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      ehrbase_blood_pressure_simple.de.v0.opt
    Create EHR For AQL With Custom EHR Status       file_name=status1.json
    Commit Composition For AQL      ehrbase_blood_pressure_simple.de.v0.json
    Set Suite Variable      ${c_uid1}      ${composition_short_uid}

Execute Query
    [Arguments]     ${path}     ${expected_file}    ${nr_of_results}
    ${query}    Set Variable    SELECT l/name/value FROM EHR e CONTAINS ${path}
    Set AQL And Execute Ad Hoc Query    ${query}
    Log     ${expected_file}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${EXPECTED_JSON_DATA_SETS}/from/${expected_file}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!