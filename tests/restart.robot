# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            common.robot

*** Variables ***
${RESPAWN_TIMEOUT}      75s
${RESPAWN_INTERVAL}     1s
${VPP_RECOVERY_TIMEOUT}    90s
${VPP_RECOVERY_INTERVAL}   2s

*** Keywords ***
Restart osvbngd
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} sh -c 'pkill -TERM osvbngd'
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0    Failed to signal osvbngd

Wait For osvbngd Down
    [Arguments]    ${container}    ${timeout}=${RESPAWN_TIMEOUT}    ${interval}=${RESPAWN_INTERVAL}
    Wait Until Keyword Succeeds    ${timeout}    ${interval}
    ...    Check osvbngd Not Running    ${container}

Check osvbngd Not Running
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} pgrep -x osvbngd
    Should Not Be Equal As Integers    ${rc}    0    osvbngd still running

Restart VPP
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} sh -c 'pkill -KILL vpp_main'
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0    Failed to signal vpp_main

Wait For VPP Recovery
    [Arguments]    ${container}    ${timeout}=${VPP_RECOVERY_TIMEOUT}    ${interval}=${VPP_RECOVERY_INTERVAL}
    Wait Until Keyword Succeeds    ${timeout}    ${interval}
    ...    Check VPP Responsive    ${container}

Check VPP Responsive
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} vppctl -s ${VPPCTL_SOCK} show version
    Should Be Equal As Integers    ${rc}    0    VPP not responsive
    Should Contain    ${output}    vpp

Restart BNG Container
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker restart ${container}
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0    docker restart failed
