# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
L2GW over VXLAN overlay suite: both NNIs arrive as VXLAN tunnels instead
of physical ports. The access operator NNI is vxlan-an1 (VNI 10101) and
the ISP handoff is vxlan-isp-blue (VNI 10201); the subscribers container
emulates the leaf stitches with linux vxlan endpoints bridged to the
blaster access / a10nsp interfaces. Decapsulated frames re-enter the
device-input feature arc via the osvbng_tunnel plugin, so the l2gw
trigger snoop and circuit switching behave exactly as on physical ports:
DHCP DISCOVER triggers RADIUS, the circuit installs between the two
tunnel sw_if_indexes, and TX encapsulates through the tunnel output
nodes with flow-hash UDP source ports. Asserts 39-level behavior:
establish, bidirectional traffic, per-circuit counters, restart restore.

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
Suite Teardown      Teardown L2GW VXLAN Test

*** Variables ***
${lab-name}         osvbng-l2gw-vxlan
${lab-file}         ${CURDIR}/40-l2gw-vxlan.clab.yml
${bng1}             clab-${lab-name}-bng1
${bng1-mgmt-ip}     172.20.21.2
${subscribers}      clab-${lab-name}-subscribers
${session-count}    2

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify Tunnel Plugin Hooked Into VXLAN Decap
    [Documentation]    The osvbng_tunnel plugin must be loaded and
    ...    registered as a next node of the vxlan input nodes.
    ${output} =    Execute VPP Command    ${bng1}    show osvbng tunnel
    Should Not Contain    ${output}    unknown input
    Should Not Contain    ${output}    vxlan plugin not loaded
    Should Contain    ${output}    vxlan4-input

Verify VXLAN Tunnel Interfaces Created
    [Documentation]    Both tunnels exist under their config names and are
    ...    admin up, proving southbound create + rename + ifMgr registration.
    ${output} =    Execute VPP Command    ${bng1}    show interface
    Should Contain    ${output}    vxlan-an1
    Should Contain    ${output}    vxlan-isp-blue
    ${output} =    Execute VPP Command    ${bng1}    show vxlan tunnel
    Should Contain    ${output}    vni 10101
    Should Contain    ${output}    vni 10201
    # default l2-input decap prints nothing; a custom decap next renders
    # as decap-next-index N, proving the tunnels point at osvbng-tunnel-input
    Should Contain    ${output}    decap-next-index

Establish Wholesale Circuits Over VXLAN
    [Documentation]    DHCP DISCOVER arrives through vxlan decap, triggers
    ...    AAA, the circuit installs between the two tunnels and the
    ...    replayed DISCOVER reaches the a10nsp side through the handoff
    ...    tunnel. Blaster sessions establishing proves the entire loop.
    Start BNG Blaster In Background    ${subscribers}
    Wait For Blaster Sessions Established    ${subscribers}    ${session-count}

Verify Circuits Installed On Tunnel Interfaces
    [Documentation]    Dynamic circuits keyed on the tunnel interfaces with
    ...    egress VLANs from the group allocator ranges.
    Wait For L2GW Circuit Count    ${bng1}    ${session-count}
    Verify L2GW Circuit Field    ${bng1}    not c.get('static')
    ...    circuits must be dynamic
    Verify L2GW Circuit Field    ${bng1}    c.get('handoff_group')=='isp-blue'
    ...    handoff group must come from the RADIUS VSA
    Verify L2GW Circuit Field    ${bng1}    c.get('access_interface')=='vxlan-an1'
    ...    access side must be the vxlan NNI
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
    ...    a10nsp sides, encapsulated on both NNIs.
    Wait Until Keyword Succeeds    60s    5s
    ...    Verify Traffic Flowing    ${subscribers}    ${session-count}

Verify Traffic Rides The Tunnels
    [Documentation]    Leak guard: session traffic must transit BOTH vxlan
    ...    tunnels in BOTH directions inside VPP. All four tunnel counter
    ...    directions must actively increment while traffic flows; a path
    ...    that leaks around the dataplane leaves some direction static.
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
    ...    the tunnels.
    Verify VPP L2GW Circuits Counters Non-Zero    ${bng1}

Restart Survives With Circuits On Tunnels
    [Documentation]    osvbngd restart: tunnels are re-resolved idempotently,
    ...    circuits re-install from opdb against the tunnel sw_if_indexes,
    ...    traffic resumes.
    ${snapshot} =    Snapshot L2GW Circuit IDs    ${bng1}
    Restart osvbngd    ${bng1}
    Wait For osvbngd Down    ${bng1}
    Wait For osvbng Healthy    bng1    ${lab-name}
    Wait For L2GW Circuit Count    ${bng1}    ${session-count}
    ${restored} =    Snapshot L2GW Circuit IDs    ${bng1}
    Should Be Equal As Strings    ${restored}    ${snapshot}    circuit set changed across restart
    Reset Stream Verification    ${subscribers}
    Wait Until Keyword Succeeds    60s    5s
    ...    Verify Traffic Flowing    ${subscribers}    ${session-count}

*** Keywords ***
Teardown L2GW VXLAN Test
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}

Get VPP Interface Counter
    [Documentation]    Cumulative rx or tx packet counter of a VPP interface.
    [Arguments]    ${container}    ${iface}    ${direction}
    ${output} =    Execute VPP Command    ${container}    show interface ${iface}
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    echo '${output}' | awk '/${direction} packets/ {print $NF; found=1} END {if (!found) print 0}'
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${count}
