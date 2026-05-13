# 30-l2tp-lns — placeholder

This directory contains scaffolding for an L2TPv2 LNS Robot test but
is **not yet runnable**. BNG Blaster does not provide a LAC
server-emulation mode that can drive our LNS implementation; the
clab topology, osvbng config, and bngblaster config in this directory
were written against the wrong assumption and need to be rebuilt
once a working LAC is available.

The intended end-state for this suite is a two-osvbng topology:

- `lac1` running osvbng as an L2TPv2 LAC (Phase 7 implementation)
- `lns1` running osvbng as an L2TPv2 LNS (Phase 5 implementation)
- BNG Blaster as PPPoE subscribers terminating on `lac1`
- BGP/transit via `corerouter1` upstream of `lns1`

Optional additional client coverage:

- accel-ppp-ng as a third-party LAC pointing at `lns1`

The files here are kept as a sketch of the topology shape but should
not be considered authoritative until the suite is rebuilt against
the two-osvbng model after Phase 7 lands.
