package hello

import (
	confpaths "github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	showpaths "github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

const (
	ShowStatusPath  = showpaths.Path("example.hello.status")
	ConfMessagePath = confpaths.Path("example.hello.message")
)
