# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
L2GW packet-trigger suite: circuits created by the first frame of ANY
protocol, fully self-contained (no RADIUS container anywhere). The access
range runs trigger: packet with auth_provider: local; BNG Blaster runs
PPPoE sessions with the a10nsp interface acting as the retail ISP BNG.
PPPoE discovery establishing through the cross-connect proves a non-DHCP
protocol triggered the circuit, which is the whole point of the mode.
Usernames are the group-qualified VLAN tuple from the
$subscriber-group$.$svlan$.$cvlan$ policy. Circuits must survive an
osvbngd restart and must idle out via l2gw.idle-timeout once traffic
stops, the lease-substitute for packet mode.

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
Suite Teardown      Teardown L2GW Packet Trigger Test

*** Variables ***
${lab-name}         osvbng-l2gw-packet-trigger
${lab-file}         ${CURDIR}/40-l2gw-packet-trigger.clab.yml
${bng1}             clab-${lab-name}-bng1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    2
# idle-timeout 60s + 30s sweep cadence + first-sweep baseline + margin
${idle-teardown-timeout}    240s

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify L2GW Plugin Loaded
    [Documentation]    The osvbng_l2gw VPP plugin must be loaded and its CLI reachable.
    ${output} =    Execute VPP Command    ${bng1}    show osvbng l2gw circuits
    Should Not Contain    ${output}    unknown input

Establish PPPoE Sessions Through Packet Trigger
    [Documentation]    PPPoE discovery is not DHCP: sessions establishing
    ...    against the a10nsp (ISP) side proves the any-protocol trigger
    ...    created the circuits. There is no RADIUS server in this lab;
    ...    local auth plus the group default handoff group is the entire
    ...    provisioning state.
    Start BNG Blaster In Background    ${subscribers}
    Wait For Blaster Sessions Established    ${subscribers}    ${session-count}

Verify Circuits Installed With Tuple Usernames
    [Documentation]    Dynamic circuits authorized on the tuple alone, with
    ...    the group default handoff, allocator egress VLANs, and the
    ...    group-qualified VLAN tuple as username.
    Wait For L2GW Circuit Count    ${bng1}    ${session-count}
    Verify L2GW Circuit Field    ${bng1}    not c.get('static')
    ...    circuits must be dynamic
    Verify L2GW Circuit Field    ${bng1}    c.get('handoff_group')=='isp-blue'
    ...    handoff group must come from the subscriber group default
    Verify L2GW Circuit Field    ${bng1}    200<=c.get('handoff_svlan',0)<=204
    ...    egress S-VLAN must come from the svlan-range allocator
    Verify L2GW Circuit Field    ${bng1}    c.get('handoff_cvlan',0)==10
    ...    egress C-VLAN must come from the cvlan-range allocator
    Verify L2GW Circuit Field    ${bng1}    c.get('protocol')=='l2'
    ...    packet-mode circuits must record the generic l2 trigger protocol
    Verify L2GW Circuit Field    ${bng1}    c.get('username')=='an1.%d.%d'%(c.get('access_svlan'),c.get('access_cvlan',0))
    ...    username must be the group-qualified VLAN tuple
    Verify L2GW Circuit Field    ${bng1}    c.get('mac') and c.get('session_id')
    ...    dynamic circuits must carry subscriber identity

Verify No Local Termination
    [Documentation]    osvbng must not terminate anything for wholesale
    ...    circuits; the subscriber session table stays empty.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    ${result} =    Run Process    python3    -c
    ...    import json,os; print(len(json.loads(os.environ['JSON']).get('data') or []))
    ...    env:JSON=${output}    stderr=STDOUT
    Should Be Equal As Strings    ${result.stdout}    0    l2gw subscribers were terminated locally

Verify Session Traffic Flows
    [Documentation]    Bidirectional session traffic between access and a10nsp
    ...    sides through the installed circuits.
    Wait Until Keyword Succeeds    60s    5s
    ...    Verify Traffic Flowing    ${subscribers}    ${session-count}

Verify Dataplane Circuit Counters
    [Documentation]    Per-circuit counters in the l2gw plugin count both directions.
    Verify VPP L2GW Circuits Counters Non-Zero    ${bng1}

Restart Survives With Circuits Restored
    [Documentation]    osvbngd restart: circuits re-install from opdb with no
    ...    re-authentication; traffic resumes. PPPoE session state lives in
    ...    the blaster and the a10nsp side, so sessions must not flap.
    ${snapshot} =    Snapshot L2GW Circuit IDs    ${bng1}
    Restart osvbngd    ${bng1}
    Wait For osvbngd Down    ${bng1}
    Wait For osvbng Healthy    bng1    ${lab-name}
    Wait For osvbng State Ready    ${bng1}
    Wait For L2GW Circuit Count    ${bng1}    ${session-count}
    ${restored} =    Snapshot L2GW Circuit IDs    ${bng1}
    Should Be Equal As Strings    ${restored}    ${snapshot}    circuit set changed across restart
    Reset Stream Verification    ${subscribers}
    Wait Until Keyword Succeeds    60s    5s
    ...    Verify Traffic Flowing    ${subscribers}    ${session-count}

Idle Timeout Tears Down Circuits
    [Documentation]    Packet mode has no lease lifecycle; stopping all
    ...    traffic must age the circuits out via l2gw.idle-timeout (60s in
    ...    this lab), freeing their egress pairs.
    Stop BNG Blaster    ${subscribers}
    Wait For L2GW Circuit Count    ${bng1}    0    installed    ${idle-teardown-timeout}    10s

*** Keywords ***
Teardown L2GW Packet Trigger Test
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}
