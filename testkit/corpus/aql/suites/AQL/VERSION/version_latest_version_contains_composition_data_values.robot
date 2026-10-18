*** Settings ***
Documentation   VERSION[LATEST_VERSION] CONTAINS COMPOSITION - DATA VALUES
...             - Covers the following:
...             - https://github.com/ehrbase/conformance-testing-documentation/blob/main/VERSION_TEST_SUIT.md#data-values-1

Resource        ../../_resources/keywords/aql_keywords.robot
Library     DataDriver
...     file=${PROJECT_ROOT}/tests/robot/_resources/test_data_sets/aql/fields_and_results/version/combinations/version_latest_version_contains_composition_data_values.csv
...     dialect=excel

Suite Setup     Precondition
Suite Teardown  Run Keywords    Admin Delete EHR For AQL    ${ehr_id1}  AND
                ...             Admin Delete EHR For AQL    ${ehr_id3}


*** Test Cases ***
${query_nr} SELECT ${path} FROM VERSION cv[LATEST_VERSION] CONTAINS COMPOSITION c[openEHR-EHR-COMPOSITION.persistent_minimal.v1] ${where} ${order_by}
    [Template]      Execute Query
    ${path}     ${where}    ${order_by}     ${expected_file}    ${nr_of_results}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      persistent_minimal.opt
    Upload OPT For AQL      conformance_ehrbase.de.v0.opt
    ###
    Create EHR For AQL
    ${tenant_id}    Evaluate    '${system_id_with_tenant}'.split('.')[0:1]
    Set Suite Variable      ${tenant_id_from_system_id}     ${tenant_id}[0].
    Set Suite Variable      ${ehr_id1}  ${ehr_id}
    Commit Contribution For AQL     minimal_persistent/minimal_persistent.contribution.json
    Set Suite Variable      ${compo_uid_1}      ${versions[0]['id']['value']}
    Commit Composition For AQL      composition_file=persistent_minimal.en.v1__full.json
    Set Suite Variable      ${compo_uid_2}      ${composition_uid}
    Set Suite Variable      ${composition_id}   ${compo_uid_2}
    Update Composition For AQL      new_composition=persistent_minimal.en.v1__updated_v2.json
    Set Suite Variable      ${compo_uid_2_v2}   ${updated_version_composition_uid}
    Set Suite Variable      ${composition_id}   ${compo_uid_2_v2}
    Set Suite Variable      ${composition_uid}  ${compo_uid_2_v2}
    Update Composition For AQL      new_composition=persistent_minimal.en.v1__updated_v3.json
    Set Suite Variable      ${compo_uid_2_v3}   ${updated_version_composition_uid}
    Commit Composition For AQL      composition_file=persistent_minimal.en.v1__full.json
    Set Suite Variable      ${compo_uid_3}      ${composition_uid}
    Set Suite Variable      ${composition_id}   ${compo_uid_3}
    Update Composition For AQL      new_composition=persistent_minimal.en.v1__updated_v2.json
    Set Suite Variable      ${compo_uid_3_v2}   ${updated_version_composition_uid}
    Commit Composition For AQL      composition_file=persistent_minimal.en.v1__full.json
    Set Suite Variable      ${compo_uid_4}      ${composition_uid}
    Set Suite Variable      ${composition_id}   ${compo_uid_4}
    Delete Composition For AQL      composition_uid=${composition_id}
    Commit Composition For AQL      composition_file=conformance_ehrbase.de.v0_max.json
    Set Suite Variable      ${compo_uid_5}      ${composition_uid}
    ###
    Create EHR For AQL
    Set Suite Variable      ${ehr_id2}  ${ehr_id}
    Commit Composition For AQL      composition_file=persistent_minimal.en.v1__full.json
    Set Suite Variable      ${compo_uid_6}      ${composition_uid}
    Set Suite Variable      ${composition_id}   ${compo_uid_6}
    Delete Composition For AQL      composition_uid=${composition_id}
    Admin Delete EHR For AQL    ${ehr_id2}
    ###
    Create EHR For AQL
    Set Suite Variable      ${ehr_id3}  ${ehr_id}
    Set Suite Variable      ${ehr_status_id3}   ${resp_json['uid']['value']}
    Update EHR Status For EHR
    ...     ehr_id=${ehr_id3}   ehrstatus_uid=${ehr_status_id3}     ehr_status_file=status3.json
    ###
    Commit Composition For AQL      composition_file=persistent_minimal.en.v1__full.json
    Set Suite Variable      ${compo_uid_7}      ${composition_uid}
    Set Suite Variable      ${composition_id}   ${compo_uid_7}
    Update Composition For AQL      new_composition=persistent_minimal.en.v1__updated_v2.json
    Set Suite Variable      ${compo_uid_7_v2}   ${updated_version_composition_uid}
    ###
    Commit Composition For AQL      composition_file=conformance_ehrbase.de.v0_max.json
    Set Suite Variable      ${compo_uid_8}      ${composition_uid}

Execute Query
    [Arguments]     ${path}     ${where}    ${order_by}     ${expected_file}    ${nr_of_results}
    ${expected_result_file}     Set Variable
    ...     ${EXPECTED_JSON_DATA_SETS}/version/${expected_file}
    ${query_dict}   Create Dictionary
    ...     tmp_query=SELECT ${path} FROM VERSION cv[LATEST_VERSION] CONTAINS COMPOSITION c[openEHR-EHR-COMPOSITION.persistent_minimal.v1] ${where} ${order_by}
    ${query}    Set Variable    ${query_dict["tmp_query"]}
    Log     ${query}
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     ${nr_of_results}
    ${exclude_paths}	Create List    root['meta']     root['q']
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}    ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!