# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            common.robot
Resource            bngblaster.robot

*** Keywords ***
BNG Blaster CLI Must Succeed
    [Arguments]    ${container}    ${command}
    ${rc}    ${output} =    BNG Blaster CLI Command    ${container}    ${command}
    Should Be Equal As Integers    ${rc}    0    ${command} failed: ${output}
    RETURN    ${output}

Session IPv4 Address
    [Arguments]    ${container}    ${session_id}
    ${output} =    BNG Blaster CLI Must Succeed    ${container}    session-info session-id ${session_id}
    ${rc}    ${ip} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json;d=json.load(sys.stdin);print(d.get('session-info',{}).get('ipv4-address') or '')"
    Should Be Equal As Integers    ${rc}    0
    Should Not Be Empty    ${ip}    session ${session_id} has no IPv4 address: ${output}
    RETURN    ${ip}

Flap Session
    [Arguments]    ${container}    ${session_id}    ${count}    ${settle}=0s
    [Documentation]    Stop and start one session ${count} times. Each stop
    ...                releases the lease and each start rediscovers, so the
    ...                BNG sees a release and a fresh session for the same
    ...                subscriber back to back.
    FOR    ${i}    IN RANGE    ${count}
        BNG Blaster CLI Must Succeed    ${container}    session-stop session-id ${session_id}
        Sleep    ${settle}
        BNG Blaster CLI Must Succeed    ${container}    session-start session-id ${session_id}
        Sleep    ${settle}
    END

Verify Dataplane Mapping Count For Inside IP
    [Arguments]    ${bng}    ${ip}    ${expected}
    [Documentation]    Counts the plugin's own table, not the control plane's
    ...                view: the failure this guards is a committed control
    ...                plane mapping with no dataplane entry behind it.
    ${output} =    Execute VPP Command    ${bng}    show osvbng cgnat mappings
    ${pattern} =    Regexp Escape    ${ip}
    ${lines} =    Get Lines Matching Regexp    ${output}    ^${pattern}\\s.*
    ${count} =    Get Line Count    ${lines}
    Should Be Equal As Integers    ${count}    ${expected}
    ...    dataplane holds ${count} mappings for ${ip}, expected ${expected}:\n${output}

Verify Control Plane Mapping Count For Inside IP
    [Arguments]    ${bng}    ${ip}    ${expected}
    ${output} =    Get osvbng API Response    ${bng}    /api/show/cgnat/mappings
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json;d=json.load(sys.stdin);m=d.get('data') if isinstance(d,dict) else d;print(len([x for x in (m or []) if x.get('inside_ip')=='${ip}']))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Integers    ${count}    ${expected}
    ...    control plane holds ${count} mappings for ${ip}, expected ${expected}:\n${output}

Verify Control Plane Mapping Count
    [Arguments]    ${bng}    ${expected}
    ${output} =    Get osvbng API Response    ${bng}    /api/show/cgnat/mappings
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json;d=json.load(sys.stdin);m=d.get('data') if isinstance(d,dict) else d;print(len(m or []))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Integers    ${count}    ${expected}
    ...    control plane holds ${count} mappings, expected ${expected}:\n${output}
