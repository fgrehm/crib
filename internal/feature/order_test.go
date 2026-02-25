package feature

import (
	"testing"
)

func makeFeatureSet(id string, deps map[string]any, installsAfter []string) *FeatureSet {
	return &FeatureSet{
		ConfigID: id,
		Config: &FeatureConfig{
			ID:            id,
			DependsOn:     deps,
			InstallsAfter: installsAfter,
		},
	}
}

func TestOrderFeaturesNoDeps(t *testing.T) {
	features := []*FeatureSet{
		makeFeatureSet("c", nil, nil),
		makeFeatureSet("a", nil, nil),
		makeFeatureSet("b", nil, nil),
	}

	result, err := OrderFeatures(features, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With no dependencies, output should be deterministic (sorted by key).
	want := []string{"a", "b", "c"}
	assertOrder(t, result, want)
}

func TestOrderFeaturesSimpleDep(t *testing.T) {
	features := []*FeatureSet{
		makeFeatureSet("app", map[string]any{"lib": map[string]any{}}, nil),
		makeFeatureSet("lib", nil, nil),
	}

	result, err := OrderFeatures(features, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// lib must come before app.
	want := []string{"lib", "app"}
	assertOrder(t, result, want)
}

func TestOrderFeaturesChainedDeps(t *testing.T) {
	features := []*FeatureSet{
		makeFeatureSet("c", map[string]any{"b": map[string]any{}}, nil),
		makeFeatureSet("b", map[string]any{"a": map[string]any{}}, nil),
		makeFeatureSet("a", nil, nil),
	}

	result, err := OrderFeatures(features, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"a", "b", "c"}
	assertOrder(t, result, want)
}

func TestOrderFeaturesSoftDeps(t *testing.T) {
	features := []*FeatureSet{
		makeFeatureSet("app", nil, []string{"lib"}),
		makeFeatureSet("lib", nil, nil),
	}

	result, err := OrderFeatures(features, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Soft dep: lib should come before app.
	want := []string{"lib", "app"}
	assertOrder(t, result, want)
}

func TestOrderFeaturesSoftDepMissing(t *testing.T) {
	features := []*FeatureSet{
		makeFeatureSet("app", nil, []string{"nonexistent"}),
	}

	// Missing soft dep should not cause an error.
	result, err := OrderFeatures(features, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("got %d results, want 1", len(result))
	}
}

func TestOrderFeaturesCircular(t *testing.T) {
	features := []*FeatureSet{
		makeFeatureSet("a", map[string]any{"b": map[string]any{}}, nil),
		makeFeatureSet("b", map[string]any{"a": map[string]any{}}, nil),
	}

	_, err := OrderFeatures(features, nil)
	if err == nil {
		t.Error("expected circular dependency error")
	}
}

func TestOrderFeaturesOverrideFull(t *testing.T) {
	features := []*FeatureSet{
		makeFeatureSet("a", nil, nil),
		makeFeatureSet("b", nil, nil),
		makeFeatureSet("c", nil, nil),
	}

	result, err := OrderFeatures(features, []string{"c", "a", "b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"c", "a", "b"}
	assertOrder(t, result, want)
}

func TestOrderFeaturesOverridePartial(t *testing.T) {
	features := []*FeatureSet{
		makeFeatureSet("a", nil, nil),
		makeFeatureSet("b", nil, nil),
		makeFeatureSet("c", nil, nil),
	}

	result, err := OrderFeatures(features, []string{"c"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// c goes first, then a and b in sorted order.
	want := []string{"c", "a", "b"}
	assertOrder(t, result, want)
}

func TestOrderFeaturesOverrideNonexistent(t *testing.T) {
	features := []*FeatureSet{
		makeFeatureSet("a", nil, nil),
		makeFeatureSet("b", nil, nil),
	}

	result, err := OrderFeatures(features, []string{"nonexistent", "b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// nonexistent is ignored, b goes first, then a.
	want := []string{"b", "a"}
	assertOrder(t, result, want)
}

func TestOrderFeaturesEmpty(t *testing.T) {
	result, err := OrderFeatures(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestOrderFeaturesHardDepMissing(t *testing.T) {
	features := []*FeatureSet{
		makeFeatureSet("app", map[string]any{"nonexistent": map[string]any{}}, nil),
	}

	_, err := OrderFeatures(features, nil)
	if err == nil {
		t.Error("expected error for missing hard dependency")
	}
}

func TestNormalizeID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ghcr.io/devcontainers/features/node:1", "ghcr.io/devcontainers/features/node"},
		{"ghcr.io/devcontainers/features/node", "ghcr.io/devcontainers/features/node"},
		{"ghcr.io/devcontainers/features/node@sha256:abc123", "ghcr.io/devcontainers/features/node"},
		{"./local-feature", "./local-feature"},
		{"../parent-feature", "../parent-feature"},
		{"http://example.com/feature.tgz", "http://example.com/feature.tgz"},
		{"https://example.com/feature.tgz", "https://example.com/feature.tgz"},
		{"localhost:5000/feature", "localhost:5000/feature"},
		{"localhost:5000/feature:latest", "localhost:5000/feature"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeID(tt.input)
			if got != tt.want {
				t.Errorf("normalizeID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestHasHardDep(t *testing.T) {
	lookup := map[string]string{
		"ghcr.io/devcontainers/features/node": "ghcr.io/devcontainers/features/node:1",
		"lib":                                 "lib",
	}

	app := makeFeatureSet("app", map[string]any{
		"ghcr.io/devcontainers/features/node:1": map[string]any{},
	}, nil)
	lib := makeFeatureSet("lib", nil, nil)

	if !hasHardDep(app, "ghcr.io/devcontainers/features/node:1", lookup) {
		t.Error("expected hard dep to be found")
	}
	if hasHardDep(app, "lib", lookup) {
		t.Error("expected no hard dep on lib")
	}
	if hasHardDep(lib, "app", lookup) {
		t.Error("expected no hard dep from lib to app")
	}
	if hasHardDep(nil, "app", lookup) {
		t.Error("expected no hard dep from nil feature")
	}
}

func assertOrder(t *testing.T, result []*FeatureSet, want []string) {
	t.Helper()
	if len(result) != len(want) {
		ids := make([]string, len(result))
		for i, f := range result {
			ids[i] = f.ConfigID
		}
		t.Fatalf("got %v, want %v", ids, want)
	}
	for i, id := range want {
		if result[i].ConfigID != id {
			ids := make([]string, len(result))
			for j, f := range result {
				ids[j] = f.ConfigID
			}
			t.Errorf("got %v, want %v", ids, want)
			return
		}
	}
}
