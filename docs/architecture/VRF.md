# VRF Architecture

VRF support in osvbng spans three layers. Standard IP VRFs only; MAC-VRF
is out of scope.

1. **VPP FIB tables** — dataplane routing tables, one per VRF.
2. **Linux VRF master devices** — kernel-side VRFs in the dataplane
   network namespace.
3. **netbind SDK** — `pkg/netbind`, used by core and plugins to bind
   sockets into a specific VRF.

## VPP FIB tables

Each VRF maps 1:1 to a VPP FIB ID, separately per address family
(IPv4 and IPv6). osvbng's `pkg/vrfmgr` allocates and tracks these
IDs.

The VPP FIB ID and the Linux VRF table ID for the same VRF must
match. Routing daemons (FRR `zebra`) translate FIB writes into netlink
RTM_NEWROUTE on the kernel side; mismatched IDs mean routes land in
the wrong table or get dropped silently.

## Linux VRF

The kernel VRF master device for each configured VRF lives in the
`dataplane` network namespace, not the root namespace. This is forced
by VPP's LCP (Linux Control Plane) plugin: LCP creates the tap
interfaces for each VPP-managed interface inside the dataplane netns,
and those taps must be enslaved to a VRF master device in the same
netns.

The root netns is still used. It is where:

- `osvbngd` itself runs (the process is in root netns; some of its
  sockets live elsewhere, see netbind below).
- Out-of-band management sshd listens.
- Any non-VRF-bound listener defaults to (e.g. the Unix domain
  socket file lives at `/run/osvbng/api.sock`, which is filesystem-
  scoped and reachable from either netns).

FRR runs entirely in the dataplane netns via a systemd drop-in
(`NetworkNamespacePath=/var/run/netns/dataplane`).

## netbind SDK

`pkg/netbind` is the only place in osvbng that opens a VRF-bound
socket. Core and plugins call into it instead of using `net.Listen`
or `net.Dial` directly.

It does two things at bind time:

1. **Switches the calling OS thread into the dataplane netns** so the
   subsequent `SO_BINDTODEVICE` can see the VRF master device. The
   thread is restored to its original netns once the socket is open;
   the socket itself retains the netns for its lifetime.
2. **Applies `SO_BINDTODEVICE`** with the VRF master device name so
   the kernel routes packets through the correct VRF.

Configuration: callers pass `netbind.Binding{VRF: "mgmt-vrf"}`. A
zero binding is a no-op; the socket opens in the calling thread's
current netns with no `SO_BINDTODEVICE`. The mapping from netns name
("dataplane") to the underlying handle is registered once at daemon
startup via `netbind.SetLCPNetNs`.

Two kinds of config structs use this:

- `netbind.ListenerBinding` — single `vrf` field for server-side
  listeners. Inlined into plugin configs (`northbound.api`,
  `exporter.prometheus`, HA listener).
- `netbind.EndpointBinding` — `vrf` plus `source_ip` / `source_ipv6` /
  `source_interface` for outbound clients. Inlined into RADIUS
  server entries, DHCP relay upstreams, HA peer entries.

UDS sockets are not VRF-aware. They are filesystem-scoped (governed
by the mount namespace, not the network namespace) and skip netbind
entirely.
