package ip

type DHCPConfig struct {
	Provider      string       `json:"provider,omitempty" yaml:"provider,omitempty"`
	DefaultServer string       `json:"default_server,omitempty" yaml:"default_server,omitempty"`
	Servers       []DHCPServer `json:"servers,omitempty" yaml:"servers,omitempty"`
	Pools         []DHCPPool   `json:"pools,omitempty" yaml:"pools,omitempty"`
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

type DHCPPool struct {
	Name       string   `json:"name,omitempty" yaml:"name,omitempty"`
	Network    string   `json:"network,omitempty" yaml:"network,omitempty"`
	RangeStart string   `json:"range_start,omitempty" yaml:"range_start,omitempty"`
	RangeEnd   string   `json:"range_end,omitempty" yaml:"range_end,omitempty"`
	Gateway    string   `json:"gateway,omitempty" yaml:"gateway,omitempty"`
	DNSServers []string `json:"dns_servers,omitempty" yaml:"dns_servers,omitempty"`
	LeaseTime  uint32   `json:"lease_time,omitempty" yaml:"lease_time,omitempty"`
}
