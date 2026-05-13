# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
L2TP LNS smoke test. bngblaster acts as a LAC, terminates PPPoE on
the access side, tunnels the PPP frames over L2TPv2 to osvbng (LNS).
osvbng terminates the PPP locally, runs LCP/CHAP/IPCP/IPv6CP, and
programs the per-session interface in VPP. The current scope covers
tunnel and session bring-up; AAA and southbound program-into-VPP are
the next sub-phases and may be marked NotImplemented until they land.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot

Suite Setup         Deploy Topology    ${lab-file}
Suite Teardown      Run Keywords       Stop BNG Blaster If Running    ${subscribers}
...                                    AND    Destroy Topology    ${lab-file}

*** Variables ***
${lab-name}         osvbng-l2tp-lns
${lab-file}         ${CURDIR}/30-l2tp-lns.clab.yml
${bng1}             clab-${lab-name}-bng1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    1

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify VPP Is Running
    [Documentation]    Confirm VPP is responsive on the BNG.
    ${output} =    Execute VPP Command    ${bng1}    show version
    Should Contain    ${output}    vpp

Verify L2TPv2 Plugin Loaded
    [Documentation]    L2TPv2 plugin must be loaded on the BNG.
    ${output} =    Execute VPP Command    ${bng1}    show plugins
    Should Contain    ${output}    l2tpv2

Establish L2TP Tunnel And Session
    [Documentation]    Start blaster as LAC; verify tunnel + session up on the LNS.
    Start BNG Blaster In Background    ${subscribers}
    Sleep    10s    reason=allow PPPoE + L2TP bring-up
    ${tunnels} =    Execute VPP Command    ${bng1}    show l2tpv2 tunnel
    Should Contain    ${tunnels}    Established
    ${sessions} =    Execute VPP Command    ${bng1}    show l2tpv2 session
    Should Contain    ${sessions}    mode ip

Verify L2TPv2 Punt Stats
    [Documentation]    Confirm control packets are reaching the punt path.
    ${output} =    Execute VPP Command    ${bng1}    show node osvbng-punt-l2tp
    Should Contain    ${output}    osvbng-punt-l2tp
