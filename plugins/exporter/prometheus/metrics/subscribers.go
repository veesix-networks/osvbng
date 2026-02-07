package metrics

import (
	"github.com/veesix-networks/osvbng/pkg/models/subscribers"
	"github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	RegisterMetricMulti[subscribers.Statistics](paths.SubscriberSessions)
}
