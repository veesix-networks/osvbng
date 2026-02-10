package metrics

import (
	"github.com/veesix-networks/osvbng/pkg/state/paths"
)

type subscriberSessionStats struct {
	Total    uint32 `json:"total" prometheus:"name=osvbng_subscriber_sessions_total,help=Total number of subscriber sessions,type=gauge"`
	IPoEV4   uint32 `json:"ipoe_v4" prometheus:"name=osvbng_subscriber_sessions_ipoe_v4,help=Number of IPoE IPv4 subscriber sessions,type=gauge"`
	IPoEV6   uint32 `json:"ipoe_v6" prometheus:"name=osvbng_subscriber_sessions_ipoe_v6,help=Number of IPoE IPv6 subscriber sessions,type=gauge"`
	PPP      uint32 `json:"ppp" prometheus:"name=osvbng_subscriber_sessions_ppp,help=Number of PPP subscriber sessions,type=gauge"`
	Active   uint32 `json:"active" prometheus:"name=osvbng_subscriber_sessions_active,help=Number of active subscriber sessions,type=gauge"`
	Released uint32 `json:"released" prometheus:"name=osvbng_subscriber_sessions_released,help=Number of released subscriber sessions,type=gauge"`
}

func init() {
	RegisterMetricSingle[subscriberSessionStats](paths.SubscriberStats)
}
