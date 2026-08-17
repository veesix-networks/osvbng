# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
IPoE termination over an EVPN-discovered VXLAN access network via a
pseudowire headend. vxlan-an1 is configured with signaling: evpn and no
remote VTEP: osvbng advertises VNI 10101 from a kernel mirror device,
learns the leaf's VTEP from its type-3 route, programs the VPP tunnel,
and only then binds the pw-an1 headend to it (the bind is deferred at
startup because the transport does not exist yet). On top of the
discovered transport the suite asserts 42-level behavior: local DHCP
termination on headend subinterfaces, per-session forwarding via VPP
ping, full-MTU frames, tunnel counter conservation, restart restore.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot
Resource            ../restart.robot

Suite Setup         Deploy Topology    ${lab-file}
Suite Teardown      Teardown PWHE Test

*** Variables ***
${lab-name}         osvbng-ipoe-evpn-pwhe
${lab-file}         ${CURDIR}/47-ipoe-evpn-pwhe.clab.yml
${bng1}             clab-${lab-name}-bng1
${bng1-mgmt-ip}     172.20.27.2
${subscribers}      clab-${lab-name}-subscribers
${session-count}    2
${leaf-vtep}        10.254.2.1

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify Discovery Programs Transport And Binds Headend
    [Documentation]    The remote VTEP is learned via EVPN, the VPP
    ...    tunnel is programmed with the learned dst, and the deferred
    ...    pw-an1 binding lands once the transport exists.
    Wait Until Keyword Succeeds    60s    5s
    ...    Verify Tunnel And Binding Present
    ${output} =    Execute VPP Command    ${bng1}    show osvbng tunnel
    Should Contain    ${output}    osvbng-pw-input decap next indices
    Should Not Contain    ${output}    vxlan plugin not loaded

Establish IPoE Sessions Over Discovered Pseudowire
    [Documentation]    DHCP DISCOVER decapsulates, classifies on a pw-an1
    ...    subinterface and terminates locally; OFFER/ACK return through
    ...    the headend TX redirect into the discovered tunnel.
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}

Verify Sessions Terminate On Headend
    [Documentation]    Sessions exist in the local session table with
    ...    addresses from the pool.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); s=d.get('data') or []; print(len([x for x in s if str(x.get('IPv4Address','')).startswith('10.255.')]))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${result}    ${session-count}    sessions missing pool addresses

Verify Forwarding Over Discovered Pseudowire
    [Documentation]    VPP ping to each subscriber address across the
    ...    EVPN-discovered transport.
    ${ips} =    Get Session IPv4 Addresses    ${bng1}
    FOR    ${ip}    IN    @{ips}
        ${output} =    Execute VPP Command    ${bng1}    ping ${ip} source loop100 repeat 5
        Should Match Regexp    ${output}    [1-5] received    no ping replies from ${ip}
    END

Verify Full MTU Frames Traverse The Pseudowire
    [Documentation]    MTU guard: a full-size inner IP packet must cross
    ...    the discovered pseudowire in both directions unfragmented.
    ${ips} =    Get Session IPv4 Addresses    ${bng1}
    FOR    ${ip}    IN    @{ips}
        ${output} =    Execute VPP Command    ${bng1}    ping ${ip} source loop100 repeat 3 size 1472
        Should Match Regexp    ${output}    [1-3] received    full-mtu ping to ${ip} failed
    END

Verify Traffic Rides The Tunnel
    [Documentation]    Leak guard: the ping exchange must move the tunnel
    ...    counters in both directions inside VPP.
    ${rx-0} =    Get VPP Interface Counter    ${bng1}    vxlan-an1    rx
    ${tx-0} =    Get VPP Interface Counter    ${bng1}    vxlan-an1    tx
    ${ips} =    Get Session IPv4 Addresses    ${bng1}
    FOR    ${ip}    IN    @{ips}
        ${output} =    Execute VPP Command    ${bng1}    ping ${ip} source loop100 repeat 5
    END
    ${rx-1} =    Get VPP Interface Counter    ${bng1}    vxlan-an1    rx
    ${tx-1} =    Get VPP Interface Counter    ${bng1}    vxlan-an1    tx
    Should Be True    ${tx-1} - ${tx-0} >= 8    pings not leaving through the tunnel
    Should Be True    ${rx-1} - ${rx-0} >= 8    replies not entering through the tunnel

Restart Survives With Sessions On Headend
    [Documentation]    osvbngd restart: the watcher re-seeds the learned
    ...    VTEP from existing fdb entries, tunnel programming and the
    ...    headend binding re-resolve idempotently, sessions restore
    ...    from opdb, forwarding resumes.
    Restart osvbngd    ${bng1}
    Wait For osvbngd Down    ${bng1}
    Wait For osvbng Healthy    bng1    ${lab-name}
    Wait For osvbng State Ready    ${bng1}
    Wait Until Keyword Succeeds    60s    5s
    ...    Verify Tunnel And Binding Present
    Verify Sessions In API    ${bng1}    ${session-count}
    ${ips} =    Get Session IPv4 Addresses    ${bng1}
    FOR    ${ip}    IN    @{ips}
        ${output} =    Execute VPP Command    ${bng1}    ping ${ip} source loop100 repeat 5
        Should Match Regexp    ${output}    [1-5] received    no ping replies from ${ip} after restart
    END

*** Keywords ***
Teardown PWHE Test
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}

Verify Tunnel And Binding Present
    ${output} =    Execute VPP Command    ${bng1}    show vxlan tunnel
    Should Contain    ${output}    vni 10101
    Should Contain    ${output}    dst ${leaf-vtep}
    ${output} =    Execute VPP Command    ${bng1}    show osvbng tunnel
    Should Contain    ${output}    pseudowire bindings:
    Should Contain    ${output}    vxlan-an1 -> pw-an1

Get Session IPv4 Addresses
    [Arguments]    ${container}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${script} =    Catenate    SEPARATOR=${SPACE}
    ...    import json,os;
    ...    s=json.loads(os.environ['JSON']).get('data') or [];
    ...    print('\\n'.join(sorted(x['IPv4Address'] for x in s if x.get('IPv4Address') and x['IPv4Address']!='<nil>')))
    ${result} =    Run Process    python3    -c    ${script}
    ...    env:JSON=${output}    stderr=STDOUT
    Should Be Equal As Integers    ${result.rc}    0
    @{ips} =    Split To Lines    ${result.stdout}
    RETURN    @{ips}

Get VPP Interface Counter
    [Documentation]    Cumulative rx or tx packet counter of a VPP interface.
    [Arguments]    ${container}    ${iface}    ${direction}
    ${output} =    Execute VPP Command    ${container}    show interface ${iface}
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    echo '${output}' | awk '/${direction} packets/ {print $NF; found=1} END {if (!found) print 0}'
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${count}
