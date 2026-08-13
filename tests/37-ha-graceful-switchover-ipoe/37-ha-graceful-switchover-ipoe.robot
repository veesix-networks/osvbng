# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
HA graceful switchover test for IPoE with CGNAT.
Establishes sessions on ACTIVE, verifies sync, then triggers a graceful
switchover via the API (no force, no failure) and validates that synced
sessions forward traffic on the new active without subscriber renegotiation.
Traffic streams are NOT stopped before the switchover, and stream flow
verification is reset afterwards so the traffic check proves fresh
forwarding through the new active.
Also validates the proactive GARP flood on promotion (issue 417): the
new active must emit broadcast gratuitous ARPs from the SRG virtual MAC
onto the access network, with nothing skipped by the SRG plugin and no
frames misrouted to local0-output.

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot

Suite Setup         Deploy Graceful Topology
Suite Teardown      Destroy Graceful Topology

*** Variables ***
${lab-name}         osvbng-ha-graceful-ipoe
${lab-file}         ${CURDIR}/37-ha-graceful-switchover-ipoe.clab.yml
${bng1}             clab-${lab-name}-bng1
${bng2}             clab-${lab-name}-bng2
${corerouter1}      clab-${lab-name}-corerouter1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    5
${srg-vmac}         02:00:5e:00:01:01
${garp-pcap}        /tmp/osvbng-ha-graceful-garp.pcap

*** Test Cases ***
# --- Phase 1: Bootstrap ---

Verify bng1 Is Healthy
    Wait For osvbng Healthy    bng1    ${lab-name}

Verify bng2 Is Healthy
    Wait For osvbng Healthy    bng2    ${lab-name}

Verify bng1 Is ACTIVE
    Wait Until Keyword Succeeds    20 x    5s
    ...    Check HA Status    ${bng1}    ACTIVE

Verify bng2 Is STANDBY
    Wait Until Keyword Succeeds    20 x    5s
    ...    Check HA Status    ${bng2}    STANDBY

Verify OSPF Adjacency For bng1
    Wait Until Keyword Succeeds    12 x    10s
    ...    Check OSPF Neighbor On Router    ${corerouter1}    10.254.0.1

Verify OSPF Adjacency For bng2
    Wait Until Keyword Succeeds    12 x    10s
    ...    Check OSPF Neighbor On Router    ${corerouter1}    10.254.0.3

Verify BGP Session For bng1
    Wait Until Keyword Succeeds    12 x    10s
    ...    Verify BGP Session On Router    ${corerouter1}    10.254.0.1

Verify BGP Session For bng2
    Wait Until Keyword Succeeds    12 x    10s
    ...    Verify BGP Session On Router    ${corerouter1}    10.254.0.3

# --- Phase 2: Establish Sessions and Verify Sync ---

Establish Subscriber Sessions On bng1
    Start BNG Blaster In Background    ${subscribers}    config=/config/subscribers.json
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}

Verify CGNAT Mappings Exist On bng1
    Wait Until Keyword Succeeds    12 x    5s
    ...    Check CGNAT Mapping Count    ${bng1}    ${session-count}

Verify NAT Traffic Flowing On bng1
    Wait Until Keyword Succeeds    12 x    10s
    ...    Verify Stream Traffic Flowing    ${subscribers}    expected_flows=${session-count}

Verify Session Sync Received On STANDBY
    Wait Until Keyword Succeeds    30 x    2s
    ...    Check Sync Sequence Nonzero    ${bng2}

Snapshot local0 Drops On bng2
    ${count} =    Get Local0 Drop Count    ${bng2}
    Set Suite Variable    ${local0-drops-before}    ${count}

Start GARP Capture On Access Bridge
    Start Packet Capture    access-sw-gr    ${garp-pcap}

# --- Phase 3: Graceful Switchover ---

Trigger Graceful Switchover
    Exec osvbng API    ${bng1}    /api/exec/ha/switchover

Verify bng1 Is Now STANDBY
    Wait Until Keyword Succeeds    20 x    5s
    ...    Check HA Status    ${bng1}    STANDBY

Verify bng2 Is Now ACTIVE
    Wait Until Keyword Succeeds    20 x    5s
    ...    Check HA Status    ${bng2}    ACTIVE

Verify Sessions Restored On bng2
    Wait Until Keyword Succeeds    30 x    2s
    ...    Check Session Count On BNG    ${bng2}    ${session-count}

Verify CGNAT Mappings Restored On bng2
    Wait Until Keyword Succeeds    30 x    2s
    ...    Check CGNAT Mapping Count    ${bng2}    ${session-count}

# --- Phase 4: GARP flood on promotion (issue 417) ---

Verify Restored Sessions Carry Access Interface
    Wait Until Keyword Succeeds    12 x    5s
    ...    Check Sessions Have Access Interface    ${bng2}

Verify GARP Flood Sent On bng2
    Wait Until Keyword Succeeds    12 x    5s
    ...    Check SRG GARP Sent Nonzero    ${bng2}

Verify No GARP Entries Skipped On bng2
    ${skipped} =    Get SRG Counter Column    ${bng2}    9
    Should Be Equal As Integers    ${skipped}    0
    ...    SRG plugin refused ${skipped} GARP entries, control plane passed a non-transmittable interface

Verify GARP Not Dropped Via local0 On bng2
    ${after} =    Get Local0 Drop Count    ${bng2}
    Should Be Equal As Integers    ${after}    ${local0-drops-before}
    ...    local0-output drops grew during switchover, GARP frames were misrouted (issue 417 regression)

Verify GARP Frames On Access Network
    Stop Packet Capture    access-sw-gr
    Check GARP In Capture

Reset Stream Flow Verification
    Reset Stream Verification    ${subscribers}

Verify Traffic Recovers After Switchover
    Wait Until Keyword Succeeds    30 x    5s
    ...    Verify Stream Traffic Flowing    ${subscribers}    expected_flows=${session-count}

Verify Switchover Was Hitless
    Verify No Session Flaps    ${subscribers}

*** Keywords ***
Deploy Graceful Topology
    Create Access Bridge
    Deploy Topology    ${lab-file}

Destroy Graceful Topology
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Run Keyword And Ignore Error    Stop Packet Capture    access-sw-gr
    Destroy Topology    ${lab-file}
    Destroy Access Bridge

Create Access Bridge
    ${rc} =    Run And Return Rc    sudo ip link add access-sw-gr type bridge
    ${rc} =    Run And Return Rc    sudo ip link set access-sw-gr up

Destroy Access Bridge
    Run And Return Rc    sudo ip link del access-sw-gr

Check HA Status
    [Arguments]    ${container}    ${expected_state}
    ${output} =    Get osvbng API Response    ${container}    /api/show/ha/status
    Should Contain    ${output}    ${expected_state}

Check OSPF Neighbor On Router
    [Arguments]    ${container}    ${neighbor_rid}
    ${output} =    Execute Vtysh On Router    ${container}    show ip ospf neighbor
    Should Contain    ${output}    ${neighbor_rid}
    Should Contain    ${output}    Full

Check Sync Sequence Nonzero
    [Arguments]    ${container}
    ${output} =    Get osvbng API Response    ${container}    /api/show/ha/sync
    ${rc}    ${seq} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); print(sum(e.get('last_sync_seq',0) for e in d.get('data',[])))"
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${seq} > 0    Standby sync sequence is 0, no sessions received

Check CGNAT Mapping Count
    [Arguments]    ${container}    ${expected}
    ${output} =    Get osvbng API Response    ${container}    /api/show/cgnat/mappings
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); entries=d.get('data',[]); print(len(entries))"
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${count} >= ${expected}    CGNAT mappings ${count} < expected ${expected}

Check Session Count On BNG
    [Arguments]    ${container}    ${expected}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); entries=d.get('data',[]); print(len(entries))"
    Should Be Equal As Integers    ${rc}    0
    Should Be True    ${count} >= ${expected}    Session count ${count} < expected ${expected}

Exec osvbng API
    [Arguments]    ${container}    ${path}
    ${ip} =    Get Container IPv4    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    wget -qO- http://${ip}:${OSVBNG_API_PORT}${path} --post-data='' 2>/dev/null
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${output}

Start Packet Capture
    [Arguments]    ${iface}    ${pcap}
    Run Keyword And Ignore Error    Stop Packet Capture    ${iface}
    ${rc} =    Run And Return Rc    sudo rm -f ${pcap}
    Start Process    sudo tcpdump -i ${iface} -U -w ${pcap} 'arp or (vlan and arp)'    shell=True
    Sleep    2s    let tcpdump attach before the switchover

Stop Packet Capture
    [Arguments]    ${iface}
    Run And Return Rc    sudo pkill -f "tcpdump -i ${iface}"
    Sleep    1s    let tcpdump flush the pcap on exit

Check GARP In Capture
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo tcpdump -nn -e -r ${garp-pcap} 2>/dev/null
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0
    # A proactive GARP is a broadcast ARP reply sourced from the SRG
    # virtual MAC. A normal ARP reply from the vMAC is unicast, so the
    # broadcast destination is what proves the flood ran.
    Should Match Regexp    ${output}    ${srg-vmac} > ff:ff:ff:ff:ff:ff.*Reply.*is-at ${srg-vmac}
    ...    no broadcast gratuitous ARP from ${srg-vmac} seen on the access bridge

Get Local0 Drop Count
    [Arguments]    ${container}
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} vppctl -s ${VPPCTL_SOCK} show errors | awk '/local0-output/ { sum += $1 } END { printf "%d", sum }'
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${count}

Check SRG GARP Sent Nonzero
    [Arguments]    ${container}
    ${sent} =    Get SRG Counter Column    ${container}    5
    Should Be True    ${sent} > 0    SRG reports no GARP sent after promotion

# Columns of `show osvbng srg`: name, vmac, state, ifs, garp sent,
# na sent, mac adds, mac dels, garp skip.
Get SRG Counter Column
    [Arguments]    ${container}    ${column}
    ${rc}    ${value} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} vppctl -s ${VPPCTL_SOCK} show osvbng srg | awk '$1 == "default" { printf "%d", $${column} }'
    Should Be Equal As Integers    ${rc}    0
    Should Not Be Empty    ${value}    SRG "default" not found in show osvbng srg
    RETURN    ${value}

Check Sessions Have Access Interface
    [Arguments]    ${container}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${rc}    ${bad} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); entries=d.get('data',[]); print(sum(1 for e in entries if not e.get('AccessIfIndex')))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Integers    ${bad}    0
    ...    ${bad} restored sessions have AccessIfIndex 0, GARP for them cannot target the access interface
