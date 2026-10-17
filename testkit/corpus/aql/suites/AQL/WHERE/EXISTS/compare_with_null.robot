*** Settings ***
Documentation   WHERE - Exists - Compare with Null
...             - Covers the following:
...             - https://github.com/ehrbase/conformance-testing-documentation/blob/main/WHERE_TEST_SUIT.md#compare-with-null

Resource        ../../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/where/combinations/compare_with_null.csv
...     dialect=excel

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Admin Delete EHR For AQL    ${ehr_id}


*** Test Cases ***
SELECT ${path}${spath} FROM OBSERVATION o [openEHR-EHR-OBSERVATION.conformance_observation.v0] WHERE ${condition} ${path}
    #[Tags]      not-ready
    [Template]      Execute Query
    ${path}     ${spath}    ${condition}    ${expected_file}    ${nr_of_results}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      conformance_ehrbase.de.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      conformance_ehrbase.de.v0_max_v2.json
    Set Suite Variable       ${c_uid1}      ${composition_short_uid}

Execute Query
    [Arguments]     ${path}     ${spath}     ${condition}     ${expected_file}   ${nr_of_results}
    #Log     Add test data once 200 is returned. File: ${expected_file}    console=yes
    ${query_dict}   Create Dictionary
    ...     tmp_query=SELECT ${path}${spath} FROM OBSERVATION o [openEHR-EHR-OBSERVATION.conformance_observation.v0] WHERE ${condition} ${path}
    Log     ${query_dict["tmp_query"]}
    ${query}    Set Variable    ${query_dict["tmp_query"]}
    Log     ${query}
    Set AQL And Execute Ad Hoc Query    ${query}
    ${expected_res_tmp}      Set Variable       ${EXPECTED_JSON_DATA_SETS}/where/${expected_file}
    ${file_without_replaced_vars}   Get Binary File    ${expected_res_tmp}
    ${data_replaced_vars}    Replace Variables  ${file_without_replaced_vars}
    Log     Expected data: ${data_replaced_vars}
    Create Binary File     ${EXPECTED_JSON_DATA_SETS}/where/compare_with_null_tmp.json
    ...     ${data_replaced_vars}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${EXPECTED_JSON_DATA_SETS}/where/compare_with_null_tmp.json
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Remove File     ${EXPECTED_JSON_DATA_SETS}/where/compare_with_null_tmp.json