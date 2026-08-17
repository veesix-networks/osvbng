# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
IPoE session churn: eight dual-stack dot1ad/dot1q QinQ subscribers driven
through BNG Blaster's monkey, which kills sessions by several methods
including an ungraceful kill that sends no DHCP RELEASE. Every subscriber
must come back, every time.

This is the suite 51 deferred - its header records finding "IPoE sessions
refused on reconnect after an ungraceful kill" while exercising monkey for
HQoS, out of scope there, keywords left in bngblaster.robot for whoever
picked it up.

What made that bite was the per-subscriber session limit counting from a
cache counter that only the DHCPv4 and DHCPv6 RELEASE handlers
decremented. A subscriber the monkey killed ungracefully was reaped by
cleanupSessions, which released its addresses, its lease, its VPP session
and every index but not that counter, so the next DISCOVER was rejected
against a count no live session backed and the subscriber was locked out
until the key's TTL expired. Sessions drained away one per churn cycle.

Two things about the shape of this suite are deliberate:

The soak between churn rounds is the point, not padding. The reaper is
what turns a killed session into a leaked count, it runs on a five minute
ticker, and a session killed ungracefully is only reapable once its lease
plus grace has passed. Churn alone reconnects too fast to reach either.

The failure is asserted as an outcome - subscribers are back in the API
and in VPP - rather than by grepping for the fix. A count derived from the
session map cannot drift, so no log line proves it right, only working
subscribers do. The log checks that follow are there to name the cause
when the outcome assertion fails.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot

Suite Setup         Deploy Topology    ${lab-file}
Suite Teardown      Teardown Churn Test

*** Variables ***
${lab-name}         osvbng-ipoe-churn
${lab-file}         ${CURDIR}/53-ipoe-churn.clab.yml
${bng1}             clab-${lab-name}-bng1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    8
${churn-secs}       45
# Lease 120s + leaseGrace 30s makes a killed session reapable at 150s; the
# reaper ticks every 300s. 420s clears one full tick past that worst case.
${soak-secs}        420

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify VPP Is Running
    ${output} =    Execute VPP Command    ${bng1}    show version
    Should Contain    ${output}    vpp

Establish Subscriber Sessions
    [Documentation]    Eight dual-stack QinQ IPoE sessions across two dot1ad
    ...    S-VLANs, four C-VLANs each.
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}    check_ipv6=true

Verify Session Interfaces Created In VPP
    [Documentation]    Baseline: one ipoe_session interface per subscriber.
    ...    This count is the drain symptom - it fell by one per lost
    ...    subscriber and never recovered.
    Verify Session Interface Count    ${bng1}    ${session-count}

Churn Sessions And Recover
    [Documentation]    First monkey run. Most subscribers reconnect while
    ...    their session record is still live, which is the path that always
    ...    worked; this round mainly seeds the killed sessions the soak then
    ...    lets the reaper collect.
    Churn Sessions    ${subscribers}    ${churn-secs}
    Verify Sessions Flapped    ${subscribers}    minimum=1
    Recover After Churn    ${bng1}

Soak Past The Stale-Session Reaper
    [Documentation]    Idle long enough for any session the monkey killed
    ...    ungracefully to pass its lease and be collected by the reaper.
    ...    Subscribers must still be whole afterwards: reaping a session the
    ...    subscriber has already re-established must not disturb it, and a
    ...    reaped session must leave nothing behind that blocks its own
    ...    return.
    Sleep    ${soak-secs}
    Recover After Churn    ${bng1}

Churn Sessions Again And Recover
    [Documentation]    Second monkey run, now on top of whatever the reaper
    ...    collected during the soak. This is the round that failed: with the
    ...    counter leaked, these subscribers had no route back.
    Churn Sessions    ${subscribers}    ${churn-secs}
    Verify Sessions Flapped    ${subscribers}    minimum=1
    Recover After Churn    ${bng1}

Verify No Subscriber Was Locked Out
    [Documentation]    The specific regression. A DISCOVER rejected on the
    ...    session limit means the admission count outlived the session it
    ...    was counting, which is exactly what locked subscribers out.
    ${count} =    Count osvbngd Log Matches    ${bng1}    session limit reached
    Should Be Equal As Integers    ${count}    0
    ...    ${count} DISCOVER(s) rejected on the session limit - an admission count outlived its session

Report Dataplane Session Create Failures
    [Documentation]    Diagnostic, not a gate. A failed add leaves the
    ...    control plane with no session interface and no retry, so it would
    ...    already have failed a Recover After Churn above. Reported here so
    ...    a failure upstream names its cause instead of leaving one to go
    ...    log-diving. Known open issue: the plugin's add returns
    ...    ENTRY_ALREADY_EXISTS where the control plane expects the
    ...    0 / ENTRY_NEEDS_REFRESH contract that osvbng_pppoe implements.
    ${count} =    Count osvbngd Log Matches    ${bng1}    Failed to create IPoE session in VPP
    Log    IPoE session create failures during churn: ${count}    console=yes

*** Keywords ***
Recover After Churn
    [Documentation]    Every subscriber is back, dual-stack, and has a
    ...    dataplane interface behind it. Asserted against osvbng and VPP
    ...    rather than BNG Blaster's counters, which accumulate across flaps.
    [Arguments]    ${container}
    Wait Until Keyword Succeeds    60 x    5s
    ...    Check All Subscribers Present    ${container}
    Wait Until Keyword Succeeds    30 x    2s
    ...    Verify Session Interface Count    ${container}    ${session-count}

Check All Subscribers Present
    [Arguments]    ${container}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; s=json.load(sys.stdin).get('data') or []; v4=len([x for x in s if x.get('IPv4Address') and x['IPv4Address']!='<nil>']); v6=len([x for x in s if x.get('IPv6Address') and x['IPv6Address']!='<nil>']); print(f'{len(s)} {v4} {v6}')"
    Should Be Equal As Integers    ${rc}    0
    @{parts} =    Split String    ${result}
    Should Be Equal As Strings    ${parts}[0]    ${session-count}
    ...    ${parts}[0]/${session-count} subscribers present - a churned subscriber did not re-establish
    Should Be Equal As Strings    ${parts}[1]    ${session-count}
    ...    ${parts}[1]/${session-count} subscribers have IPv4
    Should Be Equal As Strings    ${parts}[2]    ${session-count}
    ...    ${parts}[2]/${session-count} subscribers have IPv6

Verify Session Interface Count
    [Documentation]    Count ipoe_session interfaces in VPP. The control
    ...    plane can hold a session record whose dataplane interface was
    ...    never created, so this is checked separately from the API.
    [Arguments]    ${container}    ${expected}
    ${output} =    Execute VPP Command    ${container}    show interface
    ${count} =    Get Count    ${output}    ipoe_session
    Should Be Equal As Integers    ${count}    ${expected}
    ...    ${count} ipoe_session interfaces in VPP, expected ${expected}

Count osvbngd Log Matches
    [Arguments]    ${container}    ${pattern}
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    sudo docker logs ${container} 2>&1 | grep -c "${pattern}" || true
    RETURN    ${count}

Teardown Churn Test
    Run Keyword And Ignore Error    BNG Blaster CLI Command    ${subscribers}    monkey-stop
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}
