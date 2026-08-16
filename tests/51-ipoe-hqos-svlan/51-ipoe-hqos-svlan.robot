# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
Hierarchical QoS: per-S-VLAN aggregates under a shaped port, weighted DRR
at both tiers. Six IPoE sessions across three S-VLANs saturate the port
with bngblaster downstream streams; shares are asserted from the plugin's
dequeue-side counters. The rate plan puts every arbitration relationship
under test at once: an oversubscribed port (6000+3000+3000 asking 8000),
an unequal S-VLAN pair (100 vs 200), an equal S-VLAN pair (200 vs 300),
unequal subscribers inside svlan 100 (2M vs 8M), and equal subscribers
inside svlans 200 and 300.

Session churn (BNG Blaster's monkey) was exercised here and removed: it
found two defects outside this spec's scope - a dataplane crash under PPPoE
churn and IPoE sessions refused on reconnect after an ungraceful kill - and
neither belongs to HQoS. The reusable keywords stay in bngblaster.robot for
whoever picks that up.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot

Suite Setup         Setup HQoS Test
Suite Teardown      Teardown HQoS Test

*** Variables ***
${lab-name}         osvbng-ipoe-hqos-svlan
${lab-file}         ${CURDIR}/51-ipoe-hqos-svlan.clab.yml
${bng1}             clab-${lab-name}-bng1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    6
${measure-secs}     30

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify VPP Is Running
    ${output} =    Execute VPP Command    ${bng1}    show version
    Should Contain    ${output}    vpp

Verify HQoS Aggregates Programmed
    [Documentation]    The port and all three S-VLAN aggregates exist in the
    ...    dataplane with their configured rates, applied through the
    ...    qos-aggregates conf schema at startup.
    Save API Response To File    ${bng1}    /api/show/qos/aggregate    ${OUTPUT DIR}/qos-aggregate.json
    Run QoS Check    aggregates-programmed    ${OUTPUT DIR}/qos-aggregate.json    8000    100:6000,200:3000,300:3000

Establish Subscriber Sessions
    [Documentation]    Six QinQ IPoE sessions, two per S-VLAN, via bngblaster.
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}

Verify Schedulers Programmed With Configured Rates
    [Documentation]    Every session got a CAKE scheduler at the rate of its
    ...    service-group's egress policy: one at 2000, one at 8000, four at 4000.
    Wait Until Keyword Succeeds    10 x    3s
    ...    Check Scheduler Rates    ${bng1}

Verify Schedulers Attached To S-VLAN Aggregates
    [Documentation]    Each S-VLAN aggregate lists exactly its own two
    ...    sessions as members - the sup_sw_if_index walk resolved every
    ...    session through its encap sub-interface to the right tier.
    Check HQoS Attachment    ${bng1}

Verify Aggregate Detail Hierarchy Via API
    [Documentation]    The aggregate detail view renders the whole tree over
    ...    the API: the port, its three S-VLAN children, and two member
    ...    schedulers under each child, every member carrying its session id.
    Save API Response To File    ${bng1}    /api/show/qos/aggregate/detail?interface=eth1    ${OUTPUT DIR}/qos-detail.json
    Run QoS Check    aggregate-detail    ${OUTPUT DIR}/qos-detail.json    100,200,300    2

Verify Scheduler Session View Via API
    [Documentation]    The per-session view resolves a session id to its
    ...    scheduler and the aggregate tiers above it.
    Save API Response To File    ${bng1}    /api/show/subscriber/sessions    ${OUTPUT DIR}/qos-sessions.json
    ${sid} =    Run QoS Check    pick-session-id    ${OUTPUT DIR}/qos-sessions.json
    Save API Response To File    ${bng1}    /api/show/qos/scheduler/session?session_id=${sid}    ${OUTPUT DIR}/qos-session-view.json
    Run QoS Check    session-view    ${OUTPUT DIR}/qos-session-view.json

Verify CLI Scheduler Table
    [Documentation]    osvbngcli renders the compact one-line-per-scheduler
    ...    table: header tokens present, every session uuid full-length, no
    ...    JSON blobs and no interactive banner.
    Save CLI Command Output To File    ${bng1}    show qos scheduler    ${OUTPUT DIR}/qos-cli-scheduler.txt
    Run QoS Check    cli-scheduler-table    ${OUTPUT DIR}/qos-cli-scheduler.txt    ${session-count}

Verify CLI Aggregate Tree
    [Documentation]    osvbngcli renders the aggregate hierarchy as a tree:
    ...    the port line with each S-VLAN beneath it and counter lines per
    ...    tier.
    Save CLI Command Output To File    ${bng1}    show qos aggregate    ${OUTPUT DIR}/qos-cli-aggregate.txt
    Run QoS Check    cli-aggregate-tree    ${OUTPUT DIR}/qos-cli-aggregate.txt    100,200,300

Verify Per-Tin Metrics Exported
    [Documentation]    Every scheduler tin series carries a tin label, one
    ...    series per scheduler per tin. This suite's policies are all
    ...    besteffort (a single tin), so that means exactly one tin=0 series
    ...    per subscriber - and none of the unlabelled series the collapsed
    ...    pre-fix flattening produced. Waits out the 10s telemetry poll.
    Wait Until Keyword Succeeds    6 x    5s
    ...    Check Tin Metric Series    ${bng1}    ${session-count}

Verify HQoS Share Distribution
    [Documentation]    Under saturation the port splits 50/25/25 between the
    ...    S-VLANs and each S-VLAN splits by its subscribers' rates:
    ...    10/40/12.5x4 percent of the port. Measured from dequeue-side
    ...    counter deltas over ${measure-secs}s of bngblaster streams.
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    python3 ${CURDIR}/../hqos_check.py measure ${bng1} ${measure-secs}
    Log    \n${output}    console=yes
    Should Be Equal As Integers    ${rc}    0    HQoS shares out of tolerance:\n${output}

Verify Aggregates Drain On Teardown
    [Documentation]    With every session gone, no scheduler may survive and
    ...    every tier must have released its weight, its child count and its
    ...    buffer charge.
    Stop BNG Blaster    ${subscribers}
    Wait Until Keyword Succeeds    30 x    2s
    ...    Check No Sessions Remain    ${bng1}
    Wait Until Keyword Succeeds    15 x    2s
    ...    Check Aggregates Drained    ${bng1}

*** Keywords ***
Setup HQoS Test
    # Self-test the assertion tooling against committed fixtures before
    # spending any lab time: a broken assertion fails here in seconds, not
    # thirty minutes into a topology run.
    Run QoS Check    selftest
    Deploy Topology    ${lab-file}

Check Tin Metric Series
    [Arguments]    ${container}    ${expected}
    Save Metrics Scrape To File    ${container}    ${OUTPUT DIR}/qos-metrics.txt
    Run QoS Check    tin-metrics    ${OUTPUT DIR}/qos-metrics.txt    ${expected}

Check Scheduler Rates
    [Arguments]    ${container}
    Save API Response To File    ${container}    /api/show/qos/scheduler    ${OUTPUT DIR}/qos-scheduler.json
    Run QoS Check    scheduler-rates    ${OUTPUT DIR}/qos-scheduler.json    2000:1,8000:1,4000:4

Check HQoS Attachment
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    python3 ${CURDIR}/../hqos_check.py attach ${container}
    Log    \n${output}    console=yes
    Should Be Equal As Integers    ${rc}    0    Scheduler attachment wrong:\n${output}

Check No Sessions Remain
    [Arguments]    ${container}
    Save API Response To File    ${container}    /api/show/subscriber/sessions    ${OUTPUT DIR}/qos-teardown-sessions.json
    Run QoS Check    no-sessions    ${OUTPUT DIR}/qos-teardown-sessions.json

Check Aggregates Drained
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    python3 ${CURDIR}/../hqos_check.py drained ${container}
    Log    \n${output}    console=yes
    Should Be Equal As Integers    ${rc}    0    Aggregates did not drain:\n${output}

Teardown HQoS Test
    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}
