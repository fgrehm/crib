package sandbox

import (
	"strings"

	"github.com/fgrehm/crib/internal/plugin/sandbox/cloudips"
)

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
		// RFC 1918 private ranges.
		b.WriteString("iptables -A OUTPUT -d 10.0.0.0/8 -j DROP 2>/dev/null\n")
		b.WriteString("iptables -A OUTPUT -d 172.16.0.0/12 -j DROP 2>/dev/null\n")
		b.WriteString("iptables -A OUTPUT -d 192.168.0.0/16 -j DROP 2>/dev/null\n")

		// Link-local (covers most cloud metadata endpoints).
		b.WriteString("iptables -A OUTPUT -d 169.254.0.0/16 -j DROP 2>/dev/null\n")

		// CGN range (covers Alibaba Cloud metadata at 100.100.100.200).
		b.WriteString("iptables -A OUTPUT -d 100.64.0.0/10 -j DROP 2>/dev/null\n")

		// Azure Wire Server.
		b.WriteString("iptables -A OUTPUT -d 168.63.129.16/32 -j DROP 2>/dev/null\n")

		// Oracle Cloud at Customer.
		b.WriteString("iptables -A OUTPUT -d 192.0.0.192/32 -j DROP 2>/dev/null\n")

		// IPv6 link-local and metadata endpoints.
		b.WriteString("ip6tables -A OUTPUT -d fe80::/10 -j DROP 2>/dev/null\n")
		b.WriteString("ip6tables -A OUTPUT -d fd00:ec2::254/128 -j DROP 2>/dev/null\n")
		b.WriteString("ip6tables -A OUTPUT -d fd20:ce::254/128 -j DROP 2>/dev/null\n")
		b.WriteString("ip6tables -A OUTPUT -d fd00:42::42/128 -j DROP 2>/dev/null\n")
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
