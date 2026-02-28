package engine

import (
	"testing"

	"github.com/fgrehm/crib/internal/config"
)

func TestStringMapsEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b map[string]string
		want bool
	}{
		{"both nil", nil, nil, true},
		{"both empty", map[string]string{}, map[string]string{}, true},
		{"nil vs empty", nil, map[string]string{}, true},
		{"equal", map[string]string{"a": "1", "b": "2"}, map[string]string{"a": "1", "b": "2"}, true},
		{"different values", map[string]string{"a": "1"}, map[string]string{"a": "2"}, false},
		{"different keys", map[string]string{"a": "1"}, map[string]string{"b": "1"}, false},
		{"different lengths", map[string]string{"a": "1"}, map[string]string{"a": "1", "b": "2"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stringMapsEqual(tt.a, tt.b); got != tt.want {
				t.Errorf("stringMapsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStrSlicesEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want bool
	}{
		{"both nil", nil, nil, true},
		{"both empty", []string{}, []string{}, true},
		{"nil vs empty", nil, []string{}, true},
		{"equal", []string{"a", "b"}, []string{"a", "b"}, true},
		{"different values", []string{"a"}, []string{"b"}, false},
		{"different order", []string{"a", "b"}, []string{"b", "a"}, false},
		{"different lengths", []string{"a"}, []string{"a", "b"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strSlicesEqual(tt.a, tt.b); got != tt.want {
				t.Errorf("strSlicesEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBoolPtrEqual(t *testing.T) {
	tr := true
	fa := false
	tr2 := true

	tests := []struct {
		name string
		a, b *bool
		want bool
	}{
		{"both nil", nil, nil, true},
		{"nil vs true", nil, &tr, false},
		{"true vs nil", &tr, nil, false},
		{"both true", &tr, &tr2, true},
		{"both false", &fa, &fa, true},
		{"true vs false", &tr, &fa, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := boolPtrEqual(tt.a, tt.b); got != tt.want {
				t.Errorf("boolPtrEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMountsEqual(t *testing.T) {
	m1 := config.Mount{Type: "bind", Source: "/a", Target: "/b"}
	m2 := config.Mount{Type: "bind", Source: "/a", Target: "/b"}
	m3 := config.Mount{Type: "volume", Source: "data", Target: "/data"}

	tests := []struct {
		name string
		a, b []config.Mount
		want bool
	}{
		{"both nil", nil, nil, true},
		{"both empty", []config.Mount{}, []config.Mount{}, true},
		{"equal", []config.Mount{m1}, []config.Mount{m2}, true},
		{"different", []config.Mount{m1}, []config.Mount{m3}, false},
		{"different lengths", []config.Mount{m1}, []config.Mount{m1, m3}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mountsEqual(tt.a, tt.b); got != tt.want {
				t.Errorf("mountsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildOptsEqual(t *testing.T) {
	v1 := "1"
	v2 := "2"

	tests := []struct {
		name string
		a, b *config.ConfigBuildOptions
		want bool
	}{
		{"both nil", nil, nil, true},
		{"nil vs non-nil", nil, &config.ConfigBuildOptions{}, false},
		{"non-nil vs nil", &config.ConfigBuildOptions{}, nil, false},
		{"equal empty", &config.ConfigBuildOptions{}, &config.ConfigBuildOptions{}, true},
		{
			"equal with fields",
			&config.ConfigBuildOptions{Dockerfile: "Dockerfile", Context: ".", Target: "dev"},
			&config.ConfigBuildOptions{Dockerfile: "Dockerfile", Context: ".", Target: "dev"},
			true,
		},
		{
			"different dockerfile",
			&config.ConfigBuildOptions{Dockerfile: "Dockerfile"},
			&config.ConfigBuildOptions{Dockerfile: "Dockerfile.dev"},
			false,
		},
		{
			"different context",
			&config.ConfigBuildOptions{Context: "."},
			&config.ConfigBuildOptions{Context: "./src"},
			false,
		},
		{
			"different target",
			&config.ConfigBuildOptions{Target: "dev"},
			&config.ConfigBuildOptions{Target: "prod"},
			false,
		},
		{
			"different cache_from",
			&config.ConfigBuildOptions{CacheFrom: []string{"img:cache"}},
			&config.ConfigBuildOptions{CacheFrom: []string{"img:other"}},
			false,
		},
		{
			"different options",
			&config.ConfigBuildOptions{Options: []string{"--no-cache"}},
			&config.ConfigBuildOptions{Options: []string{"--pull"}},
			false,
		},
		{
			"same args",
			&config.ConfigBuildOptions{Args: map[string]*string{"V": &v1}},
			&config.ConfigBuildOptions{Args: map[string]*string{"V": &v1}},
			true,
		},
		{
			"different args values",
			&config.ConfigBuildOptions{Args: map[string]*string{"V": &v1}},
			&config.ConfigBuildOptions{Args: map[string]*string{"V": &v2}},
			false,
		},
		{
			"arg nil vs non-nil",
			&config.ConfigBuildOptions{Args: map[string]*string{"V": nil}},
			&config.ConfigBuildOptions{Args: map[string]*string{"V": &v1}},
			false,
		},
		{
			"different args keys",
			&config.ConfigBuildOptions{Args: map[string]*string{"A": &v1}},
			&config.ConfigBuildOptions{Args: map[string]*string{"B": &v1}},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildOptsEqual(tt.a, tt.b); got != tt.want {
				t.Errorf("buildOptsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFeaturesEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b map[string]any
		want bool
	}{
		{"both nil", nil, nil, true},
		{"nil vs empty", nil, map[string]any{}, false},
		{
			"equal",
			map[string]any{"ghcr.io/devcontainers/features/go:1": map[string]any{"version": "1.22"}},
			map[string]any{"ghcr.io/devcontainers/features/go:1": map[string]any{"version": "1.22"}},
			true,
		},
		{
			"different options",
			map[string]any{"ghcr.io/devcontainers/features/go:1": map[string]any{"version": "1.22"}},
			map[string]any{"ghcr.io/devcontainers/features/go:1": map[string]any{"version": "1.23"}},
			false,
		},
		{
			"different features",
			map[string]any{"ghcr.io/devcontainers/features/go:1": map[string]any{}},
			map[string]any{"ghcr.io/devcontainers/features/node:1": map[string]any{}},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := featuresEqual(tt.a, tt.b); got != tt.want {
				t.Errorf("featuresEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}
