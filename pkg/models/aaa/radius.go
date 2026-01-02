package aaa

import "time"

type ServerStats struct {
	Address       string    `json:"address" prometheus:"label"`
	AuthRequests  uint64    `json:"authRequests" prometheus:"name=osvbng_radius_auth_requests_total,help=Total RADIUS authentication requests,type=counter"`
	AuthAccepts   uint64    `json:"authAccepts" prometheus:"name=osvbng_radius_auth_accepts_total,help=Total RADIUS authentication accepts,type=counter"`
	AuthRejects   uint64    `json:"authRejects" prometheus:"name=osvbng_radius_auth_rejects_total,help=Total RADIUS authentication rejects,type=counter"`
	AuthTimeouts  uint64    `json:"authTimeouts" prometheus:"name=osvbng_radius_auth_timeouts_total,help=Total RADIUS authentication timeouts,type=counter"`
	AuthErrors    uint64    `json:"authErrors" prometheus:"name=osvbng_radius_auth_errors_total,help=Total RADIUS authentication errors,type=counter"`
	AcctRequests  uint64    `json:"acctRequests" prometheus:"name=osvbng_radius_acct_requests_total,help=Total RADIUS accounting requests,type=counter"`
	AcctResponses uint64    `json:"acctResponses" prometheus:"name=osvbng_radius_acct_responses_total,help=Total RADIUS accounting responses,type=counter"`
	AcctTimeouts  uint64    `json:"acctTimeouts" prometheus:"name=osvbng_radius_acct_timeouts_total,help=Total RADIUS accounting timeouts,type=counter"`
	AcctErrors    uint64    `json:"acctErrors" prometheus:"name=osvbng_radius_acct_errors_total,help=Total RADIUS accounting errors,type=counter"`
	LastError     string    `json:"lastError" prometheus:"label"`
	LastErrorTime time.Time `json:"lastErrorTime" prometheus:"name=osvbng_radius_last_error_timestamp,help=Last error timestamp,type=gauge"`
}
