package sandbox

import (
	"fmt"
	"strings"
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
// All rules go into a dedicated CRIB_SANDBOX chain (created/flushed on each
// run for idempotency) with a single jump rule from OUTPUT.
//
// blockLocalNetwork uses plain iptables rules (~11 entries for RFC 1918,
// link-local, metadata endpoints).
func generateNetworkScript(cfg *sandboxConfig) string {
	var b strings.Builder

	// Ensure iptables/ip6tables are in PATH (they live in /usr/sbin on Debian).
	b.WriteString("export PATH=\"/usr/sbin:/sbin:$PATH\"\n")

	// IPv6 rules first (best-effort: ip6tables may not exist in all containers).
	// IPv4 rules last so the script's exit code reflects iptables success.
	for _, bin := range []string{"ip6tables", "iptables"} {
		fmt.Fprintf(&b, "%s -N CRIB_SANDBOX 2>/dev/null || %s -F CRIB_SANDBOX 2>/dev/null\n", bin, bin)
	}

	if cfg.BlockLocalNetwork {
		// Emit ip6tables rules before iptables rules.
		for _, rule := range localNetworkBlockedCIDRs {
			if rule.binary == "ip6tables" {
				fmt.Fprintf(&b, "%s -A CRIB_SANDBOX -d %s -j DROP 2>/dev/null\n", rule.binary, rule.cidr)
			}
		}
		for _, rule := range localNetworkBlockedCIDRs {
			if rule.binary == "iptables" {
				fmt.Fprintf(&b, "%s -A CRIB_SANDBOX -d %s -j DROP 2>/dev/null\n", rule.binary, rule.cidr)
			}
		}
	}

	// Ensure exactly one jump rule at the top of OUTPUT so it runs before
	// any pre-existing ACCEPT/RETURN rules. Delete first, then insert at position 1.
	// ip6tables first, iptables last for exit code.
	for _, bin := range []string{"ip6tables", "iptables"} {
		fmt.Fprintf(&b, "%s -D OUTPUT -j CRIB_SANDBOX 2>/dev/null; %s -I OUTPUT 1 -j CRIB_SANDBOX 2>/dev/null\n", bin, bin)
	}

	return b.String()
}
