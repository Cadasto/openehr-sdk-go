*** Settings ***
Metadata    Author    *Vladislav Ploaia*

Documentation   AQL DEBUG OPTIONS CHECKS
...             Test documentation: https://vitagroup-ag.atlassian.net/browse/CDR-1430
...             \n*PRECONDITION:*
...             \nSet in EHRBase ehrbase\configuration\src\main\resources\application.yml file the following values:
...             - ehrbase.rest.aql.debugging-enabled: true
...             - ehrbase.rest.aql.response.generator-details-enabled: true
...             - ehrbase.rest.aql.response.executed-aql-enabled: true
...             \n*To be executed locally only (unless there is a request to enable them in EHRBase OSS pipeline)*
#...             Robot cmd (from tests folder):
#...             robot -d results --skiponfailure not-ready -L DEBUG .\robot\AQL\SELECT\DEBUG_OPTS_CHECKS
Resource    ${CURDIR}${/}../../../_resources/suite_settings.robot


Force Tags    AQL_DEBUG_OPTS
