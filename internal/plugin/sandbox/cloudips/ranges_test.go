package cloudips

import (
	"strings"
	"testing"
)

func TestLoad_ParsesSeedData(t *testing.T) {
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
}

func TestGenerateIPTablesRules_EmptyProviders(t *testing.T) {
	// With the seed data (empty providers), should still produce a header.
	rules := GenerateIPTablesRules()
	if !strings.Contains(rules, "last updated") {
		t.Error("expected last updated timestamp in output")
	}
	// No actual iptables commands for empty providers.
	if strings.Contains(rules, "iptables -A") {
		t.Error("expected no iptables rules for empty providers")
	}
}
