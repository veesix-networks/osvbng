# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
IPoE family-disabled gating suite. Three subscriber groups on distinct S-VLAN
bands: v4-only (no ipv6-profile), v6-only (no ipv4-profile), and dual-stack.
Proves the opposite family is rejected at ingress while the enabled family and
the dual-stack group still work end-to-end, and that the per-family drop
counters increment.

BNG Blaster's IPoE engine bootstraps DHCPv6 behind a successful DHCPv4 bind, so
it cannot drive a standalone-v6 subscriber and does not emit Router/Neighbor
Solicitations on these streams. The v6-only group is therefore exercised by
proving its DHCPv4 DISCOVER is dropped (no OFFER, counter increments); the
standalone-v6 SOLICIT bind and the RS/NS gates are covered by unit tests in
internal/ipoe (TestProcessRSPacketFamilyGate, TestProcessNSPacketFamilyGate,
TestSessionFamilyEnabled).

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot

Suite Setup         Deploy Topology    ${lab-file}
Suite Teardown      Teardown Family Test

*** Variables ***
${lab-name}         osvbng-ipoe-family-disabled
${lab-file}         ${CURDIR}/35-ipoe-family-disabled.clab.yml
${bng1}             clab-${lab-name}-bng1
${subscribers}      clab-${lab-name}-subscribers

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify VPP Is Running
    [Documentation]    Check VPP is running and responsive.
    ${output} =    Execute VPP Command    ${bng1}    show version
    Should Contain    ${output}    vpp

Start Subscriber Sessions
    [Documentation]    Start dual-stack-capable BNG Blaster clients on all three bands.
    Start BNG Blaster In Background    ${subscribers}
    Wait For Family Sessions    ${bng1}

Dual-Stack Group Binds Both Families
    [Documentation]    The dual-stack group (S-VLAN 140) gets both IPv4 and IPv6 end-to-end.
    Assert Group Family    ${bng1}    140    yes    yes

V4-Only Group Rejects IPv6
    [Documentation]    The v4-only group (S-VLAN 100) gets IPv4 but never IPv6.
    Assert Group Family    ${bng1}    100    yes    no

V6-Only Group Rejects IPv4
    [Documentation]    The v6-only group (S-VLAN 120) never gets an IPv4 lease (DISCOVER dropped, no OFFER).
    Assert No IPv4 On Band    ${bng1}    120

Drop Counters Increment
    [Documentation]    The per-family ingress drop counters are present and non-zero in /metrics.
    ${metrics} =    Get osvbng Metrics    ${bng1}
    Assert Counter Positive    ${metrics}    osvbng_ipoe_dhcpv6_dropped_family_disabled    v4only
    Assert Counter Positive    ${metrics}    osvbng_ipoe_dhcpv4_dropped_family_disabled    v6only

*** Keywords ***
Teardown Family Test
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}

Wait For Family Sessions
    [Arguments]    ${container}    ${timeout}=150s    ${interval}=3s
    Wait Until Keyword Succeeds    ${timeout}    ${interval}    Family Sessions Ready    ${container}

Family Sessions Ready
    [Arguments]    ${container}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${script} =    Catenate    SEPARATOR=${SPACE}
    ...    import sys,json;
    ...    d=json.loads(sys.stdin.read());
    ...    s={x.get('OuterVLAN'): x for x in (d.get('data') or [])};
    ...    has=lambda v: bool(v) and v!='<nil>';
    ...    ok=(140 in s and has(s[140].get('IPv4Address')) and has(s[140].get('IPv6Address'))
    ...    and 100 in s and has(s[100].get('IPv4Address')));
    ...    sys.exit(0 if ok else (print('not ready: '+json.dumps({k:{'v4':v.get('IPv4Address'),'v6':v.get('IPv6Address')} for k,v in s.items()})) or 1))
    ${result} =    Run Process    python3    -c    ${script}    stdin=${output}    stderr=STDOUT
    Log    ${result.stdout}
    Should Be Equal As Integers    ${result.rc}    0    Family sessions not ready: ${result.stdout}

Assert Group Family
    [Arguments]    ${container}    ${svlan}    ${expect_v4}    ${expect_v6}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${script} =    Catenate    SEPARATOR=${SPACE}
    ...    import sys,json,os;
    ...    d=json.loads(sys.stdin.read());
    ...    svlan=int(os.environ['SVLAN']); ev4=os.environ['EV4']=='yes'; ev6=os.environ['EV6']=='yes';
    ...    s=[x for x in (d.get('data') or []) if x.get('OuterVLAN')==svlan];
    ...    has=lambda v: bool(v) and v!='<nil>';
    ...    err=[];
    ...    (err.append('no session on svlan %d' % svlan) if not s else None);
    ...    sess=s[0] if s else {};
    ...    v4=has(sess.get('IPv4Address')); v6=has(sess.get('IPv6Address')) or has(sess.get('IPv6Prefix'));
    ...    (err.append('v4 expected %s got %s (%s)' % (ev4, v4, sess.get('IPv4Address'))) if v4!=ev4 else None);
    ...    (err.append('v6 expected %s got %s (%s/%s)' % (ev6, v6, sess.get('IPv6Address'), sess.get('IPv6Prefix'))) if v6!=ev6 else None);
    ...    sys.exit(0 if not err else (print('; '.join(err)) or 1))
    ${result} =    Run Process    python3    -c    ${script}
    ...    stdin=${output}    env:SVLAN=${svlan}    env:EV4=${expect_v4}    env:EV6=${expect_v6}    stderr=STDOUT
    Log    ${result.stdout}
    Should Be Equal As Integers    ${result.rc}    0    svlan ${svlan}: ${result.stdout}

Assert No IPv4 On Band
    [Arguments]    ${container}    ${svlan}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${script} =    Catenate    SEPARATOR=${SPACE}
    ...    import sys,json,os;
    ...    d=json.loads(sys.stdin.read()); svlan=int(os.environ['SVLAN']);
    ...    has=lambda v: bool(v) and v!='<nil>';
    ...    bad=[x for x in (d.get('data') or []) if x.get('OuterVLAN')==svlan and has(x.get('IPv4Address'))];
    ...    sys.exit(0 if not bad else (print('v6-only band %d leaked IPv4: %s' % (svlan, [x.get('IPv4Address') for x in bad])) or 1))
    ${result} =    Run Process    python3    -c    ${script}    stdin=${output}    env:SVLAN=${svlan}    stderr=STDOUT
    Log    ${result.stdout}
    Should Be Equal As Integers    ${result.rc}    0    ${result.stdout}

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
