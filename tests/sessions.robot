# Copyright 2025 The osvbng Authors
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

Snapshot OpDB Sessions
    [Arguments]    ${container}
    ${output} =    Get osvbng API Response    ${container}    /api/show/system/opdb/sessions
    RETURN    ${output}

Verify OpDB Sessions Match Snapshot
    [Arguments]    ${container}    ${snapshot}
    ${current} =    Snapshot OpDB Sessions    ${container}
    ${script} =    Catenate    SEPARATOR=${SPACE}
    ...    import json,os,sys;
    ...    b=json.loads(os.environ['BEFORE']).get('data') or [];
    ...    a=json.loads(os.environ['AFTER']).get('data') or [];
    ...    bk=sorted((e.get('namespace'),e.get('key')) for e in b);
    ...    ak=sorted((e.get('namespace'),e.get('key')) for e in a);
    ...    sys.exit(0 if bk==ak else (print(f'opdb session set drifted:\\n  before={bk}\\n  after={ak}') or 1))
    ${result} =    Run Process    python3    -c    ${script}
    ...    env:BEFORE=${snapshot}    env:AFTER=${current}    stderr=STDOUT
    Log    ${result.stdout}
    Should Be Equal As Integers    ${result.rc}    0    OpDB session set drifted across restart

Verify OpDB Session IP Addresses Match
    [Arguments]    ${container}    ${snapshot}
    ${current} =    Snapshot OpDB Sessions    ${container}
    ${script} =    Catenate    SEPARATOR=${SPACE}
    ...    import json,os,sys;
    ...    b={(e.get('namespace'),e.get('key')): e.get('data') or {} for e in json.loads(os.environ['BEFORE']).get('data') or []};
    ...    a={(e.get('namespace'),e.get('key')): e.get('data') or {} for e in json.loads(os.environ['AFTER']).get('data') or []};
    ...    drift=[];
    ...    [drift.append((k, bf.get('IPv4') or bf.get('IPv4Address'), a.get(k,{}).get('IPv4') or a.get(k,{}).get('IPv4Address'), bf.get('IPv6Address'), a.get(k,{}).get('IPv6Address'))) for k,bf in b.items() if k in a and ((bf.get('IPv4') or bf.get('IPv4Address')) != (a[k].get('IPv4') or a[k].get('IPv4Address')) or bf.get('IPv6Address') != a[k].get('IPv6Address'))];
    ...    sys.exit(0 if not drift else (print('Session IP drift:\\n' + '\\n'.join(f'  {k}: v4 {b4}->{a4}, v6 {b6}->{a6}' for k,b4,a4,b6,a6 in drift)) or 1))
    ${result} =    Run Process    python3    -c    ${script}
    ...    env:BEFORE=${snapshot}    env:AFTER=${current}    stderr=STDOUT
    Log    ${result.stdout}
    Should Be Equal As Integers    ${result.rc}    0    Session IP addresses drifted across restart

Verify OpDB Session Count
    [Arguments]    ${container}    ${expected}
    ${output} =    Snapshot OpDB Sessions    ${container}
    ${result} =    Run Process    python3    -c
    ...    import json,os; print(len(json.loads(os.environ['JSON']).get('data') or []))
    ...    env:JSON=${output}    stderr=STDOUT
    Should Be Equal As Integers    ${result.rc}    0
    Should Be Equal As Strings    ${result.stdout}    ${expected}    Expected ${expected} opdb sessions, got ${result.stdout}
