package ppp

type Phase uint8

const (
	PhaseDead Phase = iota
	PhaseEstablish
	PhaseAuthenticate
	PhaseNetwork
	PhaseOpen
	PhaseTerminate
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
	default:
		return "Unknown"
	}
}
