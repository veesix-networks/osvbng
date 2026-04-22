# Copyright 2025 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
Smoke test suite for osvbng single-node deployment.
Verifies VPP, FRR, and osvbng are healthy after containerlab deploy,
checks OSPF/BGP adjacency with core router, and validates REST API.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot

Suite Setup         Deploy Smoke Topology
Suite Teardown      Teardown Smoke Topology

*** Variables ***
${lab-name}         osvbng-smoke
${lab-file}         ${CURDIR}/01-smoke.clab.yml
${bng1}             clab-${lab-name}-bng1
${corerouter1}      clab-${lab-name}-corerouter1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    5

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify VPP Is Running
    [Documentation]    Check VPP is running and responsive via vppctl.
    ${output} =    Execute VPP Command    ${bng1}    show version
    Should Contain    ${output}    vpp

Verify FRR Is Running On BNG
    [Documentation]    Check FRR is running inside the dataplane netns on bng1.
    ${output} =    Execute Vtysh On BNG    ${bng1}    show version
    Should Contain    ${output}    FRR

Verify OSPF Adjacency Established
    [Documentation]    Wait for OSPF adjacency between bng1 and corerouter1.
    Wait Until Keyword Succeeds    12 x    10s
    ...    Verify OSPF Adjacency On Router    ${corerouter1}

Verify BGP Session Established
    [Documentation]    Wait for BGP session from bng1 (10.254.0.1) on corerouter1.
    Wait Until Keyword Succeeds    12 x    10s
    ...    Verify BGP Session On Router    ${corerouter1}    10.254.0.1

Verify REST API Responds
    [Documentation]    Verify the osvbng REST API returns valid data.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/system/version
    Should Not Be Empty    ${output}

Verify VPP Interfaces Configured
    [Documentation]    Verify VPP has the expected interfaces.
    ${output} =    Execute VPP Command    ${bng1}    show interface
    Should Contain    ${output}    eth1
    Should Contain    ${output}    eth2

Establish Subscriber Sessions
    [Documentation]    Start BNG Blaster in background and wait for sessions.
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}

Verify Sessions Have IPv4
    [Documentation]    All sessions have an IPv4 address assigned.
    Verify Sessions Have IPv4    ${bng1}

Verify Traffic Flowing
    [Documentation]    Verify end-to-end traffic between access and network interfaces.
    Wait Until Keyword Succeeds    6 x    10s
    ...    Verify Traffic Flowing    ${subscribers}    expected_flows=${session-count}

*** Keywords ***
Deploy Smoke Topology
    Deploy Topology    ${lab-file}

Teardown Smoke Topology
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}

