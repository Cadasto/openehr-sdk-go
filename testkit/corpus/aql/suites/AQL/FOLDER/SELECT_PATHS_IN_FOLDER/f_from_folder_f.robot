*** Settings ***
Documentation   CHECK SELECT PATHS IN FOLDER
...         - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FOLDER.md#select-paths-in-folder
...         - *Precondition:*
...         - 1. Create EHR;
...         - 2. Create Directory with folder_details.json;
...         - Send AQL 'SELECT f FROM FOLDER f' query and compare response body with expected file content.
...         - *Postcondition:* Delete EHR using ADMIN endpoint.
Resource        ../../../_resources/keywords/aql_keywords.robot

Suite Setup     Precondition
Suite Teardown  Admin Delete EHR For AQL


*** Variables ***
${q}   SELECT f FROM FOLDER f


*** Test Cases ***
1. SELECT f FROM FOLDER f
    Set Test Variable   ${query}    ${q}
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     2
    Dictionary Should Contain Key     ${resp_body['rows'][0][0]}     name
    Dictionary Should Contain Key     ${resp_body['rows'][0][0]}     archetype_node_id
    Dictionary Should Contain Key     ${resp_body['rows'][0][0]}     details
    Dictionary Should Contain Key     ${resp_body['rows'][0][0]}     uid
    Should Be Equal As Strings      ${resp_body['rows'][0][0]['_type']}    FOLDER
    Should Be Equal As Strings      ${resp_body['rows'][1][0]['_type']}    FOLDER


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Create EHR For AQL
    Create Directory For AQL    folder_details.json   has_robot_vars=${FALSE}
