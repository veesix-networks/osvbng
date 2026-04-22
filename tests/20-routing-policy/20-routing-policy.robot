# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
Routing policy functional test suite.
Verifies prefix-set filtering, community tagging/stripping,
AS-path filtering/prepend, local-preference, large communities,
and redistribute route-policy via a real eBGP peering with an FRR peer.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot

Suite Setup         Deploy Routing Policy Topology
Suite Teardown      Teardown Routing Policy Topology

*** Variables ***
${lab-name}         osvbng-routing-policy
${lab-file}         ${CURDIR}/20-routing-policy.clab.yml
${bng1}             clab-${lab-name}-bng1
${corerouter1}      clab-${lab-name}-corerouter1

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify FRR Is Running On BNG
    [Documentation]    Check FRR is running inside the dataplane netns on bng1.
    ${output} =    Execute Vtysh On BNG    ${bng1}    show version
    Should Contain    ${output}    FRR

Verify BGP Session Established
    [Documentation]    Wait for eBGP session between bng1 and corerouter1.
    Wait Until Keyword Succeeds    12 x    10s
    ...    Verify BGP Session On Router    ${corerouter1}    10.0.0.1

# --- Section 1: Prefix-Set Filtering (Inbound) ---

Verify Allowed Prefix 192.0.2.0/24 Accepted
    [Documentation]    Prefix in ALLOWED set should be accepted by PEER-IN policy.
    Wait Until Keyword Succeeds    6 x    10s
    ...    Check BGP Route On BNG    ${bng1}    192.0.2.0/24

Verify Allowed Prefix 198.51.100.0/24 Accepted
    [Documentation]    Second allowed prefix should also be accepted.
    Wait Until Keyword Succeeds    6 x    10s
    ...    Check BGP Route On BNG    ${bng1}    198.51.100.0/24

Verify Bogon 10.0.0.0/8 Rejected
    [Documentation]    RFC1918 prefix should be rejected by BOGONS prefix-set match.
    Sleep    5s    Wait for routes to settle
    ${output} =    Execute Vtysh On BNG    ${bng1}    show ip bgp 10.0.0.0/8
    Should Not Contain    ${output}    10.0.0.0/8

Verify Bogon 172.16.0.0/12 Rejected
    [Documentation]    RFC1918 prefix should be rejected by BOGONS prefix-set match.
    ${output} =    Execute Vtysh On BNG    ${bng1}    show ip bgp 172.16.0.0/12
    Should Not Contain    ${output}    172.16.0.0/12

# --- Section 2: Community-Based Blackholing ---

Verify Blackhole Prefix Rejected
    [Documentation]    Prefix tagged with community 64500:666 should be denied.
    ${output} =    Execute Vtysh On BNG    ${bng1}    show ip bgp 203.0.113.1/32
    Should Not Contain    ${output}    203.0.113.1/32

# --- Section 3: AS-Path Filtering ---

Verify Private ASN Prefix Rejected
    [Documentation]    Prefix with private ASN 64512 in path should be denied.
    ${output} =    Execute Vtysh On BNG    ${bng1}    show ip bgp 198.18.0.0/15
    Should Not Contain    ${output}    198.18.0.0/15

# --- Section 4: Local-Preference Setting ---

Verify Local Preference Set To 150
    [Documentation]    Accepted routes should have local-preference 150.
    ${output} =    Execute Vtysh On BNG    ${bng1}    show ip bgp 192.0.2.0/24 json
    Should Contain    ${output}    150

# --- Section 5: Community Tagging (Inbound) ---

Verify Standard Community Tagged
    [Documentation]    Accepted routes should be tagged with community 64500:100.
    ${output} =    Execute Vtysh On BNG    ${bng1}    show ip bgp 192.0.2.0/24 json
    Should Contain    ${output}    64500:100

# --- Section 6: Large Community Tagging ---

Verify Large Community Tagged
    [Documentation]    Accepted routes should have large community 64500:100:200.
    ${output} =    Execute Vtysh On BNG    ${bng1}    show ip bgp 192.0.2.0/24 json
    Should Contain    ${output}    64500:100:200

# --- Section 7: AS-Path Prepend (Outbound) ---

Verify AS Path Prepend On Export
    [Documentation]    corerouter1 should see bng1's exported prefix with prepended AS path.
    Wait Until Keyword Succeeds    6 x    10s
    ...    Check BGP Route On Router    ${corerouter1}    203.0.113.0/24
    ${output} =    Execute Vtysh On Router    ${corerouter1}    show ip bgp 203.0.113.0/24
    Should Contain    ${output}    64500 64500 64500

# --- Section 8: Community Stripping (Outbound) ---

Verify Internal Community Stripped On Export
    [Documentation]    Exported routes should NOT carry internal community 64500:999.
    ${output} =    Execute Vtysh On Router    ${corerouter1}    show ip bgp 203.0.113.0/24 json
    Should Not Contain    ${output}    64500:999

# --- Section 9: Redistribute with Route-Policy ---

Verify Redistribute Filtered
    [Documentation]    Only prefixes matching REDIST-FILTER should appear on corerouter1.
    ...    The loopback 10.254.0.1/32 and link 10.0.0.0/30 should NOT be redistributed.
    ${output} =    Execute Vtysh On Router    ${corerouter1}    show ip bgp
    Should Not Contain    ${output}    10.254.0.1/32
    Should Not Contain    ${output}    10.0.0.0/30

# --- Section 10: FRR Config Rendering Order ---

Verify Policy Objects Rendered Before BGP
    [Documentation]    Routing policy objects must appear before router bgp in FRR config.
    ${output} =    Execute Vtysh On BNG    ${bng1}    show running-config
    ${policy_pos} =    Get Regexp Match Position    ${output}    ip prefix-list
    ${bgp_pos} =    Get Regexp Match Position    ${output}    router bgp
    Should Be True    ${policy_pos} < ${bgp_pos}

Verify Prefix List Rendered
    [Documentation]    FRR running config should contain ip prefix-list entries.
    ${output} =    Execute Vtysh On BNG    ${bng1}    show running-config
    Should Contain    ${output}    ip prefix-list BOGONS-V4
    Should Contain    ${output}    ip prefix-list OWN-PREFIXES-V4

Verify Community List Rendered
    [Documentation]    FRR running config should contain community-list entries.
    ${output} =    Execute Vtysh On BNG    ${bng1}    show running-config
    Should Contain    ${output}    bgp community-list standard BLACKHOLE
    Should Contain    ${output}    bgp community-list standard PEER-TAGGED

Verify Large Community List Rendered
    [Documentation]    FRR running config should contain large-community-list entries.
    ${output} =    Execute Vtysh On BNG    ${bng1}    show running-config
    Should Contain    ${output}    bgp large-community-list standard LC-PEER-TAGGED

Verify AS Path Access List Rendered
    [Documentation]    FRR running config should contain as-path access-list entries.
    ${output} =    Execute Vtysh On BNG    ${bng1}    show running-config
    Should Contain    ${output}    bgp as-path access-list PRIVATE-ASN

Verify Route Map Rendered
    [Documentation]    FRR running config should contain route-map entries.
    ${output} =    Execute Vtysh On BNG    ${bng1}    show running-config
    Should Contain    ${output}    route-map PEER-IN
    Should Contain    ${output}    route-map PEER-OUT
    Should Contain    ${output}    route-map REDIST-FILTER

*** Keywords ***
Deploy Routing Policy Topology
    Deploy Topology    ${lab-file}

Teardown Routing Policy Topology
    Destroy Topology    ${lab-file}

Check BGP Route On BNG
    [Arguments]    ${container}    ${prefix}
    ${output} =    Execute Vtysh On BNG    ${container}    show ip bgp ${prefix}
    Should Contain    ${output}    ${prefix}    BGP route ${prefix} not found on BNG

Get Regexp Match Position
    [Arguments]    ${string}    ${pattern}
    ${result} =    Evaluate    __import__('re').search(r'${pattern}', '''${string}''').start()
    RETURN    ${result}
