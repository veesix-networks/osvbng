# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
L2TPv2 LAC end-to-end test. Topology:
   bngblaster (subscriber + LNS roles) <--eth1 PPPoE--> osvbng-LAC
                                         <--eth2 L2TP--> osvbng-LAC
Subscriber authenticates via CHAP against osvbng's local auth provider,
which returns Tunnel-* attributes pointing at the bngblaster LNS. osvbng
brings up an L2TPv2 tunnel + session to bngblaster and bridges the
subscriber's PPP frames through it.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot
Resource            ../localauth.robot

Suite Setup         Deploy Topology    ${lab-file}
Suite Teardown      Destroy Topology    ${lab-file}

*** Variables ***
${lab-name}         osvbng-l2tp-lac
${lab-file}         ${CURDIR}/31-l2tp-lac.clab.yml
${bng1}             clab-${lab-name}-bng1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    1
${lns-ipv4}         10.0.0.2
${lns-secret}       shared

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify VPP Is Running
    [Documentation]    Check VPP is running and responsive.
    ${output} =    Execute VPP Command    ${bng1}    show version
    Should Contain    ${output}    vpp

Provision Local User With Tunnel Attributes
    [Documentation]    Wholesale LAC authorization: AAA policy is
    ...                format=$agent-remote-id$ + authenticate=false, so
    ...                osvbng-LAC looks up the AAA user by the PPPoE
    ...                agent-remote-id (NOT the PPP CHAP username) and
    ...                returns Tunnel-* attributes. Real PPP auth happens
    ...                at the LNS via proxy-auth AVPs carried in ICCN.
    ...
    ...                The bngblaster config sets agent-remote-id="user1"
    ...                but PPP CHAP username="wholesale-ppp-username" —
    ...                "wholesale-ppp-username" is NOT in the local DB,
    ...                so this test only passes if osvbng truly uses
    ...                remote-id for the lookup, never the PPP username.
    Create Local Auth User    ${bng1}    user1
    ${user_id} =    Lookup Local Auth User ID    ${bng1}    user1
    Should Not Be Empty    ${user_id}
    Set Local Auth User Attribute    ${bng1}    ${user_id}    tunnel.type             L2TP
    Set Local Auth User Attribute    ${bng1}    ${user_id}    tunnel.medium-type      IPv4
    Set Local Auth User Attribute    ${bng1}    ${user_id}    tunnel.server-endpoint  ${lns-ipv4}
    Set Local Auth User Attribute    ${bng1}    ${user_id}    tunnel.password         ${lns-secret}

Establish LAC Subscriber Session
    [Documentation]    Start bngblaster (acting as both PPPoE subscriber AND
    ...                LNS). osvbng-LAC brings up the tunnel + session to
    ...                bngblaster's LNS side and bridges the PPP frames.
    ...                LAC sessions never get a local IPv4 — verification
    ...                checks the PhaseLACTunneled state instead of an IP.
    Start BNG Blaster In Background    ${subscribers}
    Wait Until Keyword Succeeds    60s    2s
    ...    LAC Session Is Tunneled    ${bng1}    ${session-count}

Verify Tunnel In osvbng API
    [Documentation]    osvbng REST API reports the session as tunneled +
    ...                points at the bngblaster LNS IP.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    Should Contain    ${output}    tunneled
    Should Contain    ${output}    ${lns-ipv4}

Verify L2TP Tunnel Established On LNS
    [Documentation]    bngblaster's LNS side reports the L2TPv2 tunnel as
    ...                Established (SCCRQ → SCCRP → SCCCN handshake completed
    ...                with Challenge-AVP auth).
    Wait Until Keyword Succeeds    30s    2s
    ...    Bngblaster L2TP Tunnel State Is    ${subscribers}    Established

Verify L2TP Session Established On LNS
    [Documentation]    bngblaster's LNS side reports the L2TPv2 session as
    ...                Established (ICRQ → ICRP → ICCN with proxy-auth AVPs
    ...                carrying the LAC's CHAP material).
    Wait Until Keyword Succeeds    30s    2s
    ...    Bngblaster L2TP Session State Is    ${subscribers}    Established

Verify Subscriber Got IP From LNS
    [Documentation]    The subscriber-side PPPoE session terminates PPP at
    ...                the LNS, so the LNS hands out the IP. Verify both
    ...                IPCP and IP6CP converge through the tunnel.
    Wait Until Keyword Succeeds    60s    2s
    ...    Bngblaster Subscriber Has IPv4    ${subscribers}

Verify L2TP Data Flowed
    [Documentation]    bngblaster's tunnel counters reflect data packets in
    ...                both directions, confirming the VPP LAC bridge
    ...                (PPPoE to l2tpv2-encap-raw and l2tpv2-input to
    ...                osvbng-pppoe-lac-tx) is forwarding PPP frames.
    ${rc}    ${output} =    BNG Blaster CLI Command    ${subscribers}    l2tp-tunnels
    Should Be Equal As Integers    ${rc}    0
    ${rc}    ${counts} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); t=d['l2tp-tunnels'][0]; print(t['data-packets-rx'], t['data-packets-tx'])"
    Should Be Equal As Integers    ${rc}    0
    @{parts} =    Split String    ${counts}
    Should Be True    ${parts}[0] > 0    Expected non-zero L2TP data-packets-rx on LNS
    Should Be True    ${parts}[1] > 0    Expected non-zero L2TP data-packets-tx on LNS

Verify Show L2TP Tunnels
    [Documentation]    osvbng's show handler returns the tunnel-level view
    ...                with local/peer IPs, state, role and bound session
    ...                count populated.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/l2tp/tunnels
    Should Contain    ${output}    Established
    Should Contain    ${output}    "Role":"LAC"
    Should Contain    ${output}    "PeerIP":"${lns-ipv4}"

Verify Subscriber Session Has L2TP Binding
    [Documentation]    A tunneled PPPoE subscriber surfaces an L2TP sub-
    ...                object with tunnel and session IDs alongside the
    ...                normal subscriber fields. Non-LAC sessions omit it.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    Should Contain    ${output}    "L2TP":{
    Should Contain    ${output}    "LocalTunnelID":1
    Should Contain    ${output}    "PeerTunnelID":1

*** Keywords ***
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
