# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
L2GW over EVPN-discovered VXLAN NNIs. Both NNI tunnels are configured
with signaling: evpn and no remote VTEP; osvbng advertises VNIs 10101
and 10201 as EVPN type-3 routes from kernel mirror devices, learns the
leaf's VTEP from the routes the leaf originates, and programs the VPP
tunnels dynamically. The leaf is an independent EVPN implementation
(FRR + kernel vxlan dataplane), so establishment also proves EVPN
interop with a non-osvbng VTEP. On top of the discovered tunnels the
suite asserts full 40-level l2gw behavior: dynamic wholesale circuits,
bidirectional traffic with tunnel leak guards, per-circuit counters,
and restart restore including watcher re-seeding.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot
Resource            ../l2gw.robot
Resource            ../restart.robot

Suite Setup         Deploy Topology    ${lab-file}
Suite Teardown      Teardown L2GW EVPN Test

*** Variables ***
${lab-name}         osvbng-l2gw-evpn
${lab-file}         ${CURDIR}/46-l2gw-evpn.clab.yml
${bng1}             clab-${lab-name}-bng1
${bng1-mgmt-ip}     172.20.26.2
${leaf}             clab-${lab-name}-leaf
${subscribers}      clab-${lab-name}-subscribers
${session-count}    2
${leaf-vtep}        10.255.2.1

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify EVPN Advertises Local VNIs
    [Documentation]    Both mirror devices are detected by zebra and the
    ...    type-3 routes reach the leaf, proving the kernel-mirror
    ...    control plane against an independent EVPN implementation.
    Wait Until Keyword Succeeds    60s    5s
    ...    Verify Leaf Learned BNG VTEP
    ${output} =    Execute Vtysh On BNG    ${bng1}    show evpn vni
    Should Contain    ${output}    10101
    Should Contain    ${output}    10201

Verify Remote VTEP Discovery Programs Tunnels
    [Documentation]    osvbng learns the leaf VTEP from its type-3
    ...    routes and programs both VPP tunnels with the learned dst;
    ...    nothing configured the remote endpoint anywhere.
    Wait Until Keyword Succeeds    60s    5s
    ...    Verify VPP Tunnel Programmed    10101
    Wait Until Keyword Succeeds    60s    5s
    ...    Verify VPP Tunnel Programmed    10201
    ${output} =    Execute VPP Command    ${bng1}    show interface
    Should Contain    ${output}    vxlan-an1
    Should Contain    ${output}    vxlan-isp-blue
    ${output} =    Execute VPP Command    ${bng1}    show vxlan tunnel
    Should Contain    ${output}    decap-next-index

Establish Wholesale Circuits Over Discovered Tunnels
    [Documentation]    DHCP DISCOVER arrives through the discovered
    ...    access tunnel, triggers AAA, the circuit installs between
    ...    the two tunnels and the replayed DISCOVER reaches the
    ...    a10nsp side through the discovered handoff tunnel.
    Start BNG Blaster In Background    ${subscribers}
    Wait For Blaster Sessions Established    ${subscribers}    ${session-count}

Verify Circuits Installed On Tunnel Interfaces
    [Documentation]    Dynamic circuits keyed on the discovered tunnel
    ...    interfaces with egress VLANs from the group allocator ranges.
    Wait For L2GW Circuit Count    ${bng1}    ${session-count}
    Verify L2GW Circuit Field    ${bng1}    not c.get('static')
    ...    circuits must be dynamic
    Verify L2GW Circuit Field    ${bng1}    c.get('handoff_group')=='isp-blue'
    ...    handoff group must come from the RADIUS VSA
    Verify L2GW Circuit Field    ${bng1}    c.get('access_interface')=='vxlan-an1'
    ...    access side must be the evpn-signaled NNI
    Verify L2GW Circuit Field    ${bng1}    200<=c.get('handoff_svlan',0)<=204
    ...    egress S-VLAN must come from the svlan-range allocator
    Verify L2GW Circuit Field    ${bng1}    c.get('handoff_cvlan',0)==10
    ...    egress C-VLAN must come from the cvlan-range allocator

Verify No Local Termination
    [Documentation]    osvbng must not terminate DHCP for wholesale circuits;
    ...    the subscriber session table stays empty.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    ${result} =    Run Process    python3    -c
    ...    import json,os; print(len(json.loads(os.environ['JSON']).get('data') or []))
    ...    env:JSON=${output}    stderr=STDOUT
    Should Be Equal As Strings    ${result.stdout}    0    l2gw subscribers were terminated locally

Verify Session Traffic Flows Through Overlay
    [Documentation]    Bidirectional session traffic between access and
    ...    a10nsp sides across the discovered tunnels.
    Wait Until Keyword Succeeds    60s    5s
    ...    Verify Traffic Flowing    ${subscribers}    ${session-count}

Verify Traffic Rides The Discovered Tunnels
    [Documentation]    Leak guard: session traffic must transit BOTH
    ...    discovered tunnels in BOTH directions inside VPP. All four
    ...    tunnel counter directions must actively increment while
    ...    traffic flows; a path that leaks around the dataplane leaves
    ...    some direction static.
    ${an1-rx-0} =    Get VPP Interface Counter    ${bng1}    vxlan-an1    rx
    ${an1-tx-0} =    Get VPP Interface Counter    ${bng1}    vxlan-an1    tx
    ${isp-rx-0} =    Get VPP Interface Counter    ${bng1}    vxlan-isp-blue    rx
    ${isp-tx-0} =    Get VPP Interface Counter    ${bng1}    vxlan-isp-blue    tx
    Sleep    5s    Let session traffic accumulate.
    ${an1-rx-1} =    Get VPP Interface Counter    ${bng1}    vxlan-an1    rx
    ${an1-tx-1} =    Get VPP Interface Counter    ${bng1}    vxlan-an1    tx
    ${isp-rx-1} =    Get VPP Interface Counter    ${bng1}    vxlan-isp-blue    rx
    ${isp-tx-1} =    Get VPP Interface Counter    ${bng1}    vxlan-isp-blue    tx
    Should Be True    ${an1-rx-1} - ${an1-rx-0} >= 25    upstream not entering access tunnel
    Should Be True    ${isp-tx-1} - ${isp-tx-0} >= 25    upstream not leaving handoff tunnel
    Should Be True    ${isp-rx-1} - ${isp-rx-0} >= 25    downstream not entering handoff tunnel
    Should Be True    ${an1-tx-1} - ${an1-tx-0} >= 25    downstream not leaving access tunnel

Verify Dataplane Circuit Counters
    [Documentation]    Per-circuit counters count both directions across
    ...    the discovered tunnels.
    Verify VPP L2GW Circuits Counters Non-Zero    ${bng1}

Restart Survives With Circuits On Discovered Tunnels
    [Documentation]    osvbngd restart: the watcher re-seeds learned
    ...    VTEPs from the existing fdb entries (ListExisting), tunnel
    ...    programming is idempotent against the surviving VPP tunnels,
    ...    circuits re-install from opdb, traffic resumes.
    ${snapshot} =    Snapshot L2GW Circuit IDs    ${bng1}
    Restart osvbngd    ${bng1}
    Wait For osvbngd Down    ${bng1}
    Wait For osvbng Healthy    bng1    ${lab-name}
    Wait Until Keyword Succeeds    60s    5s
    ...    Verify VPP Tunnel Programmed    10101
    Wait Until Keyword Succeeds    60s    5s
    ...    Verify VPP Tunnel Programmed    10201
    Wait For L2GW Circuit Count    ${bng1}    ${session-count}
    ${restored} =    Snapshot L2GW Circuit IDs    ${bng1}
    Should Be Equal As Strings    ${restored}    ${snapshot}    circuit set changed across restart
    Reset Stream Verification    ${subscribers}
    Wait Until Keyword Succeeds    60s    5s
    ...    Verify Traffic Flowing    ${subscribers}    ${session-count}

*** Keywords ***
Teardown L2GW EVPN Test
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}

Verify Leaf Learned BNG VTEP
    ${output} =    Execute Vtysh On Router    ${leaf}    show evpn vni detail
    Should Contain    ${output}    10.255.1.1

Verify VPP Tunnel Programmed
    [Arguments]    ${vni}
    ${output} =    Execute VPP Command    ${bng1}    show vxlan tunnel
    Should Contain    ${output}    vni ${vni}
    Should Contain    ${output}    dst ${leaf-vtep}

Get VPP Interface Counter
    [Documentation]    Cumulative rx or tx packet counter of a VPP interface.
    [Arguments]    ${container}    ${iface}    ${direction}
    ${output} =    Execute VPP Command    ${container}    show interface ${iface}
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    echo '${output}' | awk '/${direction} packets/ {print $NF; found=1} END {if (!found) print 0}'
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${count}
