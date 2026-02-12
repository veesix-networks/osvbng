# Overview

This is a single source of truth for the osvbng roadmap which is taking the concepts from my ([BSpendlove](https://github.com/BSpendlove)) brain and putting it into a document so that everyone is aware of the direction of this project.

The concept of osvbng is simply to build a BNG targeted towards a typical deployment where the ISP doesn't own any access network and traffic is typically handed off by the access network provider as 802.1Q tagged Ethernet frames. This is mostly:

- Untagged (Rare if not never used)
- Single Tagged (802.1Q - Not extremely rare, but not common)
- Double Tagged (QinQ, 802.1AD + 802.1Q or 802.1Q + 802.1Q if you're weird and want more CPU cycles on layer 2 processing)

The goal is not to support every single access network method, eg.

- VPLS bridges with LDP+BGP MPLS tunnels terminated directly on the BNG as a pseudowire/virtual interface
- LDP/EVPN based VPWS terminated directly on the BNG
- VXLAN termination
- etc...

BNGs in 2026 need to be dumb. Not have 500 different feature sets with 1000s of ways to configure it. There are pros and cons on both sides to keeping it as simple as an ethernet handover vs bringing in MPLS based tunnel termination directly into the BNG, this is NOT my vision of osvbng.

The traditional deployments of bringing complex network overlays into the BNG is typically to solve problems of a network deployment architecture issue, not the actual BNG itself. Therefore a BNG done correctly (AAA, VLAN termination, Subscriber Management, QoS and DHCP/PPP processing) is more valuable in my eyes rather than a BNG that can support every type of network deployment architecture. osvbng could potentially expose the internals of the dataplane (via osvbng itself as an SDK) so that people can introduce new access termination methods, but it's not the core goal, I just simply want to build a fast, reliable, low-cost and dumb BNG.

Separating control and user plane (CUPS) in the BNG world is also not the direction of osvbng. As per the plan, we will always re-evaluate the direction of this project but CUPS is overly complex and not the way forward for BNGs in my personal opinion, it may be wrong but until I see proven otherwise then it's not wrong to me.

---

The 3 main deployment methods are described in the following documents:

- [Single Tagged - N:1](../../architecture/REFERENCE_DEPLOYMENT_METHODS) 
- [Double Tagged - N:1](../../architecture/REFERENCE_DEPLOYMENT_METHODS) 
- [Double Tagged - 1:1](../../architecture/REFERENCE_DEPLOYMENT_METHODS)

---

## The plan

So what is the plan? Initially I am winging it, however what I have in my head:

### Every

#### Week

- Re-evaluate/Add any top priority features/fixes for the following week
- Community update (short status post - Slack/GitHub Discussions)
- Triage new issues and PRs

#### Fortnight

- Review and merge outstanding PRs which have been open for more than a few days
- Core team review/call and document talking points

#### Month

- Re-evaluate documentation
- Feature/Fix cleanup
- Re-context my brain with existing solutions and problems in production
- Publish a summary (what shipped, what's next), not like a release summary from GitHub but just a monthly review
- Reach out to known deployments/lab testers for feedback on pain points (this will mostly be more common than monthly, but when the project is more stable that I don't need to help someone after a month of uptime...)

#### 3 Months

- Roadmap review - reprioritize backlog against real-world deployment feedback
- Dependency audit (VPP version compatibility, Go module updates, security patches)
- Performance benchmarking against previous quarter (catch regressions early) - Hopefully this will be automated and just a part of each minor/major release
- Write a short blog post / project update for visibility

#### 6 Months

- Broader architecture review - are the current abstractions holding up?
- Evaluate community contributions and identify areas where docs/onboarding could improve
- Review deployment methods - are the N:1 / 1:1 docs still accurate to reality?

#### 12 Months

- Annual project retrospective - what worked, what didn't, what could work, etc...
- Re-evaluate project scope and vision (is the "BNGs should be dumb" thesis still right?)
- Major version planning if breaking changes have built up
- Review CI/CD pipeline, test coverage, and build tooling for modernization
- Write a "State of osvbng" post covering the year's progress and next year's direction