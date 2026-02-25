package feature

import (
	"testing"

	"github.com/fgrehm/crib/internal/config"
)

func TestFeatureEnvVarsMap(t *testing.T) {
	fc := &FeatureConfig{
		Options: map[string]FeatureOption{
			"version": {Default: config.StrBool("latest")},
			"tools":   {Default: config.StrBool("true")},
		},
	}

	userOptions := map[string]any{
		"version": "3.12",
	}

	lines := FeatureEnvVars(fc, userOptions)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}

	// Sorted output.
	want := []string{
		`TOOLS="true"`,
		`VERSION="3.12"`,
	}
	for i, line := range lines {
		if line != want[i] {
			t.Errorf("line[%d] = %q, want %q", i, line, want[i])
		}
	}
}

func TestFeatureEnvVarsString(t *testing.T) {
	fc := &FeatureConfig{
		Options: map[string]FeatureOption{
			"version": {Default: config.StrBool("latest")},
		},
	}

	lines := FeatureEnvVars(fc, "3.12")
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	if lines[0] != `VERSION="3.12"` {
		t.Errorf("got %q, want %q", lines[0], `VERSION="3.12"`)
	}
}

func TestFeatureEnvVarsDefaults(t *testing.T) {
	fc := &FeatureConfig{
		Options: map[string]FeatureOption{
			"version": {Default: config.StrBool("latest")},
			"tools":   {Default: config.StrBool("true")},
		},
	}

	lines := FeatureEnvVars(fc, nil)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}

	want := []string{
		`TOOLS="true"`,
		`VERSION="latest"`,
	}
	for i, line := range lines {
		if line != want[i] {
			t.Errorf("line[%d] = %q, want %q", i, line, want[i])
		}
	}
}

func TestFeatureEnvVarsEmpty(t *testing.T) {
	fc := &FeatureConfig{}
	lines := FeatureEnvVars(fc, nil)
	if len(lines) != 0 {
		t.Errorf("got %d lines, want 0", len(lines))
	}
}

func TestSafeID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"version", "VERSION"},
		{"installTools", "INSTALLTOOLS"},
		{"my-option", "MY_OPTION"},
		{"my.option", "MY_OPTION"},
		{"my option", "MY_OPTION"},
		{"ALREADY_UPPER", "ALREADY_UPPER"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := safeID(tt.input)
			if got != tt.want {
				t.Errorf("safeID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFeatureEnvVarsSorted(t *testing.T) {
	fc := &FeatureConfig{
		Options: map[string]FeatureOption{
			"zebra":  {Default: config.StrBool("z")},
			"alpha":  {Default: config.StrBool("a")},
			"middle": {Default: config.StrBool("m")},
		},
	}

	lines := FeatureEnvVars(fc, nil)
	for i := 1; i < len(lines); i++ {
		if lines[i] < lines[i-1] {
			t.Errorf("output not sorted: %v", lines)
			break
		}
	}
}
