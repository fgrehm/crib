package cloudips

import (
	"strings"
	"testing"
)

func TestLoad_ParsesData(t *testing.T) {
	ranges, err := Load()
	if err != nil {
		t.Fatalf("failed to load ranges: %v", err)
	}
	if ranges.LastUpdated == "" {
		t.Error("expected non-empty lastUpdated")
	}
	if ranges.Providers == nil {
		t.Error("expected non-nil providers map")
	}
	for _, name := range []string{"aws", "gcp", "azure", "oraclecloud", "cloudflare"} {
		p, ok := ranges.Providers[name]
		if !ok {
			t.Errorf("missing provider %s", name)
			continue
		}
		if len(p.IPv4) == 0 {
			t.Errorf("expected IPv4 ranges for %s", name)
		}
	}
}

func TestGenerateIPSetRules_ProducesRules(t *testing.T) {
	rules := GenerateIPSetRules()
	if !strings.Contains(rules, "last updated") {
		t.Error("expected last updated timestamp in output")
	}
	if !strings.Contains(rules, "ipset create crib-cloud-v4") {
		t.Error("expected ipset create for IPv4")
	}
	if !strings.Contains(rules, "ipset create crib-cloud-v6") {
		t.Error("expected ipset create for IPv6")
	}
	if !strings.Contains(rules, "iptables -C CRIB_SANDBOX -m set --match-set crib-cloud-v4") {
		t.Error("expected iptables check rule for ipset in CRIB_SANDBOX chain")
	}
	if !strings.Contains(rules, "ip6tables -C CRIB_SANDBOX -m set --match-set crib-cloud-v6") {
		t.Error("expected ip6tables check rule for ipset in CRIB_SANDBOX chain")
	}
}
