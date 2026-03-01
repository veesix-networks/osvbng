# Copyright 2025 Veesix Networks Ltd
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
HA smoke test suite for osvbng two-node deployment.
Verifies both BNG nodes are healthy, SRG election completes correctly
(bng1 ACTIVE, bng2 STANDBY), and both have routing adjacency.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot

Suite Setup         Deploy HA Topology
Suite Teardown      Destroy HA Topology

*** Variables ***
${lab-name}         osvbng-smoke-ha
${lab-file}         ${CURDIR}/02-smoke-ha.clab.yml
${bng1}             clab-${lab-name}-bng1
${bng2}             clab-${lab-name}-bng2
${corerouter1}      clab-${lab-name}-corerouter1

*** Test Cases ***
Verify bng1 Is Healthy
    [Documentation]    Wait for osvbng REST API to respond on bng1.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify bng2 Is Healthy
    [Documentation]    Wait for osvbng REST API to respond on bng2.
    Wait For osvbng Healthy    bng2    ${lab-name}

Verify bng1 Is ACTIVE
    [Documentation]    Verify bng1 (priority 100) wins SRG election and becomes ACTIVE.
    Wait Until Keyword Succeeds    20 x    5s
    ...    Check HA Status    ${bng1}    ACTIVE

Verify bng2 Is STANDBY
    [Documentation]    Verify bng2 (priority 50) becomes STANDBY.
    Wait Until Keyword Succeeds    20 x    5s
    ...    Check HA Status    ${bng2}    STANDBY

Verify VPP Running On bng1
    [Documentation]    Check VPP is running on bng1.
    ${output} =    Execute VPP Command    ${bng1}    show version
    Should Contain    ${output}    vpp

Verify VPP Running On bng2
    [Documentation]    Check VPP is running on bng2.
    ${output} =    Execute VPP Command    ${bng2}    show version
    Should Contain    ${output}    vpp

Verify FRR Running On bng1
    [Documentation]    Check FRR is running on bng1.
    ${output} =    Execute Vtysh On BNG    ${bng1}    show version
    Should Contain    ${output}    FRR

Verify FRR Running On bng2
    [Documentation]    Check FRR is running on bng2.
    ${output} =    Execute Vtysh On BNG    ${bng2}    show version
    Should Contain    ${output}    FRR

Verify OSPF Adjacency For bng1
    [Documentation]    Wait for OSPF adjacency between bng1 and corerouter1.
    Wait Until Keyword Succeeds    12 x    10s
    ...    Check OSPF Neighbor On Router    ${corerouter1}    10.254.0.1

Verify OSPF Adjacency For bng2
    [Documentation]    Wait for OSPF adjacency between bng2 and corerouter1.
    Wait Until Keyword Succeeds    12 x    10s
    ...    Check OSPF Neighbor On Router    ${corerouter1}    10.254.0.3

Verify BGP Session For bng1
    [Documentation]    Wait for BGP session from bng1 on corerouter1.
    Wait Until Keyword Succeeds    12 x    10s
    ...    Verify BGP Session On Router    ${corerouter1}    10.254.0.1

Verify BGP Session For bng2
    [Documentation]    Wait for BGP session from bng2 on corerouter1.
    Wait Until Keyword Succeeds    12 x    10s
    ...    Verify BGP Session On Router    ${corerouter1}    10.254.0.3

*** Keywords ***
Deploy HA Topology
    Deploy Topology    ${lab-file}

Destroy HA Topology
    Destroy Topology    ${lab-file}

Check HA Status
    [Arguments]    ${container}    ${expected_state}
    ${output} =    Get osvbng API Response    ${container}    /api/show/ha/status
    Should Contain    ${output}    ${expected_state}

Check OSPF Neighbor On Router
    [Arguments]    ${container}    ${neighbor_rid}
    ${output} =    Execute Vtysh On Router    ${container}    show ip ospf neighbor
    Should Contain    ${output}    ${neighbor_rid}
    Should Contain    ${output}    Full
