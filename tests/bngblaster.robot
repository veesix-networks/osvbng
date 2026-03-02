# Copyright 2025 Veesix Networks Ltd
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

Stop BNG Blaster
    [Arguments]    ${container}
    Run And Return Rc And Output
    ...    sudo docker exec ${container} bash -c 'kill -INT $(pidof bngblaster) 2>/dev/null'
    Wait Until Keyword Succeeds    30s    1s
    ...    BNG Blaster Has Exited    ${container}

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
