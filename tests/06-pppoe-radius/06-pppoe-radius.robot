# Copyright 2026 Veesix Networks Ltd
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
PPPoE session test suite with RADIUS authentication.
Establishes a PPPoE session via BNG Blaster with PAP authentication
against FreeRADIUS, and verifies session state through the osvbng API.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot

Suite Setup         Deploy Topology    ${lab-file}
Suite Teardown      Teardown PPPoE RADIUS Test

*** Variables ***
${lab-name}         osvbng-pppoe-radius
${lab-file}         ${CURDIR}/06-pppoe-radius.clab.yml
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

Establish Subscriber Sessions
    [Documentation]    Start BNG Blaster PPPoE sessions with RADIUS auth.
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}    check_ipv6=true

Verify Sessions In osvbng API
    [Documentation]    Verify osvbng REST API reports the correct session count.
    Verify Sessions In API    ${bng1}    ${session-count}

Verify IPv4 Addresses Assigned
    [Documentation]    Verify all PPPoE sessions have an IPv4 address in the API.
    Verify Sessions Have IPv4    ${bng1}

Verify IPv6 Addresses Assigned
    [Documentation]    Verify all PPPoE sessions have an IPv6 address in the API.
    Verify Sessions Have IPv6    ${bng1}

Verify VPP Sub-Interfaces Created
    [Documentation]    Verify PPPoE sub-interfaces exist in VPP.
    Verify VPP Sub-Interfaces Created    ${bng1}    eth1.200

Verify RADIUS Server Stats
    [Documentation]    Verify RADIUS server stats show auth accepts.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/aaa/radius/servers
    ${rc}    ${accepts} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); s=d.get('data',[]); print(sum(x.get('auth_accepts',0) for x in s))"
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${accepts} >= ${session-count}    Expected at least ${session-count} auth accepts but got ${accepts}

Verify BNG Blaster Report
    [Documentation]    Stop BNG Blaster and verify report shows all sessions established.
    Stop BNG Blaster    ${subscribers}
    ${established} =    Get BNG Blaster Report Field    ${subscribers}    sessions-established
    Should Be Equal As Strings    ${established}    ${session-count}

*** Keywords ***
Teardown PPPoE RADIUS Test
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}
