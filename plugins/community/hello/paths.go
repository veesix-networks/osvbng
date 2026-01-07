package hello

import (
	confpaths "github.com/veesix-networks/osvbng/pkg/conf/paths"
	showpaths "github.com/veesix-networks/osvbng/pkg/show/paths"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

const (
	ShowStatusPath  = showpaths.Path("example.hello.status")
	StateStatusPath = statepaths.Path("example.hello.status")
	ConfMessagePath = confpaths.Path("example.hello.message")
)
