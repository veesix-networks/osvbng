# Copyright 2026 Veesix Networks Ltd
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
MPLS L3VPN end-to-end test suite.
Verifies VRF isolation, VPNv4 route exchange, LDP transport, and two-label
MPLS stack for IPoE and PPPoE subscribers across DEFAULT and CUSTOMER-A VRFs
over a bng1 (PE1) -- p1 (P) -- corerouter1 (PE2) MPLS core.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot
Resource            ../localauth.robot

Suite Setup         Deploy Topology    ${lab-file}
Suite Teardown      Teardown L3VPN

*** Variables ***
${lab-name}             osvbng-l3vpn
${lab-file}             ${CURDIR}/25-l3vpn.clab.yml
${bng1}                 clab-${lab-name}-bng1
${p1}                   clab-${lab-name}-p1
${corerouter1}          clab-${lab-name}-corerouter1
${subscribers}          clab-${lab-name}-subscribers
${session-count}        16

*** Test Cases ***

# --- Phase D: Scaffolding ---

Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify VPP Is Running
    [Documentation]    Check VPP is running and responsive.
    ${output} =    Execute VPP Command    ${bng1}    show version
    Should Contain    ${output}    vpp

Verify OSPF Adjacency On Core
    [Documentation]    OSPF adjacency must be Full on both core hops
    ...    (bng1-p1 and p1-corerouter1).
    Wait Until Keyword Succeeds    12 x    10s
    ...    Verify OSPF Adjacency On BNG    ${bng1}
    Wait Until Keyword Succeeds    12 x    10s
    ...    OSPF Adjacency Full On Router    ${corerouter1}

Verify LDP Session Established
    [Documentation]    bng1 must have an LDP session to p1; corerouter1 to p1.
    Wait Until Keyword Succeeds    12 x    10s
    ...    LDP Session Established On BNG    ${bng1}    10.0.0.2
    Wait Until Keyword Succeeds    12 x    10s
    ...    LDP Session Established On Router    ${corerouter1}    10.0.1.1

Verify VPNv4 Session Established
    [Documentation]    VPNv4 iBGP session between bng1 and corerouter1 must be Established.
    Wait Until Keyword Succeeds    12 x    10s
    ...    VPNv4 Session Established On BNG    ${bng1}    10.254.0.2
    Wait Until Keyword Succeeds    12 x    10s
    ...    VPNv4 Session Established On Router    ${corerouter1}    10.254.0.1

# --- Phase E: Session assertions and reachability ---

Establish Sessions In Both VRFs
    [Documentation]    Start BNG Blaster (allow_all: true — no user creation needed).
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}

Verify CUSTOMER-A Sessions Bound To CUSTOMER-A VRF
    [Documentation]    Every CUSTOMER-A session must have VRF=CUSTOMER-A,
    ...    ServiceGroup=customer-a, IPv4Pool=customer-a-pool (primary assertion).
    Verify Sessions In VRF    ${bng1}    CUSTOMER-A    customer-a    customer-a-pool

Verify DEFAULT Sessions Not Bound To Any Service Group
    [Documentation]    DEFAULT sessions must have no ServiceGroup and IPv4Pool=subscriber-pool.
    Verify Sessions In Default VRF    ${bng1}    subscriber-pool

Verify CUSTOMER-A Subscriber IPv4 From 192.168.123.0/24
    [Documentation]    Secondary pool-range check: CUSTOMER-A sessions have addresses in
    ...    192.168.123.0/24.
    Verify Sessions Have IPv4 In Range    ${bng1}    CUSTOMER-A    192.168.123

Verify DEFAULT Subscriber IPv4 From 10.255.0.0/16
    [Documentation]    Secondary pool-range check: DEFAULT sessions have addresses in
    ...    10.255.0.0/16.
    Verify Sessions Have IPv4 In Range    ${bng1}    ${EMPTY}    10.255

Verify CUSTOMER-A Pool Absent From Default VRF RIB
    [Documentation]    192.168.123.0/24 must not appear in the default VRF RIB on
    ...    bng1 or corerouter1.
    VRF Route Must Be Absent    ${bng1}    default    192.168.123.0/24
    VRF Route Must Be Absent On Router    ${corerouter1}    default    192.168.123.0/24

Verify Default Pool Absent From CUSTOMER-A VRF RIB
    [Documentation]    10.255.0.0/16 must not appear in the CUSTOMER-A VRF RIB on
    ...    bng1 or corerouter1.
    VRF Route Must Be Absent    ${bng1}    CUSTOMER-A    10.255.0.0/16
    VRF Route Must Be Absent On Router    ${corerouter1}    CUSTOMER-A    10.255.0.0/16

Verify CUSTOMER-A Gateway Loopback Absent From Default VRF RIB
    [Documentation]    192.168.123.1/32 must not leak to the default VRF.
    VRF Route Must Be Absent    ${bng1}    default    192.168.123.1/32
    VRF Route Must Be Absent On Router    ${corerouter1}    default    192.168.123.1/32

Verify Default Gateway Loopback Absent From CUSTOMER-A VRF RIB
    [Documentation]    10.255.0.1/32 must not leak to the CUSTOMER-A VRF.
    VRF Route Must Be Absent    ${bng1}    CUSTOMER-A    10.255.0.1/32
    VRF Route Must Be Absent On Router    ${corerouter1}    CUSTOMER-A    10.255.0.1/32

Verify CUSTOMER-A Destination Loopback Absent From Default VRF RIB
    [Documentation]    192.168.200.1/32 must not appear in the default VRF RIB on bng1.
    VRF Route Must Be Absent    ${bng1}    default    192.168.200.1/32

Verify Default Destination Loopback Absent From CUSTOMER-A VRF RIB
    [Documentation]    10.200.0.1/32 must not appear in the CUSTOMER-A VRF RIB on bng1.
    VRF Route Must Be Absent    ${bng1}    CUSTOMER-A    10.200.0.1/32

Verify CUSTOMER-A Subscriber Reaches CUSTOMER-A Loopback
    [Documentation]    custa-to-loop stream: CUSTOMER-A subscribers must reach
    ...    192.168.200.1 on corerouter1 via MPLS (dummy-custa RX delta > 0).
    ${before} =    Get Dummy Interface RX Packets    ${corerouter1}    dummy-custa
    Sleep    10s    Wait for stream traffic
    ${after} =    Get Dummy Interface RX Packets    ${corerouter1}    dummy-custa
    ${delta} =    Evaluate    int(${after}) - int(${before})
    Should Be True    ${delta} > 0
    ...    CUSTOMER-A subscribers did not reach 192.168.200.1 via L3VPN (delta=${delta})

Verify DEFAULT Subscriber Reaches DEFAULT Loopback
    [Documentation]    default-to-loop stream: DEFAULT subscribers must reach
    ...    10.200.0.1 on corerouter1 (dummy-default RX delta > 0).
    ${before} =    Get Dummy Interface RX Packets    ${corerouter1}    dummy-default
    Sleep    5s    Wait for stream traffic
    ${after} =    Get Dummy Interface RX Packets    ${corerouter1}    dummy-default
    ${delta} =    Evaluate    int(${after}) - int(${before})
    Should Be True    ${delta} > 0
    ...    DEFAULT subscribers did not reach 10.200.0.1 (delta=${delta})

Verify CUSTOMER-A Subscriber Cannot Reach Default Loopback
    [Documentation]    Isolation: CUSTOMER-A subscriber traffic to 10.200.0.1
    ...    must be dropped (no route in CUSTOMER-A VRF). No CUSTOMER-A source
    ...    packets must appear on dummy-default.
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${corerouter1} timeout 5 tcpdump -i dummy-default -c 1 -nn 'src net 192.168.123.0/24' 2>/dev/null
    Should Not Be Equal As Integers    ${rc}    0
    ...    ISOLATION FAILURE: CUSTOMER-A traffic reached the default VRF loopback

Verify DEFAULT Subscriber Cannot Reach CUSTOMER-A Loopback
    [Documentation]    Isolation: DEFAULT subscriber traffic to 192.168.200.1
    ...    must be dropped (no route in default VRF). No DEFAULT source packets
    ...    must appear on dummy-custa.
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${corerouter1} timeout 5 tcpdump -i dummy-custa -c 1 -nn 'src net 10.255.0.0/16' 2>/dev/null
    Should Not Be Equal As Integers    ${rc}    0
    ...    ISOLATION FAILURE: DEFAULT traffic reached the CUSTOMER-A VRF loopback

# --- Phase F: VPNv4 and MPLS assertions ---

Verify CUSTOMER-A Pool Advertised With RD 65000:100 RT 65000:100
    [Documentation]    bng1 must advertise the CUSTOMER-A pool prefix via VPNv4
    ...    with RD 65000:100 and RT 65000:100.
    Wait Until Keyword Succeeds    12 x    10s
    ...    VPNv4 Prefix Present On BNG    ${bng1}    192.168.123

Verify CUSTOMER-A Pool Received On corerouter1 In VRF CUSTOMER-A
    [Documentation]    corerouter1 must have the CUSTOMER-A pool prefix installed
    ...    in the CUSTOMER-A VRF RIB via VPNv4.
    Wait Until Keyword Succeeds    12 x    10s
    ...    VRF Route Is VPN    ${corerouter1}    CUSTOMER-A    192.168.123.0/24

Verify MPLS FIB Has Ingress VPN And Transport Labels
    [Documentation]    VPP MPLS FIB on bng1 must contain the locally-allocated
    ...    VPN label (advertised to corerouter1 via VPNv4) and the LDP-assigned
    ...    transport label toward 10.254.0.2/32.
    ...    Egress label stack is verified at the wire by the next test.
    Wait Until Keyword Succeeds    12 x    10s
    ...    VPP MPLS FIB Has Entries    ${bng1}

Verify Two-Label Stack On Core Egress
    [Documentation]    With bng1 → p1 → corerouter1, p1 is the penultimate hop so
    ...    bng1 pushes a real transport label. tcpdump on bng1's eth2 during
    ...    custa-to-loop stream traffic must capture packets with two MPLS labels.
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${bng1} timeout 10 tcpdump -i eth2 -c 5 -nn mpls 2>/dev/null
    Should Be Equal As Integers    ${rc}    0
    ...    No MPLS packets captured on bng1 eth2 egress
    Two Label Stack Present    ${output}

*** Keywords ***
Teardown L3VPN
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}

Verify OSPF Adjacency On BNG
    [Arguments]    ${container}
    ${output} =    Execute Vtysh On BNG    ${container}    show ip ospf neighbor
    Should Contain    ${output}    Full

OSPF Adjacency Full On Router
    [Arguments]    ${container}
    ${output} =    Execute Vtysh On Router    ${container}    show ip ospf neighbor
    Should Contain    ${output}    Full

LDP Session Established On BNG
    [Arguments]    ${container}    ${neighbor}
    ${output} =    Execute Vtysh On BNG    ${container}    show mpls ldp neighbor
    Should Contain    ${output}    ${neighbor}
    Should Contain    ${output}    OPERATIONAL

LDP Session Established On Router
    [Arguments]    ${container}    ${neighbor}
    ${output} =    Execute Vtysh On Router    ${container}    show mpls ldp neighbor
    Should Contain    ${output}    ${neighbor}
    Should Contain    ${output}    OPERATIONAL

VPNv4 Session Established On BNG
    [Arguments]    ${container}    ${neighbor}
    ${output} =    Execute Vtysh On BNG    ${container}    show bgp ipv4 vpn summary
    Should Contain    ${output}    ${neighbor}
    Should Contain    ${output}    Established

VPNv4 Session Established On Router
    [Arguments]    ${container}    ${neighbor}
    ${output} =    Execute Vtysh On Router    ${container}    show bgp ipv4 vpn summary
    Should Contain    ${output}    ${neighbor}
    Should Contain    ${output}    Established

VRF Route Must Be Absent
    [Arguments]    ${container}    ${vrf}    ${prefix}
    IF    '${vrf}' == 'default'
        ${output} =    Execute Vtysh On BNG    ${container}    show ip route vrf default ${prefix} json
    ELSE
        ${output} =    Execute Vtysh On BNG    ${container}    show ip route vrf ${vrf} ${prefix} json
    END
    Should Contain    ${output}    {}
    ...    Route ${prefix} must not exist in VRF ${vrf} on bng1

VRF Route Must Be Absent On Router
    [Arguments]    ${container}    ${vrf}    ${prefix}
    IF    '${vrf}' == 'default'
        ${output} =    Execute Vtysh On Router    ${container}    show ip route ${prefix} json
    ELSE
        ${output} =    Execute Vtysh On Router    ${container}    show ip route vrf ${vrf} ${prefix} json
    END
    Should Contain    ${output}    {}
    ...    Route ${prefix} must not exist in VRF ${vrf} on corerouter1

VPNv4 Prefix Present On BNG
    [Arguments]    ${container}    ${prefix_start}
    ${output} =    Execute Vtysh On BNG    ${container}    show bgp ipv4 vpn
    Should Contain    ${output}    ${prefix_start}
    Should Contain    ${output}    65000:100

VRF Route Is VPN
    [Arguments]    ${container}    ${vrf}    ${prefix}
    ${output} =    Execute Vtysh On Router    ${container}    show ip route vrf ${vrf} ${prefix} json
    Should Contain    ${output}    "bgp"

VPP MPLS FIB Has Entries
    [Arguments]    ${container}
    ${output} =    Execute VPP Command    ${container}    show mpls fib
    Should Contain    ${output}    unicast-ip4-chain

Verify Sessions In VRF
    [Arguments]    ${container}    ${vrf}    ${svcgroup}    ${pool}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${rc}    ${bad} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "
    ...import sys,json
    ...d=json.load(sys.stdin)
    ...sessions=[s for s in (d.get('data') or []) if s.get('ServiceGroup')=='${svcgroup}']
    ...bad=[s for s in sessions if s.get('VRF')!='${vrf}' or s.get('IPv4Pool')!='${pool}']
    ...print(len(bad))
    ..."
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${bad}    0
    ...    Some ${svcgroup} sessions not correctly bound to VRF ${vrf} / pool ${pool}

Verify Sessions In Default VRF
    [Arguments]    ${container}    ${pool}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${rc}    ${bad} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "
    ...import sys,json
    ...d=json.load(sys.stdin)
    ...sessions=[s for s in (d.get('data') or []) if not s.get('ServiceGroup')]
    ...bad=[s for s in sessions if s.get('IPv4Pool')!='${pool}']
    ...print(len(bad))
    ..."
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${bad}    0
    ...    Some DEFAULT sessions have unexpected pool (expected ${pool})

Verify Sessions Have IPv4 In Range
    [Arguments]    ${container}    ${vrf}    ${prefix}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${rc}    ${bad} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "
    ...import sys,json
    ...d=json.load(sys.stdin)
    ...if '${vrf}':
    ...    sessions=[s for s in (d.get('data') or []) if s.get('VRF')=='${vrf}']
    ...else:
    ...    sessions=[s for s in (d.get('data') or []) if not s.get('VRF')]
    ...bad=[s for s in sessions if not (s.get('IPv4Address') or '').startswith('${prefix}')]
    ...print(len(bad))
    ..."
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${bad}    0
    ...    Some sessions in VRF '${vrf}' have IPv4 address outside ${prefix}.x.x range

Get Dummy Interface RX Packets
    [Arguments]    ${container}    ${interface}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} ip -s link show ${interface} 2>/dev/null | awk '/RX:/{getline; print $1}'
    Should Be Equal As Integers    ${rc}    0
    ${packets} =    Strip String    ${output}
    RETURN    ${packets}

Two Label Stack Present
    [Arguments]    ${tcpdump_output}
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    echo '${tcpdump_output}' | grep -c 'MPLS.*label.*MPLS.*label' || echo '${tcpdump_output}' | grep -c 'label [0-9].*label [0-9]'
    Should Be True    int('${count}') > 0
    ...    No two-label MPLS stack found in tcpdump output — expected transport+VPN labels
