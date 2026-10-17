*** Settings ***
Documentation   WHERE - COMPARE WITH EXTRACTED COLUMN II
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/WHERE_TEST_SUIT.md#compare-with-extracted-column-ii--httpsvitagroup-agatlassiannetwikispacespenpages38216361architecture-aqlfeaturelistwhere
...         - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition
...         - Send AQL 'SELECT o FROM COMPOSITION CONTAINS OBSERVATION o WHERE {where}'
...         - {where} can be:
...         - o/archetype_node_id = 'openEHR-EHR-OBSERVATION.conformance_observation.v0',
...         - o/name/value = 'Blood pressure'
...         Check if actual response == expected response
...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
Resource        ../../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/where/combinations/compare_extracted_column_ii.csv
...     dialect=excel

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Admin Delete EHR For AQL


*** Test Cases ***
Test Compare With Extracted Column II: SELECT o FROM COMPOSITION CONTAINS OBSERVATION o WHERE ${where}
    #[Tags]      not-ready
    [Template]      Execute Query
    ${where}    ${expected_file}    ${nr_of_results}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      aql-conformance-ehrbase.org.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      aql-conformance-ehrbase.org.v0_contains.json

Execute Query
    [Arguments]     ${where}    ${expected_file}    ${nr_of_results}
    ${temp_query}    Set Variable       SELECT o FROM COMPOSITION CONTAINS OBSERVATION o WHERE ${where}
    ${query}    Replace Variables       ${temp_query}
    Log     ${query}
    Set AQL And Execute Ad Hoc Query    ${query}
    Log     ${expected_file}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/where/${expected_file}
    ${rows_length}      Get Length      ${resp_body['rows']}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    @{obs_list}     Create List
    IF      ${rows_length} > 1
        #Collect all o/ui/value results in {obs_list}
        FOR     ${INDEX}   IN RANGE     0   ${rows_length}
            Append To List      ${obs_list}     ${resp_body['rows'][${INDEX}][0]['uid']['value']}
        END
        List Should Contain Value   ${obs_list}     94c0e756-e892-4985-884b-46829605a236
        List Should Contain Value   ${obs_list}     55415141-17e4-4c71-9429-aa0fe6694c83
        List Should Contain Value   ${obs_list}     d4cccdfc-9c90-402f-b4bb-94e8dc4ea429
        List Should Contain Value   ${obs_list}     893506a7-462b-40b8-9638-0aa3990642d9
    ELSE
        Should Be Equal As Strings
        ...     ${resp_body['rows'][0][0]['uid']['value']}    2183807d-af68-41c5-9bfe-28cd150d62f7
    END
    #${diff}     compare json-string with json-file
    #...     ${resp_body_actual}     ${expected_result}
    #...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    #Should Be Empty    ${diff}    msg=DIFF DETECTED!