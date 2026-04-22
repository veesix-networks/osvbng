# Copyright 2025 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process

*** Keywords ***
Start BNG Blaster
    [Arguments]    ${container}    ${config}=/config/config.json    ${report}=/tmp/report.json    ${timeout}=120
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} timeout --signal=INT ${timeout} bngblaster -C ${config} -J ${report} -L /tmp/bngblaster.log -b -f
    Log    ${output}
    RETURN    ${rc}    ${output}

Start BNG Blaster In Background
    [Arguments]    ${container}    ${config}=/config/config.json    ${report}=/tmp/report.json    ${socket}=/run/bngblaster.sock
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec -d ${container} bngblaster -C ${config} -J ${report} -L /tmp/bngblaster.log -S ${socket} -b -f
    Should Be Equal As Integers    ${rc}    0
    Wait Until Keyword Succeeds    30 x    2s
    ...    BNG Blaster Is Running    ${container}

Stop BNG Blaster
    [Arguments]    ${container}
    Run And Return Rc And Output
    ...    sudo docker exec ${container} bash -c 'kill -INT $(pidof bngblaster) 2>/dev/null'
    Wait Until Keyword Succeeds    30s    1s
    ...    BNG Blaster Has Exited    ${container}

BNG Blaster Is Running
    [Arguments]    ${container}    ${socket}=/run/bngblaster.sock
    ${rc}    ${output} =    BNG Blaster CLI Command    ${container}    session-counters    ${socket}
    Should Be Equal As Integers    ${rc}    0    BNG Blaster control socket not ready yet

BNG Blaster Has Exited
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} pidof bngblaster
    Should Not Be Equal As Integers    ${rc}    0

Get BNG Blaster Report
    [Arguments]    ${container}    ${report}=/tmp/report.json
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} cat ${report}
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${output}

Verify BNG Blaster Sessions Established
    [Arguments]    ${container}    ${expected_count}    ${report}=/tmp/report.json
    ${report_json} =    Get BNG Blaster Report    ${container}    ${report}
    ${rc}    ${established} =    Run And Return Rc And Output
    ...    echo '${report_json}' | python3 -c "import sys,json; r=json.load(sys.stdin); print(r.get('report',{}).get('sessions-established',0))"
    Should Be Equal As Strings    ${established}    ${expected_count}

BNG Blaster CLI Command
    [Arguments]    ${container}    ${command}    ${socket}=/run/bngblaster.sock
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} /usr/sbin/bngblaster-cli ${socket} ${command}
    Log    ${output}
    RETURN    ${rc}    ${output}

Verify Traffic Flowing
    [Arguments]    ${container}    ${expected_flows}=5
    [Documentation]    Verify all BNG Blaster session-traffic flows (both directions) are verified.
    ${expected_total} =    Evaluate    ${expected_flows} * 2
    ${rc}    ${output} =    BNG Blaster CLI Command    ${container}    session-counters
    Should Be Equal As Integers    ${rc}    0    session-counters CLI failed
    Log    \nSession Counters:\n${output}    console=yes
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json;d=json.load(sys.stdin);c=d.get('session-counters',{});flows=c.get('session-traffic-flows',0);verified=c.get('session-traffic-flows-verified',0);print('%d %d'%(flows,verified))"
    Should Be Equal As Integers    ${rc}    0
    @{parts} =    Split String    ${result}
    Should Be True    ${parts}[0] == ${expected_total}    Expected ${expected_total} flows (${expected_flows} sessions x 2 directions) but got ${parts}[0]
    Should Be True    ${parts}[1] == ${expected_total}    Only ${parts}[1]/${expected_total} traffic flows verified — traffic not flowing bidirectionally through BNG

Verify Stream Traffic Flowing
    [Arguments]    ${container}    ${expected_flows}=5
    [Documentation]    Verify BNG Blaster stream-traffic flows are verified (NAT-aware streams).
    ${expected_total} =    Evaluate    ${expected_flows} * 2
    ${rc}    ${output} =    BNG Blaster CLI Command    ${container}    session-counters
    Should Be Equal As Integers    ${rc}    0    session-counters CLI failed
    Log    \nSession Counters:\n${output}    console=yes
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json;d=json.load(sys.stdin);c=d.get('session-counters',{});flows=c.get('stream-traffic-flows',0);verified=c.get('stream-traffic-flows-verified',0);print('%d %d'%(flows,verified))"
    Should Be Equal As Integers    ${rc}    0
    @{parts} =    Split String    ${result}
    Should Be True    ${parts}[0] >= ${expected_total}    Expected at least ${expected_total} stream flows but got ${parts}[0]
    Should Be True    ${parts}[1] >= ${expected_total}    Only ${parts}[1]/${expected_total} stream flows verified — NAT traffic not flowing bidirectionally
