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
${EXPECTED_JSON_RESULTS}    ${EXPECTED_JSON_DATA_SETS}/external/test_set_5


*** Test Cases ***
SELECT i/name/value FROM EHR e CONTAINS SECTION CONTAINS INSTRUCTION i
    ${query}    Set Variable    SELECT i/name/value FROM EHR e CONTAINS SECTION CONTAINS INSTRUCTION i
    ${expected_result}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_section_instruction.json
    Execute Query   query=${query}    expected_rows_nr=14
    ...     expected_file=${expected_result}

SELECT c/name/value FROM EHR e CONTAINS SECTION s[openEHR-EHR-SECTION.adhoc.v1] CONTAINS CLUSTER c[openEHR-EHR-CLUSTER.medication.v1]
    ${query}    Set Variable    SELECT c/name/value FROM EHR e CONTAINS SECTION s[openEHR-EHR-SECTION.adhoc.v1] CONTAINS CLUSTER c[openEHR-EHR-CLUSTER.medication.v1]
    ${expected_result}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_section_x_cluster_x.json
    Execute Query   query=${query}    expected_rows_nr=7
    ...     expected_file=${expected_result}

SELECT obs/name/value FROM EHR e CONTAINS SECTION s CONTAINS OBSERVATION obs
    ${query}    Set Variable    SELECT obs/name/value FROM EHR e CONTAINS SECTION s CONTAINS OBSERVATION obs
    ${expected_result}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_section_observation.json
    Execute Query   query=${query}    expected_rows_nr=28
    ...     expected_file=${expected_result}

SELECT s/name/value as name FROM EHR e CONTAINS COMPOSITION CONTAINS SECTION s[openEHR-EHR-SECTION.adhoc.v1]
    ${query}    Set Variable    SELECT s/name/value as name FROM EHR e CONTAINS COMPOSITION CONTAINS SECTION s[openEHR-EHR-SECTION.adhoc.v1]
    ${expected_result}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_composition_section_x.json
    Execute Query   query=${query}    expected_rows_nr=42
    ...     expected_file=${expected_result}

SELECT obs/name/value FROM EHR e CONTAINS COMPOSITION c CONTAINS SECTION s1 CONTAINS SECTION s2 CONTAINS OBSERVATION obs
    ${query}    Set Variable    SELECT obs/name/value FROM EHR e CONTAINS COMPOSITION c CONTAINS SECTION s1 CONTAINS SECTION s2 CONTAINS OBSERVATION obs
    ${expected_result}      Set Variable    ${EXPECTED_JSON_RESULTS}/expected_composition_section_section_observation.json
    Execute Query   query=${query}    expected_rows_nr=7
    ...     expected_file=${expected_result}


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      external/ehrbase.testcase09.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      external/ehrbase.testcase09.v0__compo1_test_set_5_flat.json     format=FLAT
    Set Suite Variable      ${c_uid1}      ${composition_short_uid}
    Commit Composition For AQL      external/ehrbase.testcase09.v0__compo2_test_set_5_flat.json     format=FLAT
    Set Suite Variable      ${c_uid2}      ${composition_short_uid}
    Commit Composition For AQL      external/ehrbase.testcase09.v0__compo3_test_set_5_flat.json     format=FLAT
    Set Suite Variable      ${c_uid3}      ${composition_short_uid}
    Commit Composition For AQL      external/ehrbase.testcase09.v0__compo4_test_set_5_flat.json     format=FLAT
    Set Suite Variable      ${c_uid4}      ${composition_short_uid}
    Commit Composition For AQL      external/ehrbase.testcase09.v0__compo5_test_set_5_flat.json     format=FLAT
    Set Suite Variable      ${c_uid5}      ${composition_short_uid}
    Commit Composition For AQL      external/ehrbase.testcase09.v0__compo6_test_set_5_flat.json     format=FLAT
    Set Suite Variable      ${c_uid6}      ${composition_short_uid}
    Commit Composition For AQL      external/ehrbase.testcase09.v0__compo7_test_set_5_flat.json     format=FLAT
    Set Suite Variable      ${c_uid7}      ${composition_short_uid}

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