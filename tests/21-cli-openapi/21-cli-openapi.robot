# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Comments ***
Integration tests for the OpenAPI-generated CLI and candidate-session config
flow (osvbng-context issue #26). Deploys a single-node containerlab topology
with the northbound API and community.hello plugin enabled, then exercises:
  - OpenAPI contract serving, ETag, and conditional revalidation
  - Generated show commands including path-param placeholders
  - Immediate scalar set without --value
  - configure -> set -> commit -> verify running config changed
  - configure -> set -> discard -> verify running config unchanged
  - Config lock contention (409), lock release after commit/discard
  - Best-effort discard cleanup on exit/quit
  - CLI binary help and ? suggestion sanity

Run:
  make robot-test suite=21-cli-openapi
  scripts/run-qa-tests.sh -t 21-cli-openapi -r 3

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Library             Collections
Resource            ../common.robot

Suite Setup         Deploy CLI Topology
Suite Teardown      Teardown CLI Topology

*** Variables ***
${lab-name}         osvbng-cli-openapi
${lab-file}         ${CURDIR}/21-cli-openapi.clab.yml
${bng1}             clab-${lab-name}-bng1

*** Test Cases ***

# --- 1. CLI startup against a live daemon with /api/openapi.json -----------

OpenAPI Contract Is Served With Show Set And Exec Paths
    [Documentation]    GET /api/openapi.json returns a valid spec containing
    ...    show, set, and exec paths.
    ${ip} =    Get Container IPv4    ${bng1}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    curl -sf http://${ip}:${OSVBNG_API_PORT}/api/openapi.json -o /tmp/rf-openapi.json
    Should Be Equal As Integers    ${rc}    0
    ${rc}    ${result} =    Run And Return Rc And Output
    ...    python3 -c "import json; ps=list(json.load(open('/tmp/rf-openapi.json')).get('paths',{}).keys()); print(any('/api/show/' in p for p in ps), any('/api/set/' in p for p in ps), any('/api/exec/' in p for p in ps))"
    Should Be Equal    ${result}    True True True

OpenAPI Contract Returns ETag Header
    [Documentation]    ETag allows CLI cache-revalidation without a full
    ...    re-download.
    ${ip} =    Get Container IPv4    ${bng1}
    ${rc}    ${headers} =    Run And Return Rc And Output
    ...    curl -sI http://${ip}:${OSVBNG_API_PORT}/api/openapi.json
    Should Be Equal As Integers    ${rc}    0
    Should Match Regexp    ${headers}    (?i)etag:\\s*\\S+

OpenAPI Conditional Request Returns 304
    [Documentation]    If-None-Match with the current ETag returns 304.
    ${ip} =    Get Container IPv4    ${bng1}
    ${etag} =    Get ETag    ${ip}
    ${rc}    ${status} =    Run And Return Rc And Output
    ...    curl -s -o /dev/null -w '\%{http_code}' -H 'If-None-Match: ${etag}' http://${ip}:${OSVBNG_API_PORT}/api/openapi.json
    Should Be Equal    ${status}    304

# --- 2. Generated show commands from OpenAPI --------------------------------

Show Running Config Returns Data
    [Documentation]    Canonical GET /api/show/running-config.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/running-config
    Should Contain    ${output}    "path"
    Should Contain    ${output}    running-config

Show Config History Returns Versions
    [Documentation]    GET /api/show/config/history returns a version list.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/config/history
    Should Contain    ${output}    config.history

Show Interfaces Via Generated Path
    [Documentation]    Exercises a generated show path derived from OpenAPI.
    ${output} =    Get osvbng API Response    ${bng1}    /api/show/interfaces
    Should Contain    ${output}    "path"

# --- 3. Immediate scalar set without --value --------------------------------

Immediate Set Changes Running Config
    [Documentation]    POST /api/set/ applies and commits atomically.
    [Teardown]    Restore Description    Test Interface
    ${ip} =    Get Container IPv4    ${bng1}
    Post JSON    ${ip}    /api/set/interfaces/eth1/description    "robot-immediate-set"
    ${desc} =    Get Interface Description    ${ip}    eth1
    Should Be Equal    ${desc}    robot-immediate-set

# --- 4. configure -> set -> commit -> verify --------------------------------

Config Session Set And Commit Changes Running Config
    [Documentation]    Create session, session-scoped set, commit, verify.
    [Teardown]    Restore Description    Test Interface
    ${ip} =    Get Container IPv4    ${bng1}
    ${sid} =    Create Config Session    ${ip}
    Post JSON    ${ip}    /api/config/session/${sid}/set/interfaces/eth1/description    "robot-commit-test"
    Post JSON    ${ip}    /api/config/session/${sid}/commit    ${EMPTY}
    ${desc} =    Get Interface Description    ${ip}    eth1
    Should Be Equal    ${desc}    robot-commit-test

# --- 5. configure -> set -> discard -> verify unchanged ---------------------

Config Session Set And Discard Preserves Running Config
    [Documentation]    Candidate changes are discarded without touching running
    ...    config.
    ${ip} =    Get Container IPv4    ${bng1}
    ${original} =    Get Interface Description    ${ip}    eth1
    ${sid} =    Create Config Session    ${ip}
    Post JSON    ${ip}    /api/config/session/${sid}/set/interfaces/eth1/description    "robot-discard-test"
    Post JSON    ${ip}    /api/config/session/${sid}/discard    ${EMPTY}
    ${after} =    Get Interface Description    ${ip}    eth1
    Should Be Equal    ${after}    ${original}

# --- 6. Lock contention: second client cannot acquire config session --------

Second Config Session Returns 409
    [Documentation]    Only one candidate session may be active at a time.
    [Teardown]    Force Discard Session    ${sid}
    ${ip} =    Get Container IPv4    ${bng1}
    ${sid} =    Create Config Session    ${ip}
    ${rc}    ${status} =    Run And Return Rc And Output
    ...    curl -s -o /dev/null -w '\%{http_code}' -X POST http://${ip}:${OSVBNG_API_PORT}/api/config/session
    Should Be Equal    ${status}    409

# --- 7. Lock release after commit ------------------------------------------

Lock Released After Commit
    [Documentation]    Committing a session releases the lock for a new session.
    [Teardown]    Restore Description    Test Interface
    ${ip} =    Get Container IPv4    ${bng1}
    ${first} =    Create Config Session    ${ip}
    Post JSON    ${ip}    /api/config/session/${first}/set/interfaces/eth1/description    "robot-lock-commit"
    Post JSON    ${ip}    /api/config/session/${first}/commit    ${EMPTY}
    ${second} =    Create Config Session    ${ip}
    Should Not Be Empty    ${second}
    Post JSON    ${ip}    /api/config/session/${second}/discard    ${EMPTY}

# --- 8. Lock release after discard -----------------------------------------

Lock Released After Discard
    [Documentation]    Discarding a session releases the lock for a new session.
    ${ip} =    Get Container IPv4    ${bng1}
    ${first} =    Create Config Session    ${ip}
    Post JSON    ${ip}    /api/config/session/${first}/discard    ${EMPTY}
    ${second} =    Create Config Session    ${ip}
    Should Not Be Empty    ${second}
    Post JSON    ${ip}    /api/config/session/${second}/discard    ${EMPTY}

# --- 9. Best-effort discard on exit/quit -----------------------------------

Discard Cleans Up Session And Preserves Config
    [Documentation]    After discard the session ID is invalid (the server-side
    ...    cleanup that CLI exit/quit best-effort discard relies on).
    ${ip} =    Get Container IPv4    ${bng1}
    ${original} =    Get Interface Description    ${ip}    eth1
    ${sid} =    Create Config Session    ${ip}
    Post JSON    ${ip}    /api/config/session/${sid}/set/interfaces/eth1/description    "robot-exit-test"
    Post JSON    ${ip}    /api/config/session/${sid}/discard    ${EMPTY}
    ${rc}    ${status} =    Run And Return Rc And Output
    ...    curl -s -o /dev/null -w '\%{http_code}' -X POST -H 'Content-Type: application/json' -d '"probe"' http://${ip}:${OSVBNG_API_PORT}/api/config/session/${sid}/set/interfaces/eth1/description
    Should Be Equal    ${status}    404
    ${after} =    Get Interface Description    ${ip}    eth1
    Should Be Equal    ${after}    ${original}

# --- 10. Basic ? help sanity for generated commands -------------------------

CLI Help Shows Generated Command Groups
    [Documentation]    The help built-in lists show, exec, and set from the
    ...    OpenAPI contract.
    ${output} =    Run CLI In Container    help\nexit\n
    Should Contain    ${output}    show
    Should Contain    ${output}    set

CLI Positional Scalar Set Without Value Flag
    [Documentation]    Top-level scalar config paths accept the value as a
    ...    positional argument without --value.
    ${output} =    Run CLI In Container    set example hello message robot-positional\nexit\n
    Should Contain    ${output}    OK

CLI Question Mark Shows Suggestions For Show
    [Documentation]    Typing 'show ?' lists next-level show subcommands.
    ${output} =    Run CLI In Container    show ?\nexit\n
    ${has_suggestion} =    Evaluate
    ...    any(w in """${output}""" for w in ['interfaces', 'running-config', 'system', 'config'])
    Should Be True    ${has_suggestion}

*** Keywords ***

Deploy CLI Topology
    Deploy Topology    ${lab-file}
    Wait For osvbng Healthy    bng1    ${lab-name}
    Verify Config Session Endpoint    ${bng1}

Teardown CLI Topology
    Destroy Topology    ${lab-file}
    Remove File    /tmp/rf-openapi.json

Verify Config Session Endpoint
    [Arguments]    ${container}
    ${ip} =    Get Container IPv4    ${container}
    ${rc}    ${status} =    Run And Return Rc And Output
    ...    curl -s -o /tmp/rf-session-check.json -w '\%{http_code}' -X POST http://${ip}:${OSVBNG_API_PORT}/api/config/session
    IF    '${status}' == '404'
        Fail    POST /api/config/session returned 404 -- daemon is stale, rebuild osvbng
    END
    IF    '${status}' == '201'
        ${rc2}    ${sid} =    Run And Return Rc And Output
        ...    python3 -c "import json; print(json.load(open('/tmp/rf-session-check.json')).get('session_id',''))"
        IF    '${rc2}' == '0' and '${sid}' != ''
            Run    curl -s -X POST http://${ip}:${OSVBNG_API_PORT}/api/config/session/${sid}/discard > /dev/null 2>&1
        END
    END
    Remove File    /tmp/rf-session-check.json

Get ETag
    [Arguments]    ${ip}
    ${rc}    ${headers} =    Run And Return Rc And Output
    ...    curl -sI http://${ip}:${OSVBNG_API_PORT}/api/openapi.json
    Should Be Equal As Integers    ${rc}    0
    ${rc}    ${etag} =    Run And Return Rc And Output
    ...    echo '${headers}' | grep -i '^etag:' | head -1 | sed 's/^[^:]*:[[:space:]]*//' | tr -d '\r\n'
    RETURN    ${etag}

Post JSON
    [Arguments]    ${ip}    ${path}    ${body}
    IF    '${body}' != ''
        ${rc}    ${output} =    Run And Return Rc And Output
        ...    curl -sf -X POST -H 'Content-Type: application/json' -d '${body}' http://${ip}:${OSVBNG_API_PORT}${path}
    ELSE
        ${rc}    ${output} =    Run And Return Rc And Output
        ...    curl -sf -X POST http://${ip}:${OSVBNG_API_PORT}${path}
    END
    Should Be Equal As Integers    ${rc}    0    POST ${path} failed
    RETURN    ${output}

Create Config Session
    [Arguments]    ${ip}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    curl -sf -X POST http://${ip}:${OSVBNG_API_PORT}/api/config/session
    Should Be Equal As Integers    ${rc}    0    Failed to create config session
    ${rc}    ${sid} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; print(json.load(sys.stdin).get('session_id',''))"
    Should Be Equal As Integers    ${rc}    0
    Should Not Be Empty    ${sid}
    RETURN    ${sid}

Get Interface Description
    [Arguments]    ${ip}    ${iface}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    curl -sf http://${ip}:${OSVBNG_API_PORT}/api/show/running-config
    Should Be Equal As Integers    ${rc}    0
    ${rc}    ${desc} =    Run And Return Rc And Output
    ...    echo '${output}' | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',{}).get('interfaces',{}).get('${iface}',{}).get('description',''))"
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${desc}

Save Interface Description
    [Arguments]    ${iface}=eth1
    ${ip} =    Get Container IPv4    ${bng1}
    ${desc} =    Get Interface Description    ${ip}    ${iface}
    Set Suite Variable    ${SAVED_DESCRIPTION}    ${desc}

Restore Description
    [Arguments]    ${expected_original}
    ${ip} =    Get Container IPv4    ${bng1}
    Run And Return Rc And Output
    ...    curl -sf -X POST -H 'Content-Type: application/json' -d '"${expected_original}"' http://${ip}:${OSVBNG_API_PORT}/api/set/interfaces/eth1/description

Force Discard Session
    [Arguments]    ${session_id}
    ${ip} =    Get Container IPv4    ${bng1}
    Run    curl -s -X POST http://${ip}:${OSVBNG_API_PORT}/api/config/session/${session_id}/discard > /dev/null 2>&1

Run CLI In Container
    [Arguments]    ${input_text}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${bng1} sh -c "printf '${input_text}' | osvbngcli --server http://127.0.0.1:8080"
    Log    ${output}
    RETURN    ${output}
