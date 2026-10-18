*** Settings ***
Documentation   Covers:
...             - https://github.com/ehrbase/openEHR_SDK/blob/develop/client/src/test/java/org/ehrbase/openehr/sdk/client/openehrclient/defaultrestclient/systematic/compositionquery/CanonicalMinimalInstructionTypeQueryIT.java#L83
...             \n*Test testArbitrary1()*
...             - *First case* should return *"PT30M"* -> should pass
...             - *Second case* should return *"PT30M"* -> BETWEEN not yet implemented
Resource        ../../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/external/canonical_minimal_instruction_type_query/combinations/test_arbitrary_minimal_instruction_1.csv
...     dialect=excel

Suite Setup     Precondition
Suite Teardown  Admin Delete EHR For AQL


*** Variables ***
${EXPECTED_JSON_RESULTS}    ${EXPECTED_JSON_DATA_SETS}/external/canonical_minimal_instruction_type_query


*** Test Cases ***
${query_nr} SELECT i/${path} FROM EHR e CONTAINS COMPOSITION c CONTAINS INSTRUCTION i[openEHR-EHR-INSTRUCTION.minimal.v1] WHERE ${where}
    [Tags]      not-ready
    [Template]      Execute Query
    ${query_nr}     ${path}     ${where}    ${expected_file}    ${nr_of_results}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      external/minimal_instruction.opt
    Set Suite Variable      ${template_id}      minimal_instruction.en.v1
    Create EHR For AQL With Custom EHR Status       file_name=status1.json
    Commit Composition For AQL      external/minimal_instruction.json
    Set Suite Variable      ${c_uid}        ${composition_short_uid}

Execute Query
    [Arguments]     ${query_nr}     ${path}     ${where}    ${expected_file}    ${nr_of_results}
    ${expected_result_file}      Set Variable    ${EXPECTED_JSON_RESULTS}/${expected_file}
    ${query_dict}   Create Dictionary
    ...     tmp_query=SELECT i/${path} FROM EHR e CONTAINS COMPOSITION c CONTAINS INSTRUCTION i[openEHR-EHR-INSTRUCTION.minimal.v1] WHERE ${where}
    ${query}    Set Variable    ${query_dict["tmp_query"]}
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    ${exclude_paths}	Create List    root['meta']     root['q']
    Set Test Variable   ${query_nr}     ${query_nr}
    Set Test Variable   ${path}         ${path}
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!