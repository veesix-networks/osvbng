# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
L2GW dynamic circuit suite: the full trigger -> RADIUS -> install -> replay
wholesale loop. The first DHCP DISCOVER on an l2gw access range triggers an
Access-Request; FreeRADIUS answers with the handoff group label as a VSA
(vendor 32473, the RFC 5612 documentation enterprise number, mapped through
response_mappings); osvbng allocates egress VLANs from the group ranges,
installs the bidirectional cross-connect, and replays the held DISCOVER out
the handoff. The BNG Blaster a10nsp interface acts as the retail ISP BNG
(answers DHCP, matches sessions by preserved client MAC). Accounting
Start/Interim carry the resolved l2gw.* values back as VSAs plus per-circuit
counters. Circuits must survive an osvbngd restart without re-auth or a
duplicate Accounting-Start, and RADIUS Disconnect-Message must tear a
circuit down with an Accounting-Stop.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot
Resource            ../l2gw.robot
Resource            ../restart.robot

Suite Setup         Deploy Topology    ${lab-file}
Suite Teardown      Teardown L2GW Dynamic Test

*** Variables ***
${lab-name}             osvbng-l2gw-dynamic
${lab-file}             ${CURDIR}/39-l2gw-dynamic.clab.yml
${bng1}                 clab-${lab-name}-bng1
${bng1-mgmt-ip}         172.20.38.2
${subscribers}          clab-${lab-name}-subscribers
${freeradius}           clab-${lab-name}-freeradius
${session-count}        2
${coa-secret}           testing123
${acct-interim-wait}    90s
${detail-file-glob}     /var/log/freeradius/radacct/172.20.38.2/detail-*

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify L2GW Plugin Loaded
    [Documentation]    The osvbng_l2gw VPP plugin must be loaded and its CLI reachable.
    ${output} =    Execute VPP Command    ${bng1}    show osvbng l2gw circuits
    Should Not Contain    ${output}    unknown input

Establish Wholesale Circuits
    [Documentation]    DHCP DISCOVER triggers AAA; Access-Accept returns the
    ...    handoff group; the circuit installs and the replayed DISCOVER
    ...    reaches the a10nsp (ISP) side, which answers. Blaster sessions
    ...    establishing proves the entire loop.
    Start BNG Blaster In Background    ${subscribers}
    Wait For Blaster Sessions Established    ${subscribers}    ${session-count}

Verify Circuits Installed With Allocated Egress
    [Documentation]    Dynamic circuits with handoff group from RADIUS and
    ...    egress VLANs from the group allocator ranges.
    Wait For L2GW Circuit Count    ${bng1}    ${session-count}
    Verify L2GW Circuit Field    ${bng1}    not c.get('static')
    ...    circuits must be dynamic
    Verify L2GW Circuit Field    ${bng1}    c.get('handoff_group')=='isp-blue'
    ...    handoff group must come from the RADIUS VSA
    Verify L2GW Circuit Field    ${bng1}    200<=c.get('handoff_svlan',0)<=204
    ...    egress S-VLAN must come from the svlan-range allocator
    # cvlan-range is pinned to the access C-VLAN: the BNG Blaster a10nsp
    # side sends downstream session-traffic with the session's access
    # inner VLAN, so a translated C-VLAN would never match the reverse
    # circuit.
    Verify L2GW Circuit Field    ${bng1}    c.get('handoff_cvlan',0)==10
    ...    egress C-VLAN must come from the cvlan-range allocator
    Verify L2GW Circuit Field    ${bng1}    c.get('mac') and c.get('session_id')
    ...    dynamic circuits must carry subscriber identity

Verify No Local Termination
    [Documentation]    osvbng must not terminate DHCP for wholesale circuits;
    ...    the subscriber session table stays empty.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    ${result} =    Run Process    python3    -c
    ...    import json,os; print(len(json.loads(os.environ['JSON']).get('data') or []))
    ...    env:JSON=${output}    stderr=STDOUT
    Should Be Equal As Strings    ${result.stdout}    0    l2gw subscribers were terminated locally

Verify RADIUS Server Stats
    [Documentation]    One Access-Accept per circuit, zero rejects.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/aaa/radius/servers
    ${rc}    ${accepts} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); s=d.get('data',[]); print(sum(x.get('auth_accepts',0) for x in s))"
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${accepts} >= ${session-count}    Expected at least ${session-count} auth accepts but got ${accepts}
    ${rc}    ${rejects} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); s=d.get('data',[]); print(sum(x.get('auth_rejects',0) for x in s))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Integers    ${rejects}    0    Expected zero auth rejects but got ${rejects}

Verify Accounting Start With L2GW Attributes
    [Documentation]    Accounting-Start per circuit must carry the resolved
    ...    l2gw.* values as VSAs, the wholesale billing feed.
    Wait Until Keyword Succeeds    30s    2s
    ...    Radius Detail Contains    Acct-Status-Type = Start    ${session-count}
    Wait Until Keyword Succeeds    10s    2s
    ...    Radius Detail Contains    OSVBNG-L2GW-Handoff-Group    ${session-count}
    Wait Until Keyword Succeeds    10s    2s
    ...    Radius Detail Contains    isp-blue    ${session-count}
    Wait Until Keyword Succeeds    10s    2s
    ...    Radius Detail Contains    OSVBNG-L2GW-SVLAN    ${session-count}

Verify Session Traffic Flows
    [Documentation]    Bidirectional session traffic between access and a10nsp
    ...    sides through the installed circuits.
    Wait Until Keyword Succeeds    60s    5s
    ...    Verify Traffic Flowing    ${subscribers}    ${session-count}

Verify Acct-Interim Carries Non-Zero Counters
    [Documentation]    Interim records must carry non-zero octets/packets from
    ...    the per-circuit dataplane counters (/osvbng/l2gw stats segment via
    ...    the AAA baseline machinery).
    Sleep    ${acct-interim-wait}    Wait for at least one Acct-Interim bucket tick.
    ${max-input-octets} =    Get Max Radius Attribute    ${freeradius}    Acct-Input-Octets
    Should Be True    ${max-input-octets} > 0    Acct-Input-Octets stayed zero, l2gw counters not reaching RADIUS accounting.
    ${max-output-octets} =    Get Max Radius Attribute    ${freeradius}    Acct-Output-Octets
    Should Be True    ${max-output-octets} > 0    Acct-Output-Octets stayed zero across all accounting records.
    ${max-input-packets} =    Get Max Radius Attribute    ${freeradius}    Acct-Input-Packets
    Should Be True    ${max-input-packets} > 0    Acct-Input-Packets stayed zero across all accounting records.

Verify Dataplane Circuit Counters
    [Documentation]    Per-circuit counters in the l2gw plugin count both directions.
    Verify VPP L2GW Circuits Counters Non-Zero    ${bng1}

Verify Prometheus L2GW Circuit Metrics
    [Documentation]    Per-circuit counters must be exported through the
    ...    telemetry SDK with the access/handoff identity labels.
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    curl -sf http://172.20.38.2:9090/metrics | grep "^osvbng_dataplane_vpp_l2gw_upstream_packets"
    Should Be Equal As Integers    ${rc}    0    no l2gw upstream metrics exported
    Log    ${output}
    Should Contain    ${output}    handoff_group="isp-blue"
    ${rc}    ${nonzero} =    Run And Return Rc And Output
    ...    curl -sf http://172.20.38.2:9090/metrics | awk '/^osvbng_dataplane_vpp_l2gw_upstream_packets/ {if ($NF+0 > 0) n++} END {print n+0}'
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${nonzero} >= ${session-count}    l2gw upstream packet metrics all zero

Restart Survives With No Duplicate Accounting Start
    [Documentation]    osvbngd restart: circuits re-install from opdb with no
    ...    re-authentication and no second Accounting-Start; traffic resumes.
    ${snapshot} =    Snapshot L2GW Circuit IDs    ${bng1}
    ${accepts-before} =    Get RADIUS Auth Accepts    ${bng1}
    ${starts-before} =    Count Radius Records    Acct-Status-Type = Start
    Restart osvbngd    ${bng1}
    Wait For osvbngd Down    ${bng1}
    Wait For osvbng Healthy    bng1    ${lab-name}
    Wait For osvbng State Ready    ${bng1}
    Wait For L2GW Circuit Count    ${bng1}    ${session-count}
    ${restored} =    Snapshot L2GW Circuit IDs    ${bng1}
    Should Be Equal As Strings    ${restored}    ${snapshot}    circuit set changed across restart
    ${starts-after} =    Count Radius Records    Acct-Status-Type = Start
    Should Be Equal As Integers    ${starts-after}    ${starts-before}    duplicate Accounting-Start after restore
    ${accepts-after} =    Get RADIUS Auth Accepts    ${bng1}
    Should Be Equal As Integers    ${accepts-after}    0    circuits re-authenticated after restart instead of restoring from opdb
    Reset Stream Verification    ${subscribers}
    Wait Until Keyword Succeeds    60s    5s
    ...    Verify Traffic Flowing    ${subscribers}    ${session-count}

Disconnect Message Tears Down Circuit
    [Documentation]    RADIUS DM by Acct-Session-Id removes the circuit and
    ...    emits an Accounting-Stop.
    ${acct-id} =    Get First Circuit Acct Session ID    ${bng1}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${freeradius} bash -c "echo 'Acct-Session-Id = ${acct-id}' | radclient -x ${bng1-mgmt-ip}:3799 disconnect ${coa-secret}"
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0    radclient disconnect failed
    Should Contain    ${output}    Disconnect-ACK
    Wait For L2GW Circuit Count    ${bng1}    1
    Wait Until Keyword Succeeds    20s    2s
    ...    Radius Detail Contains    Acct-Status-Type = Stop    1

*** Keywords ***
Teardown L2GW Dynamic Test
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}

Radius Detail Contains
    [Documentation]    Fail unless the FreeRADIUS detail log holds at least
    ...    ${min} occurrences of ${needle}.
    [Arguments]    ${needle}    ${min}
    ${count} =    Count Radius Records    ${needle}
    Should Be True    ${count} >= ${min}    Expected at least ${min} of '${needle}' in RADIUS detail, got ${count}

Count Radius Records
    [Arguments]    ${needle}
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    sudo docker exec ${freeradius} sh -c "grep -c '${needle}' ${detail-file-glob} 2>/dev/null | awk -F: '{s+=\\$NF} END {print s+0}'"
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${count}

Get Max Radius Attribute
    [Documentation]    Maximum integer value of ${attr} across every accounting
    ...    record FreeRADIUS has logged for the BNG.
    [Arguments]    ${container}    ${attr}
    ${rc}    ${value} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} sh -c "awk -v a=${attr} 'index(\\$0, a \\" = \\") {n=\\$NF+0; if (n>m) m=n} END {print m+0}' ${detail-file-glob}"
    Should Be Equal As Integers    ${rc}    0    detail file read on ${container} failed
    RETURN    ${value}

Get RADIUS Auth Accepts
    [Documentation]    Sum of auth_accepts across servers. data is an
    ...    explicit JSON null on a freshly restarted provider with no
    ...    requests yet, so the `or []` guard is load-bearing.
    [Arguments]    ${container}
    ${output} =    Get osvbng API Response    ${container}    /api/show/aaa/radius/servers
    ${rc}    ${accepts} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); s=d.get('data') or []; print(sum(x.get('auth_accepts',0) for x in s))"
    Should Be Equal As Integers    ${rc}    0    radius servers API parse failed
    RETURN    ${accepts}

Get First Circuit Acct Session ID
    [Documentation]    Acct-Session-Id is the first 8 hex chars of the circuit
    ...    session id with dashes removed (session.ToAcctSessionID).
    [Arguments]    ${container}
    ${output} =    Get L2GW Circuits    ${container}
    ${script} =    Catenate    SEPARATOR=${SPACE}
    ...    import json,os;
    ...    d=json.loads(os.environ['JSON']);
    ...    c=sorted(x.get('session_id') for x in (d.get('data') or []) if x.get('session_id'));
    ...    print(c[0].replace('-','')[:8])
    ${result} =    Run Process    python3    -c    ${script}
    ...    env:JSON=${output}    stderr=STDOUT
    Should Be Equal As Integers    ${result.rc}    0
    RETURN    ${result.stdout}
