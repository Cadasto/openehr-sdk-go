*** Settings ***
Documentation   CHECK Identified Path: Path from Entry to Data value in Cluster
...             - Covers the following:
...             - https://github.com/ehrbase/conformance-testing-documentation/blob/main/SELECT_TEST_SUIT.md#path-from-entry-to-data-value-in-cluster

Resource        ../../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/select/combinations/path_from_entry_to_data_value_in_cluster.csv
...     dialect=excel

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Admin Delete EHR For AQL    ${ehr_id}


*** Test Cases ***
SELECT o/data[at0001]/events[at0002]/data[at0003]/items[openEHR-EHR-CLUSTER.conformance_cluster.v0]/${path} FROM OBSERVATION o[openEHR-EHR-OBSERVATION.conformance_observation.v0]
    #[Tags]      not-ready
    [Template]      Execute Query
    ${path}    ${expected_file}    ${nr_of_results}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      conformance_ehrbase.de.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      conformance_ehrbase.de.v0_max_v3.json
    Set Suite Variable      ${c_uid1}      ${composition_short_uid}

Execute Query
    [Arguments]     ${path}     ${expected_file}    ${nr_of_results}
    ${query_dict}   Create Dictionary
    ...     tmp_query=SELECT o/data[at0001]/events[at0002]/data[at0003]/items[openEHR-EHR-CLUSTER.conformance_cluster.v0]/${path} FROM OBSERVATION o[openEHR-EHR-OBSERVATION.conformance_observation.v0]
    Log     ${query_dict["tmp_query"]}
    ${query}    Set Variable    ${query_dict["tmp_query"]}
    Log     ${query}
    Set AQL And Execute Ad Hoc Query    ${query}
    ${expected_result_file}      Set Variable       ${EXPECTED_JSON_DATA_SETS}/select/${expected_file}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    ${exclude_paths}	Create List    root['meta']     root['q']
    Set Test Variable   ${path}     ${path}
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!