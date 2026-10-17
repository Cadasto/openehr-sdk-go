*** Settings ***
Documentation   Checks API calls response for Ad-Hoc and Stored Query endpoints, having request header with:
                ...     - EHRbase-AQL-Dry-Run
                ...     - EHRbase-AQL-Executed-SQL
                ...     - EHRbase-AQL-Query-Plan.
                ...     \nCovers: https://vitagroup-ag.atlassian.net/browse/CDR-1430
                ...     \nPR: https://github.com/ehrbase/ehrbase/pull/1324
                ...     \n*PRECONDITION:*
                ...     \nSet in EHRBase ehrbase\configuration\src\main\resources\application.yml file the following values:
                ...     - ehrbase.rest.aql.debugging-enabled: true
                ...     - ehrbase.rest.aql.response.generator-details-enabled: true
                ...     - ehrbase.rest.aql.response.executed-aql-enabled: true
                ...     \n*To be executed locally only (unless there is a request to enable them in EHRBase OSS pipeline)*
#                ...     Robot cmd (from tests folder):
#                ...     robot -d results --skiponfailure not-ready -L DEBUG .\robot\AQL\SELECT\DEBUG_OPTS_CHECKS
Resource        ../../../_resources/keywords/aql_keywords.robot
Resource        ../../../_resources/keywords/aql_query_keywords.robot
Suite Setup     Set Library Search Order For Tests


*** Test Cases ***
1. Query - Ad-Hoc Query POST - Check Debug Options
    [Documentation]     - *Precondition:* 1. Create OPT; 2. Create EHR; 3. Create Composition
    ...         - Send AQL "SELECT e/ehr_id/value, c/uid/value, o/uid/value, p/time/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o CONTAINS POINT_EVENT p WHERE c/uid/value = '{compo_id}::{system_id_with_tenant}::1'"
    [Setup]     Precondition
    &{request_headers}      Create Dictionary
    ...     Content-Type=application/json
    ...     EHRbase-AQL-Dry-Run=true
    ...     EHRbase-AQL-Executed-SQL=true
    ...     EHRbase-AQL-Query-Plan=true
    Set Test Variable      ${query1}
    ...     SELECT e/ehr_id/value, c/uid/value, o/uid/value, p/time/value FROM EHR e CONTAINS COMPOSITION c[uid/value='${compo_id}::${system_id_with_tenant}::1'] CONTAINS OBSERVATION o CONTAINS POINT_EVENT p
    Set Test Variable       ${test_data}    {"q":"${query1}"}
    Send Ad Hoc Request     aql_body=${test_data}   req_headers=${request_headers}
    Check Reponse Body      columns_nr=4         rows_nr=0
    Should Be Equal As Strings      ${resp_body_query}   ${query1}

1.a Query - Ad-Hoc Query POST - Query Params - Check Debug Options
    &{request_headers}      Create Dictionary
    ...     Content-Type=application/json
    ...     EHRbase-AQL-Dry-Run=false
    ...     EHRbase-AQL-Executed-SQL=true
    ...     EHRbase-AQL-Query-Plan=true
    Set Test Variable       ${query2}
    ...     SELECT e/ehr_id/value, c/uid/value, o/uid/value, p/time/value FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o CONTAINS POINT_EVENT p WHERE c/uid/value = \$compo_id
    ${parameter_obj}    Set Variable    {"compo_id": \"${compo_id}::${system_id_with_tenant}::1\" }
    Set Test Variable       ${test_data}    {"q":"${query2}","query_parameters":${parameter_obj}}
    Send Ad Hoc Request     aql_body=${test_data}   req_headers=${request_headers}
    Check Reponse Body      columns_nr=4         rows_nr=5      dry_run=false
    #Should Be Equal As Strings      ${resp_body['meta']['dry_run']}    ${FALSE}
    Should Be Equal As Strings      ${resp_body_query}   ${query2}
    Should Be Equal As Strings      ${executed_aql}
    ...     SELECT e/ehr_id/value, c/uid/value, o/uid/value, p/time/value FROM EHR e CONTAINS COMPOSITION c[uid/value='${compo_id}::${system_id_with_tenant}::1'] CONTAINS OBSERVATION o CONTAINS POINT_EVENT p

1.b Query - Ad-Hoc Query POST - Query Params - Limit And Offset - Check Debug Options
    &{request_headers}      Create Dictionary
    ...     Content-Type=application/json
    ...     EHRbase-AQL-Dry-Run=true
    ...     EHRbase-AQL-Executed-SQL=true
    ...     EHRbase-AQL-Query-Plan=true
    Set Test Variable       ${query2}
    ...     SELECT e/ehr_id/value, c/uid/value, o/uid/value, p/time/value FROM EHR e CONTAINS COMPOSITION c[uid/value=\$compo_id] CONTAINS OBSERVATION o CONTAINS POINT_EVENT p
    ${parameter_obj}    Set Variable    {"compo_id": \"${compo_id}::${system_id_with_tenant}::1\" }
    Set Test Variable       ${test_data}    {"q":"${query2}","query_parameters":${parameter_obj}, "offset": 1, "fetch": 2}
    Send Ad Hoc Request     aql_body=${test_data}   req_headers=${request_headers}
    Check Reponse Body      columns_nr=4         rows_nr=0

2. Query - Ad-Hoc Query GET - Check Debug Options
    &{request_headers}      Create Dictionary
    ...     Content-Type=application/json
    ...     EHRbase-AQL-Dry-Run=true
    ...     EHRbase-AQL-Executed-SQL=false
    ...     EHRbase-AQL-Query-Plan=true
    Set Test Variable       ${query3}
    ...     SELECT c/uid/value FROM EHR e[ehr_id/value='${ehr_id}'] CONTAINS COMPOSITION c
    Send Ad Hoc Request Through GET     q_param=${query3}   req_headers=${request_headers}
    Length Should Be    ${resp_body_columns}    ${1}
    Length Should Be    ${resp_body['rows']}    ${0}
    Should Be Equal As Strings      ${resp_body_query}      ${query3}
    Should Be Equal As Strings      ${resp_body['meta']['_executed_aql']}
    ...     ${query3}
    Dictionary Should Not Contain Key   ${resp_body['meta']}    executed_sql

2.a Query - Ad-Hoc Query GET - Query Params - Check Debug Options
    &{request_headers}      Create Dictionary
    ...     Content-Type=application/json
    ...     EHRbase-AQL-Dry-Run=true
    ...     EHRbase-AQL-Executed-SQL=true
    ...     EHRbase-AQL-Query-Plan=false
    Set Test Variable       ${query3}
    ...     SELECT c/uid/value FROM EHR e CONTAINS COMPOSITION c WHERE e/ehr_id/value = \$ehr_id
    ${parameter_obj}    Set Variable    {"ehr_id": \"${ehr_id}\" }
    Set Test Variable      ${test_data}    {"q":"${query3}","query_parameters":${parameter_obj}}
    Send Ad Hoc Request     aql_body=${test_data}   req_headers=${request_headers}
    Length Should Be    ${resp_body_columns}    ${1}
    Length Should Be    ${resp_body['rows']}    ${0}
    Should Be Equal As Strings      ${resp_body_query}      ${query3}
    Should Be Equal As Strings      ${resp_body['meta']['_executed_aql']}
    ...     SELECT c/uid/value FROM EHR e[ehr_id/value='${ehr_id}'] CONTAINS COMPOSITION c
    Dictionary Should Not Contain Key   ${resp_body['meta']}    query_plan

2.b Query - Ad-Hoc Query GET - Query Params - Limit And Offset - Check Debug Options
    &{request_headers}      Create Dictionary
    ...     Content-Type=application/json
    ...     EHRbase-AQL-Dry-Run=true
    ...     EHRbase-AQL-Executed-SQL=true
    ...     EHRbase-AQL-Query-Plan=true
    Set Test Variable       ${query3}
    ...     SELECT c/uid/value FROM EHR e CONTAINS COMPOSITION c WHERE e/ehr_id/value = \$ehr_id
    ${parameter_obj}    Set Variable    {"ehr_id": \"${ehr_id}\" }
    Set Test Variable       ${test_data}    {"q":"${query3}","query_parameters":${parameter_obj}, "offset": 1, "fetch": 1}
    Send Ad Hoc Request     aql_body=${test_data}   req_headers=${request_headers}
    Should Be Equal As Strings      ${resp_body_query}      ${query3}
    Should Be Equal As Strings      ${resp_body['meta']['_executed_aql']}
    ...     SELECT c/uid/value FROM EHR e[ehr_id/value='${ehr_id}'] CONTAINS COMPOSITION c LIMIT 1 OFFSET 1
    Check Reponse Body      columns_nr=1         rows_nr=0

3. Query - Stored Query GET By Qualified Query Name And Version - Check Debug Options
    &{request_headers}      Create Dictionary
    ...     Content-Type=application/json
    ...     EHRbase-AQL-Dry-Run=true
    ...     EHRbase-AQL-Executed-SQL=true
    ...     EHRbase-AQL-Query-Plan=true
    Set Test Variable      ${query3}
    ...     SELECT c/uid/value, o/uid/value, p/time/value FROM COMPOSITION c[uid/value='${compo_id}::${system_id_with_tenant}::1'] CONTAINS OBSERVATION o CONTAINS POINT_EVENT p
    ${resp_qualified_query_name_version}     PUT /definition/query/{qualified_query_name}/{version}
    ...     query_to_store=${query3}     format=text
    GET /query/{qualified_query_name}
    ...     qualif_name=${resp_qualified_query_name_version}    req_headers=${request_headers}
    Set Test Variable   ${resp_body}    ${resp}
    Should Be Equal As Strings      ${resp_body['meta']['_executed_aql']}      ${query3}
    Set Test Variable   ${resp_body_columns}    ${resp_body['columns']}
    Check Reponse Body      columns_nr=3         rows_nr=0

4. Query - Stored Query POST By Qualified Query Name - Check Debug Options
    [Documentation]     - *Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
    &{request_headers}      Create Dictionary
    ...     Content-Type=application/json
    ...     EHRbase-AQL-Dry-Run=true
    ...     EHRbase-AQL-Executed-SQL=true
    ...     EHRbase-AQL-Query-Plan=true
    Set Test Variable      ${query4}
    ...     SELECT o/uid/value, p/time/value FROM COMPOSITION c[uid/value='${compo_id}::${system_id_with_tenant}::1'] CONTAINS OBSERVATION o CONTAINS POINT_EVENT p
    ${resp_qualified_query_name}     PUT /definition/query/{qualified_query_name}
    ...     query_to_store=${query4}     format=text
    ${resp_query}       POST /query/{qualified_query_name}
    ...     qualif_name=${resp_qualified_query_name}    req_headers=${request_headers}
    Set Test Variable   ${resp_body}    ${resp}
    Length Should Be    ${resp_body['columns']}     ${2}
    Should Be Equal As Strings      ${resp_body['meta']['_executed_aql']}      ${query4}
    Set Test Variable   ${resp_body_columns}    ${resp_body['columns']}
    Check Reponse Body      columns_nr=2        rows_nr=0
    [Teardown]      Admin Delete EHR For AQL


*** Keywords ***
Precondition
    Upload OPT For AQL      aql-conformance-ehrbase.org.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      aql-conformance-ehrbase.org.v0_contains.json
    Set Suite Variable       ${compo_id}     ${composition_short_uid}

Check Reponse Body
    [Arguments]     ${columns_nr}=0   ${rows_nr}=0  ${dry_run}=true
    Length Should Be    ${resp_body_columns}    ${columns_nr}
    ${rows_count}   Get Length      ${resp_body['rows']}
    Length Should Be    ${resp_body['rows']}    ${rows_nr}
    Dictionary Should Contain Key   ${resp_body['meta']}    _executed_aql
    Run Keyword If      '${dry_run}' == 'true'
    ...     Dictionary Should Contain Key   ${resp_body['meta']}    dry_run
    Dictionary Should Contain Key   ${resp_body['meta']}    executed_sql
    Dictionary Should Contain Key   ${resp_body['meta']}    query_plan
    Set Test Variable   ${executed_aql}     ${resp_body['meta']['_executed_aql']}
    #Should Be Equal     ${resp_body['meta']['resultsize']}      ${rows_count}
