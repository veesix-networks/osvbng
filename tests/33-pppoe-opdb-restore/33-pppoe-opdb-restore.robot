# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
opdb restore validation for PPPoE — sticky pppd client across three restart
flavors:
  2a — osvbngd-only (VPP intact; restoreSessions re-attaches by sw_if_index)
  2b — VPP-only (osvbngd intact, watchdog respawn) — SKIPPED. PPPoE recovery
       from a local VPP crash is not implemented today: stale-sw_if_index
       opdb entries are deleted rather than recreated via AddPPPoESession,
       so the session is silently wiped. When that path lands, drop the
       Skip statement from this test case.
  2c — full container restart (both fresh; opdb survives docker restart)

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../sessions.robot
Resource            ../localauth.robot
Resource            ../restart.robot

Suite Setup         Suite Setup PPPoE OpDB Restore
Suite Teardown      Suite Teardown PPPoE OpDB Restore

*** Variables ***
${lab-name}             osvbng-pppoe-opdb-restore
${lab-file}             ${CURDIR}/33-pppoe-opdb-restore.clab.yml
${bng1}                 clab-${lab-name}-bng1
${subscribers}          clab-${lab-name}-subscribers
${corerouter1}          clab-${lab-name}-corerouter1
${subscriber-image}     veesixnetworks/bngtester:debian-latest
${v4-gateway}           10.64.64.64
${v6-gateway}           2001:db8:0:1::1
${session-count}        1

*** Test Cases ***
2a — Restart osvbngd Only
    [Documentation]    Kill osvbngd, OSVBNG_RESPAWN wrapper respawns it.
    ...    VPP keeps running so PPPoE restoreSessions re-attaches by
    ...    sw_if_index (validIfIndexes branch). pppd LCP echoes should
    ...    ride through the ~30s window without re-PADI.
    ...
    ...    IPv6 ping verification deferred: RAs are emitted from the
    ...    global loopback source IPv6 rather than a link-local, so
    ...    Linux subscribers per RFC 4861 §6.1.2 discard them and never
    ...    install the on-link prefix. Add the v6 ping check back once
    ...    RA source IP is link-local.
    Restart osvbngd                                ${bng1}
    Wait For osvbngd Down                          ${bng1}
    Wait For osvbng Healthy                        bng1    ${lab-name}
    Verify OpDB Sessions Match Snapshot            ${bng1}    ${OPDB_SNAPSHOT}
    Verify PPPoE Session Has Not Re-Established
    Verify Subscriber Can Ping                     ${v4-gateway}

2b — Restart VPP Only
    [Documentation]    SKIPPED — PPPoE recovery from a local VPP crash is
    ...    not implemented: stale-sw_if_index opdb entries are deleted
    ...    rather than recreated via AddPPPoESession. The test
    ...    infrastructure lands here so that when the recovery path ships,
    ...    its PR only needs to delete the Skip statement below.
    [Tags]    skip    pppoe-vpp-recovery-pending
    Skip    PPPoE VPP-crash recovery not implemented — see 2b documentation
    Restart VPP                                    ${bng1}
    Wait For VPP Recovery                          ${bng1}
    Wait Until Keyword Succeeds    60s    2s
    ...    Verify OpDB Session Count               ${bng1}    ${session-count}
    Verify OpDB Sessions Match Snapshot            ${bng1}    ${OPDB_SNAPSHOT}
    Verify PPPoE Session Has Not Re-Established
    Verify Subscriber Can Ping                     ${v4-gateway}
    Verify Subscriber Can Ping                     ${v6-gateway}    -6

2c — Restart Full Container
    [Documentation]    SKIPPED — containerlab veth pairs to access/core do
    ...    not survive a docker restart of the BNG container, so the BNG
    ...    entrypoint hangs waiting for eth1/eth2 after the restart and
    ...    osvbngd never starts. Drop the Skip once the restart mechanism
    ...    preserves access links (or substitute a different cold-restart
    ...    approach).
    [Tags]    skip    clab-veth-detach
    Skip    docker restart drops the clab-managed veth pairs to subscribers and corerouter, so the BNG entrypoint hangs waiting for access interfaces and osvbngd never restarts. Drop this Skip once the restart mechanism preserves access links (or substitute a different cold-restart approach).
    Restart BNG Container                          ${bng1}
    Wait For osvbng Healthy                        bng1    ${lab-name}
    Verify OpDB Sessions Match Snapshot            ${bng1}    ${OPDB_SNAPSHOT}
    Verify PPPoE Session Has Not Re-Established
    Verify Subscriber Can Ping                     ${v4-gateway}

*** Keywords ***
Suite Setup PPPoE OpDB Restore
    Set Environment Variable        BNGTESTER_IMAGE    ${subscriber-image}
    Deploy Topology                 ${lab-file}
    Wait For osvbng Healthy         bng1    ${lab-name}
    Create PPPoE Users              ${bng1}    ${session-count}
    Wait Until Keyword Succeeds    90s    5s
    ...    Verify Subscriber PPP Up
    Wait Until Keyword Succeeds    60s    2s
    ...    Verify OpDB Session Count    ${bng1}    ${session-count}
    Capture PPP Establishment Marker
    ${snapshot} =    Snapshot OpDB Sessions    ${bng1}
    Set Suite Variable    ${OPDB_SNAPSHOT}    ${snapshot}

Suite Teardown PPPoE OpDB Restore
    Destroy Topology    ${lab-file}

Verify Subscriber PPP Up
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${subscribers} sh -c 'ip -o link show | grep -E "ppp[0-9]+" || true'
    Should Be Equal As Integers    ${rc}    0
    Should Not Be Empty    ${output}    No ppp interface yet
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${subscribers} sh -c 'ip -4 addr show dev ppp0 2>/dev/null'
    Should Be Equal As Integers    ${rc}    0
    Should Contain    ${output}    inet    No IPv4 on ppp0 yet

Capture PPP Establishment Marker
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    sudo docker exec ${subscribers} sh -c 'grep -c "PAP authentication succeeded" /var/log/pppd.log 2>/dev/null || echo 0'
    Should Be Equal As Integers    ${rc}    0
    Set Suite Variable    ${PPP_AUTH_COUNT}    ${count.strip()}
    Log    Pre-restart PAP-success count: ${PPP_AUTH_COUNT}

Verify PPPoE Session Has Not Re-Established
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    sudo docker exec ${subscribers} sh -c 'grep -c "PAP authentication succeeded" /var/log/pppd.log 2>/dev/null || echo 0'
    Should Be Equal As Integers    ${rc}    0
    ${count} =    Set Variable    ${count.strip()}
    Log    Post-restart PAP-success count: ${count} (pre: ${PPP_AUTH_COUNT})
    Should Be Equal As Strings    ${count}    ${PPP_AUTH_COUNT}
    ...    pppd ran PAP again during the restart window — session was not sticky

Verify Subscriber Can Ping
    [Arguments]    ${target}    ${flag}=${EMPTY}
    Wait Until Keyword Succeeds    45s    3s
    ...    Subscriber Ping    ${target}    ${flag}

Subscriber Ping
    [Arguments]    ${target}    ${flag}=${EMPTY}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${subscribers} ping ${flag} -c 3 -W 2 -I ppp0 ${target}
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0    Cannot ping ${target} from subscriber ppp0
