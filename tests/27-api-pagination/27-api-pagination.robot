# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
Northbound API pagination smoke test (osvbng-context issue #39).
Establishes 50 dual-stack IPoE sessions via bng-blaster + local auth, then
exercises ?limit=&offset= on /api/show/subscriber/sessions and verifies:
  - default page returns the new {path, data, pagination} envelope
  - limit=10 walked across all pages: no duplicates, total stable, sum returned == total
  - limit=50000 clamped to MaxLimit (1000)
  - offset past end returns empty page, has_more=false
  - non-list endpoint (/api/show/system/version) keeps the legacy
    {path, data} envelope with NO pagination block

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Library             Collections
Resource            ../common.robot
Resource            ../bngblaster.robot
Resource            ../sessions.robot
Resource            ../localauth.robot

Suite Setup         Deploy Pagination Topology
Suite Teardown      Teardown Pagination Test

*** Variables ***
${lab-name}         osvbng-api-pagination
${lab-file}         ${CURDIR}/27-api-pagination.clab.yml
${bng1}             clab-${lab-name}-bng1
${subscribers}      clab-${lab-name}-subscribers
${session-count}    50

*** Test Cases ***
Verify BNG Is Healthy
    [Documentation]    Wait for osvbng to fully start.
    Wait For osvbng Healthy    bng1    ${lab-name}

Establish Subscriber Sessions
    [Documentation]    Create local auth users and bring up 50 dual-stack IPoE sessions.
    Create IPoE Users    ${bng1}    ${session-count}
    Start BNG Blaster In Background    ${subscribers}
    Wait For Sessions Established    ${bng1}    ${subscribers}    ${session-count}    check_ipv6=true

Default Page Returns Pagination Envelope
    [Documentation]    GET without query params returns the new envelope with
    ...    a pagination block bounded at the default limit.
    ${ip} =    Get Container IPv4    ${bng1}
    ${body} =    Get osvbng API Response    ${bng1}    /api/show/subscriber/sessions
    Should Contain    ${body}    "pagination"
    ${limit} =    Get Pagination Field From Path    ${ip}    /api/show/subscriber/sessions    limit
    ${total} =    Get Total From Path    ${ip}    /api/show/subscriber/sessions
    Should Be Equal As Integers    ${limit}    100
    Should Be Equal As Integers    ${total}    ${session-count}

Walk Pages With Limit Ten And Verify Total
    [Documentation]    Walk every page at limit=10, verify no duplicates,
    ...    total stable, and sum of returned == total.
    ${ip} =    Get Container IPv4    ${bng1}
    ${total} =    Get Total From Path    ${ip}    /api/show/subscriber/sessions?limit=1
    ${seen} =    Create List
    ${offset} =    Set Variable    ${0}
    FOR    ${page}    IN RANGE    100
        ${ids_raw} =    Get Session IDs From Path    ${ip}    /api/show/subscriber/sessions?limit=10&offset=${offset}
        @{page_ids} =    Split To Lines    ${ids_raw}
        ${returned} =    Get Length    ${page_ids}
        IF    ${returned} == 0    Exit For Loop
        FOR    ${sid}    IN    @{page_ids}
            Should Not Contain    ${seen}    ${sid}    Duplicate session_id ${sid} returned at offset ${offset}
            Append To List    ${seen}    ${sid}
        END
        ${has_more} =    Get Pagination Field From Path    ${ip}    /api/show/subscriber/sessions?limit=10&offset=${offset}    has_more
        ${reported_total} =    Get Total From Path    ${ip}    /api/show/subscriber/sessions?limit=10&offset=${offset}
        Should Be Equal As Integers    ${reported_total}    ${total}    total drifted mid-walk
        IF    '${has_more}' == 'False'    Exit For Loop
        ${offset} =    Evaluate    ${offset} + 10
    END
    ${seen_count} =    Get Length    ${seen}
    Should Be Equal As Integers    ${seen_count}    ${total}

Limit Above Maximum Is Clamped
    [Documentation]    limit=50000 is clamped server-side to MaxLimit (1000).
    ${ip} =    Get Container IPv4    ${bng1}
    ${limit} =    Get Pagination Field From Path    ${ip}    /api/show/subscriber/sessions?limit=50000    limit
    Should Be Equal As Integers    ${limit}    1000

Offset Past End Returns Empty Page
    [Documentation]    offset >> total returns an empty page with has_more=false.
    ${ip} =    Get Container IPv4    ${bng1}
    ${returned} =    Get Pagination Field From Path    ${ip}    /api/show/subscriber/sessions?limit=10&offset=999999    returned
    ${has_more} =    Get Pagination Field From Path    ${ip}    /api/show/subscriber/sessions?limit=10&offset=999999    has_more
    Should Be Equal As Integers    ${returned}    0
    Should Be Equal    ${has_more}    False

Non List Endpoints Are Backwards Compatible
    [Documentation]    /api/show/system/version returns the legacy {path,data}
    ...    envelope with no pagination block.
    ${body} =    Get osvbng API Response    ${bng1}    /api/show/system/version
    Should Contain    ${body}    "path"
    Should Contain    ${body}    "data"
    Should Not Contain    ${body}    "pagination"

*** Keywords ***
Deploy Pagination Topology
    Deploy Topology    ${lab-file}

Teardown Pagination Test
    Run Keyword And Ignore Error    Stop BNG Blaster    ${subscribers}
    Destroy Topology    ${lab-file}

Get Total From Path
    [Arguments]    ${ip}    ${path}
    ${rc}    ${total} =    Run And Return Rc And Output
    ...    curl -sf 'http://${ip}:${OSVBNG_API_PORT}${path}' | python3 -c "import json,sys; print(json.load(sys.stdin).get('pagination',{}).get('total',0))"
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${total}

Get Pagination Field From Path
    [Arguments]    ${ip}    ${path}    ${field}
    ${rc}    ${value} =    Run And Return Rc And Output
    ...    curl -sf 'http://${ip}:${OSVBNG_API_PORT}${path}' | python3 -c "import json,sys; print(json.load(sys.stdin).get('pagination',{}).get('${field}'))"
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${value}

Get Session IDs From Path
    [Arguments]    ${ip}    ${path}
    ${rc}    ${ids} =    Run And Return Rc And Output
    ...    curl -sf 'http://${ip}:${OSVBNG_API_PORT}${path}' | python3 -c "import json,sys; d=json.load(sys.stdin); items=d.get('data') or []; print('\\n'.join((s.get('SessionID','') for s in items)))"
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${ids}
