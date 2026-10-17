*** Settings ***
Documentation   CHECK FOLDER FIND BY SPECIFIC ITEM ID - VERSIONED_COMPOSITION - HIER_OBJECT_ID
...         - Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FOLDER.md#find-by-specific-item-id---versioned_composition---hier_object_id
...         - *Precondition:* 1. Create EHR; 2. Create Directory with folder_details.json;
...         - Send AQL query and compare response body with expected file content.
...         - *Postcondition:* Delete EHR using ADMIN endpoint.
Resource        ../../../_resources/keywords/aql_keywords.robot

Suite Setup     Precondition
Suite Teardown  Admin Delete EHR For AQL


*** Test Cases ***
Find By Specific Item Id: SELECT f/uid/value, f/name/value, f/items/id/value FROM FOLDER f WHERE f/items/id/value = '7c0a9df0-564f-4f34-8e65-92586c64ef56'
    ${query}    Set Variable    SELECT f/uid/value, f/name/value, f/items/id/value FROM FOLDER f WHERE f/items/id/value = '7c0a9df0-564f-4f34-8e65-92586c64ef56'
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     1
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/folder/expected_folder_find_by_item_id.json
    ${exclude_paths}    Create List    root['meta']     root['q']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}      exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}
    ...		ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Create EHR For AQL
    Create Directory For AQL    folder_details.json