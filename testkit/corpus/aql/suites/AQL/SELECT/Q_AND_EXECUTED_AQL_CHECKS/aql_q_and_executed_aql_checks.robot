*** Settings ***
Documentation   Checks q and _executed_aql API response body for AQL call -> Ad-Hoc and Stored Query.
...             - Covers: https://vitagroup-ag.atlassian.net/browse/CDR-1397
...             - PR: https://github.com/ehrbase/ehrbase/pull/1296
...             - Latest requirements changes here: https://vitagroup-ag.atlassian.net/browse/CDR-1258
Resource        ../../../_resources/keywords/aql_keywords.robot
Resource        ../../../_resources/keywords/aql_query_keywords.robot
Suite Setup     Set Library Search Order For Tests


*** Test Cases ***
1. Query - Ad-Hoc Query POST
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition
    ...         - Send AQL "SELECT e/ehr_id/value, c/uid/value, o/uid/value, p/time/value FROM EHR e CONTAINS COMPOSITION c[uid/value='${compo_id}::${system_id_with_tenant}::1'] CONTAINS OBSERVATION o CONTAINS POINT_EVENT p"
    ...         - Check _executed_aql item is present in response body, meta object.
    [Setup]     Precondition
    Set Test Variable      ${query1}
    ...     SELECT e/ehr_id/value, c/uid/value, o/uid/value, p/time/value FROM EHR e CONTAINS COMPOSITION c[uid/value='${compo_id}::${system_id_with_tenant}::1'] CONTAINS OBSERVATION o CONTAINS POINT_EVENT p
    Set Test Variable       ${test_data}    {"q":"${query1}"}
    Send Ad Hoc Request     aql_body=${test_data}
    Check Reponse Body
    Should Be Equal As Strings      ${resp_body_query}   ${query1}

1.a Query - Ad-Hoc Query POST - Query Params
    Set Test Variable       ${query2}
    ...     SELECT e/ehr_id/value, c/uid/value, o/uid/value, p/time/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o CONTAINS POINT_EVENT p WHERE c/uid/value = \$compo_id
    ${parameter_obj}    Set Variable    {"compo_id": \"${compo_id}::${system_id_with_tenant}::1\" }
    Set Test Variable       ${test_data}    {"q":"${query2}","query_parameters":${parameter_obj}}
    Send Ad Hoc Request     aql_body=${test_data}
    Check Reponse Body
    Should Be Equal As Strings      ${resp_body_query}   ${query2}
    Should Be Equal As Strings      ${executed_aql}
    ...     SELECT e/ehr_id/value, c/uid/value, o/uid/value, p/time/value FROM EHR e CONTAINS COMPOSITION c[uid/value='${compo_id}::${system_id_with_tenant}::1'] CONTAINS OBSERVATION o CONTAINS POINT_EVENT p

1.b Query - Ad-Hoc Query POST - Query Params - Limit And Offset
    Set Test Variable       ${query2}
    ...     SELECT e/ehr_id/value, c/uid/value, o/uid/value, p/time/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o CONTAINS POINT_EVENT p WHERE c/uid/value = \$compo_id
    ${parameter_obj}    Set Variable    {"compo_id": \"${compo_id}::${system_id_with_tenant}::1\" }
    Set Test Variable       ${test_data}    {"q":"${query2}","query_parameters":${parameter_obj}, "offset": 1, "fetch": 2}
    Send Ad Hoc Request     aql_body=${test_data}
    Length Should Be    ${resp_body['rows']}    ${2}
    Should Be Equal     ${resp_body['meta']['resultsize']}      ${2}
    Should Be Equal As Strings      ${resp_body_query}      ${query2}
    Should Be Equal As Strings      ${resp_body['meta']['_executed_aql']}
    ...     SELECT e/ehr_id/value, c/uid/value, o/uid/value, p/time/value FROM EHR e CONTAINS COMPOSITION c[uid/value='${compo_id}::${system_id_with_tenant}::1'] CONTAINS OBSERVATION o CONTAINS POINT_EVENT p LIMIT 2 OFFSET 1

1.c Query - Ad-Hoc Query POST - Query Params - MATCHES Multiple Values
    Set Test Variable       ${query_matches1}
    ...     SELECT pe/time/value, s/name/value FROM COMPOSITION c CONTAINS SECTION s CONTAINS POINT_EVENT pe WHERE s/name/value MATCHES {'Section 1', 'Section 2'}
    Set Test Variable       ${test_data}    {"q":"${query_matches1}"}
    Send Ad Hoc Request     aql_body=${test_data}
    Length Should Be    ${resp_body['rows']}    ${2}
    Should Be Equal     ${resp_body['meta']['resultsize']}      ${2}
    Should Be Equal As Strings      ${resp_body_query}      ${query_matches1}
    Should Be Equal As Strings      ${resp_body['meta']['_executed_aql']}       ${query_matches1}
    Log      ${resp_body['rows'][0]}, ${resp_body['rows'][1]}

1.d Query - Ad-Hoc Query POST - MATCHES Multiple Values
    Set Test Variable       ${query_matches2}
    ...     SELECT pe/time/value, s/name/value FROM EHR e CONTAINS COMPOSITION c CONTAINS SECTION s CONTAINS POINT_EVENT pe WHERE s/name/value MATCHES {$sections}
    Set Test Variable       ${test_data}    {"q":"${query_matches2}","query_parameters": {"sections": ["Section 1","Section 2"]}}
    Send Ad Hoc Request     aql_body=${test_data}
    Length Should Be    ${resp_body['rows']}    ${2}
    Should Be Equal     ${resp_body['meta']['resultsize']}      ${2}
    Should Be Equal As Strings      ${resp_body_query}      ${query_matches2}
    Log      ${resp_body['rows'][0]}, ${resp_body['rows'][1]}
    ### Check with one item in list
    Set Test Variable       ${query_matches3}
    ...     SELECT pe/time/value, s/name/value FROM EHR e CONTAINS COMPOSITION c CONTAINS SECTION s CONTAINS POINT_EVENT pe WHERE s/name/value MATCHES {$sections}
    Set Test Variable       ${test_data}    {"q":"${query_matches3}","query_parameters": {"sections": ["Section 1"]}}
    Send Ad Hoc Request     aql_body=${test_data}
    Length Should Be    ${resp_body['rows']}    ${1}
    Should Be Equal     ${resp_body['meta']['resultsize']}      ${1}
    ### Check with non-existing Section name value
    Set Test Variable       ${query_matches4}
    ...     SELECT pe/time/value, s/name/value FROM EHR e CONTAINS COMPOSITION c CONTAINS SECTION s CONTAINS POINT_EVENT pe WHERE s/name/value MATCHES {$sections}
    Set Test Variable       ${test_data}    {"q":"${query_matches4}","query_parameters": {"sections": ["Non existing section"]}}
    Send Ad Hoc Request     aql_body=${test_data}
    Length Should Be    ${resp_body['rows']}    ${0}

2. Query - Ad-Hoc Query GET
    Set Test Variable       ${query3}
    ...     SELECT c/uid/value FROM EHR e[ehr_id/value='${ehr_id}'] CONTAINS COMPOSITION c
    Send Ad Hoc Request Through GET     q_param=${query3}
    Length Should Be    ${resp_body_columns}    ${1}
    Should Be Equal     ${resp_body['meta']['resultsize']}      ${1}
    Should Be Equal As Strings      ${resp_body_query}      ${query3}
    Should Be Equal As Strings      ${resp_body['meta']['_executed_aql']}
    ...     ${query3}

2.a Query - Ad-Hoc Query GET - Query Params
    Set Test Variable       ${query3}
    ...     SELECT c/uid/value FROM EHR e CONTAINS COMPOSITION c WHERE e/ehr_id/value = \$ehr_id
    ${parameter_obj}    Set Variable    {"ehr_id": \"${ehr_id}\" }
    Set Test Variable      ${test_data}    {"q":"${query3}","query_parameters":${parameter_obj}}
    Send Ad Hoc Request     aql_body=${test_data}
    Length Should Be    ${resp_body_columns}    ${1}
    Should Be Equal     ${resp_body['meta']['resultsize']}      ${1}
    Should Be Equal As Strings      ${resp_body_query}      ${query3}
    Should Be Equal As Strings      ${resp_body['meta']['_executed_aql']}
    ...     SELECT c/uid/value FROM EHR e[ehr_id/value='${ehr_id}'] CONTAINS COMPOSITION c

2.b Query - Ad-Hoc Query GET - Query Params - Limit And Offset
    Set Test Variable       ${query3}
    ...     SELECT c/uid/value FROM EHR e CONTAINS COMPOSITION c WHERE e/ehr_id/value = \$ehr_id
    ${parameter_obj}    Set Variable    {"ehr_id": \"${ehr_id}\" }
    Set Test Variable       ${test_data}    {"q":"${query3}","query_parameters":${parameter_obj}, "offset": 1, "fetch": 1}
    Send Ad Hoc Request     aql_body=${test_data}
    Length Should Be    ${resp_body['rows']}    ${0}
    Should Be Equal As Strings      ${resp_body_query}      ${query3}
    Should Be Equal As Strings      ${resp_body['meta']['_executed_aql']}
    ...     SELECT c/uid/value FROM EHR e[ehr_id/value='${ehr_id}'] CONTAINS COMPOSITION c LIMIT 1 OFFSET 1

2.c Query - Ad-Hoc Query GET - Query Params Fetch And Offset - Query Limit And Offset
    [Documentation]     Covers https://vitagroup-ag.atlassian.net/browse/CDR-1391
    Set Test Variable       ${query3}
    ...     SELECT c/uid/value FROM EHR e CONTAINS COMPOSITION c WHERE e/ehr_id/value = \$ehr_id LIMIT 1 OFFSET 1
    ${parameter_obj}    Set Variable    {"ehr_id": \"${ehr_id}\" }
    Set Test Variable       ${test_data}    {"q":"${query3}","query_parameters":${parameter_obj}, "offset": 1, "fetch": 1}
    ${err_msg}  Run Keyword And Expect Error    *
    ...     Send Ad Hoc Request     aql_body=${test_data}
    Should Contain      ${err_msg}      422 != 200
    Should Contain      ${err_msg}      Query contains a LIMIT clause, fetch and offset parameters must not be used

2.d Query - Ad-Hoc Query GET - Query Param Fetch - Query Limit And Offset
    [Documentation]     Covers https://vitagroup-ag.atlassian.net/browse/CDR-1391
    Set Test Variable       ${query3}
    ...     SELECT c/uid/value FROM EHR e CONTAINS COMPOSITION c WHERE e/ehr_id/value = \$ehr_id LIMIT 1 OFFSET 1
    ${parameter_obj}    Set Variable    {"ehr_id": \"${ehr_id}\" }
    Set Test Variable       ${test_data}    {"q":"${query3}","query_parameters":${parameter_obj}, "fetch": 1}
    ${err_msg}  Run Keyword And Expect Error    *
    ...     Send Ad Hoc Request     aql_body=${test_data}
    Should Contain      ${err_msg}      422 != 200
    Should Contain      ${err_msg}      Query contains a LIMIT clause, fetch and offset parameters must not be used

2.e Query - Ad-Hoc Query GET - Query Param Offset - Query Limit And Offset
    [Documentation]     Covers https://vitagroup-ag.atlassian.net/browse/CDR-1391
    Set Test Variable       ${query3}
    ...     SELECT c/uid/value FROM EHR e CONTAINS COMPOSITION c WHERE e/ehr_id/value = \$ehr_id LIMIT 1 OFFSET 1
    ${parameter_obj}    Set Variable    {"ehr_id": \"${ehr_id}\" }
    Set Test Variable       ${test_data}    {"q":"${query3}","query_parameters":${parameter_obj}, "offset": 1}
    ${err_msg}  Run Keyword And Expect Error    *
    ...     Send Ad Hoc Request     aql_body=${test_data}
    Should Contain      ${err_msg}      422 != 200
    Should Contain      ${err_msg}      Query parameter for offset provided, but no fetch parameter

2.f Query - Ad-Hoc Query GET - Query Param ehr_id - Query Limit And Offset
    [Documentation]     Covers https://vitagroup-ag.atlassian.net/browse/CDR-1391
    Set Test Variable       ${query3}
    ...     SELECT c/uid/value FROM EHR e CONTAINS COMPOSITION c WHERE e/ehr_id/value = \$ehr_id LIMIT 1 OFFSET 1
    ${parameter_obj}    Set Variable    {"ehr_id": \"${ehr_id}\" }
    Set Test Variable       ${test_data}    {"q":"${query3}","query_parameters":${parameter_obj}}
    Send Ad Hoc Request     aql_body=${test_data}
    Length Should Be    ${resp_body['rows']}    ${0}
    Should Be Equal As Strings      ${resp_body_query}      ${query3}

3. Query - Stored Query GET By Qualified Query Name And Version
    Set Test Variable      ${query3}
    ...     SELECT c/uid/value, o/uid/value, p/time/value FROM COMPOSITION c[uid/value='${compo_id}::${system_id_with_tenant}::1'] CONTAINS OBSERVATION o CONTAINS POINT_EVENT p
    ${resp_qualified_query_name_version}     PUT /definition/query/{qualified_query_name}/{version}
    ...     query_to_store=${query3}     format=text
    GET /query/{qualified_query_name}
    ...     qualif_name=${resp_qualified_query_name_version}
    Set Test Variable   ${resp_body}    ${resp}
    Length Should Be    ${resp_body['columns']}    ${3}
    Should Be Equal As Strings      ${resp_body['meta']['_executed_aql']}      ${query3}

4. Query - Stored Query POST By Qualified Query Name
    [Documentation]     - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    Set Test Variable      ${query4}
    ...     SELECT o/uid/value, p/time/value FROM COMPOSITION c[uid/value='${compo_id}::${system_id_with_tenant}::1'] CONTAINS OBSERVATION o CONTAINS POINT_EVENT p
    ${resp_qualified_query_name}     PUT /definition/query/{qualified_query_name}
    ...     query_to_store=${query4}     format=text
    ${resp_query}       POST /query/{qualified_query_name}
    ...     qualif_name=${resp_qualified_query_name}
    Set Test Variable   ${resp_body}    ${resp}
    Length Should Be    ${resp_body['columns']}    ${2}
    Should Be Equal As Strings      ${resp_body['meta']['_executed_aql']}      ${query4}
    [Teardown]      Admin Delete EHR For AQL


*** Keywords ***
Precondition
    Upload OPT For AQL      aql-conformance-ehrbase.org.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      aql-conformance-ehrbase.org.v0_contains.json
    Set Suite Variable       ${compo_id}     ${composition_short_uid}

Check Reponse Body
    Length Should Be    ${resp_body_columns}    ${4}
    ${rows_count}   Get Length      ${resp_body['rows']}
    Length Should Be    ${resp_body['rows']}    ${5}
    Dictionary Should Contain Key   ${resp_body['meta']}    _executed_aql
    Set Test Variable   ${executed_aql}     ${resp_body['meta']['_executed_aql']}
    Should Be Equal     ${resp_body['meta']['resultsize']}      ${rows_count}
