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
Suite Teardown      Teardown PPPoE Test

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
    [Documentation]    The AAA policy uses agent-remote-id as the lookup key
    ...                with authenticate=false (LAC mode: real auth happens
    ...                at the LNS via proxy-auth AVPs in ICCN). The local
    ...                entry exists only to return the Tunnel-* attributes
    ...                that pick the bngblaster LNS.
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

*** Keywords ***
LAC Session Is Tunneled
    [Arguments]    ${container}    ${expected_count}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); sessions=d.get('data') or []; tunneled=[x for x in sessions if str(x.get('State','')) == 'tunneled']; print(len(tunneled))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${result}    ${expected_count}
    ...    Expected ${expected_count} tunneled sessions but got ${result}
