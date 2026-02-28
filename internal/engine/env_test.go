package engine

import (
	"sort"
	"testing"
)

func TestCopyStringMap(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if got := copyStringMap(nil); got != nil {
			t.Errorf("copyStringMap(nil) = %v, want nil", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := copyStringMap(map[string]string{})
		if got == nil || len(got) != 0 {
			t.Errorf("copyStringMap({}) = %v, want empty map", got)
		}
	})

	t.Run("copies values", func(t *testing.T) {
		orig := map[string]string{"a": "1", "b": "2"}
		cp := copyStringMap(orig)
		if cp["a"] != "1" || cp["b"] != "2" {
			t.Errorf("copyStringMap values wrong: %v", cp)
		}
		// Mutating the copy should not affect the original.
		cp["a"] = "changed"
		if orig["a"] != "1" {
			t.Error("mutating copy changed the original")
		}
	})
}

func TestEnvSlice(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if got := envSlice(nil); got != nil {
			t.Errorf("envSlice(nil) = %v, want nil", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		if got := envSlice(map[string]string{}); got != nil {
			t.Errorf("envSlice({}) = %v, want nil", got)
		}
	})

	t.Run("converts to KEY=VALUE", func(t *testing.T) {
		got := envSlice(map[string]string{"FOO": "bar", "BAZ": "qux"})
		sort.Strings(got)
		want := []string{"BAZ=qux", "FOO=bar"}
		if len(got) != len(want) {
			t.Fatalf("envSlice len = %d, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("envSlice[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("empty value", func(t *testing.T) {
		got := envSlice(map[string]string{"KEY": ""})
		if len(got) != 1 || got[0] != "KEY=" {
			t.Errorf("envSlice empty value = %v, want [KEY=]", got)
		}
	})
}
