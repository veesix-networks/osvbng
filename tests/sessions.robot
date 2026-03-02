# Copyright 2025 Veesix Networks Ltd
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            common.robot
Resource            bngblaster.robot

*** Keywords ***
Wait For Sessions Established
    [Arguments]    ${container}    ${bngblaster}    ${expected_count}    ${check_ipv6}=false    ${timeout}=120s    ${interval}=2s
    Wait Until Keyword Succeeds    ${timeout}    ${interval}
    ...    All Sessions Ready    ${container}    ${bngblaster}    ${expected_count}    ${check_ipv6}

All Sessions Ready
    [Arguments]    ${container}    ${bngblaster}    ${expected_count}    ${check_ipv6}=false
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); sessions=d.get('data') or []; total=len(sessions); v4=len([x for x in sessions if x.get('IPv4Address') and x['IPv4Address']!='<nil>']); v6=len([x for x in sessions if x.get('IPv6Address') and x['IPv6Address']!='<nil>']); print(f'{total} {v4} {v6}')"
    Should Be Equal As Integers    ${rc}    0
    ${parts} =    Split String    ${result}
    Should Be Equal As Strings    ${parts}[0]    ${expected_count}    Expected ${expected_count} sessions but got ${parts}[0]
    Should Be Equal As Strings    ${parts}[1]    ${expected_count}    ${parts}[1]/${expected_count} sessions have IPv4
    IF    '${check_ipv6}' == 'true'
        Should Be Equal As Strings    ${parts}[2]    ${expected_count}    ${parts}[2]/${expected_count} sessions have IPv6
    END
    ${rc}    ${cli_output} =    BNG Blaster CLI Command    ${bngblaster}    session-counters
    Should Be Equal As Integers    ${rc}    0
    ${rc}    ${established} =    Run And Return Rc And Output
    ...    echo '${cli_output}' | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('session-counters',{}).get('sessions-established',0))"
    Should Be Equal As Strings    ${established}    ${expected_count}    BNG Blaster established ${established}/${expected_count} sessions

Run BNG Blaster And Verify Sessions
    [Arguments]    ${container}    ${subscribers}    ${expected_count}    ${timeout}=120
    ${rc}    ${output} =    Start BNG Blaster    ${subscribers}    timeout=${timeout}
    ${report_json} =    Get BNG Blaster Report    ${subscribers}
    ${established} =    Get BNG Blaster Report Field    ${subscribers}    sessions-established
    Should Be Equal As Strings    ${established}    ${expected_count}
    RETURN    ${report_json}

Get BNG Blaster Report Field
    [Arguments]    ${container}    ${field}    ${report}=/tmp/report.json
    ${report_json} =    Get BNG Blaster Report    ${container}    ${report}
    ${rc}    ${value} =    Run And Return Rc And Output
    ...    echo '${report_json}' | python3 -c "import sys,json; r=json.load(sys.stdin); print(r.get('report',{}).get('${field}',0))"
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${value}

Verify Sessions In API
    [Arguments]    ${container}    ${expected_count}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${rc}    ${count} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('data') or []))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${count}    ${expected_count}

Verify Sessions Have IPv4
    [Arguments]    ${container}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${rc}    ${missing} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); s=[x for x in (d.get('data') or []) if not x.get('IPv4Address') or x['IPv4Address']=='<nil>']; print(len(s))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${missing}    0    Some sessions missing IPv4 address

Verify Sessions Have IPv6
    [Arguments]    ${container}
    ${output} =    Get osvbng API Response    ${container}    /api/show/subscriber/sessions
    ${rc}    ${missing} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); s=[x for x in (d.get('data') or []) if not x.get('IPv6Address') or x['IPv6Address']=='<nil>']; print(len(s))"
    Should Be Equal As Integers    ${rc}    0
    Should Be Equal As Strings    ${missing}    0    Some sessions missing IPv6 address

Verify VPP Sub-Interfaces Created
    [Arguments]    ${container}    ${pattern}
    ${output} =    Execute VPP Command    ${container}    show interface
    Should Contain    ${output}    ${pattern}
