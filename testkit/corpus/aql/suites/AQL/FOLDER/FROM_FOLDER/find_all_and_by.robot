*** Settings ***
Documentation   CHECK AQL RESPONSE ON FOLDER FROM
...         - *Precondition:* 1. Create EHR; 2. Create Directory with folder_simple_hierarchy.json;
...         - Send AQL query and compare response body with expected file content.
...         - *Postcondition:* Delete EHR using ADMIN endpoint.
Resource        ../../../_resources/keywords/aql_keywords.robot

Suite Setup     Precondition        #enable this keyword if AQL checks are passing !!!
Suite Teardown  Admin Delete EHR For AQL       #enable this keyword if AQL checks are passing !!!


*** Test Cases ***
Find All: SELECT f/uid/value, f/name/value, f/archetype_node_id FROM FOLDER f
    [Documentation]
    ...     Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FOLDER.md#find-all
    ${query}    Set Variable    SELECT f/uid/value, f/name/value, f/archetype_node_id FROM FOLDER f
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     3
    ${expected_result_file}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/folder/expected_find_all.json
    ${exclude_paths}    Create List    root['meta']     root['q']
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}      exclude_paths=${exclude_paths}
    ...     ignore_order=${TRUE}
    ...		ignore_string_case=${TRUE}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!

Find By Archetype: SELECT f/uid/value FROM FOLDER f[openEHR-EHR-FOLDER.episode_of_care.v1]
    [Documentation]
    ...     Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FOLDER.md#find-by-archetype
    ${query}    Set Variable    SELECT f/uid/value FROM FOLDER f[openEHR-EHR-FOLDER.episode_of_care.v1]
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     1
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/folder/expected_find_by_archetype.json
    ${exclude_paths}    Create List    root['meta']     root['q']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}      exclude_paths=${exclude_paths}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!

Find By Name: SELECT f/uid/value FROM FOLDER f WHERE f/name/value = 'root1'
    [Documentation]
    ...     Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FOLDER.md#find-by-name
    ${query}    Set Variable    SELECT f/uid/value FROM FOLDER f WHERE f/name/value = 'root1'
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     1
    ${expected_result_file}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/folder/expected_find_by_name_root1.json
    ${exclude_paths}    Create List    root['meta']     root['q']
    ${expected_json}    Get File And Replace Dynamic Vars In File And Store As String
    ...     test_data_file=${expected_result_file}
    ${diff}     compare json-strings
    ...     ${resp_body_actual}     ${expected_json}      exclude_paths=${exclude_paths}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!

Find By Name: SELECT f/uid/value FROM FOLDER f WHERE f/name/value = 'subfolder1'
    [Documentation]
    ...     Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FOLDER.md#find-by-name
    ${query}    Set Variable    SELECT f/uid/value FROM FOLDER f WHERE f/name/value = 'subfolder1'
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     1
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/folder/expected_find_by_name_subfolder1.json
    ${exclude_paths}    Create List    root['meta']     root['q']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}      exclude_paths=${exclude_paths}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!

Find By Name: SELECT f/uid/value FROM FOLDER f WHERE f/name/value = 'subsubfolder1'
    [Documentation]
    ...     Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FOLDER.md#find-by-name
    ${query}    Set Variable    SELECT f/uid/value FROM FOLDER f WHERE f/name/value = 'subsubfolder1'
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     1
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/folder/expected_find_by_name_subsubfolder1.json
    ${exclude_paths}    Create List    root['meta']     root['q']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}      exclude_paths=${exclude_paths}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!

Find By Folder Id: SELECT f/name/value, f/uid/value FROM FOLDER f WHERE f/uid/value = '0cc504b1-4d6d-4cd5-81d9-0ef1b870edb3'
    [Documentation]
    ...     Covers: https://github.com/ehrbase/conformance-testing-documentation/blob/main/FOLDER.md#find-by-folder-id
    ${query}    Set Variable
    ...     SELECT f/name/value, f/uid/value FROM FOLDER f WHERE f/uid/value = '0cc504b1-4d6d-4cd5-81d9-0ef1b870edb3'
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}     1
    ${expected_result}      Set Variable    ${EXPECTED_JSON_DATA_SETS}/folder/expected_find_by_folder_id.json
    ${exclude_paths}    Create List    root['meta']     root['q']
    ${diff}     compare json-string with json-file
    ...     ${resp_body_actual}     ${expected_result}      exclude_paths=${exclude_paths}
    Should Be Empty    ${diff}    msg=DIFF DETECTED!


*** Keywords ***
Precondition
    Set Library Search Order For Tests
    Create EHR For AQL
    Create Directory For AQL    folder_simple_hierarchy.json
