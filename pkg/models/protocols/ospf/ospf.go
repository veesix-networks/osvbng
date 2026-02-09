package ospf

type Neighbor struct {
	NbrState                           string `json:"nbrState"`
	NbrPriority                        int    `json:"nbrPriority"`
	Converged                          string `json:"converged"`
	Role                               string `json:"role"`
	UpTimeInMsec                       int    `json:"upTimeInMsec"`
	RouterDeadIntervalTimerDueMsec     int    `json:"routerDeadIntervalTimerDueMsec"`
	UpTime                             string `json:"upTime"`
	DeadTime                           string `json:"deadTime"`
	IfaceAddress                       string `json:"ifaceAddress"`
	IfaceName                          string `json:"ifaceName"`
	LinkStateRetransmissionListCounter int    `json:"linkStateRetransmissionListCounter"`
	LinkStateRequestListCounter        int    `json:"linkStateRequestListCounter"`
	DatabaseSummaryListCounter         int    `json:"databaseSummaryListCounter"`
}
