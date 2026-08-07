# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
HA graceful switchover with IPoE subscribers over a VXLAN pseudowire
headend. Both BNGs share an anycast VTEP on the access bridge; the
blaster's leaf stitch pins the VTEP to the SRG virtual MAC, which the
SRG plugin holds on eth1 only while ACTIVE - so encapsulated frames are
processed by exactly one node, and the standby drops them at
ethernet-input. The headend MAC is pinned to the same vMAC on both
nodes so the subscriber gateway MAC never changes. Sessions sync to the
standby, restore onto pw-an1 subinterfaces on promotion (name-based
resolution), and forwarding through the surviving node's own tunnel is
verified with VPP ping before and after switchover.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot

Suite Setup         Deploy PWHE HA Topology
Suite Teardown      Destroy PWHE HA Topology

*** Variables ***
${lab-name}         osvbng-ha-pwhe-ipoe
${lab-file}         ${CURDIR}/45-ha-pwhe-ipoe.clab.yml
${bng1}             clab-${lab-name}-bng1
${bng2}             clab-${lab-name}-bng2
${subscribers}      clab-${lab-name}-subscribers
${session-count}    2

*** Test Cases ***
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

Verify Pseudowire Bound On Both Nodes
    [Documentation]    Identical tunnel + headend config must be live on
    ...    both nodes so the standby's dataplane is ready pre-promotion.
    ${output} =    Execute VPP Command    ${bng1}    show osvbng tunnel
    Should Contain    ${output}    vxlan-an1 -> pw-an1
    ${output} =    Execute VPP Command    ${bng2}    show osvbng tunnel
    Should Contain    ${output}    vxlan-an1 -> pw-an1

Establish Sessions Over Pseudowire On Active
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}

Verify Standby Received Session Sync
    Wait Until Keyword Succeeds    15 x    2s
    ...    Check Sync Sequence Nonzero    ${bng2}

Verify Forwarding On Active Before Switchover
    ${ips} =    Get Session IPv4 Addresses    ${bng1}
    FOR    ${ip}    IN    @{ips}
        ${output} =    Execute VPP Command    ${bng1}    ping ${ip} source loop100 repeat 5
        Should Match Regexp    ${output}    [1-5] received    no ping replies from ${ip} on bng1
    END

Trigger Graceful Switchover
    Exec osvbng API    ${bng1}    /api/exec/ha/switchover

Verify bng1 Is Now STANDBY
    Wait Until Keyword Succeeds    20 x    5s
    ...    Check HA Status    ${bng1}    STANDBY

Verify bng2 Is Now ACTIVE
    Wait Until Keyword Succeeds    20 x    5s
    ...    Check HA Status    ${bng2}    ACTIVE

Verify Sessions Restored On bng2
    Wait Until Keyword Succeeds    30 x    2s
    ...    Check Session Count On BNG    ${bng2}    ${session-count}

Verify Forwarding On New Active After Switchover
    [Documentation]    The decisive check: subscriber traffic now rides
    ...    bng2's OWN tunnel and headend. Ping through the overlay from
    ...    the new active proves the full dataplane failover.
    ${ips} =    Get Session IPv4 Addresses    ${bng2}
    FOR    ${ip}    IN    @{ips}
        ${output} =    Execute VPP Command    ${bng2}    ping ${ip} source loop100 repeat 5
        Should Match Regexp    ${output}    [1-5] received    no ping replies from ${ip} on bng2 after switchover
    END

Verify Switchover Was Hitless
    Verify No Session Flaps    ${subscribers}

*** Keywords ***
Deploy PWHE HA Topology
    ${rc} =    Run And Return Rc    sudo ip link add access-sw-pwhe type bridge
    ${rc} =    Run And Return Rc    sudo ip link set access-sw-pwhe up
    Deploy Topology    ${lab-file}

Destroy PWHE HA Topology
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}
    Run And Return Rc    sudo ip link del access-sw-pwhe

Exec osvbng API
    [Arguments]    ${container}    ${path}
    ${ip} =    Get Container IPv4    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    wget -qO- http://${ip}:${OSVBNG_API_PORT}${path} --post-data='' 2>/dev/null
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${output}

Check HA Status
    [Arguments]    ${container}    ${expected_state}
    ${output} =    Get osvbng API Response    ${container}    /api/show/ha/status
    Should Contain    ${output}    ${expected_state}

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
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); entries=d.get('data') or []; print(len(entries))"
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${count} >= ${expected}    Session count ${count} < expected ${expected}

Get Session IPv4 Addresses
    [Arguments]    ${container}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${script} =    Catenate    SEPARATOR=${SPACE}
    ...    import json,os;
    ...    s=json.loads(os.environ['JSON']).get('data') or [];
    ...    print('\\n'.join(sorted(x['IPv4Address'] for x in s if x.get('IPv4Address') and x['IPv4Address']!='<nil>')))
    ${result} =    Run Process    python3    -c    ${script}
    ...    env:JSON=${output}    stderr=STDOUT
    Should Be Equal As Integers    ${result.rc}    0
    @{ips} =    Split To Lines    ${result.stdout}
    RETURN    @{ips}
