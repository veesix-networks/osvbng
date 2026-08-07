# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
LAC over a VXLAN access network via a pseudowire headend. PPPoE
discovery and the whole PPP session arrive through the vxlan-an1 /
pw-an1 stitch; osvbng-LAC looks up the agent-remote-id, brings up an
L2TPv2 tunnel to bngblaster's LNS side on the physical backbone, and
bridges PPP frames between the pseudowire and the tunnel. The LNS
terminates PPP (CHAP via proxy-auth AVPs) and assigns the subscriber
address, proving the full wholesale LAC path works when the access
network is an overlay.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot
Resource            ../localauth.robot

Suite Setup         Deploy Topology    ${lab-file}
Suite Teardown      Teardown LAC PWHE Test

*** Variables ***
${lab-name}         osvbng-lac-vxlan-pwhe
${lab-file}         ${CURDIR}/44-lac-vxlan-pwhe.clab.yml
${bng1}             clab-${lab-name}-bng1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    1
${lns-ipv4}         10.0.0.2
${lns-secret}       shared

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify Pseudowire Bound To Transport
    [Documentation]    The tunnel plugin must hold the vxlan-an1 -> pw-an1
    ...    binding.
    ${output} =    Execute VPP Command    ${bng1}    show osvbng tunnel
    Should Contain    ${output}    vxlan-an1 -> pw-an1

Provision Local User With Tunnel Attributes
    [Documentation]    Wholesale LAC authorization: AAA policy is
    ...                format=$agent-remote-id$ + authenticate=false, so
    ...                osvbng-LAC looks up the AAA user by the PPPoE
    ...                agent-remote-id and returns Tunnel-* attributes.
    ...                Real PPP auth happens at the LNS via proxy-auth
    ...                AVPs carried in ICCN.
    Create Local Auth User    ${bng1}    user1
    ${user_id} =    Lookup Local Auth User ID    ${bng1}    user1
    Should Not Be Empty    ${user_id}
    Set Local Auth User Attribute    ${bng1}    ${user_id}    tunnel.type             L2TP
    Set Local Auth User Attribute    ${bng1}    ${user_id}    tunnel.medium-type      IPv4
    Set Local Auth User Attribute    ${bng1}    ${user_id}    tunnel.server-endpoint  ${lns-ipv4}
    Set Local Auth User Attribute    ${bng1}    ${user_id}    tunnel.password         ${lns-secret}

Establish LAC Subscriber Session Over Pseudowire
    [Documentation]    Start bngblaster (PPPoE subscriber behind the vxlan
    ...                stitch AND LNS on the backbone). osvbng-LAC brings
    ...                up the tunnel + session and bridges PPP frames
    ...                between pw-an1 and the LNS.
    Start BNG Blaster In Background    ${subscribers}
    Wait Until Keyword Succeeds    60s    2s
    ...    LAC Session Is Tunneled    ${bng1}    ${session-count}

Verify Tunnel In osvbng API
    [Documentation]    osvbng REST API reports the session as tunneled +
    ...                points at the bngblaster LNS IP.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    Should Contain    ${output}    tunneled
    Should Contain    ${output}    ${lns-ipv4}

Verify L2TP Tunnel And Session Established On LNS
    [Documentation]    bngblaster's LNS side reports the L2TPv2 tunnel and
    ...                session as Established.
    Wait Until Keyword Succeeds    30s    2s
    ...    Bngblaster L2TP Tunnel State Is    ${subscribers}    Established
    Wait Until Keyword Succeeds    30s    2s
    ...    Bngblaster L2TP Session State Is    ${subscribers}    Established

Verify Subscriber Got IP From LNS
    [Documentation]    PPP terminates at the LNS through the pseudowire and
    ...                the L2TP tunnel; the LNS hands out the address.
    Wait Until Keyword Succeeds    60s    2s
    ...    Bngblaster Subscriber Has IPv4    ${subscribers}

Verify L2TP Data Flowed
    [Documentation]    LNS tunnel counters show data in both directions,
    ...                confirming the VPP LAC bridge forwards PPP frames
    ...                between the pseudowire and l2tpv2.
    ${rc}    ${output} =    BNG Blaster CLI Command    ${subscribers}    l2tp-tunnels
    Should Be Equal As Integers    ${rc}    0
    ${rc}    ${counts} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); t=d['l2tp-tunnels'][0]; print(t['data-packets-rx'], t['data-packets-tx'])"
    Should Be Equal As Integers    ${rc}    0
    @{parts} =    Split String    ${counts}
    Should Be True    ${parts}[0] > 0    Expected non-zero L2TP data-packets-rx on LNS
    Should Be True    ${parts}[1] > 0    Expected non-zero L2TP data-packets-tx on LNS

Verify Traffic Rides The Access Tunnel
    [Documentation]    Leak guard: the PPP session traffic must transit the
    ...    access pseudowire in both directions inside VPP (LCP keepalives
    ...    and the LNS exchange ride it continuously).
    ${rx-0} =    Get VPP Interface Counter    ${bng1}    vxlan-an1    rx
    ${tx-0} =    Get VPP Interface Counter    ${bng1}    vxlan-an1    tx
    Sleep    12s    Let PPP keepalives accumulate.
    ${rx-1} =    Get VPP Interface Counter    ${bng1}    vxlan-an1    rx
    ${tx-1} =    Get VPP Interface Counter    ${bng1}    vxlan-an1    tx
    Should Be True    ${rx-1} - ${rx-0} >= 1    no frames entering the access tunnel
    Should Be True    ${tx-1} - ${tx-0} >= 1    no frames leaving the access tunnel

Verify Show L2TP Tunnels
    [Documentation]    osvbng's show handler returns the tunnel-level view.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/l2tp/tunnels
    Should Contain    ${output}    Established
    Should Contain    ${output}    "Role":"LAC"
    Should Contain    ${output}    "PeerIP":"${lns-ipv4}"

Verify Subscriber Session Has L2TP Binding
    [Documentation]    The tunneled subscriber surfaces the L2TP sub-object
    ...                with tunnel and session IDs.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    Should Contain    ${output}    "L2TP":{
    Should Contain    ${output}    "LocalTunnelID":1
    Should Contain    ${output}    "PeerTunnelID":1

*** Keywords ***
Teardown LAC PWHE Test
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}

LAC Session Is Tunneled
    [Arguments]    ${container}    ${expected_count}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); sessions=d.get('data') or []; tunneled=[x for x in sessions if str(x.get('State','')) == 'tunneled']; print(len(tunneled))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${result}    ${expected_count}
    ...    Expected ${expected_count} tunneled sessions but got ${result}

Bngblaster L2TP Tunnel State Is
    [Arguments]    ${container}    ${expected_state}
    ${rc}    ${output} =    BNG Blaster CLI Command    ${container}    l2tp-tunnels
    Should Be Equal As Integers    ${rc}    0
    ${rc}    ${state} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['l2tp-tunnels'][0]['state'])"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${state}    ${expected_state}

Bngblaster L2TP Session State Is
    [Arguments]    ${container}    ${expected_state}
    ${rc}    ${output} =    BNG Blaster CLI Command    ${container}    l2tp-sessions
    Should Be Equal As Integers    ${rc}    0
    ${rc}    ${state} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['l2tp-sessions'][0]['state'])"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${state}    ${expected_state}

Bngblaster Subscriber Has IPv4
    [Arguments]    ${container}    ${session_id}=1
    ${rc}    ${output} =    BNG Blaster CLI Command    ${container}    session-info session-id ${session_id}
    Should Be Equal As Integers    ${rc}    0
    ${rc}    ${ipv4} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); s=d.get('session-info',{}); print(s.get('ipv4-address',''))"
    Should Be Equal As Integers    ${rc}    0
    Should Not Be Empty    ${ipv4}    Subscriber did not receive an IPv4 from the LNS

Get VPP Interface Counter
    [Documentation]    Cumulative rx or tx packet counter of a VPP interface.
    [Arguments]    ${container}    ${iface}    ${direction}
    ${output} =    Execute VPP Command    ${container}    show interface ${iface}
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    echo '${output}' | awk '/${direction} packets/ {print $NF; found=1} END {if (!found) print 0}'
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${count}
