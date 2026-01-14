package subscribers

import "net"

type Statistics struct {
	SessionID string `json:"sessionId" display:"order=1" prometheus:"label"`
	IP        net.IP `json:"ip" display:"order=2" prometheus:"label"`
	RxPackets uint64 `json:"rxPackets" display:"order=3" prometheus:"name=osvbng_subscriber_rx_packets,help=Subscriber received packets,type=counter"`
	RxBytes   uint64 `json:"rxBytes" display:"order=5" prometheus:"name=osvbng_subscriber_rx_bytes,help=Subscriber received bytes,type=counter"`
	TxPackets uint64 `json:"txPackets" display:"order=4" prometheus:"name=osvbng_subscriber_tx_packets,help=Subscriber transmitted packets,type=counter"`
	TxBytes   uint64 `json:"txBytes" display:"order=6" prometheus:"name=osvbng_subscriber_tx_bytes,help=Subscriber transmitted bytes,type=counter"`
}
