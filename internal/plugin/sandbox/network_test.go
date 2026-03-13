package sandbox

import (
	"strings"
	"testing"
)

func TestGenerateNetworkScript_NoBlocking(t *testing.T) {
	cfg := &sandboxConfig{}
	script := generateNetworkScript(cfg)
	if script != "" {
		t.Errorf("expected empty script, got: %s", script)
	}
}

func TestGenerateNetworkScript_BlockLocalNetwork(t *testing.T) {
	cfg := &sandboxConfig{BlockLocalNetwork: true}
	script := generateNetworkScript(cfg)

	// RFC 1918.
	for _, cidr := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
		if !strings.Contains(script, cidr) {
			t.Errorf("missing RFC 1918 block for %s", cidr)
		}
	}

	// Link-local (covers most cloud metadata).
	if !strings.Contains(script, "169.254.0.0/16") {
		t.Error("missing link-local block")
	}

	// CGN range (Alibaba).
	if !strings.Contains(script, "100.64.0.0/10") {
		t.Error("missing CGN range block")
	}

	// Azure Wire Server.
	if !strings.Contains(script, "168.63.129.16/32") {
		t.Error("missing Azure Wire Server block")
	}

	// Oracle Cloud at Customer.
	if !strings.Contains(script, "192.0.0.192/32") {
		t.Error("missing Oracle Cloud at Customer block")
	}

	// IPv6.
	if !strings.Contains(script, "fe80::/10") {
		t.Error("missing IPv6 link-local block")
	}
	if !strings.Contains(script, "fd00:ec2::254/128") {
		t.Error("missing AWS IPv6 metadata block")
	}
	if !strings.Contains(script, "fd20:ce::254/128") {
		t.Error("missing GCP IPv6 metadata block")
	}
	if !strings.Contains(script, "fd00:42::42/128") {
		t.Error("missing Scaleway IPv6 metadata block")
	}

	// All iptables rules should suppress errors (2>/dev/null).
	for line := range strings.SplitSeq(strings.TrimSpace(script), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasSuffix(line, "2>/dev/null") {
			t.Errorf("iptables rule should suppress errors: %s", line)
		}
	}
}

func TestGenerateNetworkScript_BlockCloudProviders(t *testing.T) {
	cfg := &sandboxConfig{BlockCloudProviders: true}
	script := generateNetworkScript(cfg)

	// Should include ipset-based cloud provider rules.
	if !strings.Contains(script, "ipset create crib-cloud-v4") {
		t.Error("expected ipset create for cloud provider ranges")
	}
	if !strings.Contains(script, "--match-set crib-cloud-v4") {
		t.Error("expected iptables rule matching cloud ipset")
	}
}

func TestGenerateNetworkScript_UsesDedicatedChain(t *testing.T) {
	cfg := &sandboxConfig{BlockLocalNetwork: true}
	script := generateNetworkScript(cfg)

	// Should create CRIB_SANDBOX chain (idempotent).
	if !strings.Contains(script, "iptables -N CRIB_SANDBOX") {
		t.Error("missing chain creation for iptables")
	}
	if !strings.Contains(script, "ip6tables -N CRIB_SANDBOX") {
		t.Error("missing chain creation for ip6tables")
	}

	// Rules should target CRIB_SANDBOX, not OUTPUT directly.
	if strings.Contains(script, "-A OUTPUT -d") {
		t.Error("rules should target CRIB_SANDBOX chain, not OUTPUT directly")
	}
	if !strings.Contains(script, "-A CRIB_SANDBOX -d") {
		t.Error("expected rules in CRIB_SANDBOX chain")
	}

	// Should have a jump rule from OUTPUT to CRIB_SANDBOX.
	if !strings.Contains(script, "-C OUTPUT -j CRIB_SANDBOX") {
		t.Error("missing check for existing jump rule")
	}
	if !strings.Contains(script, "-A OUTPUT -j CRIB_SANDBOX") {
		t.Error("missing conditional jump rule addition")
	}
}

func TestGenerateNetworkScript_BothFlags(t *testing.T) {
	cfg := &sandboxConfig{
		BlockLocalNetwork:   true,
		BlockCloudProviders: true,
	}
	script := generateNetworkScript(cfg)

	// Should have both RFC 1918 and cloud provider rules.
	if !strings.Contains(script, "10.0.0.0/8") {
		t.Error("missing RFC 1918 rules")
	}
	if !strings.Contains(script, "ipset create crib-cloud-v4") {
		t.Error("missing cloud provider ipset rules")
	}
}
