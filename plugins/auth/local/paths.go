package local

import (
	confpaths "github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	operpaths "github.com/veesix-networks/osvbng/pkg/handlers/oper/paths"
	showpaths "github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

const (
	ShowUsersPath    = showpaths.Path("subscriber.auth.local.users")
	ShowUserPath     = showpaths.Path("subscriber.auth.local.users.*")
	ShowServicesPath = showpaths.Path("subscriber.auth.local.services")
	ShowServicePath  = showpaths.Path("subscriber.auth.local.services.*")

	OperCreateUserPath          = operpaths.Path("subscriber.auth.local.users.create")
	OperDeleteUserPath          = operpaths.Path("subscriber.auth.local.user.<*>.delete")
	OperSetUserPasswordPath     = operpaths.Path("subscriber.auth.local.user.<*>.password")
	OperSetUserEnabledPath      = operpaths.Path("subscriber.auth.local.user.<*>.enabled")
	OperSetUserServicePath      = operpaths.Path("subscriber.auth.local.user.<*>.service.<*>")
	OperSetUserAttributePath    = operpaths.Path("subscriber.auth.local.user.<*>.attribute")
	OperCreateServicePath       = operpaths.Path("subscriber.auth.local.services.create")
	OperDeleteServicePath       = operpaths.Path("subscriber.auth.local.services.<*>.delete")
	OperSetServiceAttributePath = operpaths.Path("subscriber.auth.local.services.<*>.attribute")

	ConfUsersPath            = confpaths.Path("subscriber.auth.local.users")
	ConfUserPath             = confpaths.Path("subscriber.auth.local.users.<*>")
	ConfUserPasswordPath     = confpaths.Path("subscriber.auth.local.users.<*>.password")
	ConfUserEnabledPath      = confpaths.Path("subscriber.auth.local.users.<*>.enabled")
	ConfUserServicePath      = confpaths.Path("subscriber.auth.local.users.<*>.service.<*>")
	ConfUserAttributePath    = confpaths.Path("subscriber.auth.local.users.<*>.attribute.<*>")
	ConfServicesPath         = confpaths.Path("subscriber.auth.local.services")
	ConfServicePath          = confpaths.Path("subscriber.auth.local.services.<*>")
	ConfServiceAttributePath = confpaths.Path("subscriber.auth.local.services.<*>.attribute.<*>")
)
