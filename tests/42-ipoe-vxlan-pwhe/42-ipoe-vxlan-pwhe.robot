# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
IPoE termination over a VXLAN access network via a pseudowire headend.
The access NNI arrives as vxlan-an1 (VNI 10101) and pw-an1 is the
headend: decapsulated frames are re-attributed to pw-an1 by the
osvbng_tunnel plugin, VLAN subinterfaces classify on the headend
exactly as on a physical port, and DHCP/IPoE terminates locally. TX
(including subif TX) is redirected into the tunnel by the headend's
replaced output node, so DHCP OFFER/ACK and downstream forwarding ride
the pseudowire. Asserts establishment, per-session forwarding via VPP
ping, tunnel counter conservation (leak guard), and restart restore.

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
${lab-name}         osvbng-ipoe-vxlan-pwhe
${lab-file}         ${CURDIR}/42-ipoe-vxlan-pwhe.clab.yml
${bng1}             clab-${lab-name}-bng1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    2

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify Pseudowire Bound To Transport
    [Documentation]    The tunnel plugin must expose the pw decap next
    ...    indices and hold the vxlan-an1 -> pw-an1 binding.
    ${output} =    Execute VPP Command    ${bng1}    show osvbng tunnel
    Should Contain    ${output}    osvbng-pw-input decap next indices
    Should Not Contain    ${output}    vxlan plugin not loaded
    Should Contain    ${output}    pseudowire bindings:
    Should Contain    ${output}    vxlan-an1 -> pw-an1
    ${output} =    Execute VPP Command    ${bng1}    show vxlan tunnel
    Should Contain    ${output}    vni 10101
    Should Contain    ${output}    decap-next-index

Establish IPoE Sessions Over Pseudowire
    [Documentation]    DHCP DISCOVER decapsulates, classifies on a pw-an1
    ...    subinterface and terminates locally; OFFER/ACK return through
    ...    the headend TX redirect into the tunnel.
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}

Verify Sessions Terminate On Headend
    [Documentation]    Sessions exist in the local session table (unlike
    ...    l2gw wholesale, these are OURS) with addresses from the pool.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); s=d.get('data') or []; print(len([x for x in s if str(x.get('IPv4Address','')).startswith('10.255.')]))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${result}    ${session-count}    sessions missing pool addresses

Verify Forwarding Over Pseudowire
    [Documentation]    VPP ping to each subscriber address: request TX
    ...    rides the headend redirect into the tunnel, reply RX comes back
    ...    through decap. Proves the L3 datapath end to end.
    ${ips} =    Get Session IPv4 Addresses    ${bng1}
    FOR    ${ip}    IN    @{ips}
        ${output} =    Execute VPP Command    ${bng1}    ping ${ip} source loop100 repeat 5
        Should Match Regexp    ${output}    [1-5] received    no ping replies from ${ip}
    END

Verify Full MTU Frames Traverse The Pseudowire
    [Documentation]    MTU guard: a full-size inner IP packet (1500 bytes,
    ...    1522-byte QinQ frame, ~1572 encapsulated) must cross the
    ...    pseudowire in both directions without fragmentation.
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
    [Documentation]    osvbngd restart: headend and binding re-resolve
    ...    idempotently, sessions restore from opdb, forwarding resumes.
    Restart osvbngd    ${bng1}
    Wait For osvbngd Down    ${bng1}
    Wait For osvbng Healthy    bng1    ${lab-name}
    Wait For osvbng State Ready    ${bng1}
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
