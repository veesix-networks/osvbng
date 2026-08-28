# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
CGNAT PPPoE + PBA smoke test.
Verifies PPPoE subscribers get shared address space IPs (100.64.x.x),
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
${lab-name}         osvbng-cgnat-pppoe-pba
${lab-file}         ${CURDIR}/10-cgnat-pppoe-pba.clab.yml
${bng1}             clab-${lab-name}-bng1
${corerouter1}      clab-${lab-name}-corerouter1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    5
${stream-count}     2
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
    [Documentation]    Both UDP and a raw-TCP CGNAT stream must verify
    ...                bidirectionally. Expected flows = session-count ×
    ...                stream-count; the keyword doubles for both directions.
    ${expected} =    Evaluate    ${session-count} * ${stream-count}
    Wait Until Keyword Succeeds    6 x    10s
    ...    Verify Stream Traffic Flowing    ${subscribers}    expected_flows=${expected}

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

Verify cgnat-in2out Runs After sv-reass And The MSS Clamp
    [Documentation]    Translated packets continue the ip4-unicast arc, so the
    ...    clamp's position relative to cgnat-in2out must come from
    ...    cgnat-in2out's own constraints, not from plugin load order.
    ${ifname} =    Get PPPoE Session Interface    ${bng1}
    ${output} =    Execute VPP Command    ${bng1}    show interface features ${ifname}
    ${features} =    Evaluate    [l.strip() for l in $output.split('ip4-unicast:')[1].split('\\n\\n')[0].splitlines() if l.strip()]
    Log    ip4-unicast on ${ifname}: ${features}
    ${reass} =    Evaluate    $features.index('ip4-sv-reassembly-feature')
    ${clamp} =    Evaluate    $features.index('tcp-mss-clamping-ip4-in')
    ${cgnat} =    Evaluate    $features.index('cgnat-in2out')
    Should Be True    ${reass} < ${cgnat}    ip4-unicast order on ${ifname}: ${features}
    Should Be True    ${clamp} < ${cgnat}    ip4-unicast order on ${ifname}: ${features}

Verify Translated TCP SYN Carries The Clamped MSS On The Core Side
    [Documentation]    bngblaster's TCP stack advertises MSS 1024 and the group
    ...    clamps to 960, so a SYN captured on the core side tells a clamped
    ...    packet from one that left the arc before the clamp.
    ${handle} =    Start Process
    ...    sudo docker exec -i ${corerouter1} python3 - tcp-syn eth1 20 80 < ${CURDIR}/../capture.py
    ...    shell=True
    Sleep    1s
    BNG Blaster CLI Command    ${subscribers}    http-clients-start
    ${result} =    Wait For Process    ${handle}    timeout=30s    on_timeout=kill
    Log    ${result.stdout}
    Should Be Equal As Integers    ${result.rc}    0    no TCP SYN to port 80 seen on the core side
    Should Contain    ${result.stdout}    mss=960

*** Keywords ***
Session Dump Has Active Flows
    [Arguments]    ${bng}
    ${output} =    Get osvbng API Response    ${bng}    /api/show/cgnat/sessions
    Should Contain    ${output}    "sessions"
    Should Contain    ${output}    "total"
    Should Contain    ${output}    100.64.
    Should Contain    ${output}    203.0.113.

Get PPPoE Session Interface
    [Arguments]    ${bng}
    ${output} =    Execute VPP Command    ${bng}    show interface
    ${matches} =    Get Regexp Matches    ${output}    pppoe_session\\d+
    Should Not Be Empty    ${matches}    no pppoe_session interface in VPP
    RETURN    ${matches}[0]

Deploy CGNAT Topology
    Deploy Topology    ${lab-file}

Teardown CGNAT Topology
    Run Keyword And Ignore Error    Dump VPP Trace    ${bng1}
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}
