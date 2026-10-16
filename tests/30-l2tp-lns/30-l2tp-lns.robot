# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
L2TPv2 LNS end-to-end test. Topology:
   lac (xl2tpd + pppd)       --10.10.0.0/30--  corerouter1 (FRR)  --10.20.0.0/30--  bng1 (osvbng LNS)
   blaster (bngblaster LAC)  --10.30.0.0/30--  corerouter1

Two LACs dial the same osvbng LNS. xl2tpd opens one tunnel and pppd
brings up one PPP session over it, which keeps a real pppd in the
interop set. bngblaster opens a second tunnel as a PPPoL2TP LAC and
brings up ten PPP sessions inside it, which is the scale shape: one
UDP flow carrying many sessions. osvbng terminates LCP / CHAP / IPCP /
IPv6CP for all of them, authenticates against the local auth provider
(allow_all=true for the test), and allocates IPv4 from the LNS pool
plus IPv6 IANA and delegated prefixes from the local IPv6 pools.

Dataplane is exercised in both directions:
  - pppd: ping subscriber -> LNS gateway, BNG loopback and corerouter1
    loopback (local termination and transit)
  - bngblaster: upstream session traffic verified at its network
    interface (transit through osvbng), downstream proven by pinging
    each session from corerouter1 and reading the session's ICMP
    receive counter. bngblaster 1.0.0 builds no downstream session
    traffic and sends no echo reply for PPPoL2TP sessions, so the
    receive counter is the downstream proof.
corerouter1 has a static return route for the subscriber pool so the
reply traffic traverses osvbng-LNS back into the L2TP tunnels.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot

Suite Setup         Deploy Topology     ${lab-file}
Suite Teardown      Destroy Topology    ${lab-file}

*** Variables ***
${lab-name}                 osvbng-l2tp-lns
${lab-file}                 ${CURDIR}/30-l2tp-lns.clab.yml
${bng1}                     clab-${lab-name}-bng1
${lac}                      clab-${lab-name}-lac
${blaster}                  clab-${lab-name}-blaster
${corerouter1}              clab-${lab-name}-corerouter1
${blaster-session-count}    10
${session-count}            11
${lac-hostname}             lac
${blaster-hostname}         blaster
${lns-gateway-ipv4}         100.64.0.1
${bng1-loopback-ipv4}       10.254.0.1
${corerouter1-loop-ipv4}    10.254.0.2
${subscriber-ipv4-prefix}   100.64.0.

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify VPP Is Running
    [Documentation]    Confirm VPP is responsive on the BNG.
    ${output} =    Execute VPP Command    ${bng1}    show version
    Should Contain    ${output}    vpp

Verify L2TPv2 Plugin Loaded
    [Documentation]    L2TPv2 VPP plugin must be loaded on the BNG.
    ${output} =    Execute VPP Command    ${bng1}    show plugins
    Should Contain    ${output}    l2tpv2

Verify ppp0 Comes Up On LAC
    [Documentation]    xl2tpd auto-dials at startup. Wait for pppd to
    ...                bring ppp0 up with an IPv4 from the LNS pool.
    Wait Until Keyword Succeeds    90s    3s
    ...    LAC ppp0 Has IPv4    ${lac}

Establish BNG Blaster LAC Sessions
    [Documentation]    bngblaster dials the LNS as a PPPoL2TP LAC and
    ...                brings up ten PPP sessions in one tunnel.
    Start BNG Blaster In Background    ${blaster}
    Wait Until Keyword Succeeds    90s    3s
    ...    BNG Blaster Sessions Established    ${blaster}    ${blaster-session-count}

Verify LNS Sessions Active
    [Documentation]    osvbng reports every L2TP subscriber session active:
    ...                one from pppd and ten from bngblaster.
    Wait Until Keyword Succeeds    30s    2s
    ...    LNS Sessions Are Active    ${bng1}    ${session-count}

Verify L2TP Tunnels In osvbng API
    [Documentation]    osvbng's show l2tp tunnels reports both inbound
    ...                tunnels as Role LNS, State Established, and the
    ...                bngblaster tunnel carrying all ten sessions.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/l2tp/tunnels
    Should Contain    ${output}    "PeerHostname":"${lac-hostname}"
    Should Contain    ${output}    "PeerHostname":"${blaster-hostname}"
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); t=d.get('data') or []; print(len([x for x in t if x.get('Role')=='LNS' and x.get('State')=='Established'])); print(sum(x.get('SessionCount',0) for x in t if x.get('PeerHostname')=='${blaster-hostname}'))"
    Should Be Equal As Integers    ${rc}    0
    @{lines} =    Split To Lines    ${result}
    Should Be Equal As Strings    ${lines}[0]    2    Expected 2 established LNS tunnels: ${output}
    Should Be Equal As Strings    ${lines}[1]    ${blaster-session-count}    bngblaster tunnel session count: ${output}

Verify Subscribers Got IPs From Local Pool
    [Documentation]    osvbng-LNS allocates IPv4 from the local pool
    ...                100.64.0.0/16 for every session. The pppd
    ...                session authenticates as user1, the bngblaster
    ...                sessions as blaster1..blaster10.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    Should Contain    ${output}    "Username":"user1"
    Should Contain    ${output}    "Username":"blaster1"
    Should Contain    ${output}    "Username":"blaster${blaster-session-count}"
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); s=d.get('data') or []; print(len([x for x in s if str(x.get('IPv4Address','')).startswith('${subscriber-ipv4-prefix}')]))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${result}    ${session-count}
    ...    Expected ${session-count} sessions with an IPv4 from ${subscriber-ipv4-prefix}0/24: ${output}

Verify Per-Session VPP Interfaces
    [Documentation]    DECAP_IP creates a per-session vnet interface in
    ...                VPP, set unnumbered to the gateway loopback, with
    ...                tunnel-output as the L3 output node. One per
    ...                session across both tunnels.
    ${output} =    Execute VPP Command    ${bng1}    show l2tpv2 session
    Should Contain    ${output}    mode ip
    ${addr_out} =    Execute VPP Command    ${bng1}    show interface addr
    Should Contain    ${addr_out}    l2tpv2_session0
    Should Contain    ${addr_out}    unnumbered, use loop100
    ${count} =    Get Count    ${addr_out}    unnumbered, use loop100
    Should Be Equal As Integers    ${count}    ${session-count}
    ...    Expected ${session-count} unnumbered session interfaces: ${addr_out}

Verify Dataplane Ping LNS Gateway
    [Documentation]    pppd subscriber pings the LNS-side gateway IP (the
    ...                address shared via unnumbered loop100). Exercises
    ...                local-termination at osvbng: L2TP decap,
    ...                ip4-input, ip4-local, echo-reply, midchain
    ...                encap, eth1.
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${lac} ping -c 3 -W 2 -I ppp0 ${lns-gateway-ipv4}
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0    Ping to ${lns-gateway-ipv4} failed: ${output}

Verify Dataplane Ping BNG Loopback
    [Documentation]    pppd subscriber pings the BNG control-plane
    ...                loopback (10.254.0.1, advertised via OSPF). Same
    ...                local-termination path as the LNS gateway test
    ...                but exercises a different FIB entry.
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${lac} ping -c 3 -W 2 -I ppp0 ${bng1-loopback-ipv4}
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0    Ping to ${bng1-loopback-ipv4} failed: ${output}

Verify Dataplane Transit Through BNG
    [Documentation]    pppd subscriber pings the corerouter1 loopback
    ...                (10.254.0.2), a destination beyond osvbng. Proves
    ...                packets transit osvbng (decap, FIB lookup, eth1
    ...                forward) and the return path (corerouter1,
    ...                osvbng, L2TP encap, ppp0) works.
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${lac} ping -c 3 -W 2 -I ppp0 ${corerouter1-loop-ipv4}
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0    Transit ping to ${corerouter1-loop-ipv4} failed: ${output}

Verify BNG Blaster Upstream Traffic
    [Documentation]    Every bngblaster session sends IPv4 session
    ...                traffic up the tunnel to its own network
    ...                interface address. osvbng decapsulates, forwards
    ...                via corerouter1, and bngblaster verifies the flow
    ...                on arrival. Ten upstream flows, all verified.
    Wait Until Keyword Succeeds    60s    5s
    ...    BNG Blaster Upstream Flows Verified    ${blaster}    ${blaster-session-count}

Verify BNG Blaster Downstream Delivery
    [Documentation]    corerouter1 pings every bngblaster session
    ...                address. The echo requests route via the static
    ...                pool route into osvbng, onto the per-session
    ...                interface and down the tunnel; each session's
    ...                ICMP receive counter proves arrival at the LAC.
    ${last} =    Evaluate    ${blaster-session-count} + 1
    FOR    ${id}    IN RANGE    1    ${last}
        ${ip} =    BNG Blaster Session IPv4    ${blaster}    ${id}
        Should Match Regexp    ${ip}    ^${subscriber-ipv4-prefix}
        ${before} =    BNG Blaster Session ICMP Received    ${blaster}    ${id}
        ${rc}    ${output} =    Run And Return Rc And Output
        ...    sudo docker exec ${corerouter1} ping -c 3 -W 2 ${ip}
        Log    ${output}
        Wait Until Keyword Succeeds    10s    1s
        ...    BNG Blaster Session Received ICMP    ${blaster}    ${id}    ${before + 3}
    END

*** Keywords ***
LAC ppp0 Has IPv4
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} ip -4 addr show ppp0
    Should Be Equal As Integers    ${rc}    0    ppp0 not yet present on ${container}
    Should Match Regexp    ${output}    inet ${subscriber-ipv4-prefix}

LNS Sessions Are Active
    [Arguments]    ${container}    ${expected_count}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); sessions=d.get('data') or []; active=[x for x in sessions if str(x.get('State','')) == 'active' and str(x.get('AccessType','')) == 'l2tp']; print(len(active))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${result}    ${expected_count}
    ...    Expected ${expected_count} active L2TP sessions but got ${result}

BNG Blaster Sessions Established
    [Arguments]    ${container}    ${expected_count}
    ${rc}    ${output} =    BNG Blaster CLI Command    ${container}    session-counters
    Should Be Equal As Integers    ${rc}    0    session-counters CLI failed
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; c=json.load(sys.stdin).get('session-counters',{}); print(c.get('sessions-established',0))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${result}    ${expected_count}
    ...    Expected ${expected_count} established bngblaster sessions but got ${result}

BNG Blaster Upstream Flows Verified
    [Arguments]    ${container}    ${expected_count}
    ${rc}    ${output} =    BNG Blaster CLI Command    ${container}    session-counters
    Should Be Equal As Integers    ${rc}    0    session-counters CLI failed
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; c=json.load(sys.stdin).get('session-counters',{}); print(c.get('session-traffic-flows-verified',0))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${result}    ${expected_count}
    ...    Expected ${expected_count} verified upstream flows but got ${result}

BNG Blaster Session IPv4
    [Arguments]    ${container}    ${session_id}
    ${rc}    ${output} =    BNG Blaster CLI Command    ${container}    session-info session-id ${session_id}
    Should Be Equal As Integers    ${rc}    0    session-info CLI failed
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; print(json.load(sys.stdin).get('session-info',{}).get('ipv4-address',''))"
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${result}

BNG Blaster Session ICMP Received
    [Arguments]    ${container}    ${session_id}
    ${rc}    ${output} =    BNG Blaster CLI Command    ${container}    session-info session-id ${session_id}
    Should Be Equal As Integers    ${rc}    0    session-info CLI failed
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; print(json.load(sys.stdin).get('session-info',{}).get('rx-icmp',0))"
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${{ int($result) }}

BNG Blaster Session Received ICMP
    [Arguments]    ${container}    ${session_id}    ${minimum}
    ${count} =    BNG Blaster Session ICMP Received    ${container}    ${session_id}
    Should Be True    ${count} >= ${minimum}
    ...    Session ${session_id} received ${count} ICMP packets, expected at least ${minimum}
