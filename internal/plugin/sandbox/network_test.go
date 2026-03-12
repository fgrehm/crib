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

	// Should include the cloud provider rules header.
	if !strings.Contains(script, "Cloud provider IP ranges") {
		t.Error("expected cloud provider rules section")
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
	if !strings.Contains(script, "Cloud provider IP ranges") {
		t.Error("missing cloud provider rules")
	}
}
