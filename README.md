# Open Source Virtual Broadband Network Gateway (osvbng)

NOTE: This will not build successfully as I am still cleaning up code and integrating the VPP development (few custom plugins to expose some internals for osvbng, this should be available some time mid Jan 2026)

osvbng is an open source vBNG for basic IPoE (DHCPv4) access subscribers and not just marketed as 'open'. You can explore the source code, raise issues, and create PR requests to fix source code.

This repository can be considered the `osvbng-core` project which open sources a vBNG implementation that can reach hundreds of gigabits (200/400Gbps) with DHCPv4 IPoE subscribers. During the initial phases of this project, a lot will change... I mean A LOT. So please do not depend on this for production deployments as we may change the architecture. However, the initial core architecture has been decided which includes the following components:

- Plugin based architecture for the following core components
    - Access Technology
    - Authentication
    - Cache

We are already 90% of the way through research and development of an "alpha" implementation for IPoE technology and are using this opportunity to outline the expectations of this project, the features that will be available and supported as part of the open source project, but also have a clear line between what we can do in our own time (no funding) vs offering commercial plugins and support to help fund the project and help pay the bills.

To set the current expectations, our open source implementation should be able to achieve the following goals:

- Integrate seamlessly with VPP to provide a native 100Gbps (minimum, with aims to hit 400+Gbps) throughput
- Provide IPoE access termination for only DHCPv4 subscribers (with the possibility of open sourcing our DHCPv6 + PD plugins in the future)
- DHCPv4 Option 82 (Remote-ID sub-opt 2) support using local authentication
- Basic routing integration with BGP/IS-IS/OSPF (FRR) to advertise subscriber pools
- Only Default VRF implementation (No plans for MPLS L3VPN but this can change at any point)
- No QoS/HQoS support from day 1 of the v1.0.0 release
- CLI
- Basic monitoring integration (with Prometheus Exporter)

## Plugin Based Architecture

The idea of the plugin-based architecture is to allow the extension of core components so that the community can extend the BNG functionality. These 3 core components that we believe give the most flexibility to achieve a lot of the important production features are Access Technology, Authentication, and Cache.

A very important distinction here is that plugins which are not certified are community maintained and the core developers are not responsible for maintaining them. We believe this gives the flexibility of extending the BNG itself to fit the user's specified use case which may not align fully with the project scope or end goal of the specific release we are working towards.

### Access Technology

IPoE implementation will be open source, however PPP/L2TP implementation will not. However, the community may build their own plugins to handle non-IPoE implementations and even integrate other network architecture differences such as VXLAN termination, pseudowire termination, etc.

### Authentication

BNGs typically authenticate subscribers via an external source which is mostly (if not always) RADIUS. You don't actually need RADIUS at all to run the BNG function to authenticate subscribers. We can bring the subscriber database directly into the BNG and not maintain any RADIUS infrastructure. Therefore, we want to provide a local authentication method that allows people to tightly couple their BNG with the OSS and move the intent of a subscriber as close as the termination point for the session. This will however be plugin-based to allow the use of external authentication methods like RADIUS or even as a direct HTTP call to the OSS.

### Cache

The Cache functionality in osvbng is used for a variety of tasks, e.g., buffering the DHCP Discover while the authentication occurs, maintaining a cache of metrics so we're not constantly polling the dataplane, and caching subscriber sessions so that if a client restarts and attempts to DHCP Request the same IP an existing session is active for, we don't generate more control plane traffic. Being able to support the cache in a plugin architecture also allows us to support HA across multiple osvbng instances without introducing a complex state sync protocol between all the instances.

# What are the plans

I need to spend some time to scope out and outline the long-term plans for this project. However, as of right now, early 2026, the short-term plans are:

- [] Build QEMU and Docker image
- [] Build documentation
- [] Build a test framework for the control plane features
- [] Cleanup config/show handlers so they work nicely with plugins (auto-register feature with namespace uniqueness)
- [] Monitoring integration

A big cleanup is required in the current codebase because it's based on a private repo which was used during the initial discovery phase, working out how to integrate with VPP and just loads of ideas bundled in without being fully complete. The initial idea began with a daemon-type structure of the project where we could in the future convert the codebase to individual daemons which make it easier to scale out parts of the application when running in a container environment. However, right now I feel like coupling the control plane with the physical hardware makes more sense for the direction of this project. Currently, a tightly coupled application with the hardware (where hardware vertically scales and will just naturally greatly increase as the years go on) feels more like the direction I want to take this project.

Hardware has become impressively fast over the years compared to generic CPUs, hence why we are seeing more and more adoption of general x86 hardware and packet forwarding functions today. Modern networks are becoming increasingly programmable and able to run on non-vendor silicon. We see vendors taking the approach to move away from hardware and focus more efforts on their software these days, and I believe while vendors and global hyperscalers, carriers, etc., are leading the way for the software plane, the hardware plane is becoming less of an issue in the realms of packet forwarding and manipulation. Long live the days of ASICs, and in the next 10-15 years we will certainly see these chips rust away in landfills or in your grandson's lab used to get a feeling of what physical cables feel like because virtual labs are too mainstream and they want "hands-on" experience.

# How can you help?

Right now, talk... Let's explore ideas together and discuss how we can improve the ecosystem around BNGs or even redefine the meaning of a BNG. It doesn't need to be this complex magic black box that terminates subscribers... it just needs to be simple enough so the underlying network of a broadband service is dumb enough to just work, 24/7.

# Architecture

I want this project to be flexible enough that we don't just sit on the first iterated design of the osvbng architecture, but also not rewrite it every 3-4 weeks. The foundation of the high-level architecture I believe has been laid out with a few minor exceptions. BNG is a software function, period. It may program rules into an underlying dataplane (whether this be a software dataplane or into an ASIC chip via its APIs or its own abstracted dataplane SDK), but at the end of the day, osvbng needs to be aware of some parameters to identify a subscriber, apply some logic to this subscriber, and allow (or deny) them access to network resources.

While I have mentioned components before, here is a high-level overview of the architecture. Links to more low-level understanding of the architecture will also be placed below when I get around to documenting them properly.

## Components

A component in osvbng represents the implementation of a large feature set. If you modify a component, you typically swap it out with another implementation of the component rather than plug into its existing functionality. The core features that are open source include:

| Name | Description |
| --- | --- |
| aaa | Base implementation for subscriber authentication and accounting. |
| arp | Responsible for dealing with ARP control plane packets |
| dataplane | FIB programmability/lifecycle, ingress and egress packet handling for control plane packets |
| gateway | gRPC gateway for the current CLI implementation |
| ipoe | Entrypoint for IPoE based control packets |
| routing | Bridge between osvbng and FRR/routing southbound |
| subscriber | Subscriber lifecycle and caching |

## Providers

Providers are the pieces which are fully pluggable and typically sit within a component. A great example is the aaa authentication provider.

If you need a specific implementation to reach out to your OSS instead of using a local authentication database, you can implement your own authentication provider and extend the aaa functionality by implementing the relevant methods and returning the right response.


### Random Notes

References may include `OpenBNG` or `openbng` in the current source code, this is already a term many people use loosely which this project was also originally called during the research and initial findings/development. The only reference I believe exist is a custom VPP plugin I have built to track rx/tx packets/bytes for subscribers without having to build a literal sub-interface per subscriber (also on N:1 VLAN), so this VPP plugin needs to be cleanedup, govpp bindings will be regenerated and reflected in code in the next few days from openbng to osvbng.