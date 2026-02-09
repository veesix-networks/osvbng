package all

import (
	_ "github.com/veesix-networks/osvbng/pkg/handlers/show/aaa"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/show/ip"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/show/plugins"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/show/protocols/bgp"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/show/protocols/isis"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/show/protocols/ospf"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/show/protocols/ospf6"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/show/subscriber"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/show/system"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/show/system/dataplane"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/show/vrf"
)
