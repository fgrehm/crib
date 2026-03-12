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
	for _, name := range []string{"aws", "gcp", "oraclecloud", "cloudflare"} {
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

func TestGenerateIPTablesRules_ProducesRules(t *testing.T) {
	rules := GenerateIPTablesRules()
	if !strings.Contains(rules, "last updated") {
		t.Error("expected last updated timestamp in output")
	}
	if !strings.Contains(rules, "iptables -A OUTPUT") {
		t.Error("expected iptables rules for populated providers")
	}
	if !strings.Contains(rules, "ip6tables -A OUTPUT") {
		t.Error("expected ip6tables rules for providers with IPv6 ranges")
	}
}
