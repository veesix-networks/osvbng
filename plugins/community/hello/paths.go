package hello

import (
	confpaths "github.com/veesix-networks/osvbng/pkg/conf/paths"
	showpaths "github.com/veesix-networks/osvbng/pkg/show/paths"
)

const (
	ShowStatusPath  = showpaths.Path("example.hello.status")
	ConfMessagePath = confpaths.Path("plugins.example.hello.message")
)
