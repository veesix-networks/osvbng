# Copyright 2025 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

*** Settings ***
Library             OperatingSystem
Library             String
Library             Process
Resource            common.robot

*** Keywords ***
Create Local Auth User
    [Arguments]    ${container}    ${username}    ${enabled}=true
    ${ip} =    Get Container IPv4    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    curl -sf -X POST http://${ip}:${OSVBNG_API_PORT}/api/exec/subscriber/auth/local/users/create -H "Content-Type: application/json" -d '{"username":"${username}","enabled":${enabled}}'
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${output}

Create Local Auth User With Password
    [Arguments]    ${container}    ${username}    ${password}    ${enabled}=true
    ${ip} =    Get Container IPv4    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    curl -sf -X POST http://${ip}:${OSVBNG_API_PORT}/api/exec/subscriber/auth/local/users/create -H "Content-Type: application/json" -d '{"username":"${username}","password":"${password}","enabled":${enabled}}'
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${output}

Delete Local Auth User
    [Arguments]    ${container}    ${user_id}
    ${ip} =    Get Container IPv4    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    curl -sf -X POST http://${ip}:${OSVBNG_API_PORT}/api/exec/subscriber/auth/local/user/${user_id}/delete -H "Content-Type: application/json"
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${output}

Set Local Auth User Password
    [Arguments]    ${container}    ${user_id}    ${password}
    ${ip} =    Get Container IPv4    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    curl -sf -X POST http://${ip}:${OSVBNG_API_PORT}/api/exec/subscriber/auth/local/user/${user_id}/password -H "Content-Type: application/json" -d '{"password":"${password}"}'
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${output}

Get Local Auth Users
    [Arguments]    ${container}
    ${ip} =    Get Container IPv4    ${container}
    ${rc}    ${output} =    Run And Return Rc And Output
    ...    curl -sf http://${ip}:${OSVBNG_API_PORT}/api/show/subscriber/auth/local/users
    Log    ${output}
    Should Be Equal As Integers    ${rc}    0
    RETURN    ${output}

Create IPoE Users
    [Arguments]    ${container}    ${count}    ${prefix}=DEV-
    FOR    ${i}    IN RANGE    1    ${count} + 1
        Create Local Auth User    ${container}    ${prefix}${i}
    END

Create PPPoE Users
    [Arguments]    ${container}    ${count}    ${password}=test    ${prefix}=user
    FOR    ${i}    IN RANGE    1    ${count} + 1
        Create Local Auth User With Password    ${container}    ${prefix}${i}    ${password}
    END
