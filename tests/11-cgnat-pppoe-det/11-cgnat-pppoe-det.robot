# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
CGNAT PPPoE + Deterministic NAT smoke test.
Verifies PPPoE subscribers get shared address space IPs (100.64.x.x),
CGNAT deterministic mode translates algorithmically to outside addresses,
and traffic flows through NAT end-to-end.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot

Suite Setup         Deploy CGNAT Topology
Suite Teardown      Teardown CGNAT Topology

*** Variables ***
${lab-name}         osvbng-cgnat-pppoe-det
${lab-file}         ${CURDIR}/11-cgnat-pppoe-det.clab.yml
${bng1}             clab-${lab-name}-bng1
${corerouter1}      clab-${lab-name}-corerouter1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    5
${trace-input}      af-packet-input

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify VPP Is Running
    ${output} =    Execute VPP Command    ${bng1}    show version
    Should Contain    ${output}    vpp

Verify CGNAT Plugin Loaded
    ${output} =    Execute VPP Command    ${bng1}    show plugins
    Should Contain    ${output}    osvbng_cgnat

Verify OSPF Adjacency Established
    Wait Until Keyword Succeeds    12 x    10s
    ...    Verify OSPF Adjacency On Router    ${corerouter1}

Verify BGP Session Established
    Wait Until Keyword Succeeds    12 x    10s
    ...    Verify BGP Session On Router    ${corerouter1}    10.254.0.1

Start Subscriber Sessions
    Start BNG Blaster In Background    ${subscribers}

Verify Sessions Have IPv4 In Shared Address Space
    Wait Until Keyword Succeeds    12 x    10s
    ...    Verify Sessions In API    ${bng1}    ${session-count}
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    Should Contain    ${output}    100.64.

Verify CGNAT Pool Configured
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/cgnat/pools
    Should Contain    ${output}    residential

Verify NAT Traffic Flowing
    Wait Until Keyword Succeeds    6 x    10s
    ...    Verify Stream Traffic Flowing    ${subscribers}    expected_flows=${session-count}

Verify BNG Blaster Sessions Established
    Wait Until Keyword Succeeds    6 x    10s
    ...    All Sessions Ready    ${bng1}    ${subscribers}    ${session-count}

Verify Outside Addresses Advertised Via BGP
    ${output} =    Execute Vtysh On Router    ${corerouter1}    show ip bgp
    Should Contain    ${output}    203.0.113.

*** Keywords ***
Deploy CGNAT Topology
    Deploy Topology    ${lab-file}

Teardown CGNAT Topology
    Run Keyword And Ignore Error    Dump VPP Trace    ${bng1}
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}
