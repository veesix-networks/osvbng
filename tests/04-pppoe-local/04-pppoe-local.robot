# Copyright 2025 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
PPPoE session test suite with local auth (allow_all: false).
Creates a user with password via REST API, establishes a PPPoE session
via BNG Blaster with PAP authentication, and verifies session state.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot
Resource            ../localauth.robot

Suite Setup         Deploy Topology    ${lab-file}
Suite Teardown      Teardown PPPoE Test

*** Variables ***
${lab-name}         osvbng-pppoe-local
${lab-file}         ${CURDIR}/04-pppoe-local.clab.yml
${bng1}             clab-${lab-name}-bng1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    1

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify VPP Is Running
    [Documentation]    Check VPP is running and responsive.
    ${output} =    Execute VPP Command    ${bng1}    show version
    Should Contain    ${output}    vpp

Establish Subscriber Sessions
    [Documentation]    Create local auth users and start PPPoE sessions.
    Create PPPoE Users    ${bng1}    ${session-count}
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}    check_ipv6=true

Verify Sessions In osvbng API
    [Documentation]    Verify osvbng REST API reports the correct session count.
    Verify Sessions In API    ${bng1}    ${session-count}

Verify IPv4 Addresses Assigned
    [Documentation]    Verify all PPPoE sessions have an IPv4 address in the API.
    Verify Sessions Have IPv4    ${bng1}

Verify IPv6 Addresses Assigned
    [Documentation]    Verify all PPPoE sessions have an IPv6 address in the API.
    Verify Sessions Have IPv6    ${bng1}

Verify IPv6 Bindings In VPP FIB
    [Documentation]    The delegated /56 must exist as a route in the VPP
    ...    FIB, not only in the API view: the API stayed green while the
    ...    dataplane binding calls failed against a plugin that lacked the
    ...    messages. DHCPv6 starts off the initial RA burst and completes
    ...    seconds after establishment, so the check retries briefly. The
    ...    IA_NA /128 is not asserted because bngblaster requests IA_NA
    ...    only on IPoE access (hard-gated in its source), so a PPPoE
    ...    client here never asks for an address; the IA_NA bind path is
    ...    covered by TestPPPoEDHCPv6BindProgramsDataplane instead.
    Wait Until Keyword Succeeds    30 x    2s
    ...    Check IPv6 Bindings In VPP FIB    ${bng1}    ${session-count}

Verify VPP Sub-Interfaces Created
    [Documentation]    Verify PPPoE sub-interfaces exist in VPP.
    Verify VPP Sub-Interfaces Created    ${bng1}    eth1.200

Verify BNG Blaster Report
    [Documentation]    Stop BNG Blaster and verify report shows all sessions established.
    Stop BNG Blaster    ${subscribers}
    ${established} =    Get BNG Blaster Report Field    ${subscribers}    sessions-established
    Should Be Equal As Strings    ${established}    ${session-count}

*** Keywords ***
Check IPv6 Bindings In VPP FIB
    [Arguments]    ${container}    ${count}
    ${output} =    Execute VPP Command    ${container}    show ip6 fib
    ${pd} =    Get Regexp Matches    ${output}    2001:db8:1[0-9a-f:]*/56
    Length Should Be    ${pd}    ${count}    delegated /56 missing from VPP ip6 fib

Teardown PPPoE Test
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}
