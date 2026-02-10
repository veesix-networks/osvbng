# VRFs

!!! warning "Not yet supported"
    VRF configuration is not currently supported. This page documents the planned configuration format.

Virtual Routing and Forwarding instance configuration. VRFs provide isolated routing tables for traffic separation.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `name` | string | VRF name | `customers` |
| `description` | string | Human-readable description | `Customer traffic VRF` |
| `rd` | string | Route distinguisher in ASN:NN or IP:NN format | `65000:100` |
| `import-route-targets` | array | Route targets to import into this VRF | `[65000:100]` |
| `export-route-targets` | array | Route targets to export from this VRF | `[65000:100]` |
| `address-families` | [AddressFamilies](#address-families) | Address family configuration | |

## Address Families

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `ipv4-unicast` | [IPv4/IPv6 Unicast](#ipv4ipv6-unicast) | IPv4 unicast address family | |
| `ipv6-unicast` | [IPv4/IPv6 Unicast](#ipv4ipv6-unicast) | IPv6 unicast address family | |

### IPv4/IPv6 Unicast

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `import-route-policy` | string | Route policy applied to imported routes | `IMPORT-POLICY` |
| `export-route-policy` | string | Route policy applied to exported routes | `EXPORT-POLICY` |

## Example

```yaml
vrfs:
  - name: customers
    description: Customer traffic VRF
    rd: "65000:100"
    import-route-targets:
      - "65000:100"
    export-route-targets:
      - "65000:100"
    address-families:
      ipv4-unicast:
        import-route-policy: IMPORT-CUSTOMERS
        export-route-policy: EXPORT-CUSTOMERS
      ipv6-unicast:
        import-route-policy: IMPORT-CUSTOMERS-V6
        export-route-policy: EXPORT-CUSTOMERS-V6
```
