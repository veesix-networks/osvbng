package configmgr

import (
	"net/netip"
)

func parsePrefix(addr string) (netip.Prefix, error) {
	return netip.ParsePrefix(addr)
}
