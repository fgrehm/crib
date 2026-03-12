// Package cloudips provides embedded cloud provider IP ranges for firewall
// rules. The data is version-controlled and updated periodically via
// scripts/update-cloud-ips.go.
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

// GenerateIPTablesRules produces iptables commands that block outbound
// traffic to all known cloud provider IP ranges.
func GenerateIPTablesRules() string {
	ranges, err := Load()
	if err != nil || ranges == nil {
		return "# cloud provider IP ranges: failed to load\n"
	}

	var b strings.Builder
	b.WriteString("# Cloud provider IP ranges (last updated: " + ranges.LastUpdated + ")\n")

	for name, provider := range ranges.Providers {
		if len(provider.IPv4) == 0 && len(provider.IPv6) == 0 {
			continue
		}
		b.WriteString("# " + name + "\n")
		for _, cidr := range provider.IPv4 {
			b.WriteString("iptables -A OUTPUT -d " + cidr + " -j DROP 2>/dev/null\n")
		}
		for _, cidr := range provider.IPv6 {
			b.WriteString("ip6tables -A OUTPUT -d " + cidr + " -j DROP 2>/dev/null\n")
		}
	}

	return b.String()
}
