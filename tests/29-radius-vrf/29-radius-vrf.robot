# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
RADIUS-in-MGMT-VRF integration test.

FreeRADIUS is reachable only via MGMT-VRF (10.99.0.10/24 on a dedicated link
to bng1:eth3, enslaved to MGMT-VRF). The RADIUS plugin is configured with
vrf: MGMT-VRF and source_ip: 10.99.0.2; default routing has no path to the
RADIUS server. Successful IPoE auth proves the socket egressed via MGMT-VRF.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot

Suite Setup         Setup RADIUS VRF Test
Suite Teardown      Teardown RADIUS VRF Test

*** Variables ***
${lab-name}         osvbng-radius-vrf
${lab-file}         ${CURDIR}/29-radius-vrf.clab.yml
${bng1}             clab-${lab-name}-bng1
${subscribers}      clab-${lab-name}-subscribers
${freeradius}       clab-${lab-name}-freeradius
${session-count}    1
${vrf-name}         MGMT-VRF
${radius-server}    10.99.0.10
${radius-source}    10.99.0.2

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start. Failure here means the
    ...    RADIUS dial through netbind could not bring up the per-server
    ...    connection in MGMT-VRF.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify MGMT-VRF Master And eth3 Enslavement
    [Documentation]    Pre-osvbng entrypoint must have created the VRF master
    ...    and enslaved eth3. Asserts the kernel-level state vrfmgr expects.
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${bng1} ip -d link show ${vrf-name}
    Should Be Equal As Integers    ${rc}    0
    Should Contain    ${output}    vrf table 99
    ${rc}    ${eth3} =    Run And Return Rc And Output
    ...    sudo docker exec ${bng1} ip link show eth3
    Should Be Equal As Integers    ${rc}    0
    Should Contain    ${eth3}    master ${vrf-name}

Verify FreeRADIUS Route Lives In MGMT-VRF Table Only
    [Documentation]    The 10.99.0.0/24 connected route must be in MGMT-VRF's
    ...    table (99) and absent from the main routing table. A main-table
    ...    entry would defeat VRF isolation.
    ${rc}    ${main} =    Run And Return Rc And Output
    ...    sudo docker exec ${bng1} ip route show table main 10.99.0.0/24
    Should Be Equal As Integers    ${rc}    0
    Should Be Empty    ${main}    Main table unexpectedly has 10.99.0.0/24 route: ${main}
    ${rc}    ${vrf} =    Run And Return Rc And Output
    ...    sudo docker exec ${bng1} ip route show table 99 10.99.0.0/24
    Should Be Equal As Integers    ${rc}    0
    Should Contain    ${vrf}    dev eth3    MGMT-VRF table missing 10.99.0.0/24 via eth3: ${vrf}

Verify RADIUS Sockets Bound To MGMT-VRF Source
    [Documentation]    The RADIUS auth and acct sockets must show
    ...    "10.99.0.2%MGMT-VRF" as the local address — ss prints the VRF
    ...    suffix only when SO_BINDTODEVICE is set to the VRF master.
    ...    This is the strongest user-space proof that netbind.DialUDP
    ...    bound the socket to MGMT-VRF.
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${bng1} ss -anup
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0
    Should Contain    ${output}    ${radius-source}%${vrf-name}:    Socket source not annotated %${vrf-name} (no SO_BINDTODEVICE)
    Should Contain    ${output}    ${radius-server}:1812    No RADIUS auth socket to ${radius-server}:1812
    Should Contain    ${output}    ${radius-server}:1813    No RADIUS acct socket to ${radius-server}:1813

Establish Subscriber Sessions
    [Documentation]    Start BNG Blaster. Sessions establishing means RADIUS
    ...    Access-Request egressed via MGMT-VRF and FreeRADIUS responded.
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}    check_ipv6=true

Verify Sessions In osvbng API
    Verify Sessions In API    ${bng1}    ${session-count}

Verify IPv4 Addresses Assigned
    Verify Sessions Have IPv4    ${bng1}

Verify IPv6 Addresses Assigned
    Verify Sessions Have IPv6    ${bng1}

Verify RADIUS Server Stats
    [Documentation]    Server stats prove auth-accept count >= session count
    ...    and that the server keyed in the API matches the VRF-side IP.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/aaa/radius/servers
    Should Contain    ${output}    ${radius-server}    RADIUS server in API does not match MGMT-VRF address
    ${rc}    ${accepts} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); s=d.get('data',[]); print(sum(x.get('auth_accepts',0) for x in s))"
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${accepts} >= ${session-count}    Expected at least ${session-count} auth accepts but got ${accepts}

Verify FreeRADIUS Saw Access-Request From MGMT-VRF Source
    [Documentation]    Server-side proof: FreeRADIUS debug logs must show an
    ...    Access-Request arriving from ${radius-source} (only reachable via
    ...    MGMT-VRF) and a corresponding Access-Accept response.
    ${rc}    ${logs} =    Run And Return Rc And Output
    ...    sudo docker logs ${freeradius} 2>&1
    Should Be Equal As Integers    ${rc}    0
    Should Contain    ${logs}    Access-Request    FreeRADIUS never received an Access-Request
    Should Contain    ${logs}    ${radius-source}    Access-Request did not originate from ${radius-source}
    Should Contain    ${logs}    Access-Accept    FreeRADIUS did not send Access-Accept

Verify BNG Blaster Report
    Stop BNG Blaster    ${subscribers}
    ${established} =    Get BNG Blaster Report Field    ${subscribers}    sessions-established
    Should Be Equal As Strings    ${established}    ${session-count}

*** Keywords ***
Setup RADIUS VRF Test
    [Documentation]    Deploy the topology then configure the freeradius
    ...    container's eth1 IP from the host (the freeradius image has no
    ...    iproute2). This must happen before osvbng's RADIUS plugin tries
    ...    to dial, otherwise the first auth attempt times out and the
    ...    bng comes up unhealthy.
    Deploy Topology    ${lab-file}
    Configure Freeradius VRF Address

Configure Freeradius VRF Address
    ${rc}    ${pid} =    Run And Return Rc And Output
    ...    sudo docker inspect -f '{{.State.Pid}}' ${freeradius}
    Should Be Equal As Integers    ${rc}    0
    Should Not Be Empty    ${pid}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo nsenter --target ${pid} --net ip addr add ${radius-server}/24 dev eth1
    Log    ${output}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo nsenter --target ${pid} --net ip link set eth1 up
    Should Be Equal As Integers    ${rc}    0    Failed to bring eth1 up in freeradius netns

Teardown RADIUS VRF Test
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}
