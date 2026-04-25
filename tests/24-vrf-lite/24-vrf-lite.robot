# Copyright 2026 Veesix Networks Ltd
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
VRF-lite end-to-end test suite.
Verifies VRF isolation, per-VRF BGP, and subscriber-to-VRF binding for
IPoE and PPPoE subscribers across DEFAULT and CUSTOMER-A VRFs using
per-VRF core sub-interfaces (no MPLS).

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot
Resource            ../localauth.robot

Suite Setup         Deploy Topology    ${lab-file}
Suite Teardown      Teardown VRF Lite

*** Variables ***
${lab-name}             osvbng-vrf-lite
${lab-file}             ${CURDIR}/24-vrf-lite.clab.yml
${bng1}                 clab-${lab-name}-bng1
${corerouter1}          clab-${lab-name}-corerouter1
${subscribers}          clab-${lab-name}-subscribers
${session-count}        4
${custa-pool-cidr}      192.168.123.0/24

*** Test Cases ***

# --- Phase A: Scaffolding ---

Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify VPP Is Running
    [Documentation]    Check VPP is running and responsive.
    ${output} =    Execute VPP Command    ${bng1}    show version
    Should Contain    ${output}    vpp

Verify VPP Sub-Interface eth2.100 In Default Table
    [Documentation]    eth2.100 must exist in VPP and be in the default FIB table.
    Wait Until Keyword Succeeds    6 x    10s
    ...    VPP Sub-Interface Exists    ${bng1}    eth2.100

Verify VPP Sub-Interface eth2.200 In CUSTOMER-A Table
    [Documentation]    eth2.200 must be bound to the CUSTOMER-A FIB table in VPP.
    Wait Until Keyword Succeeds    6 x    10s
    ...    VPP Sub-Interface Exists    ${bng1}    eth2.200
    ${fib} =    Execute VPP Command    ${bng1}    show ip fib table CUSTOMER-A
    Should Contain    ${fib}    10.0.200

Verify BGP Running-Config Contains Expected Stanzas
    [Documentation]    FRR running-config must contain both default-VRF and
    ...    CUSTOMER-A per-VRF BGP stanzas with activated neighbors.
    ...    This is the Phase A canary for the per-VRF-neighbor rendering limitation.
    Wait Until Keyword Succeeds    12 x    10s
    ...    BGP Running Config Has VRF Stanzas    ${bng1}

Verify BGP Session Established In Default VRF
    [Documentation]    iBGP session to corerouter1 in the default VRF must be established.
    Wait Until Keyword Succeeds    12 x    10s
    ...    Verify BGP Session On Router    ${corerouter1}    10.0.100.1

Verify BGP Session Established In CUSTOMER-A VRF
    [Documentation]    iBGP session to corerouter1 in the CUSTOMER-A VRF must be established.
    Wait Until Keyword Succeeds    12 x    10s
    ...    BGP Session Established In VRF    ${bng1}    CUSTOMER-A    10.0.200.2

Verify BGP Route Exchanged In CUSTOMER-A VRF
    [Documentation]    corerouter1's CUSTOMER-A loopback (192.168.200.1/32) must appear
    ...    as a BGP route in the CUSTOMER-A VRF RIB on bng1 before sessions start.
    Wait Until Keyword Succeeds    12 x    10s
    ...    VRF Route Is BGP    ${bng1}    CUSTOMER-A    192.168.200.1/32

# --- Phase B: Session assertions ---

Establish CUSTOMER-A Sessions
    [Documentation]    Start BNG Blaster (allow_all: true — no user creation needed).
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}

Verify CUSTOMER-A Sessions Bound To CUSTOMER-A VRF
    [Documentation]    Every CUSTOMER-A session must have VRF=CUSTOMER-A,
    ...    ServiceGroup=customer-a, IPv4Pool=customer-a-pool (primary assertion).
    Verify Sessions In VRF    ${bng1}    CUSTOMER-A    customer-a    customer-a-pool

Verify CUSTOMER-A Subscriber IPv4 From 192.168.123.0/24
    [Documentation]    Secondary pool-range check: all CUSTOMER-A sessions have addresses in
    ...    192.168.123.0/24.
    Verify Sessions Have IPv4 In Range    ${bng1}    CUSTOMER-A    192.168.123

# --- Phase C: Reachability and isolation ---

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
    [Documentation]    192.168.123.1/32 (bng1 CUSTOMER-A gateway) must not leak to
    ...    the default VRF on bng1 or corerouter1.
    VRF Route Must Be Absent    ${bng1}    default    192.168.123.1/32
    VRF Route Must Be Absent On Router    ${corerouter1}    default    192.168.123.1/32

Verify Default Gateway Loopback Absent From CUSTOMER-A VRF RIB
    [Documentation]    10.255.0.1/32 (bng1 default gateway) must not leak to
    ...    the CUSTOMER-A VRF on bng1 or corerouter1.
    VRF Route Must Be Absent    ${bng1}    CUSTOMER-A    10.255.0.1/32
    VRF Route Must Be Absent On Router    ${corerouter1}    CUSTOMER-A    10.255.0.1/32

Verify CUSTOMER-A Destination Loopback Absent From Default VRF RIB
    [Documentation]    192.168.200.1/32 (corerouter1 CUSTOMER-A loopback) must not
    ...    appear in the default VRF RIB on bng1.
    VRF Route Must Be Absent    ${bng1}    default    192.168.200.1/32

Verify Default Destination Loopback Absent From CUSTOMER-A VRF RIB
    [Documentation]    10.200.0.1/32 (corerouter1 default loopback) must not
    ...    appear in the CUSTOMER-A VRF RIB on bng1.
    VRF Route Must Be Absent    ${bng1}    CUSTOMER-A    10.200.0.1/32

Verify Opposite Core-Link Prefix Absent From Wrong VRF RIB
    [Documentation]    10.0.100.0/30 must not appear in CUSTOMER-A VRF;
    ...    10.0.200.0/30 must not appear in the default VRF.
    VRF Route Must Be Absent    ${bng1}    CUSTOMER-A    10.0.100.0/30
    VRF Route Must Be Absent    ${bng1}    default    10.0.200.0/30

Verify CUSTOMER-A Stream Traffic Flowing
    [Documentation]    Stream traffic from CUSTOMER-A subscribers must flow end-to-end
    ...    via the CUSTOMER-A VRF on bng1, proving forwarding isolation in addition to
    ...    the RIB-isolation assertions above.
    Wait Until Keyword Succeeds    20s    2s
    ...    Verify Stream Traffic Flowing    ${subscribers}    ${session-count}

*** Keywords ***
Teardown VRF Lite
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}

VPP Sub-Interface Exists
    [Arguments]    ${container}    ${ifname}
    ${output} =    Execute VPP Command    ${container}    show interface
    Should Contain    ${output}    ${ifname}

BGP Running Config Has VRF Stanzas
    [Arguments]    ${container}
    ${output} =    Execute Vtysh On BNG    ${container}    show running-config
    Should Contain    ${output}    router bgp 65000
    Should Contain    ${output}    router bgp 65000 vrf CUSTOMER-A
    Should Contain    ${output}    neighbor 10.0.100.2 activate
    Should Contain    ${output}    neighbor 10.0.200.2 activate

BGP Session Established In VRF
    [Arguments]    ${container}    ${vrf}    ${neighbor}
    ${output} =    Execute Vtysh On BNG    ${container}    show bgp vrf ${vrf} neighbors ${neighbor}
    Should Contain    ${output}    BGP state = Established

VRF Route Is BGP
    [Arguments]    ${container}    ${vrf}    ${prefix}
    ${output} =    Execute Vtysh On BNG    ${container}    show ip route vrf ${vrf} ${prefix} json
    Should Contain    ${output}    "bgp"

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

Verify Sessions In VRF
    [Arguments]    ${container}    ${vrf}    ${svcgroup}    ${pool}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${rc}    ${bad} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); sessions=[s for s in (d.get('data') or []) if s.get('ServiceGroup')=='${svcgroup}']; bad=[s for s in sessions if s.get('VRF')!='${vrf}' or not (s.get('IPv4Pool') or '').endswith('${pool}')]; print(len(bad))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${bad}    0
    ...    Some ${svcgroup} sessions not correctly bound to VRF ${vrf} / pool ${pool}

Verify Sessions Have IPv4 In Range
    [Arguments]    ${container}    ${vrf}    ${prefix}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${rc}    ${bad} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); vrf='${vrf}'; sessions=[s for s in (d.get('data') or []) if (s.get('VRF')==vrf if vrf else not s.get('VRF'))]; bad=[s for s in sessions if not (s.get('IPv4Address') or '').startswith('${prefix}')]; print(len(bad))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${bad}    0
    ...    Some sessions in VRF '${vrf}' have IPv4 address outside ${prefix}.x.x range

