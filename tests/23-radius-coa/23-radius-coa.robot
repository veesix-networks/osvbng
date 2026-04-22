# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
RADIUS CoA and Disconnect-Message test suite.
Establishes 10 IPoE sessions via RADIUS with session traffic, exercises
CoA-Request (attribute change on a random session), then Disconnect-Request
on a different random session. Validates that the remaining 9 sessions are
still active in both BNG Blaster and osvbng, and that the disconnected
session is gone from both.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot

Suite Setup         Deploy Topology    ${lab-file}
Suite Teardown      Teardown CoA Test

*** Variables ***
${lab-name}         osvbng-radius-coa
${lab-file}         ${CURDIR}/23-radius-coa.clab.yml
${bng1}             clab-${lab-name}-bng1
${subscribers}      clab-${lab-name}-subscribers
${freeradius}       clab-${lab-name}-freeradius
${session-count}    10
${bng1-mgmt-ip}    172.20.20.2
${coa-secret}       testing123

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify VPP Is Running
    [Documentation]    Check VPP is running and responsive.
    ${output} =    Execute VPP Command    ${bng1}    show version
    Should Contain    ${output}    vpp

Establish Subscriber Sessions
    [Documentation]    Start BNG Blaster with 10 IPoE sessions and session traffic.
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}    check_ipv6=true

Verify All Sessions In osvbng API
    [Documentation]    Verify osvbng REST API reports all 10 sessions.
    Verify Sessions In API    ${bng1}    ${session-count}

Verify Traffic Flowing
    [Documentation]    Verify session traffic is flowing for all 10 sessions.
    Wait Until Keyword Succeeds    30s    5s
    ...    Verify Traffic Flowing    ${subscribers}    ${session-count}

Pick Random Session For CoA
    [Documentation]    Pick a random session and store its Acct-Session-Id for CoA testing.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    ${rc}    ${acct_id} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json,random; d=json.load(sys.stdin); sessions=d.get('data',[]); s=random.choice(sessions); print(s.get('AAASessionID',''))"
    Should Be Equal As Integers    ${rc}    0
    Should Not Be Empty    ${acct_id}
    Set Suite Variable    ${COA_ACCT_SESSION_ID}    ${acct_id}
    Log    Selected session for CoA: ${acct_id}    console=yes

Send CoA-Request With Session-Timeout
    [Documentation]    Send a CoA-Request to change Session-Timeout on the selected session.
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${freeradius} bash -c "echo 'Acct-Session-Id = ${COA_ACCT_SESSION_ID}, Session-Timeout = 7200' | radclient -x ${bng1-mgmt-ip}:3799 coa ${coa-secret}"
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0
    Should Contain    ${output}    CoA-ACK

Verify CoA Updated Only Target Session
    [Documentation]    Verify Session-Timeout was changed to 7200 on the CoA target only.
    Sleep    2s
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); sessions=d.get('data',[]); target='${COA_ACCT_SESSION_ID}'; matched=[s for s in sessions if s.get('AAASessionID')==target]; other=[s for s in sessions if s.get('AAASessionID')!=target]; t_val=(matched[0].get('Attributes') or {}).get('session_timeout','') if matched else ''; o_changed=[s for s in other if (s.get('Attributes') or {}).get('session_timeout','')=='7200']; print(f'{t_val} {len(o_changed)}')"
    Should Be Equal As Integers    ${rc}    0
    @{parts} =    Split String    ${result}
    Should Be Equal As Strings    ${parts}[0]    7200    CoA target session_timeout should be 7200
    Should Be Equal As Strings    ${parts}[1]    0    No other sessions should have session_timeout changed to 7200

Verify All Sessions Still Active After CoA
    [Documentation]    All 10 sessions should still be active after CoA.
    Verify Sessions In API    ${bng1}    ${session-count}

Send CoA-Request To Non-Existent Session
    [Documentation]    CoA to a non-existent session should return NAK.
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${freeradius} bash -c "echo 'Acct-Session-Id = does-not-exist, Session-Timeout = 3600' | radclient -x ${bng1-mgmt-ip}:3799 coa ${coa-secret}"
    Log    ${output}
    Should Contain    ${output}    CoA-NAK

Pick Random Session For Disconnect
    [Documentation]    Pick a different random session for Disconnect testing.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json,random; d=json.load(sys.stdin); sessions=d.get('data',[]); candidates=[s for s in sessions if s.get('AAASessionID')!='${COA_ACCT_SESSION_ID}']; s=random.choice(candidates); print(s.get('AAASessionID',''))"
    Should Be Equal As Integers    ${rc}    0
    Should Not Be Empty    ${result}
    Set Suite Variable    ${DISCONNECT_ACCT_SESSION_ID}    ${result}
    Log    Selected session for Disconnect: ${result}    console=yes

Send Disconnect-Request
    [Documentation]    Send a Disconnect-Request to tear down the selected session.
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${freeradius} bash -c "echo 'Acct-Session-Id = ${DISCONNECT_ACCT_SESSION_ID}' | radclient -x ${bng1-mgmt-ip}:3799 disconnect ${coa-secret}"
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0
    Should Contain    ${output}    Disconnect-ACK

Verify osvbng Shows 9 Sessions
    [Documentation]    After disconnect, osvbng should report exactly 9 active sessions.
    Sleep    15s
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('data',[])))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${count}    9

Verify Disconnected Session Is Gone From osvbng
    [Documentation]    The specific disconnected session should not appear in the API.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    ${rc}    ${found} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); sessions=d.get('data',[]); matches=[s for s in sessions if s.get('AAASessionID')=='${DISCONNECT_ACCT_SESSION_ID}']; print(len(matches))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${found}    0    Disconnected session ${DISCONNECT_ACCT_SESSION_ID} should not be in API

Verify Disconnected Session Not Reachable
    [Documentation]    Verify the disconnected session's VPP FIB entry was removed.
    ${output} =    Execute VPP Command    ${bng1}    show ip fib
    ${rc}    ${fib_count} =    Run And Return Rc And Output
    ...    echo '${output}' | grep -cP "10\\.255\\.0\\.\\d+/32"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${fib_count}    10    Expected 10 FIB entries (9 subscribers + 1 gateway) but got ${fib_count}

Verify CoA Stats
    [Documentation]    Verify CoA stats show the expected counts.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/aaa/radius/coa
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); s=d.get('data',[]); acks=sum(x.get('coa_acks',0) for x in s); naks=sum(x.get('coa_naks',0) for x in s); dacks=sum(x.get('disconnect_acks',0) for x in s); print(f'{acks} {naks} {dacks}')"
    Should Be Equal As Integers    ${rc}    0
    @{parts} =    Split String    ${result}
    Should Be True    ${parts}[0] >= 1    Expected at least 1 CoA ACK
    Should Be True    ${parts}[1] >= 1    Expected at least 1 CoA NAK
    Should Be True    ${parts}[2] >= 1    Expected at least 1 Disconnect ACK

*** Keywords ***
Teardown CoA Test
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}
