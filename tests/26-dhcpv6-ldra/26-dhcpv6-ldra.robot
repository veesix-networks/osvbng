# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
DHCPv6 LDRA termination suite. bngblaster acts as an RFC 6221 LDRA
("ldra": true), wrapping subscriber Solicit/Rebind in Relay-Forward with
Interface-ID and Remote-Id before sending upstream to osvbng. osvbng's
local DHCPv6 provider must emit matching Relay-Reply responses so the
LDRA can route replies to the correct downstream port.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot
Resource            ../localauth.robot

Suite Setup         Deploy LDRA Topology
Suite Teardown      Teardown LDRA Test

*** Variables ***
${lab-name}         osvbng-dhcpv6-ldra
${lab-file}         ${CURDIR}/26-dhcpv6-ldra.clab.yml
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

Establish LDRA-Fronted Sessions
    [Documentation]    Bngblaster wraps subscriber DHCPv6 in Relay-Forward; osvbng terminates locally.
    Create IPoE Users    ${bng1}    ${session-count}
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}    check_ipv6=true

Verify Sessions In osvbng API
    [Documentation]    Verify osvbng REST API reports the correct session count.
    Verify Sessions In API    ${bng1}    ${session-count}

Verify IPv6 IANA And PD Assigned
    [Documentation]    Verify DHCPv6 IANA address and PD prefix were allocated.
    Verify Sessions Have IPv6    ${bng1}

Verify BNG Blaster Report
    [Documentation]    Bngblaster's report must show all wrapped sessions established.
    Stop BNG Blaster    ${subscribers}
    ${established} =    Get BNG Blaster Report Field    ${subscribers}    sessions-established
    Should Be Equal As Strings    ${established}    ${session-count}

*** Keywords ***
Deploy LDRA Topology
    Deploy Topology    ${lab-file}

Teardown LDRA Test
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}
