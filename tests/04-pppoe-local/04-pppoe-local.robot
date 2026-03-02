# Copyright 2025 Veesix Networks Ltd
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
PPPoE session test suite with local auth (allow_all: false).
Creates 69 users with password via REST API, establishes PPPoE sessions
via BNG Blaster with PAP authentication, and verifies session state.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot
Resource            ../localauth.robot

Suite Setup         Setup PPPoE Test
Suite Teardown      Teardown PPPoE Test

*** Variables ***
${lab-name}         osvbng-pppoe-local
${lab-file}         ${CURDIR}/04-pppoe-local.clab.yml
${bng1}             clab-${lab-name}-bng1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    69

*** Test Cases ***
Verify Sessions In osvbng API
    [Documentation]    Verify osvbng REST API reports the correct session count.
    Verify Sessions In API    ${bng1}    ${session-count}

Verify IPCP Addresses Assigned
    [Documentation]    Verify all PPPoE sessions received an IPv4 address via IPCP.
    Verify Sessions Have IPv4    ${bng1}

Verify VPP Sub-Interfaces Created
    [Documentation]    Verify PPPoE sub-interfaces exist in VPP.
    Verify VPP Sub-Interfaces Created    ${bng1}    eth1.200

Verify BNG Blaster Report
    [Documentation]    Stop BNG Blaster and verify report shows all sessions established.
    Stop BNG Blaster    ${subscribers}
    ${established} =    Get BNG Blaster Report Field    ${subscribers}    sessions-established
    Should Be Equal As Strings    ${established}    ${session-count}

*** Keywords ***
Setup PPPoE Test
    Deploy Topology    ${lab-file}
    Wait For osvbng Healthy    bng1    ${lab-name}
    Create PPPoE Users    ${bng1}    ${session-count}
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${session-count}

Teardown PPPoE Test
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}
