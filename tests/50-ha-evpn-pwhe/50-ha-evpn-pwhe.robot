# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
HA graceful switchover with EVPN-discovered transports and route-driven
failover. Both BNGs advertise VNI 10101 from the same anycast VTEP
loopback; the leaf (independent FRR EVPN VTEP) sees one logical VTEP
and encapsulates toward the anycast /32, which only the ACTIVE injects
into BGP via SRG networks. Both BNGs discover the leaf's VTEP through
EVPN and hold fully programmed tunnels + bound headends, so the standby
is dataplane-ready before promotion. Switchover = the old active
withdraws the /32 and the new active advertises it; the leaf's routing
reconverges and encap steers to the new node - no pinned neighbors, no
GARP, no shared L2. Sessions sync and restore name-based onto the
peer's pw subinterfaces, and forwarding is verified through each node's
own tunnel before and after.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot

Suite Setup         Deploy Topology    ${lab-file}
Suite Teardown      Teardown EVPN HA Test

*** Variables ***
${lab-name}         osvbng-ha-evpn-pwhe
${lab-file}         ${CURDIR}/50-ha-evpn-pwhe.clab.yml
${bng1}             clab-${lab-name}-bng1
${bng2}             clab-${lab-name}-bng2
${leaf}             clab-${lab-name}-leaf
${subscribers}      clab-${lab-name}-subscribers
${session-count}    2
${anycast-vtep}     10.254.1.1
${leaf-vtep}        10.254.2.1
${bng1-underlay}    10.98.1.1
${bng2-underlay}    10.98.2.1

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

Verify Leaf Sees One Logical VTEP From Both Nodes
    [Documentation]    Both BNGs advertise VNI 10101 with the same
    ...    anycast VTEP; the leaf's remote VTEP list for the VNI is
    ...    exactly the anycast address.
    Wait Until Keyword Succeeds    60s    5s
    ...    Leaf Remote VTEP Is Anycast

Verify Both Nodes Discovered The Leaf VTEP
    [Documentation]    EVPN discovery programs a live tunnel + bound
    ...    headend on BOTH nodes, so the standby's dataplane is ready
    ...    pre-promotion.
    Wait Until Keyword Succeeds    60s    5s
    ...    Tunnel And Binding Present    ${bng1}
    Wait Until Keyword Succeeds    60s    5s
    ...    Tunnel And Binding Present    ${bng2}

Verify Leaf Routes Anycast VTEP Via Active
    [Documentation]    Only the ACTIVE injects the anycast /32; the
    ...    leaf's underlay route must point at bng1.
    Wait Until Keyword Succeeds    30s    3s
    ...    Leaf Routes Anycast Via    ${bng1-underlay}

Establish Sessions Over EVPN Transport On Active
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

Verify Leaf Rerouted Anycast VTEP To New Active
    [Documentation]    The steering mechanism itself: the /32 moved via
    ...    BGP withdraw/advertise and the leaf's encap now targets bng2.
    Wait Until Keyword Succeeds    30s    3s
    ...    Leaf Routes Anycast Via    ${bng2-underlay}

Verify Sessions Restored On bng2
    Wait Until Keyword Succeeds    30 x    2s
    ...    Check Session Count On BNG    ${bng2}    ${session-count}

Verify Forwarding On New Active After Switchover
    [Documentation]    The decisive check: subscriber traffic rides
    ...    bng2's own EVPN-discovered tunnel and headend after the
    ...    fabric reconverged.
    ${ips} =    Get Session IPv4 Addresses    ${bng2}
    FOR    ${ip}    IN    @{ips}
        ${output} =    Execute VPP Command    ${bng2}    ping ${ip} source loop100 repeat 5
        Should Match Regexp    ${output}    [1-5] received    no ping replies from ${ip} on bng2 after switchover
    END

Verify Switchover Was Hitless
    Verify No Session Flaps    ${subscribers}

*** Keywords ***
Teardown EVPN HA Test
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}

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

Leaf Remote VTEP Is Anycast
    ${output} =    Execute Vtysh On Router    ${leaf}    show evpn vni 10101 json
    Should Contain    ${output}    "${anycast-vtep}"

Tunnel And Binding Present
    [Arguments]    ${container}
    ${output} =    Execute VPP Command    ${container}    show vxlan tunnel
    Should Contain    ${output}    vni 10101
    Should Contain    ${output}    dst ${leaf-vtep}
    ${output} =    Execute VPP Command    ${container}    show osvbng tunnel
    Should Contain    ${output}    vxlan-an1 -> pw-an1

Leaf Routes Anycast Via
    [Arguments]    ${nexthop}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${leaf} ip route get ${anycast-vtep}
    Should Be Equal As Integers    ${rc}    0
    Should Contain    ${output}    via ${nexthop}

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
