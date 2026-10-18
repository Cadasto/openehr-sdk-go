*** Settings ***
Documentation   _Robot tests to check meta JSON object from AQL response._
...         ${\n}Based on specs from:
...         - 1. https://vitagroup-ag.atlassian.net/browse/CDR-1198
...         - 2. https://vitagroup-ag.atlassian.net/browse/CDR-1199
...         ${\n}*Covers 7 cases*:
...         - _1. POST {ehrbase_url}/ehrbase/rest/openehr/v1/query/aql_
...         - _2. GET {ehrbase_url}/ehrbase/rest/openehr/v1/query/{qualified_query_name}_
...         - _3. GET {ehrbase_url}/ehrbase/rest/openehr/v1/query/aql?q={query}_
...         - _4. GET {ehrbase_url}/ehrbase/rest/openehr/v1/query/aql?q={query} with limit and offset query params_
...         - _5. POST {ehrbase_url}/ehrbase/rest/openehr/v1/query/{qualified_query_name}_
...         - _6. GET {ehrbase_url}/ehrbase/rest/openehr/v1/query/{qualified_query_name}/{version}_
...         - _7. POST {ehrbase_url}/ehrbase/rest/openehr/v1/query/{qualified_query_name}/{version}_
...         ${\n}*Checks done*:
...         - meta object has the following keys (POST): _created, _executed_aql, _schema_version, _type
...         - meta object has the following keys (GET): _created, _executed_aql, _href, _schema_version, _type
...         - _created value has ISO 8601 format
...         - _type value is RESULTSET
...         - _executed_aql value is the same as value from q key (in AQL response)
...         - _href value
...         ${\n}*Postcondition:* Delete EHR using ADMIN endpoint. This is deleting compositions linked to EHR.
Resource        ../../_resources/keywords/aql_keywords.robot
Resource        ../../_resources/keywords/aql_query_keywords.robot

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Admin Delete EHR For AQL       #enable this keyword if AQL checks are passing !!!


*** Variables ***
@{expected_meta_keys_post}      _created    _dry_run    _executed_aql   _generator  _schema_version     _type   resultsize
@{expected_meta_keys_get}       _created    _dry_run    _executed_aql   _generator   _href   _schema_version     _type   resultsize
@{expected_meta_keys_get_fetch_offset}       _created    _dry_run   _executed_aql  _generator  _href   _schema_version     _type   fetch  offset   resultsize
@{expected_meta_keys_post_fetch_offset}      _created    _dry_run   _executed_aql  _generator  _schema_version     _type   fetch  offset  resultsize
${query1}   SELECT o FROM SECTION[openEHR-EHR-SECTION.conformance_section.v0] CONTAINS OBSERVATION o CONTAINS CLUSTER
${query2}   SELECT o FROM SECTION CONTAINS OBSERVATION o CONTAINS CLUSTER
${query3}   SELECT o FROM SECTION[openEHR-EHR-SECTION.conformance_section.v0] CONTAINS OBSERVATION o
${query4}   SELECT o FROM SECTION CONTAINS OBSERVATION o
${query5}   SELECT o FROM SECTION[openEHR-EHR-SECTION.conformance_section.v0] CONTAINS OBSERVATION o CONTAINS CLUSTER LIMIT 1 OFFSET 1
${query6}   SELECT o FROM SECTION[openEHR-EHR-SECTION.conformance_section.v0] CONTAINS OBSERVATION o LIMIT 1 OFFSET 1
${query7}   SELECT o FROM SECTION CONTAINS OBSERVATION o LIMIT 1
${query8}   SELECT o FROM SECTION[openEHR-EHR-SECTION.conformance_section.v0] CONTAINS OBSERVATION o LIMIT 1


*** Test Cases ***
1. Execute Ad Hoc Query With POST And Check Meta JSON Object
    [Documentation]     *Query*: ${query1}
    ...     \n*Method and endpoint:* POST http://{base_url}/ehrbase/rest/openehr/v1/query/aql
    ...     \n*Expect:* \nmeta object content: ${expected_meta_keys_post}
    ...     \nRows items length = value from meta.resultsize
    ...     \nmeta._type = RESULTSET
    Execute Query   ${query1}
    Set Test Variable       ${expected_meta_keys}   ${expected_meta_keys_post}
    Meta JSON Object Checks
    Dictionary Should Not Contain Key   ${meta_obj}     _href
    Dictionary Should Not Contain Key   ${meta_obj}     fetch
    Dictionary Should Not Contain Key   ${meta_obj}     offset

1.a Execute Ad Hoc Query With POST LIMIT 1 OFFSET 1 In Query And Check Meta JSON Object
    [Documentation]     *Query*: ${query5}
    ...     ${\n}*Method and endpoint:* POST http://{base_url}/ehrbase/rest/openehr/v1/query/aql
    ...     \n*Expect:* \nmeta object content: ${expected_meta_keys_post_fetch_offset}
    ...     \nRows items length = value from meta.resultsize
    ...     \nmeta._type = RESULTSET
    ...     \nmeta.fetch = 1 \nmeta.offset = 1
    Execute Query   ${query5}
    Set Test Variable       ${expected_meta_keys}   ${expected_meta_keys_post_fetch_offset}
    Meta JSON Object Checks
    Dictionary Should Not Contain Key   ${meta_obj}     _href
    Should Be Equal As Strings      ${meta_obj['fetch']}    1
    Should Be Equal As Strings      ${meta_obj['offset']}   1

1.b Execute Ad Hoc Query With POST LIMIT 1 In Query And Check Meta JSON Object
    [Documentation]     *Query*: ${query7}
    ...     ${\n}*Method and endpoint:* POST http://{base_url}/ehrbase/rest/openehr/v1/query/aql
    ...     \n*Expect:* \nmeta object content: ${expected_meta_keys_post_fetch_offset}
    ...     \nRows items length = value from meta.resultsize
    ...     \nmeta._type = RESULTSET
    ...     \nmeta.fetch = 1 \nmeta.offset = 0
    Execute Query   ${query7}
    Set Test Variable       ${expected_meta_keys}   ${expected_meta_keys_post_fetch_offset}
    Meta JSON Object Checks
    Dictionary Should Not Contain Key   ${meta_obj}     _href
    Should Be Equal As Strings      ${meta_obj['fetch']}    1
    Should Be Equal As Strings      ${meta_obj['offset']}   0

2. Execute Stored Query With GET And Check Meta JSON Object
    [Documentation]     *Query*: ${query2}
    ...     ${\n}*Method and endpoint:* GET http://{base_url}/ehrbase/rest/openehr/v1/query/{qualified_query_name}
    ...     \n*Expect:* \nmeta object content: ${expected_meta_keys_get}
    ...     \nRows items length = value from meta.resultsize
    ...     \nmeta._type = RESULTSET
    ...     \nmeta._href = ${BASEURL}/query/{resp_qualified_query_name}
    ${resp_qualified_query_name}     PUT /definition/query/{qualified_query_name}
    ...     query_to_store=${query2}     format=text
    GET /query/{qualified_query_name}
    ...     qualif_name=${resp_qualified_query_name}
    Set Test Variable       ${resp_body_actual}     ${resp}
    Set Test Variable       ${expected_meta_keys}   ${expected_meta_keys_get}
    Meta JSON Object Checks
    Should Be Equal As Strings      ${meta_obj['_href']}
    ...     ${BASEURL}/query/${resp_qualified_query_name}

3. Execute Ad Hoc Query With GET And Check Meta JSON Object
    [Documentation]     *Query*: ${query3}
    ...     ${\n}*Method and endpoint:* GET http://{base_url}/ehrbase/rest/openehr/v1/query/aql?q=${query3}
    ...     \n*Expect:* \nmeta object content: ${expected_meta_keys_get}
    ...     \nRows items length = value from meta.resultsize
    ...     \nmeta._type = RESULTSET
    ...     \nmeta._href = ${BASEURL}/query/aql
    Set Test Variable       ${payload}      ${query3}
    GET /query/aql?q={query}
    Set Test Variable       ${resp_body_actual}     ${response.json()}
    Set Test Variable       ${expected_meta_keys}   ${expected_meta_keys_get}
    Meta JSON Object Checks
    Should Be Equal As Strings      ${meta_obj['_href']}
    ...     ${BASEURL}/query/aql
    Dictionary Should Not Contain Key   ${meta_obj}     fetch
    Dictionary Should Not Contain Key   ${meta_obj}     offset

3.a Execute Ad Hoc Query With GET LIMIT 1 OFFSET 1 In Query And Check Meta JSON Object
    [Documentation]     *Query*: ${query6}
    ...     ${\n}*Method and endpoint:* GET http://{base_url}/ehrbase/rest/openehr/v1/query/aql?q=${query6}
    ...     \n*Expect:* \nmeta object content: ${expected_meta_keys_get_fetch_offset}
    ...     \nRows items length = value from meta.resultsize
    ...     \nmeta._type = RESULTSET
    ...     \nmeta._href = ${BASEURL}/query/aql
    ...     \nmeta.fetch = 1 \nmeta.offset = 1
    Set Test Variable       ${payload}      ${query6}
    GET /query/aql?q={query}
    Set Test Variable       ${resp_body_actual}     ${response.json()}
    Set Test Variable       ${expected_meta_keys}   ${expected_meta_keys_get_fetch_offset}
    Meta JSON Object Checks
    Should Be Equal As Strings      ${meta_obj['_href']}
    ...     ${BASEURL}/query/aql
    Should Be Equal As Strings      ${meta_obj['fetch']}    1
    Should Be Equal As Strings      ${meta_obj['offset']}   1

3.b Execute Ad Hoc Query With GET LIMIT 1 In Query And Check Meta JSON Object
    [Documentation]     *Query*: ${query8}
    ...     ${\n}*Method and endpoint:* GET http://{base_url}/ehrbase/rest/openehr/v1/query/aql?q=${query8}
    ...     \n*Expect:* \nmeta object content: ${expected_meta_keys_get_fetch_offset}
    ...     \nRows items length = value from meta.resultsize
    ...     \nmeta._type = RESULTSET
    ...     \nmeta._href = ${BASEURL}/query/aql
    ...     \nmeta.fetch = 1 \nmeta.offset = 0
    Set Test Variable       ${payload}      ${query8}
    GET /query/aql?q={query}
    Set Test Variable       ${resp_body_actual}     ${response.json()}
    Set Test Variable       ${expected_meta_keys}   ${expected_meta_keys_get_fetch_offset}
    Meta JSON Object Checks
    Should Be Equal As Strings      ${meta_obj['_href']}
    ...     ${BASEURL}/query/aql
    Should Be Equal As Strings      ${meta_obj['fetch']}    1
    Should Be Equal As Strings      ${meta_obj['offset']}   0

4. Execute Ad Hoc Query With GET Limit 1 As Parameter And Check Meta JSON Object
    [Documentation]     *Query*: ${query3}
    ...     ${\n}*Method and endpoint:* GET http://{base_url}/ehrbase/rest/openehr/v1/query/aql?q={query3}&limit=1
    ...     \n*Expect:* \nmeta object content: ${expected_meta_keys_get}
    ...     \nRows items length = value from meta.resultsize
    ...     \nmeta._type = RESULTSET
    ...     \nmeta._href = ${BASEURL}/query/aql
    ...     \nfetch, offset must not be present in meta object
    &{params}       Create Dictionary       q=${query3}   limit=1
    GET /query/aql With Params    params=${params}
    Set Test Variable       ${resp_body_actual}     ${response.json()}
    Set Test Variable       ${expected_meta_keys}   ${expected_meta_keys_get}
    Meta JSON Object Checks
    Should Be Equal As Strings      ${meta_obj['_href']}
    ...     ${BASEURL}/query/aql
    Dictionary Should Not Contain Key   ${meta_obj}     fetch
    Dictionary Should Not Contain Key   ${meta_obj}     offset

5. Execute Stored Query With POST And Check Meta JSON Object
    [Documentation]     *Query*: ${query4}
    ...     ${\n}*Method and endpoint:* POST http://{base_url}/ehrbase/rest/openehr/v1/query/{qualified_query_name}
    ...     \n*Expect:* \nmeta object content: ${expected_meta_keys_post}
    ...     \nRows items length = value from meta.resultsize
    ...     \nmeta._type = RESULTSET
    ...     \nfetch, offset must not be present in meta object
    ${resp_qualified_query_name}     PUT /definition/query/{qualified_query_name}
    ...     query_to_store=${query4}     format=text
    POST /query/{qualified_query_name}      qualif_name=${resp_qualified_query_name}
    Set Test Variable       ${resp_body_actual}     ${resp}
    Set Test Variable       ${expected_meta_keys}   ${expected_meta_keys_post}
    Meta JSON Object Checks
    Dictionary Should Not Contain Key   ${meta_obj}     fetch
    Dictionary Should Not Contain Key   ${meta_obj}     offset

6. Execute Stored Query With GET Qualified Name And Version And Check Meta JSON Object
    [Documentation]     *Query*: ${query4}
    ...     ${\n}*Method and endpoint:* GET http://{base_url}/ehrbase/rest/openehr/v1/query/{qualified_query_name}/{version}
    ...     \n*Expect:* \nmeta object content: ${expected_meta_keys_get}
    ...     \nRows items length = value from meta.resultsize
    ...     \nmeta._type = RESULTSET
    ...     \nmeta._href = ${BASEURL}/query/{resp_qualified_query_name_version}
    ...     \nfetch, offset must not be present in meta object
    ${resp_qualified_query_name_version}     PUT /definition/query/{qualified_query_name}/{version}
    ...     query_to_store=${query4}     format=text
    GET /query/{qualified_query_name}/{version}     qualif_name=${resp_qualified_query_name_version}
    Set Test Variable       ${resp_body_actual}     ${resp}
    Set Test Variable       ${expected_meta_keys}   ${expected_meta_keys_get}
    Meta JSON Object Checks
    Should Be Equal As Strings      ${meta_obj['_href']}
    ...     ${BASEURL}/query/${resp_qualified_query_name_version}
    Dictionary Should Not Contain Key   ${meta_obj}     fetch
    Dictionary Should Not Contain Key   ${meta_obj}     offset

6.a Execute Stored Query With GET Qualified Name And Version And Check Meta JSON Object
    [Documentation]     *Query*: ${query4}
    ...     ${\n}*Method and endpoint:* GET http://{base_url}/ehrbase/rest/openehr/v1/query/{qualified_query_name}/{version}
    ...     \n*Expect:* \nmeta object content: ${expected_meta_keys_get_fetch_offset}
    ...     \nRows items length = value from meta.resultsize
    ...     \nmeta._type = RESULTSET
    ...     \nmeta._href = ${BASEURL}/query/{resp_qualified_query_name_version}
    ...     \nmeta.fetch = 1 \nmeta.offset = 1
    ${resp_qualified_query_name_version}     PUT /definition/query/{qualified_query_name}/{version}
    ...     query_to_store=${query4}     format=text
    &{params}   Create Dictionary   offset=1    fetch=1
    GET /query/{qualified_query_name}/{version}     qualif_name=${resp_qualified_query_name_version}
    ...     params=${params}
    Set Test Variable       ${resp_body_actual}     ${resp}
    Set Test Variable       ${expected_meta_keys}   ${expected_meta_keys_get_fetch_offset}
    Run Keyword And Return Status   Meta JSON Object Checks
    Should Be Equal As Strings      ${meta_obj['_href']}
    ...     ${BASEURL}/query/${resp_qualified_query_name_version}
    Should Be Equal As Strings      ${meta_obj['fetch']}    1
    Should Be Equal As Strings      ${meta_obj['offset']}   1

7. Execute Stored Query With POST Qualified Name And Version And Check Meta JSON Object
    [Documentation]     *Query*: ${query4}
    ...     ${\n}*Method and endpoint:* POST http://{base_url}/ehrbase/rest/openehr/v1/query/{qualified_query_name}/{version}
    ...     \n*Expect:* \nmeta object content: ${expected_meta_keys_post}
    ...     \nRows items length = value from meta.resultsize
    ...     \nmeta._type = RESULTSET
    ${resp_qualified_query_name_version}     PUT /definition/query/{qualified_query_name}/{version}
    ...     query_to_store=${query4}     format=text
    POST /query/{qualified_query_name}/{version}    qualif_name=${resp_qualified_query_name_version}
    Set Test Variable       ${resp_body_actual}     ${resp}
    Set Test Variable       ${expected_meta_keys}   ${expected_meta_keys_post}
    Meta JSON Object Checks


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      aql-conformance-ehrbase.org.v0.opt
    Create EHR For AQL
    Commit Composition For AQL      aql-conformance-ehrbase.org.v0_contains.json

Execute Query
    [Arguments]     ${query}
    Set AQL And Execute Ad Hoc Query    ${query}

Created Timestamp Checks
    [Arguments]     ${timestamp}
    @{timestamp_list}     Split String      ${timestamp}     T
    ${date_format}     Set Variable     ^\\d{4}-\\d{2}-\\d{2}
    Should Match Regexp     ${timestamp_list}[0]    ${date_format}
    @{time_list}    Split String    ${timestamp_list}[1]    .
    ${time_format}      Set Variable    ^\\d{2}:\\d{2}:\\d{2}
    Should Match Regexp     ${time_list}[0]    ${time_format}

Meta JSON Object Checks
    ${rows_length}      Get Length      ${resp_body_actual["rows"]}
    Set Test Variable   ${rows_length}  ${rows_length}
    Set Test Variable   ${meta_obj}     ${resp_body_actual['meta']}
    Log     ${meta_obj}
    ${dict_keys}    Get Dictionary Keys     ${meta_obj}
    Run Keyword And Return Status   Lists Should Be Equal   ${expected_meta_keys}   ${dict_keys}
    Should Be Equal As Strings      ${meta_obj["_type"]}    RESULTSET
    Should Contain      ${meta_obj["_executed_aql"]}   SELECT
    Should Be Equal As Strings      ${meta_obj["resultsize"]}    ${rows_length}
    Should Contain                  ${meta_obj["_generator"]}    EHRBase/
    @{splitted_generator}       Split String     ${meta_obj["_generator"]}      /
    Log     EHRBase version: ${splitted_generator}[1]
    Created Timestamp Checks        ${meta_obj["_created"]}

GET /query/aql With Params
    [Documentation]     Executes HTTP method GET on /query/aql?{params} endpoint
    [Arguments]     ${params}
    ${headers}      Create Dictionary
    ...     content=application/json
    ...     accept=application/json
    #&{query_dict}       Create Dictionary       q=${query}      limit=1
    ${resp}         Get On Session      ${SUT}      /query/aql      params=${params}
                    ...     headers=${headers}      expected_status=anything
                    Should Be Equal As Strings      ${resp.status_code}    ${200}
                    Set Test Variable   ${response}    ${resp}