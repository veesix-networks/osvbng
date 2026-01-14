package ip

type TableInfo struct {
	TableID uint32 `json:"tableId"`
	Name    string `json:"name"`
	IsIPv6  bool   `json:"isIpv6"`
}
