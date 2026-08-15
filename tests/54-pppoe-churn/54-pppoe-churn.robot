# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
PPPoE session churn: eight authenticated dot1ad/dot1q QinQ subscribers
driven through BNG Blaster's monkey, which kills sessions by several
methods including an ungraceful kill that sends no PADT. Every subscriber
must come back, and the dataplane must still be the process it started as.

The PPPoE half of what suite 51 and 52 deferred. Their headers record
monkey finding two defects out of scope for HQoS: IPoE sessions refused
on reconnect after an ungraceful kill, now covered by suite 53, and a
dataplane crash under PPPoE churn, which is this suite.

That expected fault is why survival is a first-class assertion rather
than something inferred from a passing session count. VPP's pid is
captured at baseline and compared after every round: the watchdog
restarts a dead dataplane, so a crash and recovery leaves sessions
re-established and every session-level assertion green while the thing
under test has died and been replaced. The pid is the only check that
sees it.

This suite runs with no QoS at all. Suite 52 keeps a hand-swapped
osvbng-noqos.yaml to answer whether churn destabilises the dataplane
independently of the CAKE plugin; here that is the default, so a failure
implicates the PPPoE path rather than leaving the two entangled.

Unlike suite 53 there is no admission counter to leak, and no lease and
no reaper to wait on. A session the monkey kills ungracefully is torn
down when its LCP keepalives expire, so the soak is sized for that rather
than for a reap tick.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot
Resource            ../localauth.robot

Suite Setup         Deploy Topology    ${lab-file}
Suite Teardown      Teardown Churn Test

*** Variables ***
${lab-name}         osvbng-pppoe-churn
${lab-file}         ${CURDIR}/54-pppoe-churn.clab.yml
${bng1}             clab-${lab-name}-bng1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    8
${churn-secs}       45
# Long enough for LCP keepalives to expire on any session the monkey killed
# without a PADT, so the BNG tears it down on its own rather than on the
# subscriber's re-connect.
${soak-secs}        180
${vpp-pid}          ${EMPTY}

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify VPP Is Running
    ${output} =    Execute VPP Command    ${bng1}    show version
    Should Contain    ${output}    vpp

Record Baseline Dataplane PID
    [Documentation]    Everything after this compares against this pid. A
    ...    watchdog restart of a crashed VPP is invisible to every other
    ...    assertion in the suite.
    ${pid} =    Get VPP PID    ${bng1}
    Set Suite Variable    ${vpp-pid}    ${pid}
    Log    Baseline VPP pid: ${vpp-pid}    console=yes

Establish Subscriber Sessions
    [Documentation]    Eight QinQ PPPoE sessions across two dot1ad S-VLANs,
    ...    authenticated against pre-created local users.
    Create PPPoE Users    ${bng1}    ${session-count}
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}

Verify Session Interfaces Created In VPP
    [Documentation]    Baseline: one pppoe_session interface per subscriber.
    Verify Session Interface Count    ${bng1}    ${session-count}

Churn Sessions And Recover
    [Documentation]    First monkey run. Dataplane survival is checked before
    ...    session recovery so a crash is reported as a crash rather than as
    ...    whatever the sessions happen to look like afterwards.
    Churn Sessions    ${subscribers}    ${churn-secs}
    Verify Sessions Flapped    ${subscribers}    minimum=1
    Verify Dataplane Survived    ${bng1}
    Recover After Churn    ${bng1}

Soak Past LCP Keepalive Expiry
    [Documentation]    Idle long enough for the BNG to time out any session
    ...    the monkey killed without a PADT, so those teardowns run on the
    ...    BNG's own timers rather than racing the subscriber's reconnect.
    Sleep    ${soak-secs}
    Verify Dataplane Survived    ${bng1}
    Recover After Churn    ${bng1}

Churn Sessions Again And Recover
    [Documentation]    Second monkey run, on top of whatever the keepalive
    ...    expiries tore down during the soak.
    Churn Sessions    ${subscribers}    ${churn-secs}
    Verify Sessions Flapped    ${subscribers}    minimum=1
    Verify Dataplane Survived    ${bng1}
    Recover After Churn    ${bng1}

Verify Dataplane Never Restarted
    [Documentation]    Final gate on the whole run, not just the last round.
    Verify Dataplane Survived    ${bng1}

Report Dataplane Faults
    [Documentation]    Diagnostic, not a gate. If the pid held, nothing here
    ...    should have fired; if it did not, this names what killed it
    ...    without a trip into the container.
    ${faults} =    Count Container Log Matches    ${bng1}    received signal\\|SIGSEGV\\|SIGABRT\\|assertion\\|BUG:
    Log    Dataplane fault signatures in container log: ${faults}    console=yes
    ${restarts} =    Count Container Log Matches    ${bng1}    restarting vpp\\|vpp restart\\|dataplane restart
    Log    Watchdog dataplane restarts logged: ${restarts}    console=yes

*** Keywords ***
Get VPP PID
    [Arguments]    ${container}
    ${rc}    ${pid} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} pidof vpp
    Should Be Equal As Integers    ${rc}    0    VPP is not running in ${container}
    Should Not Be Empty    ${pid}
    RETURN    ${pid}

Verify Dataplane Survived
    [Documentation]    VPP is the same process it was at baseline and still
    ...    answers on the CLI socket. The watchdog respawns a dead VPP, so a
    ...    responsive dataplane on its own proves nothing.
    [Arguments]    ${container}
    ${pid} =    Get VPP PID    ${container}
    Should Be Equal As Strings    ${pid}    ${vpp-pid}
    ...    VPP restarted during churn: pid ${vpp-pid} -> ${pid}, the dataplane died and the watchdog replaced it
    ${output} =    Execute VPP Command    ${container}    show version
    Should Contain    ${output}    vpp

Recover After Churn
    [Documentation]    Every subscriber is back and has a dataplane interface
    ...    behind it. Asserted against osvbng and VPP rather than BNG
    ...    Blaster's counters, which accumulate across flaps.
    [Arguments]    ${container}
    Wait Until Keyword Succeeds    60 x    5s
    ...    Check All Subscribers Present    ${container}
    Wait Until Keyword Succeeds    30 x    2s
    ...    Verify Session Interface Count    ${container}    ${session-count}

Check All Subscribers Present
    [Arguments]    ${container}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; s=json.load(sys.stdin).get('data') or []; v4=len([x for x in s if x.get('IPv4Address') and x['IPv4Address']!='<nil>']); print(f'{len(s)} {v4}')"
    Should Be Equal As Integers    ${rc}    0
    @{parts} =    Split String    ${result}
    Should Be Equal As Strings    ${parts}[0]    ${session-count}
    ...    ${parts}[0]/${session-count} subscribers present - a churned subscriber did not re-establish
    Should Be Equal As Strings    ${parts}[1]    ${session-count}
    ...    ${parts}[1]/${session-count} subscribers have IPv4

Verify Session Interface Count
    [Documentation]    Count pppoe_session interfaces in VPP, separately from
    ...    the API, because the control plane can hold a session record whose
    ...    dataplane interface was never created.
    ...    The count stays exact across churn even though the plugin parks
    ...    torn-down interfaces on a free list for reuse instead of deleting
    ...    them: parking sets VNET_SW_INTERFACE_FLAG_HIDDEN, and show
    ...    interface filters on vnet_swif_is_api_visible, so a parked
    ...    interface is not listed until a new session claims it.
    [Arguments]    ${container}    ${expected}
    ${output} =    Execute VPP Command    ${container}    show interface
    ${count} =    Get Count    ${output}    pppoe_session
    Should Be Equal As Integers    ${count}    ${expected}
    ...    ${count} pppoe_session interfaces in VPP, expected ${expected}

Count Container Log Matches
    [Arguments]    ${container}    ${pattern}
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    sudo docker logs ${container} 2>&1 | grep -ci "${pattern}" || true
    RETURN    ${count}

Teardown Churn Test
    Run Keyword And Ignore Error    BNG Blaster CLI Command    ${subscribers}    monkey-stop
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}
