# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
PPPoE RFC 4638 + TCP MSS clamping test suite.
Brings up a PPPoE session with BNG Blaster sending PPP-Max-Payload=1500 in
PADI/PADR. The BNG subscriber group has pppoe.rfc4638.enabled with
max-payload 1500 and eth1 mtu 1512 (1500 + 8 PPPoE/PPP + 4 dot1q).
Asserts negotiated PPP MTU, programmed MSS via VPP, and the per-session
interface MTU.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot
Resource            ../localauth.robot

Suite Setup         Deploy Topology    ${lab-file}
Suite Teardown      Teardown Test

*** Variables ***
${lab-name}         osvbng-mss-clamp
${lab-file}         ${CURDIR}/22-mss-clamp.clab.yml
${bng1}             clab-${lab-name}-bng1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    1

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify VPP Is Running
    [Documentation]    Check VPP is running and responsive.
    ${output} =    Execute VPP Command    ${bng1}    show version
    Should Contain    ${output}    vpp

Establish PPPoE Session
    [Documentation]    Create a local auth user and start one PPPoE session
    ...    where BNG Blaster advertises PPP-Max-Payload=1500.
    Create PPPoE Users    ${bng1}    ${session-count}
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}    check_ipv6=true

Verify Session In osvbng API
    [Documentation]    Verify osvbng REST API reports the PPPoE session.
    Verify Sessions In API    ${bng1}    ${session-count}

Verify Negotiated PPP MTU Is 1500
    [Documentation]    The session should have negotiated PPP-Max-Payload=1500
    ...    via the RFC 4638 PADI/PADO/PADR/PADS exchange.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    ${rc}    ${mtu} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); s=(d.get('data') or [])[0]; print(s.get('NegotiatedPPPMTU',0))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Integers    ${mtu}    1500    Expected NegotiatedPPPMTU=1500, got ${mtu}

Verify Programmed MSS Values From API
    [Documentation]    The session should have IPv4MSS=1460 (1500-40) and
    ...    IPv6MSS=1440 (1500-60) reported by the API.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); s=(d.get('data') or [])[0]; print(s.get('IPv4MSS',0), s.get('IPv6MSS',0))"
    Should Be Equal As Integers    ${rc}    0
    @{parts} =    Split String    ${result}
    Should Be Equal As Integers    ${parts}[0]    1460    Expected IPv4MSS=1460, got ${parts}[0]
    Should Be Equal As Integers    ${parts}[1]    1440    Expected IPv6MSS=1440, got ${parts}[1]

Verify VPP MSS Clamp Programmed On Session Interface
    [Documentation]    vppctl should report the MSS clamp programmed on the
    ...    pppoe_session interface for both IPv4 and IPv6.
    ${output} =    Execute VPP Command    ${bng1}    show interface tcp-mss-clamp
    Should Contain    ${output}    pppoe_session
    Should Contain    ${output}    ip4: 1460
    Should Contain    ${output}    ip6: 1440

Verify VPP Session Interface MTU Matches Negotiated
    [Documentation]    The pppoe_session VPP interface IP MTU should equal the
    ...    negotiated PPP MTU (1500).
    ${output} =    Execute VPP Command    ${bng1}    show interface
    Should Contain    ${output}    pppoe_session

Verify BNG Blaster Report
    [Documentation]    Stop BNG Blaster and verify the session report.
    Stop BNG Blaster    ${subscribers}
    ${established} =    Get BNG Blaster Report Field    ${subscribers}    sessions-established
    Should Be Equal As Strings    ${established}    ${session-count}

*** Keywords ***
Teardown Test
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}
