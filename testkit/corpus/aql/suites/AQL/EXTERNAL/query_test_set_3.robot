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
${EXPECTED_JSON_RESULTS}    ${EXPECTED_JSON_DATA_SETS}/external/test_set_3


*** Test Cases ***
SELECT c/items[at0001]/value/value, o/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude, o/data[at0001]/events[at0006]/data[at0003]/items[at0005]/value/magnitude FROM EHR e CONTAINS COMPOSITION C[openEHR-EHR-COMPOSITION.encounter.v1] CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v2] CONTAINS CLUSTER c[openEHR-EHR-CLUSTER.device.v1]
    ${query}    Set Variable    SELECT c/items[at0001]/value/value, o/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude, o/data[at0001]/events[at0006]/data[at0003]/items[at0005]/value/magnitude FROM EHR e CONTAINS COMPOSITION C[openEHR-EHR-COMPOSITION.encounter.v1] CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v2] CONTAINS CLUSTER c[openEHR-EHR-CLUSTER.device.v1]
    ${expected_result}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_composition_x_observation_x_cluster_x.json
    Execute Query   query=${query}    expected_rows_nr=18
    ...     expected_file=${expected_result}

SELECT C/items[at0001]/value/value, o/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude, o/data[at0001]/events[at0006]/data[at0003]/items[at0005]/value/magnitude FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v2] CONTAINS CLUSTER C[openEHR-EHR-CLUSTER.device.v1]
    ${query}    Set Variable    SELECT C/items[at0001]/value/value, o/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude, o/data[at0001]/events[at0006]/data[at0003]/items[at0005]/value/magnitude FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v2] CONTAINS CLUSTER C[openEHR-EHR-CLUSTER.device.v1]
    ${expected_result}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_observation_x_cluster_x_bp_device.json
    Execute Query   query=${query}    expected_rows_nr=18
    ...     expected_file=${expected_result}

SELECT i/activities[at0001]/description[at0002]/items[at0070]/value/value, C/items[at0057]/value/magnitude FROM EHR e CONTAINS INSTRUCTION i[openEHR-EHR-INSTRUCTION.medication_order.v3] CONTAINS CLUSTER C[openEHR-EHR-CLUSTER.therapeutic_direction.v1]
    ${query}    Set Variable    SELECT i/activities[at0001]/description[at0002]/items[at0070]/value/value, C/items[at0057]/value/magnitude FROM EHR e CONTAINS INSTRUCTION i[openEHR-EHR-INSTRUCTION.medication_order.v3] CONTAINS CLUSTER C[openEHR-EHR-CLUSTER.therapeutic_direction.v1]
    ${expected_result}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_instruction_x_cluster_x.json
    Execute Query   query=${query}    expected_rows_nr=6
    ...     expected_file=${expected_result}

SELECT C/items[at0001]/value/value, a/data[at0001]/items[at0004]/value/value FROM EHR e CONTAINS ADMIN_ENTRY a[openEHR-EHR-ADMIN_ENTRY.translation_requirements.v1] CONTAINS CLUSTER C[openEHR-EHR-CLUSTER.language.v1]
    ${query}    Set Variable    SELECT C/items[at0001]/value/value, a/data[at0001]/items[at0004]/value/value FROM EHR e CONTAINS ADMIN_ENTRY a[openEHR-EHR-ADMIN_ENTRY.translation_requirements.v1] CONTAINS CLUSTER C[openEHR-EHR-CLUSTER.language.v1]
    ${expected_result}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_admin_x_cluster_x.json
    Execute Query   query=${query}    expected_rows_nr=7
    ...     expected_file=${expected_result}

SELECT C/items[at0001]/value/value, C2/items[at0001]/value/size, o/data[at0001]/events[at0006]/data[at0003]/items[at0005]/value/magnitude FROM EHR e CONTAINS COMPOSITION c[openEHR-EHR-COMPOSITION.encounter.v1] CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v2] CONTAINS CLUSTER C[openEHR-EHR-CLUSTER.anatomical_location.v1] CONTAINS CLUSTER C2[openEHR-EHR-CLUSTER.media_file.v1]
    ${query}    Set Variable    SELECT C/items[at0001]/value/value, C2/items[at0001]/value/size, o/data[at0001]/events[at0006]/data[at0003]/items[at0005]/value/magnitude FROM EHR e CONTAINS COMPOSITION c[openEHR-EHR-COMPOSITION.encounter.v1] CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v2] CONTAINS CLUSTER C[openEHR-EHR-CLUSTER.anatomical_location.v1] CONTAINS CLUSTER C2[openEHR-EHR-CLUSTER.media_file.v1]
    ${expected_result}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_composition_observation_x_cluster_x_cluster_x.json
    Execute Query   query=${query}    expected_rows_nr=6
    ...     expected_file=${expected_result}

SELECT alg/data[at0001]/items[at0002]/value/value, med/activities[at0001]/description[at0002]/items[at0070]/value/value FROM EHR e CONTAINS ((COMPOSITION c1 CONTAINS EVALUATION alg[openEHR-EHR-EVALUATION.problem_diagnosis.v1]) AND (COMPOSITION c2 CONTAINS INSTRUCTION med[openEHR-EHR-INSTRUCTION.medication_order.v3]))
    ${query}    Set Variable    SELECT alg/data[at0001]/items[at0002]/value/value, med/activities[at0001]/description[at0002]/items[at0070]/value/value FROM EHR e CONTAINS ((COMPOSITION c1 CONTAINS EVALUATION alg[openEHR-EHR-EVALUATION.problem_diagnosis.v1]) AND (COMPOSITION c2 CONTAINS INSTRUCTION med[openEHR-EHR-INSTRUCTION.medication_order.v3]))
    ${expected_result}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_composition_evaluation_x_and_composition_instruction_x.json
    Execute Query   query=${query}    expected_rows_nr=49
    ...     expected_file=${expected_result}

SELECT o1/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude, o2/data[at0002]/events[at0003]/data[at0001]/items[at0004]/value/magnitude FROM EHR e CONTAINS COMPOSITION c CONTAINS (OBSERVATION o1[openEHR-EHR-OBSERVATION.blood_pressure.v2] AND OBSERVATION o2[openEHR-EHR-OBSERVATION.body_temperature.v2])
    ${query}    Set Variable    SELECT o1/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude, o2/data[at0002]/events[at0003]/data[at0001]/items[at0004]/value/magnitude FROM EHR e CONTAINS COMPOSITION c CONTAINS (OBSERVATION o1[openEHR-EHR-OBSERVATION.blood_pressure.v2] AND OBSERVATION o2[openEHR-EHR-OBSERVATION.body_temperature.v2])
    ${expected_result}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_composition_observation_x_and_observation_x.json
    Execute Query   query=${query}    expected_rows_nr=36
    ...     expected_file=${expected_result}

SELECT o1/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude, a/description[at0001]/items[at0002]/value/value FROM EHR e CONTAINS (OBSERVATION o1[openEHR-EHR-OBSERVATION.blood_pressure.v2] OR ACTION a[openEHR-EHR-ACTION.procedure.v1])
    ${query}    Set Variable    SELECT o1/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude, a/description[at0001]/items[at0002]/value/value FROM EHR e CONTAINS (OBSERVATION o1[openEHR-EHR-OBSERVATION.blood_pressure.v2] OR ACTION a[openEHR-EHR-ACTION.procedure.v1])
    ${expected_result}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_observation_x_or_action_x.json
    Execute Query   query=${query}    expected_rows_nr=147
    ...     expected_file=${expected_result}

SELECT i/activities[at0001]/description[at0002]/items[at0070]/value/value, a/description[at0001]/items[at0002]/value/value FROM EHR e CONTAINS (INSTRUCTION i[openEHR-EHR-INSTRUCTION.medication_order.v3] OR ACTION a[openEHR-EHR-ACTION.procedure.v1])
    ${query}    Set Variable    SELECT i/activities[at0001]/description[at0002]/items[at0070]/value/value, a/description[at0001]/items[at0002]/value/value FROM EHR e CONTAINS (INSTRUCTION i[openEHR-EHR-INSTRUCTION.medication_order.v3] OR ACTION a[openEHR-EHR-ACTION.procedure.v1])
    ${expected_result}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_action_x_or_instruction_x.json
    Execute Query   query=${query}    expected_rows_nr=49
    ...     expected_file=${expected_result}

SELECT a/description[at0001]/items[at0002]/value/value, a/description[at0001]/items[at0002]/value/value FROM EHR e CONTAINS ((COMPOSITION c1[openEHR-EHR-COMPOSITION.encounter.v1] CONTAINS ACTION a) OR (COMPOSITION c2[openEHR-EHR-COMPOSITION.encounter.v1] CONTAINS OBSERVATION o))
    ${query}    Set Variable    SELECT a/description[at0001]/items[at0002]/value/value, a/description[at0001]/items[at0002]/value/value FROM EHR e CONTAINS ((COMPOSITION c1[openEHR-EHR-COMPOSITION.encounter.v1] CONTAINS ACTION a) OR (COMPOSITION c2[openEHR-EHR-COMPOSITION.encounter.v1] CONTAINS OBSERVATION o))
    ${expected_result}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_composition_x_action_or_composition_x_observation.json
    Execute Query   query=${query}    expected_rows_nr=77
    ...     expected_file=${expected_result}

SELECT C1/items[at0001]/value/value, C2/items[at0001]/value/value FROM EHR e CONTAINS OBSERVATION o CONTAINS (CLUSTER C1[openEHR-EHR-CLUSTER.device.v1] AND CLUSTER C2[openEHR-EHR-CLUSTER.anatomical_location.v1])
    ${query}    Set Variable    SELECT C1/items[at0001]/value/value, C2/items[at0001]/value/value FROM EHR e CONTAINS OBSERVATION o CONTAINS (CLUSTER C1[openEHR-EHR-CLUSTER.device.v1] AND CLUSTER C2[openEHR-EHR-CLUSTER.anatomical_location.v1])
    ${expected_result}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_observation_cluster_x_and_cluster_x.json
    Execute Query   query=${query}    expected_rows_nr=6
    ...     expected_file=${expected_result}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      external/ehrbase.testcase06.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      external/ehrbase.testcase06.v0__compo1_test_set_3_flat.json     format=FLAT
    Set Suite Variable       ${c_uid1}      ${composition_short_uid}
    Commit Composition For AQL      external/ehrbase.testcase06.v0__compo2_test_set_3_flat.json     format=FLAT
    Set Suite Variable       ${c_uid2}      ${composition_short_uid}
    Commit Composition For AQL      external/ehrbase.testcase06.v0__compo3_test_set_3_flat.json     format=FLAT
    Set Suite Variable       ${c_uid3}      ${composition_short_uid}
    Commit Composition For AQL      external/ehrbase.testcase06.v0__compo4_no_obs_cluster_test_set_3_flat.json      format=FLAT
    Set Suite Variable       ${c_uid4}      ${composition_short_uid}
    Commit Composition For AQL      external/ehrbase.testcase06.v0__compo5_no_action_cluster_test_set_3_flat.json       format=FLAT
    Set Suite Variable       ${c_uid5}      ${composition_short_uid}
    Commit Composition For AQL      external/ehrbase.testcase06.v0__compo6_no_evaluation_cluster_test_set_3_flat.json   format=FLAT
    Set Suite Variable       ${c_uid6}      ${composition_short_uid}
    Commit Composition For AQL      external/ehrbase.testcase06.v0__compo7_no_instruction_cluster_test_set_3_flat.json  format=FLAT
    Set Suite Variable       ${c_uid7}      ${composition_short_uid}

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
