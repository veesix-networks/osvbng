package southbound

type Punt interface {
	EnableARPPunt(ifaceName string) error
	EnableDHCPv4Punt(ifaceName string) error
	EnableDHCPv6Punt(ifaceName string) error
	EnableIPv6NDPunt(ifaceName string) error
	EnableL2TPPunt(ifaceName string) error
	EnablePPPoEPunt(ifaceName string) error

	DisableARPReply(ifaceName string) error

	DumpPuntRegistrations() ([]PuntRegistration, error)
	GetPuntStats() ([]PuntStats, error)
	ConfigurePuntPolicer(protocol uint8, rate float64, burst uint32) error
}
