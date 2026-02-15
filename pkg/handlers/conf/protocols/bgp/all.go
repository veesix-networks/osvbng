package bgp

import (
	_ "github.com/veesix-networks/osvbng/pkg/handlers/conf/protocols/bgp/ipv4/unicast"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/conf/protocols/bgp/ipv6/unicast"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/conf/protocols/bgp/vpn/ipv4"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/conf/protocols/bgp/vpn/ipv6"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/conf/protocols/bgp/vrf/ipv4/unicast"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/conf/protocols/bgp/vrf/ipv6/unicast"
)
