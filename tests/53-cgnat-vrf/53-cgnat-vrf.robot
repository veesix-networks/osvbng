# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
CGNAT for subscribers in customer VRFs.
An IPoE group in CUSTOMER-A (dual-family) and a PPPoE group in CUSTOMER-B
(IPv4 only) share the inside prefix 100.64.0.0/24 and one outside pool in
the default VRF, so the same inside address is live in both VRFs at once.
Each session must get its own mapping keyed by its VRF with a disjoint
port block, NAT traffic must flow end to end in both VRFs, and teardown
(DHCP release and PADT) must release the block under the same (vrf,
inside ip) key it was allocated under.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot

Suite Setup         Deploy Topology    ${lab-file}
Suite Teardown      Teardown CGNAT VRF Topology

*** Variables ***
${lab-name}         osvbng-cgnat-vrf
${lab-file}         ${CURDIR}/53-cgnat-vrf.clab.yml
${bng1}             clab-${lab-name}-bng1
${corerouter1}      clab-${lab-name}-corerouter1
${subscribers}      clab-${lab-name}-subscribers
${sessions-per-vrf}    3
${session-count}    6
${stream-count}     2

*** Test Cases ***
Verify BNG Is Healthy
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify CGNAT Plugin Loaded
    ${output} =    Execute VPP Command    ${bng1}    show plugins
    Should Contain    ${output}    osvbng_cgnat

Verify OSPF Adjacency Established
    Wait Until Keyword Succeeds    12 x    10s
    ...    Verify OSPF Adjacency On Router    ${corerouter1}

Verify BGP Session Established
    Wait Until Keyword Succeeds    12 x    10s
    ...    Verify BGP Session On Router    ${corerouter1}    10.254.0.1

Verify Gateway Address Exists In Both Customer VRFs
    [Documentation]    100.64.0.1/32 is the subscriber gateway in CUSTOMER-A and
    ...    in CUSTOMER-B, so the two VRFs overlap before any session exists.
    Wait Until Keyword Succeeds    6 x    10s
    ...    VRF Has Route    ${bng1}    CUSTOMER-A    100.64.0.1/32
    Wait Until Keyword Succeeds    6 x    10s
    ...    VRF Has Route    ${bng1}    CUSTOMER-B    100.64.0.1/32

Start Subscriber Sessions
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}

Verify Sessions Bound To Their Customer VRF
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; ss=json.load(sys.stdin).get('data') or []; a=[s for s in ss if s.get('ServiceGroup')=='customer-a']; b=[s for s in ss if s.get('ServiceGroup')=='customer-b']; print(len(a), len(b), len([s for s in a if s.get('VRF')!='CUSTOMER-A']), len([s for s in b if s.get('VRF')!='CUSTOMER-B']))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${result}    ${sessions-per-vrf} ${sessions-per-vrf} 0 0
    ...    Expected ${sessions-per-vrf} sessions per service group, each in its VRF; got (a, b, a-wrong-vrf, b-wrong-vrf) = ${result}

Verify The Same Inside Address Is Live In Both VRFs
    [Documentation]    The collision ADR 0006 exists for: without it the
    ...    per-VRF mapping assertions below would pass for the wrong reason.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    ${rc}    ${shared} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; ss=json.load(sys.stdin).get('data') or []; a={s.get('IPv4Address') for s in ss if s.get('VRF')=='CUSTOMER-A'}; b={s.get('IPv4Address') for s in ss if s.get('VRF')=='CUSTOMER-B'}; print(len(a&b))"
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${shared} > 0    No inside address is shared between the two VRFs, the collision case is not exercised

Verify Every Mapping Is Keyed By Its Session VRF
    [Documentation]    One mapping per session, none under table 0, two
    ...    distinct VRF ids, never two mappings for one inside address in
    ...    the same VRF, and every (outside ip, port block) unique.
    Wait Until Keyword Succeeds    6 x    5s
    ...    Mappings Keyed By VRF    ${bng1}    ${session-count}

Verify VPP Holds One Mapping Per Session
    ${output} =    Execute VPP Command    ${bng1}    show osvbng cgnat
    Should Contain    ${output}    Mappings: ${session-count}

Verify NAT Traffic Flowing In Both VRFs
    [Documentation]    UDP and raw-TCP NAT streams verify bidirectionally for
    ...    every session; flows = sessions x streams, doubled for direction
    ...    by the keyword.
    ${expected} =    Evaluate    ${session-count} * ${stream-count}
    Wait Until Keyword Succeeds    6 x    10s
    ...    Verify Stream Traffic Flowing    ${subscribers}    expected_flows=${expected}

Verify CGNAT Session Dump Lists Active Translations
    Wait Until Keyword Succeeds    6 x    10s
    ...    Session Dump Has Active Flows    ${bng1}

Verify Outside Addresses Advertised Via BGP
    ${output} =    Execute Vtysh On Router    ${corerouter1}    show ip bgp
    Should Contain    ${output}    203.0.113.

Release Sessions
    [Documentation]    Stopping BNG Blaster releases every DHCP lease; the
    ...    sessions must be gone before the mapping check means anything.
    Stop BNG Blaster    ${subscribers}
    Wait Until Keyword Succeeds    12 x    5s
    ...    Verify Sessions In API    ${bng1}    0

Verify Mappings Released In Both VRFs
    [Documentation]    Release looks the block up under the session's VRF; a
    ...    release keyed by table 0 would miss every mapping and leak it.
    Wait Until Keyword Succeeds    12 x    5s
    ...    No Mappings Remain    ${bng1}

*** Keywords ***
VRF Has Route
    [Arguments]    ${container}    ${vrf}    ${prefix}
    ${output} =    Execute Vtysh On BNG    ${container}    show ip route vrf ${vrf} ${prefix} json
    Should Contain    ${output}    "${prefix}"

Mappings Keyed By VRF
    [Arguments]    ${container}    ${expected}
    ${output} =    Get osvbng API Response    ${container}    /api/show/cgnat/mappings
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; ms=json.load(sys.stdin).get('data') or []; byip={}; [byip.setdefault(m['inside_ip'],[]).append(m['inside_vrf_id']) for m in ms]; vrfs={m['inside_vrf_id'] for m in ms}; dup=[ip for ip,v in byip.items() if len(v)!=len(set(v))]; blocks={(m['outside_ip'],m['port_block_start']) for m in ms}; print('ok' if len(ms)==${expected} and 0 not in vrfs and len(vrfs)==2 and not dup and len(blocks)==len(ms) else 'mappings=%d vrfs=%s dup_vrf_per_ip=%s distinct_blocks=%d' % (len(ms), sorted(vrfs), dup, len(blocks)))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${result}    ok    Mappings are not keyed by VRF: ${result}

Session Dump Has Active Flows
    [Arguments]    ${bng}
    ${output} =    Get osvbng API Response    ${bng}    /api/show/cgnat/sessions
    Should Contain    ${output}    "sessions"
    Should Contain    ${output}    100.64.
    Should Contain    ${output}    203.0.113.

No Mappings Remain
    [Arguments]    ${container}
    ${output} =    Get osvbng API Response    ${container}    /api/show/cgnat/mappings
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('data') or []))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${count}    0    ${count} mappings still held after every session released
    ${vpp} =    Execute VPP Command    ${container}    show osvbng cgnat
    Should Contain    ${vpp}    Mappings: 0

Teardown CGNAT VRF Topology
    Run Keyword And Ignore Error    Dump VPP Trace    ${bng1}
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}
