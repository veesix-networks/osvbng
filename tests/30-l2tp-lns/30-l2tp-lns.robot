# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
L2TPv2 LNS end-to-end test. Topology:
   lac (xl2tpd + pppd)  --10.10.0.0/30--  corerouter1 (FRR)  --10.20.0.0/30--  bng1 (osvbng LNS)

xl2tpd opens an L2TPv2 tunnel from the lac container to the osvbng LNS,
and pppd brings up a PPP session over it. osvbng terminates LCP / CHAP /
IPCP / IPv6CP, authenticates against the local auth provider
(allow_all=true for the test), allocates a subscriber IPv4 from the LNS
local pool and an IPv6 IANA + delegated prefix from the local IPv6 pools.

Dataplane is exercised in two directions:
  - ping subscriber → LNS gateway (osvbng-local termination)
  - ping subscriber → corerouter1 loopback (transit through osvbng)
corerouter1 has a static return route for the subscriber pool so the
echo reply traverses osvbng-LNS back into the L2TP tunnel.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot

Suite Setup         Deploy Topology     ${lab-file}
Suite Teardown      Destroy Topology    ${lab-file}

*** Variables ***
${lab-name}                osvbng-l2tp-lns
${lab-file}                ${CURDIR}/30-l2tp-lns.clab.yml
${bng1}                    clab-${lab-name}-bng1
${lac}                     clab-${lab-name}-lac
${corerouter1}             clab-${lab-name}-corerouter1
${session-count}           1
${lac-hostname}            lac
${lns-gateway-ipv4}        100.64.0.1
${bng1-loopback-ipv4}      10.254.0.1
${corerouter1-loop-ipv4}   10.254.0.2
${subscriber-ipv4-prefix}  100.64.0.

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

Verify LNS Session Active
    [Documentation]    osvbng reports one active L2TP subscriber session.
    Wait Until Keyword Succeeds    30s    2s
    ...    LNS Session Is Active    ${bng1}    ${session-count}

Verify L2TP Tunnel In osvbng API
    [Documentation]    osvbng's show l2tp tunnels reports the inbound
    ...                tunnel from xl2tpd as Role LNS, State Established.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/l2tp/tunnels
    Should Contain    ${output}    Established
    Should Contain    ${output}    "Role":"LNS"
    Should Contain    ${output}    "PeerHostname":"${lac-hostname}"

Verify Subscriber Got IP From Local Pool
    [Documentation]    osvbng-LNS allocates the IPv4 from the local pool
    ...                100.64.0.0/16. Subscriber row carries an IPv4 in
    ...                that range and the username from CHAP.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    Should Contain    ${output}    "Username":"user1"
    Should Match Regexp    ${output}    "IPv4Address":"100\\.64\\.

Verify Per-Session VPP Interface
    [Documentation]    DECAP_IP creates a per-session vnet interface in
    ...                VPP, set unnumbered to the gateway loopback, with
    ...                tunnel-output as the L3 output node.
    ${output} =    Execute VPP Command    ${bng1}    show l2tpv2 session
    Should Contain    ${output}    mode ip
    ${addr_out} =    Execute VPP Command    ${bng1}    show interface addr
    Should Contain    ${addr_out}    l2tpv2_session0
    Should Contain    ${addr_out}    unnumbered, use loop100

Verify Dataplane Ping LNS Gateway
    [Documentation]    Subscriber pings the LNS-side gateway IP (the
    ...                address shared via unnumbered loop100). Exercises
    ...                local-termination at osvbng: L2TP decap →
    ...                ip4-input → ip4-local → echo-reply → midchain
    ...                encap → eth1.
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${lac} ping -c 3 -W 2 -I ppp0 ${lns-gateway-ipv4}
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0    Ping to ${lns-gateway-ipv4} failed: ${output}

Verify Dataplane Ping BNG Loopback
    [Documentation]    Subscriber pings the BNG control-plane loopback
    ...                (10.254.0.1, advertised via OSPF). Same local-
    ...                termination path as the LNS gateway test but
    ...                exercises a different FIB entry.
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${lac} ping -c 3 -W 2 -I ppp0 ${bng1-loopback-ipv4}
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0    Ping to ${bng1-loopback-ipv4} failed: ${output}

Verify Dataplane Transit Through BNG
    [Documentation]    Subscriber pings the corerouter1 loopback
    ...                (10.254.0.2) — a destination beyond osvbng.
    ...                Proves packets transit osvbng (decap →
    ...                FIB lookup → eth1 forward) and the return path
    ...                (corerouter1 → osvbng → L2TP encap → ppp0) works.
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${lac} ping -c 3 -W 2 -I ppp0 ${corerouter1-loop-ipv4}
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0    Transit ping to ${corerouter1-loop-ipv4} failed: ${output}

*** Keywords ***
LAC ppp0 Has IPv4
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} ip -4 addr show ppp0
    Should Be Equal As Integers    ${rc}    0    ppp0 not yet present on ${container}
    Should Match Regexp    ${output}    inet ${subscriber-ipv4-prefix}

LNS Session Is Active
    [Arguments]    ${container}    ${expected_count}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); sessions=d.get('data') or []; active=[x for x in sessions if str(x.get('State','')) == 'active' and str(x.get('AccessType','')) == 'l2tp']; print(len(active))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${result}    ${expected_count}
    ...    Expected ${expected_count} active L2TP sessions but got ${result}
