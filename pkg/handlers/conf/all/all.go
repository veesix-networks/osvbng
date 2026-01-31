package all

import (
	_ "github.com/veesix-networks/osvbng/pkg/handlers/conf/interface"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/conf/interface/subinterfaces"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/conf/internal"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/conf/protocols/bgp"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/conf/protocols/static"
)
