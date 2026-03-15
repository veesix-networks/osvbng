# Copyright 2026 Veesix Networks Ltd
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
CGNAT IPoE + PBA smoke test.
Verifies subscribers get shared address space IPs (100.64.x.x),
CGNAT translates to outside addresses (203.0.113.x),
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
${lab-name}         osvbng-cgnat-ipoe-pba
${lab-file}         ${CURDIR}/08-cgnat-ipoe-pba.clab.yml
${bng1}             clab-${lab-name}-bng1
${corerouter1}      clab-${lab-name}-corerouter1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    5

*** Test Cases ***
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

Establish Subscriber Sessions
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}

Verify Sessions Have IPv4 In Shared Address Space
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    Should Contain    ${output}    100.64.

Verify CGNAT Pool Has Allocations
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/cgnat/pools
    Should Contain    ${output}    residential

Verify CGNAT Mappings Exist
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/cgnat/sessions
    Should Contain    ${output}    203.0.113.

Verify Traffic Flowing
    Wait Until Keyword Succeeds    6 x    10s
    ...    Verify Traffic Flowing    ${subscribers}    expected_flows=${session-count}

Verify Outside Addresses Advertised Via BGP
    ${output} =    Execute Vtysh On Router    ${corerouter1}    show ip bgp
    Should Contain    ${output}    203.0.113.

*** Keywords ***
Deploy CGNAT Topology
    Deploy Topology    ${lab-file}
    Wait For osvbng Healthy    bng1    ${lab-name}

Teardown CGNAT Topology
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}
