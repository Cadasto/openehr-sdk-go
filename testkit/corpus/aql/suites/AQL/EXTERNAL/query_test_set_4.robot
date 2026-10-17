*** Settings ***
Documentation   Queries provided by Medblocks Team
...         - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Compositions
...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
Resource        ../../_resources/keywords/aql_keywords.robot

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Admin Delete EHR For AQL    #enable this keyword if AQL checks are passing !!!


*** Variables ***
${EXPECTED_JSON_RESULTS}    ${EXPECTED_JSON_DATA_SETS}/external/test_set_4


*** Test Cases ***
SELECT o1/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude, o2/data[at0002]/events[at0003]/data[at0001]/items[at0004]/value/magnitude FROM EHR e CONTAINS ((COMPOSITION c[openEHR-EHR-COMPOSITION.health_summary.v1] CONTAINS OBSERVATION o1[openEHR-EHR-OBSERVATION.blood_pressure.v2]) OR OBSERVATION o2[openEHR-EHR-OBSERVATION.body_temperature.v2])
    ${query}    Set Variable    SELECT o1/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude, o2/data[at0002]/events[at0003]/data[at0001]/items[at0004]/value/magnitude FROM EHR e CONTAINS ((COMPOSITION c[openEHR-EHR-COMPOSITION.health_summary.v1] CONTAINS OBSERVATION o1[openEHR-EHR-OBSERVATION.blood_pressure.v2]) OR OBSERVATION o2[openEHR-EHR-OBSERVATION.body_temperature.v2])
    ${expected_result}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_composition_x_observation_x_or_observation_x.json
    Execute Query   query=${query}    expected_rows_nr=24
    ...     expected_file=${expected_result}

SELECT o1/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude, o2/data[at0002]/events[at0003]/data[at0001]/items[at0004]/value/magnitude FROM EHR e CONTAINS ((COMPOSITION c[openEHR-EHR-COMPOSITION.health_summary.v1] CONTAINS OBSERVATION o1[openEHR-EHR-OBSERVATION.blood_pressure.v2]) AND OBSERVATION o2[openEHR-EHR-OBSERVATION.body_temperature.v2])
    ${query}    Set Variable    SELECT o1/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude, o2/data[at0002]/events[at0003]/data[at0001]/items[at0004]/value/magnitude FROM EHR e CONTAINS ((COMPOSITION c[openEHR-EHR-COMPOSITION.health_summary.v1] CONTAINS OBSERVATION o1[openEHR-EHR-OBSERVATION.blood_pressure.v2]) AND OBSERVATION o2[openEHR-EHR-OBSERVATION.body_temperature.v2])
    ${expected_result}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_composition_x_observation_x_and_observation_x.json
    Execute Query   query=${query}    expected_rows_nr=24
    ...     expected_file=${expected_result}

SELECT o/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude, a/description[at0001]/items[at0002]/value/value FROM EHR e CONTAINS ((COMPOSITION c1[openEHR-EHR-COMPOSITION.health_summary.v1] CONTAINS ACTION a) OR (COMPOSITION c2[openEHR-EHR-COMPOSITION.encounter.v1] CONTAINS OBSERVATION o))
    ${query}    Set Variable    SELECT o/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude, a/description[at0001]/items[at0002]/value/value FROM EHR e CONTAINS ((COMPOSITION c1[openEHR-EHR-COMPOSITION.health_summary.v1] CONTAINS ACTION a) OR (COMPOSITION c2[openEHR-EHR-COMPOSITION.encounter.v1] CONTAINS OBSERVATION o))
    ${expected_result}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_composition_x_action_or_composition_x_observation.json
    Execute Query   query=${query}    expected_rows_nr=16
    ...     expected_file=${expected_result}

SELECT c/name/value, o/data[at0002]/events[at0003]/data[at0001]/items[at0004]/value/magnitude FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.body_temperature.v2] WHERE c/archetype_node_id != 'openEHR-EHR-COMPOSITION.encounter.v1'
    ${query}    Set Variable    SELECT c/name/value, o/data[at0002]/events[at0003]/data[at0001]/items[at0004]/value/magnitude FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.body_temperature.v2] WHERE c/archetype_node_id != 'openEHR-EHR-COMPOSITION.encounter.v1'
    ${expected_result}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_not_composition_x_observation_x.json
    Execute Query   query=${query}    expected_rows_nr=4
    ...     expected_file=${expected_result}

SELECT c2/name/value, o/data[at0002]/events[at0003]/data[at0001]/items[at0004]/value/magnitude FROM EHR e CONTAINS ((COMPOSITION c1 CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.body_temperature.v2]) AND NOT CONTAINS COMPOSITION c2[openEHR-EHR-COMPOSITION.encounter.v1])
    [Documentation]     Not yet supported!
    ...     Should be supported based on https://specifications.openehr.org/releases/QUERY/latest/AQL.html#:~:text=COMPOSITION.referral.v1%5D-,NOT%20CONTAINS,-OBSERVATION%20o%20%5BopenEHR
    [Tags]      not-ready
    ${query}    Set Variable    SELECT c2/name/value, o/data[at0002]/events[at0003]/data[at0001]/items[at0004]/value/magnitude FROM EHR e CONTAINS ((COMPOSITION c1 CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.body_temperature.v2]) AND NOT CONTAINS COMPOSITION c2[openEHR-EHR-COMPOSITION.encounter.v1])
    ${expected_result}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_composition_observation_x_and_not_composition_x.json
    Execute Query   query=${query}    expected_rows_nr=4
    ...     expected_file=${expected_result}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      external/ehrbase.testcase06.v0.opt
    Upload OPT For AQL      external/ehrbase.testcase008.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      external/ehrbase.testcase008.v0__compo_1_1_test_set_4_flat.json     format=FLAT
    Set Suite Variable      ${c_uid1}      ${composition_short_uid}
    Commit Composition For AQL      external/ehrbase.testcase008.v0__compo_1_2_test_set_4_flat.json     format=FLAT
    Set Suite Variable      ${c_uid2}      ${composition_short_uid}
    Commit Composition For AQL      external/ehrbase.testcase008.v0__compo_1_3_test_set_4_flat.json     format=FLAT
    Set Suite Variable      ${c_uid3}      ${composition_short_uid}
    Commit Composition For AQL      external/ehrbase.testcase008.v0__compo_1_4_no_obs_cluster_test_set_4_flat.json     format=FLAT
    Set Suite Variable      ${c_uid4}      ${composition_short_uid}
    Set Suite Variable      ${template_id}     ehrbase.testcase06.v0
    Commit Composition For AQL      external/ehrbase.testcase06.v0__compo_2_1_test_set_4.json     format=FLAT
    Set Suite Variable      ${c_uid5}      ${composition_short_uid}
    Commit Composition For AQL      external/ehrbase.testcase06.v0__compo_2_2_test_set_4.json     format=FLAT
    Set Suite Variable      ${c_uid6}      ${composition_short_uid}

Execute Query
    [Arguments]     ${query}    ${expected_rows_nr}    ${expected_file}
    ...     ${ignore_order}=${TRUE}    ${ignore_string_case}=${TRUE}
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     ${expected_rows_nr}
    ${exclude_paths}	Create List    root['meta']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_file}
    ...     exclude_paths=${exclude_paths}
    ...     ignore_order=${ignore_order}    ignore_string_case=${ignore_string_case}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!