# 31-l2tp-lac — osvbng-LAC ↔ bngblaster-LNS

End-to-end test of the L2TPv2 LAC path. osvbng-LAC receives a PPPoE
subscriber from bngblaster, AAA returns Tunnel-* attributes pointing at
bngblaster's LNS side, and osvbng establishes an L2TPv2 tunnel + session
to bridge the subscriber's PPP frames to the LNS.

## Topology

```
+--------------+         +-------------+
|  bngblaster  |---eth1--|  bng1 eth1  |  PPPoE access (L2, VLAN 200)
|  (subscriber |         |             |
|   + LNS)     |---eth2--|  bng1 eth2  |  L2TPv2 backbone (L3,
|              |         |             |  10.0.0.0/30)
+--------------+         +-------------+
                                  |
                                  eth3
                                  |
                          +---------------+
                          |  corerouter1  |  optional FRR core
                          +---------------+
```

bngblaster runs two roles in one process:
- **PPPoE subscriber** on eth1 (CHAP, agent-remote-id = `user1`)
- **L2TP LNS** on eth2 (address `10.0.0.2`, shared secret `shared`)

## Auth model

osvbng-LAC's AAA policy is keyed by `agent-remote-id` with
`authenticate: false`. The LAC never validates CHAP locally — that
happens at the LNS via the proxy-auth AVPs we forward in ICCN. Local
auth is a lookup table: `user1` → Tunnel-Type=L2TP +
Tunnel-Server-Endpoint=10.0.0.2 + Tunnel-Password=shared.

## What this exercises

- PPPoE subscriber bring-up to PhaseAuthenticate
- AAA request with `format: $agent-remote-id$`
- `shouldTunnelToLAC` branch on AAA reply
- AddPPPoESession at LAC handoff (sw_if_index for opaque)
- L2TP control: SCCRQ → SCCRP → SCCCN → ICRQ → ICRP → ICCN
- AddL2TPSessionRaw + returned pool index for the LAC opaque
- SetPPPoESessionLACTunneled flipping the dataplane bridge
- PAP/CHAP-Ack to subscriber, Phase = PhaseLACTunneled
- PPP frames bridged through the dataplane in both directions
