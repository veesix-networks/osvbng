# Copyright 2026 Veesix Networks Ltd
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
Linux client integration test for IPoE.
Validates that a real Linux subscriber with QinQ VLANs can obtain
an IP via DHCP through the BNG, ping the gateway, and reach a
service on the core network.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot

Suite Setup         Deploy Linux Client Topology
Suite Teardown      Destroy Linux Client Topology

*** Variables ***
${lab-name}         osvbng-ipoe-linux-client
${lab-file}         ${CURDIR}/18-ipoe-linux-client.clab.yml
${bng1}             clab-${lab-name}-bng1
${corerouter1}      clab-${lab-name}-corerouter1
${subscriber}       clab-${lab-name}-subscriber
${qinq-iface}       eth1.100.10
${subscriber-image}    veesixnetworks/bngtester:alpine-latest

*** Test Cases ***
Verify BNG Is Healthy
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify VPP Is Running
    [Documentation]    Check VPP is running and responsive.
    ${output} =    Execute VPP Command    ${bng1}    show version
    Should Contain    ${output}    vpp

Verify OSPF Adjacency
    Wait Until Keyword Succeeds    12 x    10s
    ...    Check OSPF Neighbor    ${corerouter1}    10.254.0.1

Verify Subscriber QinQ Interface Created
    Wait Until Keyword Succeeds    30 x    5s
    ...    Check QinQ Interface Exists    ${subscriber}

Verify Subscriber Got IPv4 Via DHCP
    Wait Until Keyword Succeeds    30 x    5s
    ...    Check Subscriber Has IPv4    ${subscriber}

Verify Session In BNG API
    Wait Until Keyword Succeeds    30 x    2s
    ...    Check BNG Session Count    ${bng1}    1

Verify Subscriber Can Ping Gateway
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    docker exec ${subscriber} ping -c 3 -W 2 10.255.0.1
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0    Subscriber cannot ping gateway

Verify Subscriber Can Ping Core Router
    Wait Until Keyword Succeeds    10 x    5s
    ...    Ping From Subscriber    ${subscriber}    10.0.0.2

Verify Subscriber Can Run Iperf3 To Core Router
    Start Iperf3 Server On Core    ${corerouter1}
    Wait Until Keyword Succeeds    6 x    5s
    ...    Run Iperf3 Client    ${subscriber}    10.0.0.2

*** Keywords ***
Deploy Linux Client Topology
    Set Environment Variable    BNGTESTER_IMAGE    ${subscriber-image}
    Deploy Topology    ${lab-file}

Destroy Linux Client Topology
    Destroy Topology    ${lab-file}

Check OSPF Neighbor
    [Arguments]    ${container}    ${neighbor_rid}
    ${output} =    Execute Vtysh On Router    ${container}    show ip ospf neighbor
    Should Contain    ${output}    ${neighbor_rid}
    Should Contain    ${output}    Full

Check QinQ Interface Exists
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    docker exec ${container} ip link show ${qinq-iface}
    Should Be Equal As Integers    ${rc}    0    QinQ interface ${qinq-iface} not found

Check Subscriber Has IPv4
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    docker exec ${container} ip -4 addr show ${qinq-iface}
    Should Be Equal As Integers    ${rc}    0
    Should Contain    ${output}    inet    No IPv4 address on ${qinq-iface}
    Should Not Contain    ${output}    169.254    Got link-local address, DHCP failed

Check BNG Session Count
    [Arguments]    ${container}    ${expected}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); entries=d.get('data',[]); print(len(entries))"
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${count} >= ${expected}    Session count ${count} < expected ${expected}

Ping From Subscriber
    [Arguments]    ${container}    ${target}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    docker exec ${container} ping -c 3 -W 2 ${target}
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0    Cannot ping ${target}

Start Iperf3 Server On Core
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    docker exec ${container} sh -c "apk add --no-cache iperf3 > /dev/null 2>&1 && iperf3 -s -D"
    Log    ${output}

Run Iperf3 Client
    [Arguments]    ${container}    ${server}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    docker exec ${container} iperf3 -c ${server} -t 3
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0    iperf3 failed
