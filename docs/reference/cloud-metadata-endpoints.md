---
title: Cloud Metadata Endpoints
description: IP addresses and hostnames blocked by the sandbox plugin's blockLocalNetwork setting.
---

Reference list of cloud provider metadata endpoints that the `crib` sandbox plugin blocks when `blockLocalNetwork` is enabled. These endpoints expose instance credentials, API tokens, and infrastructure details to any process that can make HTTP requests from inside a VM.

## Endpoints by provider

| Provider | IPv4 | IPv6 | Hostname |
|----------|------|------|----------|
| AWS | `169.254.169.254` | `fd00:ec2::254` | - |
| GCP | `169.254.169.254` | `fd20:ce::254` | `metadata.google.internal` |
| Azure IMDS | `169.254.169.254` | - | - |
| Azure Wire Server | `168.63.129.16` | - | - |
| Oracle Cloud | `169.254.169.254` | - | - |
| Oracle Cloud at Customer | `192.0.0.192` | - | - |
| DigitalOcean | `169.254.169.254` | - | - |
| Alibaba Cloud | `100.100.100.200` | - | - |
| Hetzner Cloud | `169.254.169.254` | - | - |
| Scaleway | `169.254.42.42` | `fd00:42::42` | - |
| Vultr | `169.254.169.254` | - | - |
| Linode / Akamai | `169.254.169.254` | - | - |
| IBM Cloud (VPC) | `169.254.169.254` | - | - |
| OpenStack | `169.254.169.254` | `fe80::a9fe:a9fe` | - |

Notable outliers that don't use the standard `169.254.169.254`:

- **Alibaba Cloud**: `100.100.100.200` (RFC 6598 CGN range, `100.64.0.0/10`)
- **Scaleway**: `169.254.42.42` (link-local but non-standard)
- **Azure Wire Server**: `168.63.129.16` (routable IP, exposes platform services)
- **Oracle Cloud at Customer**: `192.0.0.192`

## CIDR blocks

For defense-in-depth, block these entire ranges instead of individual IPs:

| CIDR | Purpose |
|------|---------|
| `169.254.0.0/16` | IPv4 link-local (RFC 3927), covers most providers |
| `fe80::/10` | IPv6 link-local |
| `100.64.0.0/10` | RFC 6598 CGN range, covers Alibaba Cloud |
| `168.63.129.16/32` | Azure Wire Server (not in any broader block) |
| `192.0.0.192/32` | Oracle Cloud at Customer |

## RFC 1918 private ranges

Also blocked by `blockLocalNetwork` to prevent lateral movement to other services on the local network:

| CIDR | Range |
|------|-------|
| `10.0.0.0/8` | 10.0.0.0 - 10.255.255.255 |
| `172.16.0.0/12` | 172.16.0.0 - 172.31.255.255 |
| `192.168.0.0/16` | 192.168.0.0 - 192.168.255.255 |

## Cloud provider IP ranges (machine-readable)

Major cloud providers publish their IP ranges in machine-readable format. These are documented here for reference; a future version of the sandbox plugin may use them to block outbound access to cloud infrastructure beyond metadata endpoints (see [ADR 002](../decisions/002-sandbox-plugin.md) v2 scope).

| Provider | URL | Format |
|----------|-----|--------|
| [AWS](https://docs.aws.amazon.com/vpc/latest/userguide/aws-ip-ranges.html) | `https://ip-ranges.amazonaws.com/ip-ranges.json` | JSON |
| [GCP](https://cloud.google.com/vpc/docs/configure-private-google-access) | `https://www.gstatic.com/ipranges/cloud.json` | JSON |
| [Azure](https://learn.microsoft.com/en-us/azure/virtual-network/service-tags-overview) | Download page (weekly, filename changes) | JSON |
| [Oracle Cloud](https://docs.oracle.com/en-us/iaas/Content/General/Concepts/addressranges.htm) | `https://docs.oracle.com/en-us/iaas/tools/public_ip_ranges.json` | JSON |
| [Cloudflare](https://www.cloudflare.com/ips/) | `https://www.cloudflare.com/ips-v4` | Plain text |

Community-maintained aggregations:

- [rezmoss/cloud-provider-ip-addresses](https://github.com/rezmoss/cloud-provider-ip-addresses): 20+ providers, multiple output formats, updated daily.
- [tobilg/public-cloud-provider-ip-ranges](https://github.com/tobilg/public-cloud-provider-ip-ranges): CSV, Parquet, JSON.
