# Adding NIC Vendor Support

NIC vendor implementations live in `pkg/config/system/nic/`. Each
vendor has its own file.

## 1. Create a vendor file

Example `pkg/config/system/nic/broadcom.go`:

```go
package nic

type Broadcom struct{}

func (b Broadcom) Name() string { return "Broadcom" }

func (b Broadcom) Match(vendorID string) bool {
    return vendorID == "14e4"
}

func (b Broadcom) BindStrategy() BindStrategy {
    return BindStrategyVFIO
}
```

## 2. Register the vendor

In `pkg/config/system/nic/all.go`:

```go
func init() {
    Register(Mellanox{})
    Register(Intel{})
    Register(Broadcom{})  // Add before Generic
    Register(Generic{})   // Generic must be last (matches everything)
}
```

## File layout

```
pkg/config/system/nic/
├── nic.go       # Vendor interface and registration logic
├── bind.go      # Device binding (vfio-pci, uio, bifurcated)
├── mellanox.go  # Mellanox vendor
├── intel.go     # Intel vendor
├── generic.go   # Generic fallback
└── all.go       # Registration order
```

## Bind strategy enum

| Strategy | Use |
|---|---|
| `BindStrategyBifurcated` | Kernel driver stays bound; DPDK PMD via Verbs (Mellanox). |
| `BindStrategyVFIO` | Default. Unbind from kernel, bind to `vfio-pci`. Requires IOMMU. |
| `BindStrategyUIO` | Fallback. Unbind from kernel, bind to `uio_pci_generic`. No IOMMU. |

## Add a NIC reference page

After adding code support, add a page under
`docs/reference/nics/<vendor>-<family>.md` capturing hardware specs,
driver requirements, and any throughput limits or caveats. Then update
`docs/reference/nics/index.md` and `docs/reference/index.md` to link
the new entry.
