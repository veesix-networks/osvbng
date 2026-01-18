package veesixtest

import (
	confpaths "github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	showpaths "github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

const (
	ShowStatusPath  = showpaths.Path("veesixtest.status")
	StateStatusPath = statepaths.Path("veesixtest.status")
	ConfMessagePath = confpaths.Path("plugins.veesixtest.message")
)
