# Copyright 2025 Veesix Networks Ltd
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Library             Collections

*** Variables ***
${CLAB_BIN}             sudo containerlab
${runtime}              docker
${OSVBNG_API_PORT}      8080
${HEALTH_RETRIES}       12
${HEALTH_INTERVAL}      5s
${VPPCTL_SOCK}          /run/osvbng/cli.sock
${TEST_LOG_DIR}         /tmp/test-logs

*** Keywords ***
Deploy Topology
    [Arguments]    ${topology_file}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    ${CLAB_BIN} deploy -t ${topology_file} --reconfigure
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

Wait For osvbng Healthy
    [Arguments]    ${node}    ${lab_name}
    ${container} =    Set Variable    clab-${lab_name}-${node}
    Wait Until Keyword Succeeds    ${HEALTH_RETRIES} x    ${HEALTH_INTERVAL}
    ...    Check osvbng Started    ${container}

Check osvbng Started
    [Arguments]    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    sudo docker logs ${container} 2>&1 | grep -q "osvbng started successfully"
    Should Be Equal As Integers    ${rc}    0    osvbng has not fully started yet

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
