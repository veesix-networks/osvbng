package ip

type DHCPConfig struct {
	Provider      string       `json:"provider,omitempty" yaml:"provider,omitempty"`
	DefaultServer string       `json:"default_server,omitempty" yaml:"default_server,omitempty"`
	Servers       []DHCPServer `json:"servers,omitempty" yaml:"servers,omitempty"`
	Pools         []IPv4Pool   `json:"pools,omitempty" yaml:"pools,omitempty"`
}

func (d *DHCPConfig) GetServer(name string) *DHCPServer {
	for i := range d.Servers {
		if d.Servers[i].Name == name {
			return &d.Servers[i]
		}
	}
	return nil
}

type DHCPServer struct {
	Name    string `json:"name,omitempty" yaml:"name,omitempty"`
	Address string `json:"address,omitempty" yaml:"address,omitempty"`
	GIAddr  string `json:"giaddr,omitempty" yaml:"giaddr,omitempty"`
}

type DHCPv6Config struct {
	Provider   string        `json:"provider,omitempty" yaml:"provider,omitempty"`
	IANAPools  []IANAPool    `json:"iana_pools,omitempty" yaml:"iana_pools,omitempty"`
	PDPools    []PDPool      `json:"pd_pools,omitempty" yaml:"pd_pools,omitempty"`
	DNSServers []string      `json:"dns_servers,omitempty" yaml:"dns_servers,omitempty"`
	DomainList []string      `json:"domain_list,omitempty" yaml:"domain_list,omitempty"`
	RA         *IPv6RAConfig `json:"ra,omitempty" yaml:"ra,omitempty"`
}
