*** Settings ***
Documentation   CHECK SELECT WITH MULTI SELECTS
...             - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/SELECT_TEST_SUIT.md#multi-selects
Resource        ../../../_resources/keywords/aql_keywords.robot
Suite Setup     Set Library Search Order For Tests


*** Test Cases ***
Test Multi Selects
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create 2x Compositions
    ...         - Send AQL 'SELECT e/ehr_id/value, c/uid/value, o/uid/value, p/time/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o CONTAINS POINT_EVENT p'
    ...         - Check that result contains 8 rows
    ...         - Check if actual response == expected response
    ...         - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    #[Tags]      not-ready
    [Setup]     Precondition
    ${query}    Set Variable
    ...     SELECT e/ehr_id/value, c/uid/value, o/uid/value, p/time/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o CONTAINS POINT_EVENT p
    Set AQL And Execute Ad Hoc Query        ${query}
    #${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/select/distinct_2_rows.json
    #${expected_result}      Replace Variables       ${tmp_expected_result}
    #Log     Add test data once 200 is returned. File: ${expected_result}
    #${exclude_paths}    Create List    root['rows'][0][0]['uid']
    Length Should Be    ${resp_body['rows']}     10
    ${rows_length}      Get Length      ${resp_body['rows']}
    ####
    FOR     ${INDEX}   IN RANGE     0   ${rows_length}
        Log     ${resp_body['rows'][${INDEX}]}      #enable to see the rows item at specific index
        IF  '''${resp_body['rows'][${INDEX}][0]}''' == '''${NONE}'''
           Fail     None is present within rows results.
        ELSE
            Log     Success
            #Should Be Equal As Strings     ${resp_body['rows'][${INDEX}][0]["_type"]}       OBSERVATION
        END
    END
    ####
    #${diff}     compare json-string with json-file
    #...     ${resp_body_actual}     ${expected_result}      exclude_paths=${exclude_paths}
    ##Log To Console    \n\n${diff}
    #Should Be Empty    ${diff}    msg=DIFF DETECTED!
# Check rows to contain below data - once 200 status code is returned.
#    e/ehr_id/value	c/uid/value	  o/uid/value	                            p/time/value
#    {ehr_id}	   {c_uid1}	      d86702c2-15db-4d85-be51-af335cf43123	    2022-02-03T04:05:06
#    {ehr_id}	   {c_uid1}	      d86702c2-15db-4d85-be51-af335cf43123	    2023-02-03T04:05:06
#    {ehr_id}	   {c_uid1}	      38065052-af1f-4b0b-b03d-9bda140a6edf	    2024-02-03T04:05:06
#    {ehr_id}	   {c_uid1}	      38065052-af1f-4b0b-b03d-9bda140a6edf	    2025-02-03T04:05:06
#    {ehr_id}	   {c_uid2}	      d86702c2-15db-4d85-be51-af335cf43123	    2022-02-03T04:05:06
#    {ehr_id}	   {c_uid2}	      d86702c2-15db-4d85-be51-af335cf43123	    2023-02-03T04:05:06
#    {ehr_id}	   {c_uid2}	      38065052-af1f-4b0b-b03d-9bda140a6edf	    2024-02-03T04:05:06
#    {ehr_id}	   {c_uid2}	      38065052-af1f-4b0b-b03d-9bda140a6edf	    2025-02-03T04:05:06
    [Teardown]      Admin Delete EHR For AQL


*** Keywords ***
Precondition
    Upload OPT For AQL      aql-conformance-ehrbase.org.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      aql-conformance-ehrbase.org.v0_contains.json
    Set Test Variable       ${c_uid1}      ${composition_short_uid}
    Commit Composition For AQL      aql-conformance-ehrbase.org.v0_contains.json
    Set Test Variable       ${c_uid2}      ${composition_short_uid}