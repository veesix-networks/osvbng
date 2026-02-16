package southbound

type IPv6 interface {
	EnableIPv6(ifaceName string) error
	DumpIPv6Enabled() ([]uint32, error)

	ConfigureIPv6RA(ifaceName string, config IPv6RAConfig) error
	DumpIPv6RA() ([]IPv6RAInfo, error)

	EnableDHCPv6Multicast(ifaceName string) error
}
