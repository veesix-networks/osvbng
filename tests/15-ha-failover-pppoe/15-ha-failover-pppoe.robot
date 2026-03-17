# Copyright 2026 Veesix Networks Ltd
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
HA seamless failover test for PPPoE with CGNAT.
Establishes PPPoE sessions on ACTIVE, verifies sync, hard-kills the active BNG,
and validates that sessions and CGNAT mappings are restored on the new active
without PPPoE renegotiation. Traffic streams are NOT stopped before failover.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot

Suite Setup         Deploy Failover Topology
Suite Teardown      Destroy Failover Topology

*** Variables ***
${lab-name}         osvbng-ha-failover-pppoe
${lab-file}         ${CURDIR}/15-ha-failover-pppoe.clab.yml
${bng1}             clab-${lab-name}-bng1
${bng2}             clab-${lab-name}-bng2
${corerouter1}      clab-${lab-name}-corerouter1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    5

*** Test Cases ***
# --- Phase 1: Bootstrap ---

Verify bng1 Is Healthy
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify bng2 Is Healthy
    Wait For osvbng Healthy    bng2    ${lab-name}

Verify bng1 Is ACTIVE
    Wait Until Keyword Succeeds    20 x    5s
    ...    Check HA Status    ${bng1}    ACTIVE

Verify bng2 Is STANDBY
    Wait Until Keyword Succeeds    20 x    5s
    ...    Check HA Status    ${bng2}    STANDBY

Verify CGNAT Plugin Loaded On bng1
    ${output} =    Execute VPP Command    ${bng1}    show plugins
    Should Contain    ${output}    osvbng_cgnat

Verify CGNAT Plugin Loaded On bng2
    ${output} =    Execute VPP Command    ${bng2}    show plugins
    Should Contain    ${output}    osvbng_cgnat

Verify OSPF Adjacency For bng1
    Wait Until Keyword Succeeds    12 x    10s
    ...    Check OSPF Neighbor On Router    ${corerouter1}    10.254.0.1

Verify OSPF Adjacency For bng2
    Wait Until Keyword Succeeds    12 x    10s
    ...    Check OSPF Neighbor On Router    ${corerouter1}    10.254.0.3

Verify BGP Session For bng1
    Wait Until Keyword Succeeds    12 x    10s
    ...    Verify BGP Session On Router    ${corerouter1}    10.254.0.1

Verify BGP Session For bng2
    Wait Until Keyword Succeeds    12 x    10s
    ...    Verify BGP Session On Router    ${corerouter1}    10.254.0.3

# --- Phase 2: Establish Sessions and Verify Sync ---

Establish PPPoE Sessions On bng1
    Start BNG Blaster In Background    ${subscribers}    config=/config/subscribers.json
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}

Verify Sessions Have IPv4 In Shared Address Space
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    Should Contain    ${output}    100.64.

Verify CGNAT Mappings Exist On bng1
    Wait Until Keyword Succeeds    12 x    5s
    ...    Check CGNAT Mapping Count    ${bng1}    ${session-count}

Verify NAT Traffic Flowing On bng1
    Wait Until Keyword Succeeds    12 x    10s
    ...    Verify Stream Traffic Flowing    ${subscribers}    expected_flows=${session-count}

Verify Session Sync Updates On ACTIVE
    Wait Until Keyword Succeeds    30 x    2s
    ...    Check Sync Updates    ${bng1}    ${session-count}

Verify Session Sync Received On STANDBY
    Wait Until Keyword Succeeds    30 x    2s
    ...    Check Sync Sequence Nonzero    ${bng2}

# --- Phase 3: Hard Failover ---

Kill Active BNG
    ${rc} =    Run And Return Rc    docker kill ${bng1}
    Should Be Equal As Integers    ${rc}    0

Verify bng2 Detects Peer Loss
    Wait Until Keyword Succeeds    20 x    5s
    ...    Check HA Status    ${bng2}    STANDBY_ALONE

Force Promote bng2 To Active
    Force Switchover    ${bng2}

Verify bng2 Is Now Active
    Wait Until Keyword Succeeds    20 x    5s
    ...    Check HA Status    ${bng2}    ACTIVE

Verify Sessions Restored On bng2
    Wait Until Keyword Succeeds    30 x    2s
    ...    Check Session Count On BNG    ${bng2}    ${session-count}

Verify CGNAT Mappings Restored On bng2
    Wait Until Keyword Succeeds    30 x    2s
    ...    Check CGNAT Mapping Count    ${bng2}    ${session-count}

Verify Traffic Recovers After Failover
    Wait Until Keyword Succeeds    30 x    5s
    ...    Verify Stream Traffic Flowing    ${subscribers}    expected_flows=${session-count}

*** Keywords ***
Deploy Failover Topology
    Create Access Bridge
    Deploy Topology    ${lab-file}

Destroy Failover Topology
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}
    Destroy Access Bridge

Create Access Bridge
    ${rc} =    Run And Return Rc    sudo ip link add access-sw type bridge
    ${rc} =    Run And Return Rc    sudo ip link set access-sw up

Destroy Access Bridge
    Run And Return Rc    sudo ip link del access-sw

Check HA Status
    [Arguments]    ${container}    ${expected_state}
    ${output} =    Get osvbng API Response    ${container}    /api/show/ha/status
    Should Contain    ${output}    ${expected_state}

Check OSPF Neighbor On Router
    [Arguments]    ${container}    ${neighbor_rid}
    ${output} =    Execute Vtysh On Router    ${container}    show ip ospf neighbor
    Should Contain    ${output}    ${neighbor_rid}
    Should Contain    ${output}    Full

Check Sync Updates
    [Arguments]    ${container}    ${expected}
    ${output} =    Get osvbng API Response    ${container}    /api/show/ha/sync
    ${rc}    ${updates} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); print(sum(e.get('updates',0) for e in d.get('data',[])))"
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${updates} >= ${expected}    Sync updates ${updates} < expected ${expected}

Check Sync Sequence Nonzero
    [Arguments]    ${container}
    ${output} =    Get osvbng API Response    ${container}    /api/show/ha/sync
    ${rc}    ${seq} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); print(sum(e.get('last_sync_seq',0) for e in d.get('data',[])))"
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${seq} > 0    Standby sync sequence is 0, no sessions received

Check CGNAT Mapping Count
    [Arguments]    ${container}    ${expected}
    ${output} =    Get osvbng API Response    ${container}    /api/show/cgnat/sessions
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); entries=d.get('data',[]); print(len(entries))"
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${count} >= ${expected}    CGNAT mappings ${count} < expected ${expected}

Check Session Count On BNG
    [Arguments]    ${container}    ${expected}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); entries=d.get('data',[]); print(len(entries))"
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${count} >= ${expected}    Session count ${count} < expected ${expected}

Force Switchover
    [Arguments]    ${container}
    ${ip} =    Get Container IPv4    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    wget -qO- http://${ip}:${OSVBNG_API_PORT}/api/exec/ha/switchover --post-data='{"force":true}' --header='Content-Type:application/json' 2>/dev/null
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${output}
