package all

import (
	_ "github.com/veesix-networks/osvbng/pkg/handlers/conf/interface"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/conf/interface/subinterfaces"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/conf/internal"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/conf/protocols/bgp"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/conf/protocols/isis"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/conf/protocols/ospf"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/conf/protocols/ospf6"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/conf/protocols/static"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/conf/system"
)
