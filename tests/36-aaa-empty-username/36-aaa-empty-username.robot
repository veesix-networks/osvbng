# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
Empty-User-Name hard-fail suite. The AAA policy format is `$remote-id$` but the
subscriber config injects no Option 82 / access-line, so the expansion collapses
to "". The BNG must drop the DHCP trigger (v4 DISCOVER/REQUEST and v6 SOLICIT),
increment the username-empty drop counter, and establish no subscriber session.
The drop fires upstream of the RADIUS provider, so no empty-User-Name
Access-Request is ever built (asserted directly by the unit tests; here the drop
counter + zero sessions are the deterministic end-to-end signals).

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot

Suite Setup         Deploy Topology    ${lab-file}
Suite Teardown      Teardown Empty Username Test

*** Variables ***
${lab-name}         osvbng-aaa-empty-username
${lab-file}         ${CURDIR}/36-aaa-empty-username.clab.yml
${bng1}             clab-${lab-name}-bng1
${subscribers}      clab-${lab-name}-subscribers
${group}            default

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify VPP Is Running
    [Documentation]    Check VPP is running and responsive.
    ${output} =    Execute VPP Command    ${bng1}    show version
    Should Contain    ${output}    vpp

Start Subscribers And Confirm Drops
    [Documentation]    Blaster sends DHCP whose policy username ($remote-id$) is empty,
    ...    so the BNG must drop the trigger. The drop counter must go positive.
    Start BNG Blaster In Background    ${subscribers}
    Wait Until Keyword Succeeds    90s    3s    Drop Counter Positive    ${bng1}

No Sessions Established
    [Documentation]    No subscriber session may exist in the osvbng API.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('data') or []))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Integers    ${count}    0    Expected zero sessions but got ${count}

Blaster Report Shows Zero Established
    [Documentation]    Stop BNG Blaster and verify no sessions established.
    Stop BNG Blaster    ${subscribers}
    ${established} =    Get BNG Blaster Report Field    ${subscribers}    sessions-established
    Should Be Equal As Strings    ${established}    0

*** Keywords ***
Teardown Empty Username Test
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}

Drop Counter Positive
    [Arguments]    ${container}
    ${metrics} =    Get osvbng Metrics    ${container}
    Assert Counter Positive    ${metrics}    osvbng_aaa_policy_username_fallbacks    ${group}

Get osvbng Metrics
    [Arguments]    ${container}
    ${ip} =    Get Container IPv4    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output    curl -sf http://${ip}:9090/metrics
    Should Be Equal As Integers    ${rc}    0    /metrics not responding
    RETURN    ${output}

Assert Counter Positive
    [Arguments]    ${metrics}    ${metric}    ${group}
    ${script} =    Catenate    SEPARATOR=${SPACE}
    ...    import sys,os,re;
    ...    metric=os.environ['METRIC']; group=os.environ['GROUP'];
    ...    total=0.0;
    ...    [total := total + float(m.group(1)) for line in os.environ['METRICS'].splitlines()
    ...    if line.startswith(metric) and ('group="%s"' % group) in line
    ...    for m in [re.search(r'\\s([0-9.e+-]+)$', line)] if m];
    ...    sys.exit(0 if total > 0 else (print('%s{group=%s} = %s (want > 0)' % (metric, group, total)) or 1))
    ${result} =    Run Process    python3    -c    ${script}
    ...    env:METRICS=${metrics}    env:METRIC=${metric}    env:GROUP=${group}    stderr=STDOUT
    Log    ${result.stdout}
    Should Be Equal As Integers    ${result.rc}    0    ${result.stdout}
