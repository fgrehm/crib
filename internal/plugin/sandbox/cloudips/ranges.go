// Package cloudips provides embedded cloud provider IP ranges for firewall
// rules. The data is version-controlled and updated periodically via
// scripts/update-cloud-ips.sh.
package cloudips

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed data/ranges.json
var rangesJSON []byte

// Ranges holds the parsed cloud provider IP ranges.
type Ranges struct {
	LastUpdated string              `json:"lastUpdated"` // RFC 3339 timestamp
	Providers   map[string]Provider `json:"providers"`
}

// Provider holds IPv4 and IPv6 CIDRs for a single cloud provider.
type Provider struct {
	IPv4 []string `json:"ipv4"`
	IPv6 []string `json:"ipv6"`
}

var (
	parsed     *Ranges
	parseOnce  sync.Once
	parseError error
)

// Load returns the embedded cloud provider IP ranges.
func Load() (*Ranges, error) {
	parseOnce.Do(func() {
		parsed = &Ranges{}
		parseError = json.Unmarshal(rangesJSON, parsed)
	})
	return parsed, parseError
}

// GenerateIPSetRules produces ipset+iptables commands that block outbound
// traffic to all known cloud provider IP ranges. Uses ipset hash:net sets
// for O(1) lookup regardless of set size, instead of one iptables rule per
// CIDR (which would be tens of thousands of rules).
func GenerateIPSetRules() string {
	ranges, err := Load()
	if err != nil || ranges == nil {
		return "# cloud provider IP ranges: failed to load\n"
	}

	// Collect all CIDRs across providers.
	var ipv4, ipv6 []string
	for _, provider := range ranges.Providers {
		ipv4 = append(ipv4, provider.IPv4...)
		ipv6 = append(ipv6, provider.IPv6...)
	}

	// Pre-allocate: ~45 bytes per "echo 'add crib-cloud-vN <cidr>'\n" line.
	var b strings.Builder
	b.Grow((len(ipv4) + len(ipv6)) * 45)
	b.WriteString("# Cloud provider IP ranges (last updated: " + ranges.LastUpdated + ")\n")

	if len(ipv4) > 0 {
		b.WriteString("ipset create crib-cloud-v4 hash:net -exist 2>/dev/null\n")
		b.WriteString("ipset flush crib-cloud-v4 2>/dev/null\n")
		// Bulk load via restore (much faster than individual add commands).
		b.WriteString("{\n")
		for _, cidr := range ipv4 {
			b.WriteString("echo 'add crib-cloud-v4 " + cidr + "'\n")
		}
		b.WriteString("} | ipset restore -exist 2>/dev/null\n")
		// Append to CRIB_SANDBOX chain (created by network.go) with check-before-add for idempotency.
		b.WriteString("iptables -C CRIB_SANDBOX -m set --match-set crib-cloud-v4 dst -j DROP 2>/dev/null || ")
		b.WriteString("iptables -A CRIB_SANDBOX -m set --match-set crib-cloud-v4 dst -j DROP 2>/dev/null\n")
	}

	if len(ipv6) > 0 {
		b.WriteString("ipset create crib-cloud-v6 hash:net family inet6 -exist 2>/dev/null\n")
		b.WriteString("ipset flush crib-cloud-v6 2>/dev/null\n")
		b.WriteString("{\n")
		for _, cidr := range ipv6 {
			b.WriteString("echo 'add crib-cloud-v6 " + cidr + "'\n")
		}
		b.WriteString("} | ipset restore -exist 2>/dev/null\n")
		b.WriteString("ip6tables -C CRIB_SANDBOX -m set --match-set crib-cloud-v6 dst -j DROP 2>/dev/null || ")
		b.WriteString("ip6tables -A CRIB_SANDBOX -m set --match-set crib-cloud-v6 dst -j DROP 2>/dev/null\n")
	}

	return b.String()
}
