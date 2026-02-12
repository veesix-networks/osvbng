# Reference Deployment Methods

This page documents the common subscriber encapsulation models supported by osvbng. Each section shows the Ethernet frame format as seen on the **access-facing interface** (the handover from the access network).

---

How do the packets look in each deployment method?

# IPoE

## N:1 Single Tagged IPoE - Ethernet Handover

```packet
Dst MAC | 6 | ethernet
Src MAC | 6 | ethernet
Outer VLAN | 4 | vlan
EtherType | 2 | ethertype
IP Packet | 0 | payload
---
[Outer VLAN : Outer VLAN] 802.1Q VLAN Header
---
[Dst MAC : IP Packet] Ethernet Frame
```

Subscribers share outer VLAN.

Identified by tuple `Src MAC` + `Outer VLAN ID`.

---

## N:1 Double Tagged IPoE - Ethernet Handover

```packet
Dst MAC | 6 | ethernet
Src MAC | 6 | ethernet
0x88a8 | 2 | ethertype
Outer VLAN | 2 | vlan
0x8100 | 2 | ethertype
Inner VLAN | 2 | vlan
EtherType | 2 | ethertype
IP Packet | 0 | payload
---
[0x88a8 : Outer VLAN] 802.1(ad|Q) S-Tag
[0x8100 : Inner VLAN] 802.1Q C-Tag
---
[Dst MAC : IP Packet] Ethernet Frame
```

Subscribers share outer VLAN but inner VLANs are shared by 1 or more subscribers.

Identified by tuple `Src MAC` + `Outer VLAN ID` + `Inner VLAN ID`.

---

## 1:1 Double Tagged IPoE - Ethernet Handover

```packet
Dst MAC | 6 | ethernet
Src MAC | 6 | ethernet
0x88a8 | 2 | ethertype
Outer VLAN | 2 | vlan
0x8100 | 2 | ethertype
Inner VLAN | 2 | vlan
EtherType | 2 | ethertype
IP Packet | 0 | payload
---
[0x88a8 : Outer VLAN] 802.1(ad|Q) S-Tag
[0x8100 : Inner VLAN] 802.1Q C-Tag
---
[Dst MAC : IP Packet] Ethernet Frame
```

Subscribers are unique across Outer VLAN and Inner VLAN combos.

Identified by `Src MAC` + `Outer VLAN ID` + `Inner VLAN ID`.

---

# PPPoE

## N:1 Single Tagged PPPoE - Ethernet Handover

```packet
Dst MAC | 6 | ethernet
Src MAC | 6 | ethernet
0x8100 | 2 | ethertype
Outer VLAN | 2 | vlan
EtherType | 2 | ethertype
PPPoE | 6 | protocol
PPP | 2 | protocol
IP Packet | 0 | payload
---
[0x8100 : Outer VLAN] 802.1Q S-Tag
[PPPoE : PPP] PPPoE Session
---
[Dst MAC : IP Packet] Ethernet Frame
```

Subscribers share outer VLAN.

Identified by tuple `Src MAC` + `Outer VLAN ID` + `PPP Session ID`.

---

## N:1 Double Tagged PPPoE - Ethernet Handover

```packet
Dst MAC | 6 | ethernet
Src MAC | 6 | ethernet
0x88a8 | 2 | ethertype
Outer VLAN | 2 | vlan
0x8100 | 2 | ethertype
Inner VLAN | 2 | vlan
EtherType | 2 | ethertype
PPPoE | 6 | protocol
PPP | 2 | protocol
IP Packet | 0 | payload
---
[0x88a8 : Outer VLAN] 802.1(ad|Q) S-Tag
[0x8100 : Inner VLAN] 802.1Q C-Tag
[PPPoE : PPP] PPPoE Session
---
[Dst MAC : IP Packet] Ethernet Frame
```

Subscribers share outer VLAN but inner VLANs are shared by 1 or more subscribers.

Identified by tuple `Src MAC` + `Outer VLAN ID` + `Inner VLAN ID` + `PPP Session ID`.

---

## 1:1 Double Tagged PPPoE - Ethernet Handover

```packet
Dst MAC | 6 | ethernet
Src MAC | 6 | ethernet
0x88a8 | 2 | ethertype
Outer VLAN | 2 | vlan
0x8100 | 2 | ethertype
Inner VLAN | 2 | vlan
EtherType | 2 | ethertype
PPPoE | 6 | protocol
PPP | 2 | protocol
IP Packet | 0 | payload
---
[0x88a8 : Outer VLAN] 802.1(ad|Q) S-Tag
[0x8100 : Inner VLAN] 802.1Q C-Tag
[PPPoE : PPP] PPPoE Session
---
[Dst MAC : IP Packet] Ethernet Frame
```

Subscribers are unique across Outer VLAN and Inner VLAN combos.

Identified by `Src MAC` + `Outer VLAN ID` + `Inner VLAN ID` + `PPP Session ID`.