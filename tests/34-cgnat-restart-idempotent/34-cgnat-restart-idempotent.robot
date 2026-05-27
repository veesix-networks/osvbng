# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot
Resource            ../restart.robot

Suite Setup         Setup Restart Suite
Suite Teardown      Teardown Restart Suite

*** Variables ***
${lab-name}         osvbng-cgnat-restart-idempotent
${lab-file}         ${CURDIR}/34-cgnat-restart-idempotent.clab.yml
${bng1}             clab-${lab-name}-bng1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    5
${cfg-live}         ${CURDIR}/config/bng1/osvbng.yaml
${cfg-backup}       /tmp/osvbng-restart-${lab-name}-orig.yaml

*** Test Cases ***
Identical Config Across Restart
    [Documentation]    Daemon restart with unchanged YAML completes; no -116, no replace, no orphan.
    Wait For osvbng Healthy    bng1    ${lab-name}
    Start BNG Blaster In Background    ${subscribers}
    Wait Until Keyword Succeeds    18 x    10s
    ...    Verify Sessions In API    ${bng1}    ${session-count}
    Restart osvbngd    ${bng1}
    Wait For osvbngd Down    ${bng1}
    Wait For osvbngd Up    ${bng1}
    Wait For osvbng Healthy    bng1    ${lab-name}
    ${log} =    Tail osvbngd Log    ${bng1}    400
    Should Not Contain    ${log}    cgnat reconcile: replace pool
    Should Not Contain    ${log}    cgnat reconcile: drop orphan
    Should Not Contain    ${log}    retval=-116

Soft Drift Preserves Mappings
    [Documentation]    Edit tcp_established timeout, restart, verify soft-update WARN logged.
    Wait For osvbng Healthy    bng1    ${lab-name}
    Edit Live Config Inline    s|tcp-established:.*|tcp-established: 3600|
    Restart osvbngd    ${bng1}
    Wait For osvbngd Down    ${bng1}
    Wait For osvbngd Up    ${bng1}
    Wait For osvbng Healthy    bng1    ${lab-name}
    ${log} =    Tail osvbngd Log    ${bng1}    400
    Should Contain    ${log}    cgnat reconcile: soft-update pool

# Hard-drift replace + preflight-gate scenarios are exercised by
# internal/cgnat/reconcile_test.go. Reproducing them deterministically here
# requires either (a) non-graceful shutdown so the pool stays populated, or
# (b) traffic streams so active_mappings stays non-zero across restart.
# Both are tracked as follow-ups; the unit tests fully cover the logic.

*** Keywords ***
Setup Restart Suite
    Copy File    ${cfg-live}    ${cfg-backup}
    Deploy Topology    ${lab-file}

Teardown Restart Suite
    Destroy Topology    ${lab-file}
    Run Keyword And Ignore Error    Copy File    ${cfg-backup}    ${cfg-live}

Edit Live Config Inline
    [Arguments]    ${sed_expr}
    ${result} =    Run Process    sed    -i    ${sed_expr}    ${cfg-live}
    Should Be Equal As Integers    ${result.rc}    0    sed failed: ${result.stderr}

Stop osvbngd Hard
    [Arguments]    ${container}
    Run And Return Rc    sudo docker exec ${container} sh -c 'pkill -KILL osvbngd || true'

Try Start osvbngd
    [Arguments]    ${container}    ${timeout}=20
    Run And Return Rc
    ...    sudo docker exec -d ${container} sh -c '/usr/local/bin/osvbngd -c /etc/osvbng/osvbng.yaml >/var/log/osvbngd-restart.log 2>&1'
    Sleep    ${timeout}s
    ${rc} =    Run And Return Rc    sudo docker exec ${container} pgrep -x osvbngd
    RETURN    ${rc}

Wait For osvbngd Up
    [Arguments]    ${container}    ${timeout}=60s    ${interval}=2s
    Wait Until Keyword Succeeds    ${timeout}    ${interval}    Check osvbngd Running    ${container}

Check osvbngd Running
    [Arguments]    ${container}
    ${rc} =    Run And Return Rc    sudo docker exec ${container} pgrep -x osvbngd
    Should Be Equal As Integers    ${rc}    0    osvbngd not running

Tail osvbngd Log
    [Arguments]    ${container}    ${lines}=200
    ${result} =    Run Process
    ...    sudo    docker    logs    --tail    ${lines}    ${container}
    ...    shell=False    stderr=STDOUT
    RETURN    ${result.stdout}

Log Contains
    [Arguments]    ${container}    ${needle}
    ${log} =    Tail osvbngd Log    ${container}    600
    Should Contain    ${log}    ${needle}

Grep osvbngd Log Must Contain
    [Arguments]    ${container}    ${needle}
    ${rc} =    Run And Return Rc
    ...    sudo docker logs ${container} 2>&1 | grep -F -- "${needle}"
    Should Be Equal As Integers    ${rc}    0    Log did not contain: ${needle}
