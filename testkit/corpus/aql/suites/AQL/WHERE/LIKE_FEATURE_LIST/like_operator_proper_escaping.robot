*** Settings ***
Documentation   WHERE - LIKE FEATURE LIST - LIKE OPERATOR PROPER ESCAPING
...             - Covers bug ticket: https://vitagroup-ag.atlassian.net/browse/CDR-1675

Resource        ../../../_resources/keywords/aql_keywords.robot

#Suite Setup  Skip    enable Setup 'Precondition' if AQL checks are passing !!!
#Suite Teardown  Skip    enable Teardown 'Admin Delete EHR For AQL' if AQL checks are passing !!!
Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Admin Delete EHR For AQL    ${ehr_id}

*** Variables ***
${global_query}     SELECT c0/content/items/activities/description/items[openEHR-EHR-CLUSTER.nested.v1]/items[at0001]/name/value AS col_name,
...                 c0/content/items/activities/description/items[openEHR-EHR-CLUSTER.nested.v1]/items[at0001]/value/value AS col_value
...                 FROM EHR e CONTAINS COMPOSITION c0 CONTAINS CLUSTER i[openEHR-EHR-CLUSTER.nested.v1]
...                 WHERE c0/content/items/activities/description/items[openEHR-EHR-CLUSTER.nested.v1]/items[at0001]/value/value


*** Test Cases ***
1. Like '* 20*'
    ${temp_query}    Catenate   ${global_query}
    ...                         LIKE '* 20*'
    ${query}    Replace Variables       ${temp_query}
    #Log     ${query}
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     1
    Should Be Equal As Strings      ${resp_body['rows'][0][0]}     text
    Should Contain                  ${resp_body['rows'][0][1]}     20\%

2. Like '* 20%*'
    ${temp_query}    Catenate   ${global_query}
    ...                         LIKE '* 20%*'
    ${query}    Replace Variables       ${temp_query}
    #Log     ${query}
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     1
    Should Be Equal As Strings      ${resp_body['rows'][0][0]}     text
    Should Contain                  ${resp_body['rows'][0][1]}     20\%

3. Like '*20\\\\%*'
    [Documentation]     Expects 400, with IllegalAqlException.
    ${temp_query}   Catenate   ${global_query}
    ...                         LIKE '*20\\\\\\\\%*'
    ${err_msg}      Run Keyword And Expect Error    *
    ...     Set AQL And Execute Ad Hoc Query    ${temp_query}
    Should Contain      ${err_msg}      400 != 200
    Should Contain      ${err_msg}      Invalid LIKE pattern: *20\\\\%*


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Upload OPT For AQL      nested.opt
    Create EHR For AQL
    Commit Composition For AQL      nested_compo_custom_dv_text.json
    Set Suite Variable       ${c_uid}       ${composition_short_uid}