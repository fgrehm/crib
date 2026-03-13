package sandbox

import (
	"fmt"
	"strings"

	"github.com/fgrehm/crib/internal/plugin/sandbox/cloudips"
)

// Destinations blocked by blockLocalNetwork, grouped by iptables binary.
var localNetworkBlockedCIDRs = []struct {
	binary string // "iptables" or "ip6tables"
	cidr   string
}{
	// RFC 1918 private ranges.
	{"iptables", "10.0.0.0/8"},
	{"iptables", "172.16.0.0/12"},
	{"iptables", "192.168.0.0/16"},
	// Link-local (covers most cloud metadata endpoints).
	{"iptables", "169.254.0.0/16"},
	// CGN range (covers Alibaba Cloud metadata at 100.100.100.200).
	{"iptables", "100.64.0.0/10"},
	// Azure Wire Server.
	{"iptables", "168.63.129.16/32"},
	// Oracle Cloud at Customer.
	{"iptables", "192.0.0.192/32"},
	// IPv6 link-local and metadata endpoints.
	{"ip6tables", "fe80::/10"},
	{"ip6tables", "fd00:ec2::254/128"}, // AWS
	{"ip6tables", "fd20:ce::254/128"},  // GCP
	{"ip6tables", "fd00:42::42/128"},   // Scaleway
}

// generateNetworkScript produces shell commands that block outbound traffic
// to restricted destinations. Applied once at container setup time.
//
// blockLocalNetwork uses plain iptables rules (~11 entries for RFC 1918,
// link-local, metadata endpoints). blockCloudProviders uses ipset hash:net
// sets loaded via "ipset restore" + a single iptables match rule per address
// family. We keep iptables for the small local-network ruleset so that
// users who only enable blockLocalNetwork don't need ipset installed.
func generateNetworkScript(cfg *sandboxConfig) string {
	var b strings.Builder

	if cfg.BlockLocalNetwork {
		// Use a dedicated chain so rules are idempotent across rebuilds.
		// Flush and recreate the chain each time, with a single jump rule.
		for _, bin := range []string{"iptables", "ip6tables"} {
			fmt.Fprintf(&b, "%s -N CRIB_SANDBOX 2>/dev/null || %s -F CRIB_SANDBOX 2>/dev/null\n", bin, bin)
		}

		for _, rule := range localNetworkBlockedCIDRs {
			fmt.Fprintf(&b, "%s -A CRIB_SANDBOX -d %s -j DROP 2>/dev/null\n", rule.binary, rule.cidr)
		}

		// Ensure exactly one jump rule in OUTPUT (check before adding).
		for _, bin := range []string{"iptables", "ip6tables"} {
			fmt.Fprintf(&b, "%s -C OUTPUT -j CRIB_SANDBOX 2>/dev/null || %s -A OUTPUT -j CRIB_SANDBOX 2>/dev/null\n", bin, bin)
		}
	}

	if cfg.BlockCloudProviders {
		b.WriteString(generateCloudProviderRules())
	}

	return b.String()
}

// generateCloudProviderRules produces ipset+iptables rules from embedded
// cloud provider IP ranges.
func generateCloudProviderRules() string {
	return cloudips.GenerateIPSetRules()
}
