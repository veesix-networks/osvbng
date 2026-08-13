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

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot

Suite Setup         Deploy Topology    ${lab-file}
Suite Teardown      Teardown HQoS Test

*** Variables ***
${lab-name}         osvbng-ipoe-hqos-svlan
${lab-file}         ${CURDIR}/51-ipoe-hqos-svlan.clab.yml
${bng1}             clab-${lab-name}-bng1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    6
${measure-secs}     30
${churn-secs}       60

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
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/qos/aggregate
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; e=(json.load(sys.stdin).get('data') or []); port=[a for a in e if a['level']=='port']; sv={a['svlan_id']: a['rate_kbps'] for a in e if a['level']=='svlan'}; assert len(port)==1 and port[0]['rate_kbps']==8000, port; assert sv=={100:6000,200:3000,300:3000}, sv; print('port 8000, svlans', sv)"
    Log    ${result}    console=yes
    Should Be Equal As Integers    ${rc}    0    Aggregates not programmed as configured: ${output}

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

Verify HQoS Share Distribution
    [Documentation]    Under saturation the port splits 50/25/25 between the
    ...    S-VLANs and each S-VLAN splits by its subscribers' rates:
    ...    10/40/12.5x4 percent of the port. Measured from dequeue-side
    ...    counter deltas over ${measure-secs}s of bngblaster streams.
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    python3 ${CURDIR}/../hqos_check.py measure ${bng1} ${measure-secs}
    Log    \n${output}    console=yes
    Should Be Equal As Integers    ${rc}    0    HQoS shares out of tolerance:\n${output}

Churn Sessions With Monkey
    [Documentation]    Randomly kill and reconnect sessions for ${churn-secs}s.
    ...    Every kill tears a scheduler out of its S-VLAN aggregate and every
    ...    reconnect puts one back, which is the only way the weight accounting
    ...    and buffer-charge unwind paths get exercised repeatedly.
    Churn Sessions    ${subscribers}    ${churn-secs}
    Verify Sessions Flapped    ${subscribers}

Verify Sessions Re-Established After Churn
    [Documentation]    All ${session-count} sessions come back after the churn.
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}

Verify Schedulers Rebuilt After Churn
    [Documentation]    Exactly the original schedulers exist again - no
    ...    duplicates from a re-create that did not clean up, none missing from
    ...    a teardown that removed too much.
    Wait Until Keyword Succeeds    15 x    3s
    ...    Check Scheduler Rates    ${bng1}

Verify Attachment Intact After Churn
    [Documentation]    Each S-VLAN aggregate holds exactly its own two
    ...    schedulers again, nothing landed on the port, and no scheduler
    ...    outlived its attachment.
    Wait Until Keyword Succeeds    15 x    3s
    ...    Check HQoS Attachment    ${bng1}

Verify HQoS Shares Hold After Churn
    [Documentation]    Fairness is unchanged after churn: a leaked weight would
    ...    shrink every sibling's quantum and show up here as skewed shares.
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    python3 ${CURDIR}/../hqos_check.py measure ${bng1} ${measure-secs}
    Log    \n${output}    console=yes
    Should Be Equal As Integers    ${rc}    0    HQoS shares out of tolerance after churn:\n${output}

Verify Aggregates Drain On Teardown
    [Documentation]    With every session gone, no scheduler may survive and
    ...    every tier must have released its weight, its child count and its
    ...    buffer charge. This is the leak check the churn sets up.
    Stop BNG Blaster    ${subscribers}
    Wait Until Keyword Succeeds    30 x    2s
    ...    Check No Sessions Remain    ${bng1}
    Wait Until Keyword Succeeds    15 x    2s
    ...    Check Aggregates Drained    ${bng1}

*** Keywords ***
Check Scheduler Rates
    [Arguments]    ${container}
    ${output} =    Get osvbng API Response    ${container}    /api/show/qos/scheduler
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json,collections; e=(json.load(sys.stdin).get('data') or []); c=collections.Counter(s['rate_kbps'] for s in e); assert c=={2000:1,8000:1,4000:4}, c; print(dict(c))"
    Log    ${result}
    Should Be Equal As Integers    ${rc}    0    Scheduler rates wrong: ${output}

Check HQoS Attachment
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    python3 ${CURDIR}/../hqos_check.py attach ${container}
    Log    \n${output}    console=yes
    Should Be Equal As Integers    ${rc}    0    Scheduler attachment wrong:\n${output}

Check No Sessions Remain
    [Arguments]    ${container}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('data') or []))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Integers    ${count}    0    ${count} session(s) still present after teardown

Check Aggregates Drained
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    python3 ${CURDIR}/../hqos_check.py drained ${container}
    Log    \n${output}    console=yes
    Should Be Equal As Integers    ${rc}    0    Aggregates did not drain:\n${output}

Teardown HQoS Test
    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}
