# VRFs

VRFs provide isolated routing tables for traffic separation, for example placing subscriber traffic in a CGNAT VRF while keeping other subscriber traffic in the default VRF (or individual business VRFs).

VRFs are defined as a named map under the top-level `vrfs` key.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `description` | string | Human-readable description | `Customer traffic VRF` |
| `rd` | string | Route distinguisher in ASN:NN or IP:NN format | `65000:100` |
| `import-route-targets` | array | Route targets to import into this VRF | `[65000:100]` |
| `export-route-targets` | array | Route targets to export from this VRF | `[65000:100]` |
| `address-families` | [AddressFamilies](#address-families) | Address family configuration | |

## Address Families

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `ipv4-unicast` | [AFConfig](#address-family-config) | IPv4 unicast address family | |
| `ipv6-unicast` | [AFConfig](#address-family-config) | IPv6 unicast address family | |

### Address Family Config

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `enabled` | bool | Enable this address family | `true` |
| `import-route-policy` | string | Route policy applied to imported routes | `IMPORT-POLICY` |
| `export-route-policy` | string | Route policy applied to exported routes | `EXPORT-POLICY` |

## Example

```yaml
vrfs:
  cgnat:
    description: CGNAT subscriber VRF
    address-families:
      ipv4-unicast: {}
      ipv6-unicast: {}

  enterprise:
    description: Enterprise customer VRF
    rd: "65000:200"
    import-route-targets:
      - "65000:200"
    export-route-targets:
      - "65000:200"
    address-families:
      ipv4-unicast:
        import-route-policy: IMPORT-ENTERPRISE
        export-route-policy: EXPORT-ENTERPRISE
```

!!! tip "Using VRFs with service groups"
    VRFs are typically assigned to subscribers via [service groups](service-groups.md) rather than directly on subscriber groups. This lets you bundle VRF assignment with other per-subscriber attributes (unnumbered interface, uRPF, ACLs, QoS) into a single named profile.
