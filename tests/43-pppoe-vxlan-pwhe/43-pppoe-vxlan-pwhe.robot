# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
PPPoE termination over a VXLAN access network via a pseudowire headend.
Same overlay shape as suite 42 but the access type is PPPoE: PADI/PADO
and the whole PPP session ride the pseudowire, sessions terminate on
pw-an1 subinterfaces with local PAP authentication. Asserts discovery
and LCP/IPCP establishment through the tunnel, addresses from the local
pool, forwarding via VPP ping, and restart restore.

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
${lab-name}         osvbng-pppoe-vxlan-pwhe
${lab-file}         ${CURDIR}/43-pppoe-vxlan-pwhe.clab.yml
${bng1}             clab-${lab-name}-bng1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    2

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify Pseudowire Bound To Transport
    [Documentation]    The tunnel plugin must hold the vxlan-an1 -> pw-an1
    ...    binding.
    ${output} =    Execute VPP Command    ${bng1}    show osvbng tunnel
    Should Contain    ${output}    vxlan-an1 -> pw-an1

Establish PPPoE Sessions Over Pseudowire
    [Documentation]    Create local auth users; PADI arrives through vxlan
    ...    decap onto a pw-an1 subinterface, PADO/PADS and the PPP session
    ...    return through the headend TX redirect.
    Create PPPoE Users    ${bng1}    ${session-count}
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}

Verify Sessions In osvbng API
    [Documentation]    Local session table carries both PPPoE sessions with
    ...    pool addresses.
    Verify Sessions In API    ${bng1}    ${session-count}
    Verify Sessions Have IPv4    ${bng1}

Verify Forwarding Over Pseudowire
    [Documentation]    VPP ping to each session address through the
    ...    pseudowire datapath.
    ${ips} =    Get Session IPv4 Addresses    ${bng1}
    FOR    ${ip}    IN    @{ips}
        ${output} =    Execute VPP Command    ${bng1}    ping ${ip} source loop100 repeat 5
        Should Match Regexp    ${output}    [1-5] received    no ping replies from ${ip}
    END

Verify Full MTU Frames Traverse The Pseudowire
    [Documentation]    MTU guard: the largest unfragmented PPP packet
    ...    (1484-byte IP today: the session MTU is programmed as the
    ...    1492 MRU but VPP's rewrite accounting deducts the 8-byte
    ...    PPPoE/PPP overhead again - a pre-existing quirk on all
    ...    parents, tracked as a follow-up fix) must cross the
    ...    pseudowire in both directions without fragmentation.
    ${ips} =    Get Session IPv4 Addresses    ${bng1}
    FOR    ${ip}    IN    @{ips}
        ${output} =    Execute VPP Command    ${bng1}    ping ${ip} source loop100 repeat 3 size 1456
        Should Match Regexp    ${output}    [1-3] received    max-unfragmented ping to ${ip} failed
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
    [Documentation]    osvbngd restart: headend and binding re-resolve
    ...    idempotently, PPPoE sessions restore from opdb, forwarding
    ...    resumes.
    Restart osvbngd    ${bng1}
    Wait For osvbngd Down    ${bng1}
    Wait For osvbng Healthy    bng1    ${lab-name}
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
