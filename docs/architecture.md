## Components

These components don't need to be actual "daemons" but just a defined way to architect/layout the application/codebase so if we do ever need to scale parts individually, we can with either multi processing or actual containers

| Daemon | Description |
|----|----|
| ipoed | Responsible for taking DHCP control packets, extracting DHCP options based on configuration logic in the config daemon, and interacting with the aaad and dhcprd daemons |
| pppd | Responsible for L2TP and PPP control packets, session-id generation, etc... and interacting with the aaad, potentially also implementing IPCP inside this daemon |
| subd | Responsible for maintaining subscriber database in local memory, but also syncing with an external db source (redis) for HA, maps ipoe sessions or ppp sessions to subscriber, tracks interface indexes from dpd and also QoS/security management |
| aaad | Responsible for handling sockets for UDP connections to RADIUS servers, generating access-requests, updating stats locally about accepts/rejects, and typically returns back to a daemon to further process the authenticated/unauthenticated subscriber, also handles the mapping for returned radius attributes |
| dhcprd | Responsible for handling sockets for UDP connections to DHCP servers, handles the relay of packets and also the return of offers/acks from the server |
| mond | I don't think this is required because potentially each internal service would update their own monitoring metrics, but maybe this handles external monitoring (eg. syslog and telemetry streaming - opentelemetry? / interacts with northbound systems to present the data) |
| dpd | Responsible for mostly interacting with southbound dataplane integrations and also ingress of eBPF punted packets to put into the event bus |
| routerd | Need to think about this part properly, the BNG control plane might want to inject the /32 route into the routing control plane (eg. FRR or other app) and have more control over just FRR redistributing the interface /32 IP address, if we get a Framed-Route or IPv6 delegated prefix, how does the dataplane know about this? The routing daemon needs to be aware that they need to advertise these into BGP? - We need the ability to at least be able to inform a routing daemon since we are only a subscriber management control plane


## Random Questions that relate to architecture

- Linux support MPLS encap/decap in kernel, but not pusedowire/(vpws)/vpls, what open source dataplanes support this (eg. VPP?)
- Can we redefine how BNGs should be configured rather than following the vendor footstep and providing such a complicated AAA framework DSL?
- IP fragmentation
- Do we want to split pppd into 2 sections (pppd and l2tpd) because of LAC/LNS functionality? However both LAC and LNS still look at the PPP headers
- Do we lock ourself into a corner if we specifically choose eBPF+XDP vs Intel DPDK?
- Do we even want to support vpws/vpls virtual interfaces/termination? Its quite a common pattern across ISPs however if you take the wholesale model of an access network (eg. QinQ handover or LNS) then you will never run into an issue with overlapping S-VLANs from the NNI ingress... Maybe this is no longer a problem unless we wanted large existing ISPs to adopt osvbng
- Do we support multiple northbound interfaces (config + state)
- - Do we just implement gRPC/gNMI only or is it a hard requirement to support SNMP features / HTTP REST/JSON based API
- - What about management of multiple instances? TR-459 defines the control plane being able to monitor user plane instances but there is just so much stuff with TR-459 that overcomplicates BNG functionality :(