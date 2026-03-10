# Copyright 2026 Veesix Networks Ltd
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
DHCP relay and proxy test suite.
Verifies DHCPv4 relay and proxy modes with external Kea DHCP server,
including session establishment, address assignment, and show command output.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot

Suite Setup         Setup DHCP Relay Proxy Test
Suite Teardown      Teardown DHCP Relay Proxy Test

*** Variables ***
${lab-name}         osvbng-dhcp-relay-proxy
${lab-file}         ${CURDIR}/07-dhcp-relay-proxy.clab.yml
${bng1}             clab-${lab-name}-bng1
${corerouter1}      clab-${lab-name}-corerouter1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    2

*** Test Cases ***
Verify Sessions In osvbng API
    [Documentation]    Verify osvbng REST API reports 2 sessions (1 relay, 1 proxy).
    Verify Sessions In API    ${bng1}    ${session-count}

Verify IPv4 Addresses Assigned
    [Documentation]    All sessions have an IPv4 address from Kea.
    Verify Sessions Have IPv4    ${bng1}

Verify VPP Sub-Interfaces Created
    [Documentation]    Verify QinQ sub-interfaces exist in VPP for both relay and proxy VLANs.
    Verify VPP Sub-Interfaces Created    ${bng1}    eth1.300
    Verify VPP Sub-Interfaces Created    ${bng1}    eth1.400

Verify DHCP Relay Show Command
    [Documentation]    Verify dhcp/relay show command returns server stats.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/dhcp/relay
    ${rc}    ${requests} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); s=d.get('data',{}).get('stats',{}); print(s.get('requests4',0))"
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${requests} >= 2    Expected at least 2 relay requests but got ${requests}

Verify DHCP Proxy Show Command
    [Documentation]    Verify dhcp/proxy show command returns binding counts.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/dhcp/proxy
    ${rc}    ${bindings} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); data=d.get('data',{}); print(data.get('v4Bindings',0))"
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${bindings} >= 1    Expected at least 1 proxy binding but got ${bindings}

Verify RADIUS Server Stats
    [Documentation]    Verify RADIUS server stats show auth accepts for both sessions.
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
Setup DHCP Relay Proxy Test
    Deploy Topology    ${lab-file}
    Wait For osvbng Healthy    bng1    ${lab-name}
    Wait For Kea Healthy
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}

Teardown DHCP Relay Proxy Test
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}

Wait For Kea Healthy
    [Documentation]    Wait for Kea DHCPv4 process to be running.
    ${kea} =    Set Variable    clab-${lab-name}-kea
    Wait Until Keyword Succeeds    12 x    5s
    ...    Kea Process Running    ${kea}

Kea Process Running
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} pgrep -c kea-dhcp4
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${output} >= 1    Expected kea-dhcp4 process but found none
