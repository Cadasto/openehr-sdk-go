# Copyright (c) 2023 Vladislav Ploaia (Vitagroup - CDR Core Team).
#
# This file is part of Project EHRbase
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


*** Settings ***
Metadata    Author    *Vladislav Ploaia*

Documentation   AQL TESTS TEST SUITE
...             \nBased on:
...             https://vitagroup-ag.atlassian.net/wiki/spaces/PEN/pages/38216361/Architecture+-+AQL+Feature+List
...             \nTest pack based on:
...             https://vitagroup-ag.atlassian.net/wiki/spaces/PEN/pages/37720514/Existing+tests+for+AQL+engine#Proposed-structure-for-AQL-Robot-tests
...             \nGithub repository with the majority of tests:
...             https://github.com/ehrbase/conformance-testing-documentation

Resource        ${EXECDIR}/robot/_resources/suite_settings.robot

Force Tags      AQL_TESTS_PACKAGE