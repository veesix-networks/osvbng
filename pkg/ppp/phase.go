package ppp

type Phase uint8

const (
	PhaseDead Phase = iota
	PhaseEstablish
	PhaseAuthenticate
	PhaseNetwork
	PhaseOpen
	PhaseTerminate

	// PhaseLACTunnelPending is the PPPoE LAC handoff staging state.
	// AAA returned Tunnel-* attributes and the L2TP component is
	// bringing up the tunnel/session. Local NCP and PAP/CHAP-Ack are
	// suppressed until the L2TP component reports back via the LAC
	// decision event.
	PhaseLACTunnelPending

	// PhaseLACTunneled is the steady state for a tunneled PPPoE
	// session: PPP frames are bridged to the LNS via the dataplane and
	// the local control plane only handles peer-driven teardown
	// (PADT / CDN / StopCCN).
	PhaseLACTunneled
)

func (p Phase) String() string {
	switch p {
	case PhaseDead:
		return "Dead"
	case PhaseEstablish:
		return "Establish"
	case PhaseAuthenticate:
		return "Authenticate"
	case PhaseNetwork:
		return "Network"
	case PhaseOpen:
		return "Open"
	case PhaseTerminate:
		return "Terminate"
	case PhaseLACTunnelPending:
		return "LACTunnelPending"
	case PhaseLACTunneled:
		return "LACTunneled"
	default:
		return "Unknown"
	}
}
