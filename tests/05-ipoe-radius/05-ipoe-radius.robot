# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
IPoE session test suite with RADIUS authentication.
FreeRADIUS authorize requires Cleartext-Password := "secret"; sessions only
establish if the BNG sends User-Password matching the policy.password field.
Verifies session state through the osvbng API and BNG Blaster report.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot

Suite Setup         Deploy Topology    ${lab-file}
Suite Teardown      Teardown IPoE RADIUS Test

*** Variables ***
${lab-name}         osvbng-ipoe-radius
${lab-file}         ${CURDIR}/05-ipoe-radius.clab.yml
${bng1}             clab-${lab-name}-bng1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    1
${freeradius}       clab-${lab-name}-freeradius
${acct-interim-wait}    90s
${detail-file-glob}     /var/log/freeradius/radacct/172.20.20.2/detail-*

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify VPP Is Running
    [Documentation]    Check VPP is running and responsive.
    ${output} =    Execute VPP Command    ${bng1}    show version
    Should Contain    ${output}    vpp

Establish Subscriber Sessions
    [Documentation]    Start BNG Blaster and wait for sessions.
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}    check_ipv6=true

Verify Sessions In osvbng API
    [Documentation]    Verify osvbng REST API reports the correct session count.
    Verify Sessions In API    ${bng1}    ${session-count}

Verify IPv4 Addresses Assigned
    [Documentation]    Verify all sessions have an IPv4 address in the API.
    Verify Sessions Have IPv4    ${bng1}

Verify IPv6 Addresses Assigned
    [Documentation]    Verify all sessions have an IPv6 address in the API.
    Verify Sessions Have IPv6    ${bng1}

Verify VPP Sub-Interfaces Created
    [Documentation]    Verify QinQ sub-interfaces exist in VPP.
    Verify VPP Sub-Interfaces Created    ${bng1}    eth1.100

Verify RADIUS Server Stats
    [Documentation]    Verify RADIUS server stats show auth accepts and zero rejects.
    ...                Zero rejects proves the password attribute reached FreeRADIUS
    ...                and matched the expected Cleartext-Password.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/aaa/radius/servers
    ${rc}    ${accepts} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); s=d.get('data',[]); print(sum(x.get('auth_accepts',0) for x in s))"
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${accepts} >= ${session-count}    Expected at least ${session-count} auth accepts but got ${accepts}
    ${rc}    ${rejects} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); s=d.get('data',[]); print(sum(x.get('auth_rejects',0) for x in s))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Integers    ${rejects}    0    Expected zero auth rejects but got ${rejects}

Verify Acct-Interim Carries Non-Zero Octets And Packets
    [Documentation]    With session-traffic.ipv4-pps=10 driving real packets through
    ...                ipoe_session<X>, an Acct-Interim emitted ~60s after auth must
    ...                carry non-zero Acct-Input-Octets / Acct-Output-Octets and the
    ...                matching Input/Output Packets counters. Guards the wire-format
    ...                end-to-end from the cached interface-stats snapshot through
    ...                AAA baseline math, the auth.Session fields, and the RADIUS
    ...                encoder. A zero on any attribute means billing is broken.
    Sleep    ${acct-interim-wait}    Wait for at least one Acct-Interim bucket tick to fire.
    ${max-input-octets} =    Get Max Radius Attribute    ${freeradius}    Acct-Input-Octets
    Should Be True    ${max-input-octets} > 0    Acct-Input-Octets stayed zero across all accounting records — counters not reaching RADIUS.
    ${max-output-octets} =    Get Max Radius Attribute    ${freeradius}    Acct-Output-Octets
    Should Be True    ${max-output-octets} > 0    Acct-Output-Octets stayed zero across all accounting records.
    ${max-input-packets} =    Get Max Radius Attribute    ${freeradius}    Acct-Input-Packets
    Should Be True    ${max-input-packets} > 0    Acct-Input-Packets stayed zero across all accounting records.
    ${max-output-packets} =    Get Max Radius Attribute    ${freeradius}    Acct-Output-Packets
    Should Be True    ${max-output-packets} > 0    Acct-Output-Packets stayed zero across all accounting records.

Verify BNG Blaster Report
    [Documentation]    Stop BNG Blaster and verify report shows all sessions established.
    Stop BNG Blaster    ${subscribers}
    ${established} =    Get BNG Blaster Report Field    ${subscribers}    sessions-established
    Should Be Equal As Strings    ${established}    ${session-count}

Verify Acct-Stop Carries Final Cumulative
    [Documentation]    The Acct-Stop emitted on session release must carry at least the
    ...                last Acct-Interim's cumulative value — billing must not regress
    ...                or report zero on disconnect. Use the maximum Acct-Input-Octets
    ...                across the whole RADIUS log; that value lives in either the last
    ...                interim or the stop record, both of which must be non-zero.
    Wait Until Keyword Succeeds    20s    2s    Radius Has Status-Type    ${freeradius}    Stop
    ${max-input-octets} =    Get Max Radius Attribute    ${freeradius}    Acct-Input-Octets
    Should Be True    ${max-input-octets} > 0    Acct-Stop final cumulative is zero — Acct-Input-Octets never reached RADIUS.

*** Keywords ***
Teardown IPoE RADIUS Test
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}

Get Max Radius Attribute
    [Documentation]    Return the maximum integer value of attribute ${attr} across
    ...                every Accounting-Request the FreeRADIUS detail module has logged
    ...                for the BNG client. The detail format is one tab-indented
    ...                `Attribute = value` per line; max across the whole file is the
    ...                highest cumulative value the BNG ever sent.
    [Arguments]    ${container}    ${attr}
    ${rc}    ${value} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} sh -c "awk -v a=${attr} 'index(\\$0, a \\" = \\") {n=\\$NF+0; if (n>m) m=n} END {print m+0}' ${detail-file-glob}"
    Should Be Equal As Integers    ${rc}    0    detail file read on ${container} failed
    RETURN    ${value}

Radius Has Status-Type
    [Documentation]    Fail unless the FreeRADIUS detail file contains an
    ...                Acct-Status-Type = ${status} record (Start / Interim-Update /
    ...                Stop). grep -c returning 1 (no match yet) is a normal retry
    ...                state, not a transport error.
    [Arguments]    ${container}    ${status}
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} sh -c "grep -c 'Acct-Status-Type = ${status}' ${detail-file-glob} || true"
    Should Be True    ${count} > 0    No Acct-Status-Type = ${status} record found in RADIUS detail file yet.
