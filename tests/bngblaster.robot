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
    ...    timeout ${timeout} sudo docker exec ${container} bngblaster -C ${config} -j ${report} -L /tmp/bngblaster.log
    Log    ${output}
    RETURN    ${rc}    ${output}

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
    ...    sudo docker exec ${container} bngblaster-cli -s ${socket} ${command}
    Log    ${output}
    RETURN    ${rc}    ${output}
