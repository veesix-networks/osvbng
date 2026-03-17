# Copyright 2026 Veesix Networks Ltd
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
HA failover test with RADIUS authentication.
Validates that RADIUS-assigned pool attribute (Framed-Pool) is preserved
across failover. RADIUS returns "radius-pool" (100.64.0.0/16) which overrides
the subscriber group default pool "default-pool" (10.255.0.0/16).
After failover, restored sessions must still have 100.64.x.x addresses.

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
${lab-name}         osvbng-ha-failover-radius-ipoe
${lab-file}         ${CURDIR}/16-ha-failover-radius-ipoe.clab.yml
${bng1}             clab-${lab-name}-bng1
${bng2}             clab-${lab-name}-bng2
${corerouter1}      clab-${lab-name}-corerouter1
${freeradius}       clab-${lab-name}-freeradius
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

Verify FreeRADIUS Is Ready
    Wait Until Keyword Succeeds    30 x    2s
    ...    Check FreeRADIUS Listening    ${freeradius}

# --- Phase 2: Establish Sessions with RADIUS ---

Establish Subscriber Sessions On bng1
    Start BNG Blaster In Background    ${subscribers}    config=/config/subscribers.json
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}

Verify RADIUS Auth Accepts
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/aaa/radius/servers
    ${rc}    ${accepts} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); s=d.get('data',[]); print(sum(x.get('auth_accepts',0) for x in s))"
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${accepts} >= ${session-count}    Expected at least ${session-count} RADIUS accepts but got ${accepts}

Verify Sessions Use RADIUS Pool On bng1
    [Documentation]    Sessions must have IPs from radius-pool (100.64.x.x), NOT default-pool (10.255.x.x)
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    Should Contain    ${output}    100.64.
    Should Not Contain    ${output}    10.255.

Verify Traffic Flowing On bng1
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

Verify Restored Sessions Still Use RADIUS Pool
    [Documentation]    After failover, restored sessions must still have 100.64.x.x (from RADIUS), not 10.255.x.x (default)
    ${output} =    Get osvbng API Response    ${bng2}    /api/show/subscriber/sessions
    Should Contain    ${output}    100.64.
    Should Not Contain    ${output}    10.255.

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

Check Session Count On BNG
    [Arguments]    ${container}    ${expected}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); entries=d.get('data',[]); print(len(entries))"
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${count} >= ${expected}    Session count ${count} < expected ${expected}

Check FreeRADIUS Listening
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    docker exec ${container} cat /var/log/freeradius/radius.log
    Should Be Equal As Integers    ${rc}    0
    Should Contain    ${output}    Ready to process requests

Force Switchover
    [Arguments]    ${container}
    ${ip} =    Get Container IPv4    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    wget -qO- http://${ip}:${OSVBNG_API_PORT}/api/exec/ha/switchover --post-data='{"force":true}' --header='Content-Type:application/json' 2>/dev/null
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${output}
