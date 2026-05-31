# Copyright 2026 The osvbng Authors
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

Verify CGNAT Pool Has Allocations
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/cgnat/pools
    Should Contain    ${output}    residential

Verify CGNAT Mappings Exist
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/cgnat/mappings
    Should Contain    ${output}    203.0.113.

Verify NAT Traffic Flowing
    Wait Until Keyword Succeeds    6 x    10s
    ...    Verify Stream Traffic Flowing    ${subscribers}    expected_flows=${session-count}

Verify CGNAT Session Dump Lists Active Translations
    Wait Until Keyword Succeeds    6 x    10s
    ...    Session Dump Has Active Flows    ${bng1}

Verify CGNAT Session Filter Narrows By Inside IP
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/cgnat/sessions?inside-ip=100.64.200.200
    Should Contain    ${output}    "sessions"
    Should Not Contain    ${output}    203.0.113.

Verify BNG Blaster Sessions Established
    Wait Until Keyword Succeeds    6 x    10s
    ...    All Sessions Ready    ${bng1}    ${subscribers}    ${session-count}

Verify Outside Addresses Advertised Via BGP
    ${output} =    Execute Vtysh On Router    ${corerouter1}    show ip bgp
    Should Contain    ${output}    203.0.113.

Verify No CGNAT Wrong-Worker Drops
    [Documentation]    cgnat-wrong-worker increments only when the handoff
    ...                hash routes a packet to a worker that doesn't own
    ...                the session — a correctness bug in the multi-worker
    ...                amendment. Must stay at zero under any worker count.
    ${rc}    ${non_zero} =    Run And Return Rc And Output
    ...    sudo docker exec ${bng1} vppctl -s ${VPPCTL_SOCK} show errors | awk '/Session owned by a different/ { if ($1+0 > 0) print "NON_ZERO:" $0 }'
    Should Be Equal As Integers    ${rc}    0
    Should Not Contain    ${non_zero}    NON_ZERO
    ...    cgnat-wrong-worker counter is non-zero — handoff routed packets to a worker that doesn't own the session

Verify CGNAT Handoff Nodes Are Firing
    [Documentation]    Proves the in2out and out2in worker-handoff nodes
    ...                are actually receiving traffic. Zero Calls means the
    ...                DPO / feature wiring bypasses the handoff path
    ...                entirely — the amendment is dormant.
    Verify VPP Node Calls Non-Zero    ${bng1}    cgnat-in2out-worker-handoff
    Verify VPP Node Calls Non-Zero    ${bng1}    cgnat-out2in-worker-handoff

*** Keywords ***
Session Dump Has Active Flows
    [Arguments]    ${bng}
    ${output} =    Get osvbng API Response    ${bng}    /api/show/cgnat/sessions
    Should Contain    ${output}    "sessions"
    Should Contain    ${output}    "total"
    Should Contain    ${output}    100.64.
    Should Contain    ${output}    203.0.113.

Deploy CGNAT Topology
    Deploy Topology    ${lab-file}

Teardown CGNAT Topology
    Run Keyword And Ignore Error    Dump VPP Trace    ${bng1}
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}
