# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
PPPoE termination over an EVPN-discovered VXLAN access network via a
pseudowire headend. vxlan-an1 is configured with signaling: evpn and no
remote VTEP: osvbng learns the leaf's VTEP from its type-3 route,
programs the VPP tunnel, and binds the pw-an1 headend to it (deferred
past startup because the transport does not exist yet). The headend is
sized 1508 so QinQ PPPoE reaches its full 1492-byte MRU. Asserts
43-level behavior on the discovered transport: PPPoE establishment,
forwarding, full-MRU frames, tunnel leak guard, restart restore.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot
Resource            ../localauth.robot
Resource            ../restart.robot

Suite Setup         Deploy Topology    ${lab-file}
Suite Teardown      Teardown PWHE PPPoE Test

*** Variables ***
${lab-name}         osvbng-pppoe-evpn-pwhe
${lab-file}         ${CURDIR}/48-pppoe-evpn-pwhe.clab.yml
${bng1}             clab-${lab-name}-bng1
${bng1-mgmt-ip}     172.20.28.2
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

Establish PPPoE Sessions Over Discovered Pseudowire
    [Documentation]    Create local auth users; PADI arrives through
    ...    the discovered tunnel onto a pw-an1 subinterface, PADO/PADS
    ...    and the PPP session return through the headend TX redirect.
    Create PPPoE Users    ${bng1}    ${session-count}
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}

Verify Sessions In osvbng API
    [Documentation]    Local session table carries both PPPoE sessions with
    ...    pool addresses.
    Verify Sessions In API    ${bng1}    ${session-count}
    Verify Sessions Have IPv4    ${bng1}

Verify Forwarding Over Discovered Pseudowire
    [Documentation]    VPP ping to each session address through the
    ...    discovered pseudowire datapath.
    ${ips} =    Get Session IPv4 Addresses    ${bng1}
    FOR    ${ip}    IN    @{ips}
        ${output} =    Execute VPP Command    ${bng1}    ping ${ip} source loop100 repeat 5
        Should Match Regexp    ${output}    [1-5] received    no ping replies from ${ip}
    END

Verify Full MTU Frames Traverse The Pseudowire
    [Documentation]    MTU guard: a full-MRU PPP packet (1492-byte IP)
    ...    must cross the discovered pseudowire in both directions
    ...    without fragmentation.
    ${ips} =    Get Session IPv4 Addresses    ${bng1}
    FOR    ${ip}    IN    @{ips}
        ${output} =    Execute VPP Command    ${bng1}    ping ${ip} source loop100 repeat 3 size 1464
        Should Match Regexp    ${output}    [1-3] received    full-mru ping to ${ip} failed
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
    ...    VTEP, tunnel and binding re-resolve idempotently, PPPoE
    ...    sessions restore from opdb, forwarding resumes.
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
Teardown PWHE PPPoE Test
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
