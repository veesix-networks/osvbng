# Copyright 2025 Veesix Networks Ltd
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
HA smoke test suite for osvbng two-node deployment.
Verifies SRG election, subscriber session establishment, session sync
replication to standby, DHCP release sync (deletes), graceful switchover,
and session re-establishment on the new active node.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot

Suite Setup         Deploy HA Topology
Suite Teardown      Destroy HA Topology

*** Variables ***
${lab-name}         osvbng-smoke-ha
${lab-file}         ${CURDIR}/02-smoke-ha.clab.yml
${bng1}             clab-${lab-name}-bng1
${bng2}             clab-${lab-name}-bng2
${corerouter1}      clab-${lab-name}-corerouter1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    5

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

Establish Subscriber Sessions On bng1
    [Documentation]    Start BNG Blaster via eth1 and wait for sessions on bng1 (ACTIVE).
    Start BNG Blaster In Background    ${subscribers}    config=/config/bng1.json
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}

Verify Sessions Have IPv4 On bng1
    [Documentation]    All sessions on bng1 have an IPv4 address assigned.
    Verify Sessions Have IPv4    ${bng1}

Verify Session Sync Updates On ACTIVE
    [Documentation]    Verify sync status on bng1 shows updates from session events.
    Wait Until Keyword Succeeds    30 x    2s
    ...    Check Sync Updates    ${bng1}    ${session-count}

Verify Session Sync Received On STANDBY
    [Documentation]    Verify bng2 received the synced sessions (sequence > 0).
    Wait Until Keyword Succeeds    30 x    2s
    ...    Check Sync Sequence Nonzero    ${bng2}

Release Subscriber Sessions
    [Documentation]    Stop BNG Blaster which sends DHCP release for all sessions.
    Stop BNG Blaster    ${subscribers}

Verify Session Sync Deletes On ACTIVE
    [Documentation]    Verify sync status on bng1 shows deletes matching session count.
    Wait Until Keyword Succeeds    30 x    2s
    ...    Check Sync Deletes    ${bng1}    ${session-count}

Verify No Sessions Remain On bng1
    [Documentation]    Verify bng1 has no active sessions after release.
    Wait Until Keyword Succeeds    15 x    2s
    ...    Verify Sessions In API    ${bng1}    0

Trigger Switchover
    [Documentation]    Request graceful switchover so bng2 becomes ACTIVE.
    Exec osvbng API    ${bng1}    /api/exec/ha/switchover

Verify bng2 Is Now ACTIVE
    [Documentation]    Verify bng2 became ACTIVE after switchover.
    Wait Until Keyword Succeeds    20 x    5s
    ...    Check HA Status    ${bng2}    ACTIVE

Verify bng1 Is Now STANDBY
    [Documentation]    Verify bng1 became STANDBY after switchover.
    Wait Until Keyword Succeeds    20 x    5s
    ...    Check HA Status    ${bng1}    STANDBY

Establish Subscriber Sessions On bng2
    [Documentation]    Start BNG Blaster via eth2 and wait for sessions on bng2 (new ACTIVE).
    Start BNG Blaster In Background    ${subscribers}    config=/config/bng2.json
    Wait For Sessions Established    ${bng2}    ${subscribers}    ${session-count}

Verify Sessions Have IPv4 On bng2
    [Documentation]    All sessions on bng2 have an IPv4 address assigned.
    Verify Sessions Have IPv4    ${bng2}

*** Keywords ***
Deploy HA Topology
    Deploy Topology    ${lab-file}

Destroy HA Topology
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
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

Check Sync Updates
    [Arguments]    ${container}    ${expected}
    ${output} =    Get osvbng API Response    ${container}    /api/show/ha/sync
    ${rc}    ${updates} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); print(sum(e.get('updates',0) for e in d.get('data',[])))"
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${updates} >= ${expected}    Sync updates ${updates} < expected ${expected}

Check Sync Deletes
    [Arguments]    ${container}    ${expected}
    ${output} =    Get osvbng API Response    ${container}    /api/show/ha/sync
    ${rc}    ${deletes} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); print(sum(e.get('deletes',0) for e in d.get('data',[])))"
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${deletes} >= ${expected}    Sync deletes ${deletes} < expected ${expected}

Check Sync Sequence Nonzero
    [Arguments]    ${container}
    ${output} =    Get osvbng API Response    ${container}    /api/show/ha/sync
    ${rc}    ${seq} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); print(sum(e.get('last_sync_seq',0) for e in d.get('data',[])))"
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${seq} > 0    Standby sync sequence is 0, no sessions received

Exec osvbng API
    [Arguments]    ${container}    ${path}
    ${ip} =    Get Container IPv4    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    curl -sf -X POST http://${ip}:${OSVBNG_API_PORT}${path}
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${output}
