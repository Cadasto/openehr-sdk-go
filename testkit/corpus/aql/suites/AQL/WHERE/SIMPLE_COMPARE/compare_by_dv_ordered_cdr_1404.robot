*** Settings ***
Documentation   WHERE - SIMPLE COMPARE - Compare by DV_ORDERED - CDR-1404
...             - Covers the following:
...             - https://vitagroup-ag.atlassian.net/browse/CDR-1404

Resource        ../../../_resources/keywords/aql_keywords.robot


*** Variables ***
${query}    SELECT o/data/events/data/items/value/magnitude
...         FROM OBSERVATION o [openEHR-EHR-OBSERVATION.conformance_observation.v0]
...         WHERE o/data[at0001]/events[at0002]/data[at0003]/items[at0008]/value = 82.0


*** Test Cases ***
CDR-1404: ${query}
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition
    ...         - Send AQL '${query}'
    ...         - Check if actual response == expected response
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    [Setup]     Precondition
    Set AQL And Execute Ad Hoc Query        ${query}
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/where/expected_compare_by_dv_ordered_cdr_1404.json
    ${exclude_paths}    Create List    root['meta']     root['q']   root['rows'][0][0]['uid']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}      exclude_paths=${exclude_paths}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!
    [Teardown]      Admin Delete EHR For AQL


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      conformance_ehrbase.de.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      conformance_ehrbase.de.v0_max_v2.json
    Set Suite Variable       ${c_uid1}      ${composition_short_uid}