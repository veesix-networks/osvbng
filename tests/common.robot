# Copyright 2025 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Library             Collections

*** Variables ***
# preserve-env carries the CI runner's per-slot core assignment through
# sudo so containerlab can expand it in the topologies; unset locally,
# where the ${VAR:=auto} defaults select the auto layout.
${CLAB_BIN}             sudo --preserve-env=OSVBNG_LAB_WORKER_CORES,OSVBNG_LAB_CP_CORES containerlab
${runtime}              docker
${OSVBNG_API_PORT}      8080
# 300 x 1s keeps the same 5-minute budget as the previous 60 x 5s but
# stops quantizing readiness to 5s steps: the daemon typically goes
# healthy 11-15s after deploy, and a coarse poll pays up to a full
# interval past that in every suite.
${HEALTH_RETRIES}       300
${HEALTH_INTERVAL}      1s
${VPPCTL_SOCK}          /run/osvbng/cli.sock
${TEST_LOG_DIR}         /tmp/test-logs

*** Keywords ***
Deploy Topology
    [Arguments]    ${topology_file}
    # Deploys from parallel suites serialize under a host-wide lock:
    # concurrent containerlab deploys race each other's network and
    # container setup and have deadlocked half-created labs. Deploys
    # take seconds, so serializing them costs little; the suites
    # themselves still run in parallel. The lock path carries the uid
    # because flock cannot open another user's lock file (exit 66):
    # CI's runner instances share one uid so they still serialize,
    # and a developer's local runs are single anyway.
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    flock -w 300 /tmp/osvbng-clab-deploy.$(id -u).lock ${CLAB_BIN} deploy -t ${topology_file} --reconfigure
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${output}

Destroy Topology
    [Arguments]    ${topology_file}
    Capture Container Logs    ${topology_file}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    ${CLAB_BIN} destroy -t ${topology_file} --cleanup
    Log    ${output}

Capture Container Logs
    [Arguments]    ${topology_file}
    Create Directory    ${TEST_LOG_DIR}
    ${rc}    ${containers} =    Run And Return Rc And Output
    ...    ${CLAB_BIN} inspect -t ${topology_file} --format json 2>/dev/null | python3 -c "import sys,json; cs=json.load(sys.stdin).get('containers',[]); print(' '.join(c['name'] for c in cs))" 2>/dev/null || true
    IF    '${containers}' != ''
        @{container_list} =    Split String    ${containers}
        FOR    ${container}    IN    @{container_list}
            ${log_file} =    Set Variable    ${TEST_LOG_DIR}/${container}.log
            ${result} =    Run Process    sudo    docker    logs    ${container}    stdout=${log_file}    stderr=STDOUT
            Log    Captured full container logs for ${container} to ${log_file} (rc=${result.rc})    console=yes
            ${tail_result} =    Run Process    tail    -200    ${log_file}
            Log    Container logs for ${container}:\n${tail_result.stdout}    console=no
        END
    END

Get Container IPv4
    [Arguments]    ${container}
    ${rc}    ${ip} =    Run And Return Rc And Output
    ...    sudo docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${container}
    Should Be Equal As Integers    ${rc}    0
    Should Not Be Empty    ${ip}
    RETURN    ${ip}

# The Save * To File keywords exist so suite assertions can run out-of-band
# in a checked script (tests/qos_checks.py and friends) against a captured
# file instead of inline `python3 -c` one-liners piped through the shell:
# every argument below is passed argv-style, so there is no quoting surface
# and no output-in-shell round trip.

Save API Response To File
    [Arguments]    ${container}    ${path}    ${file}
    ${ip} =    Get Container IPv4    ${container}
    ${result} =    Run Process    curl    -sf    http://${ip}:${OSVBNG_API_PORT}${path}    stdout=${file}
    Should Be Equal As Integers    ${result.rc}    0    curl ${path} failed: ${result.stderr}

Save Metrics Scrape To File
    [Arguments]    ${container}    ${file}
    ${ip} =    Get Container IPv4    ${container}
    ${result} =    Run Process    curl    -sf    http://${ip}:9090/metrics    stdout=${file}
    Should Be Equal As Integers    ${result.rc}    0    metrics scrape failed: ${result.stderr}

Save CLI Command Output To File
    [Arguments]    ${container}    ${command}    ${file}
    ${result} =    Run Process    sudo    docker    exec    ${container}
    ...    osvbngcli    --server    http://127.0.0.1:${OSVBNG_API_PORT}    -c    ${command}
    ...    stdout=${file}    stderr=STDOUT
    Should Be Equal As Integers    ${result.rc}    0    osvbngcli -c "${command}" failed, output in ${file}

Run QoS Check
    [Arguments]    @{args}
    ${result} =    Run Process    python3    ${CURDIR}/qos_checks.py    @{args}
    Log    ${result.stdout}
    Should Be Equal As Integers    ${result.rc}    0    qos_checks ${args}[0]: ${result.stdout}
    RETURN    ${result.stdout}

Wait For osvbng Healthy
    [Arguments]    ${node}    ${lab_name}
    ${container} =    Set Variable    clab-${lab_name}-${node}
    Wait Until Keyword Succeeds    ${HEALTH_RETRIES} x    ${HEALTH_INTERVAL}
    ...    Check osvbng Started    ${container}
    Wait Until Keyword Succeeds    ${HEALTH_RETRIES} x    ${HEALTH_INTERVAL}
    ...    Check osvbng API Ready    ${container}

Check osvbng Started
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker logs ${container} 2>&1 | grep -q "osvbng started successfully"
    Should Be Equal As Integers    ${rc}    0    osvbng has not fully started yet

Check osvbng API Ready
    [Arguments]    ${container}
    ${ip} =    Get Container IPv4    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    curl -sf http://${ip}:${OSVBNG_API_PORT}/api/show/system/version
    Should Be Equal As Integers    ${rc}    0    osvbng API not responding yet
    Should Not Be Empty    ${output}

Execute VPP Command
    [Arguments]    ${container}    ${command}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} vppctl -s ${VPPCTL_SOCK} ${command}
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${output}

Execute Vtysh On BNG
    [Arguments]    ${container}    ${command}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} ip netns exec dataplane vtysh -c "${command}"
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${output}

Execute Vtysh On Router
    [Arguments]    ${container}    ${command}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} vtysh -c "${command}"
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${output}

Get osvbng API Response
    [Arguments]    ${container}    ${path}
    ${ip} =    Get Container IPv4    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    curl -sf http://${ip}:${OSVBNG_API_PORT}${path}
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${output}

Verify OSPF Adjacency On Router
    [Arguments]    ${container}
    ${output} =    Execute Vtysh On Router    ${container}    show ip ospf neighbor
    Should Contain    ${output}    Full

Verify BGP Session On Router
    [Arguments]    ${container}    ${neighbor_ip}
    ${output} =    Execute Vtysh On Router    ${container}    show bgp summary
    Should Contain    ${output}    ${neighbor_ip}

Check BGP Route On Router
    [Arguments]    ${container}    ${prefix}
    ${output} =    Execute Vtysh On Router    ${container}    show ip bgp
    Should Contain    ${output}    ${prefix}    BGP route ${prefix} not found on router

Start VPP Trace
    [Arguments]    ${container}    ${input_node}=af-packet-input    ${count}=50
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} vppctl -s ${VPPCTL_SOCK} trace add ${input_node} ${count}
    Log    ${output}

Dump VPP Trace
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} vppctl -s ${VPPCTL_SOCK} show trace
    Log    VPP Trace:\n${output}    console=yes
    RETURN    ${output}

Verify VPP Node Calls Non-Zero
    [Documentation]    Asserts that ${node} shows at least one non-zero
    ...                numeric field across all threads in `show runtime`.
    ...                Use to prove a node has processed packets — first
    ...                positive integer after the node name is the Calls
    ...                column for active/polling nodes (state column may
    ...                be one word "active" or two words "any wait").
    [Arguments]    ${container}    ${node}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker exec ${container} vppctl -s ${VPPCTL_SOCK} show runtime | awk -v n=${node} '$0 ~ "^"n { for (i=2; i<=NF; i++) { if ($i ~ /^[0-9]+$/ && $i+0 > 0) { print "FIRED"; exit } } }'
    Should Be Equal As Integers    ${rc}    0
    Should Contain    ${output}    FIRED
    ...    ${node} shows no non-zero Calls/Vectors on any thread — node is registered but not processing packets
