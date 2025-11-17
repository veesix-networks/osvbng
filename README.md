# Open Source Virtual Broadband Network Gateway (osvbng)

osvbng is a **true** open source vBNG and not just marketed as 'open'. You can explore the source code, raise issues and create PR requests to fix source code.

This project is currently a concept on redefining BNG functionality while trying to keep existing broadband services supported at a network layer. While we would like to fully support TR-459, right now I don't believe its the way forward with BNGs and that we really need to redefine the BNG function in 2025, the goal is to prevent over-engineering and over-complicating such a basic network function.

If this project recieves attention and that people are actually willing to run this in production when its in a stable state, then this readme will be updated with links to join a community slack, to handle more informal discussions about the projects roadmap and progress without using GitHub discussions/issues.

## Initial roadmap

- Architect the v1 solution based on linux netlink being the first southbound data interface, combining XDP+eBPF for control packet inspection and building a basic control+user plane similar to TR-459 while dropping some of the cruft in the specification.
- Build daemon-like codebase for IPoE access technology which handles DHCPv4 (DHCPv6 at a later date), AAA functionality, DHCP relay functionality and basic Subscriber Management functions + database.
- Try to integrate a routing southbound interface so that we can advertise subscriber IP/block reachability over BGP (and potentially introduce VRFs for L3VPN termination)
- Deep research on:
- - PWHE/PWHT with existing netlink solutions
- - DDP for NIC packet classification offloading
- - Intel DPDK with VPP (or just natively with linux, eg. provide a VM on a kvm hypervisor that performs NIC PCIe passthrough)
- - VPP.io southbound integration
- - Subscriber Session Load Balance at the network level
- Basic gNMI/gRPC implementation for Northbound + CLI daemon/codebase to manage simple rule sets
- Build simple user plane + control plane separation as per TR-459 but without PFCP. I just want it to be as simple as using a redis/existing message queue that will drive all of our events. At some point we may want to branch out the various daemon-like codebases into separate containers (incase eg. we need to scale specifically the ppp daemon for PPP hellos)