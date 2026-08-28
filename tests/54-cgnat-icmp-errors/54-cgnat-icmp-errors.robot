# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
CGNAT ICMP error handling, RFC 5508 sections 4.2.1 and 4.2.2.
A real Linux subscriber behind a PBA pool, because these cases turn on the
subscriber's own stack: running traceroute, and answering an unexpected
datagram with a Port Unreachable. Both directions of ICMP error translation
are asserted from what the far side actually receives, not from counters.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Library             Collections
Resource            ../common.robot

Suite Setup         Deploy CGNAT ICMP Topology
Suite Teardown      Teardown CGNAT ICMP Topology

*** Variables ***
${lab-name}         osvbng-cgnat-icmp-errors
${lab-file}         ${CURDIR}/54-cgnat-icmp-errors.clab.yml
${bng1}             clab-${lab-name}-bng1
${corerouter1}      clab-${lab-name}-corerouter1
${subscriber}       clab-${lab-name}-subscriber
${subscriber-image}    veesixnetworks/bngtester:alpine-latest
${qinq-iface}       eth1.100.10
${remote}           10.0.0.2
# A source port the subscriber sends one datagram from and then closes, so
# the return datagram lands on a port nothing is bound to.
${probe-sport}      40000
${probe-dport}      40001

*** Test Cases ***
Verify BNG Is Healthy
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify VPP Is Running
    ${output} =    Execute VPP Command    ${bng1}    show version
    Should Contain    ${output}    vpp

Verify CGNAT Plugin Loaded
    ${output} =    Execute VPP Command    ${bng1}    show plugins
    Should Contain    ${output}    osvbng_cgnat

Verify OSPF Adjacency Established
    Wait Until Keyword Succeeds    12 x    10s
    ...    Verify OSPF Adjacency On Router    ${corerouter1}

Verify Subscriber QinQ Interface Created
    Wait Until Keyword Succeeds    30 x    5s
    ...    Check QinQ Interface Exists    ${subscriber}

Verify Subscriber Got IPv4 In Shared Address Space
    Wait Until Keyword Succeeds    30 x    5s
    ...    Subscriber Address Is In Shared Space

Verify CGNAT Mapping Exists For The Subscriber
    Wait Until Keyword Succeeds    12 x    5s
    ...    Mapping Exists    ${bng1}

Verify Subscriber Can Reach The Core Router
    [Documentation]    Baseline: plain translated echo works, so a later
    ...    failure is about ICMP errors and not about the pool.
    Wait Until Keyword Succeeds    10 x    5s
    ...    Ping From Subscriber    ${remote}

Verify Traceroute Sees The BNG As First Hop
    [Documentation]    The Time Exceeded the BNG generates for a TTL=1 probe is
    ...    addressed to the already-translated source, so it re-enters
    ...    cgnat-out2in and must be recognised as an ICMP error from its own
    ...    header. Classified from stale reassembly metadata it is dropped
    ...    NO_SESSION and hop 1 never answers (RFC 5508 section 4.2.1).
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${subscriber} traceroute -n -q 1 -w 2 -m 3 ${remote}
    Log    ${output}
    ${hop1} =    Get Lines Matching Regexp    ${output}    ^\\s*1\\s+\\d+\\.\\d+\\.\\d+\\.\\d+.*
    Should Not Be Empty    ${hop1}    first hop did not answer:\n${output}
    Should Contain    ${output}    100.64.0.1    hop 1 is not the subscriber gateway:\n${output}

Verify TTL Limited Ping Reports Time Exceeded
    [Documentation]    Same path with an echo request as the probe, so the
    ...    quoted inner header is ICMP and the stale metadata holds type 8
    ...    rather than a UDP leftover.
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${subscriber} ping -n -c 1 -t 1 -W 3 ${remote}
    Log    ${output}
    Should Contain    ${output}    Time to live exceeded

Verify Subscriber Originated ICMP Error Quotes The Outside Identity
    [Documentation]    The subscriber answers an unexpected datagram with a Port
    ...    Unreachable. RFC 5508 section 4.2.2 (REQ-5): the quoted inner header
    ...    must be reverted to the outside identity, which is its DESTINATION
    ...    fields, and the remote's own port must survive untouched or the
    ...    remote cannot match the error to its socket.
    Disable ICMP Rate Limiting    ${subscriber}
    ${inside_ip} =    Subscriber IPv4
    Open And Close A UDP Session    ${subscriber}
    ${session} =    Wait Until Keyword Succeeds    10 x    2s
    ...    Get UDP Session    ${bng1}    ${inside_ip}
    Log    ${session}
    ${capture} =    Start Process
    ...    sudo docker exec -i ${corerouter1} python3 - icmp-error eth1 20 ${session}[outside_ip] < ${CURDIR}/../capture.py
    ...    shell=True
    Sleep    2s
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    echo y | sudo docker exec -i ${corerouter1} nc -u -w 1 -p ${probe-dport} ${session}[outside_ip] ${session}[outside_port]
    Log    ${output}
    ${result} =    Wait For Process    ${capture}    timeout=40s    on_timeout=kill
    Log    ${result.stdout}
    Should Be Equal As Integers    ${result.rc}    0
    ...    no ICMP error sourced from ${session}[outside_ip] reached the core side
    Should Contain    ${result.stdout}    type=3 code=3
    Should Contain    ${result.stdout}    inner_src=${remote} inner_sport=${probe-dport}
    Should Contain    ${result.stdout}    inner_dst=${session}[outside_ip] inner_dport=${session}[outside_port]

*** Keywords ***
Deploy CGNAT ICMP Topology
    Set Environment Variable    BNGTESTER_IMAGE    ${subscriber-image}
    Deploy Topology    ${lab-file}

Teardown CGNAT ICMP Topology
    Run Keyword And Ignore Error    Dump VPP Trace    ${bng1}
    Destroy Topology    ${lab-file}

Check QinQ Interface Exists
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} ip link show ${qinq-iface}
    Should Be Equal As Integers    ${rc}    0    QinQ interface ${qinq-iface} not found

Subscriber IPv4
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${subscriber} ip -4 -o addr show ${qinq-iface}
    Should Be Equal As Integers    ${rc}    0
    ${matches} =    Get Regexp Matches    ${output}    inet (\\d+\\.\\d+\\.\\d+\\.\\d+)    1
    Should Not Be Empty    ${matches}    no IPv4 address on ${qinq-iface}
    RETURN    ${matches}[0]

Subscriber Address Is In Shared Space
    ${ip} =    Subscriber IPv4
    Should Start With    ${ip}    100.64.

Mapping Exists
    [Arguments]    ${bng}
    ${output} =    Get osvbng API Response    ${bng}    /api/show/cgnat/mappings
    Should Contain    ${output}    203.0.113.

Ping From Subscriber
    [Arguments]    ${target}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${subscriber} ping -n -c 3 -W 2 ${target}
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0    cannot ping ${target}

Disable ICMP Rate Limiting
    [Documentation]    Linux rate-limits outbound ICMP errors per destination,
    ...    and the traceroute above has just used that budget.
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} sysctl -w net.ipv4.icmp_ratelimit=0
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0

Open And Close A UDP Session
    [Documentation]    One datagram out creates the CGNAT session; nc then exits
    ...    so nothing is bound to the source port when the reply arrives.
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    echo x | sudo docker exec -i ${container} nc -u -w 1 -p ${probe-sport} ${remote} ${probe-dport}
    Log    ${output}

Get UDP Session
    [Documentation]    One query parameter only: the shared keyword interpolates
    ...    the path into an unquoted shell command, so an "&" would split it.
    [Arguments]    ${bng}    ${inside_ip}
    ${output} =    Get osvbng API Response    ${bng}
    ...    /api/show/cgnat/sessions?inside-ip=${inside_ip}
    ${sessions} =    Evaluate
    ...    [s for s in (json.loads($output)['data']['sessions'] or []) if s['proto'].lower() == 'udp' and s['inside_port'] == ${probe-sport}]
    ...    json
    Should Not Be Empty    ${sessions}
    ...    no UDP session from ${inside_ip}:${probe-sport} in the dump
    RETURN    ${sessions}[0]
