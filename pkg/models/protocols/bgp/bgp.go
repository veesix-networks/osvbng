package bgp

type Statistics struct {
	Instance            string  `json:"instance" prometheus:"label"`
	TotalPrefixes       int     `json:"totalPrefixes" prometheus:"name=osvbng_bgp_total_prefixes,help=Total BGP prefixes,type=gauge"`
	TotalAdvertisements int     `json:"totalAdvertisements" prometheus:"name=osvbng_bgp_total_advertisements,help=Total BGP advertisements,type=gauge"`
	AveragePrefixLength float64 `json:"averagePrefixLength" prometheus:"name=osvbng_bgp_average_prefix_length,help=Average BGP prefix length,type=gauge"`
}
