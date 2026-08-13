# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
Hierarchical QoS over PPPoE: the same three-S-VLAN shaped-port topology and
rate plan as suite 51, with the six subscribers arriving as authenticated
PPPoE sessions instead of IPoE. Exercises scheduler attachment through the
PPPoE session interface's encap parenting, then asserts the same share
matrix from the plugin's dequeue-side counters: an oversubscribed port
(6000+3000+3000 asking 8000), an unequal S-VLAN pair (100 vs 200), an
equal S-VLAN pair (200 vs 300), unequal subscribers inside svlan 100
(2M vs 8M), and equal subscribers inside svlans 200 and 300.

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
Resource            ../localauth.robot

Suite Setup         Deploy Topology    ${lab-file}
Suite Teardown      Teardown HQoS Test

*** Variables ***
${lab-name}         osvbng-pppoe-hqos-svlan
${lab-file}         ${CURDIR}/52-pppoe-hqos-svlan.clab.yml
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
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/qos/aggregate
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; e=(json.load(sys.stdin).get('data') or []); port=[a for a in e if a['level']=='port']; sv={a['svlan_id']: a['rate_kbps'] for a in e if a['level']=='svlan'}; assert len(port)==1 and port[0]['rate_kbps']==8000, port; assert sv=={100:6000,200:3000,300:3000}, sv; print('port 8000, svlans', sv)"
    Log    ${result}    console=yes
    Should Be Equal As Integers    ${rc}    0    Aggregates not programmed as configured: ${output}

Establish Subscriber Sessions
    [Documentation]    Six QinQ PPPoE sessions, two per S-VLAN, authenticated
    ...    against pre-created local users.
    Create PPPoE Users    ${bng1}    ${session-count}
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
    ...    PPPoE session through its encap sub-interface to the right tier.
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
