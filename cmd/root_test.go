package cmd

import "testing"

func TestVersionString_Dev(t *testing.T) {
	origV, origC, origB := Version, Commit, Built
	defer func() { Version, Commit, Built = origV, origC, origB }()

	Version = "0.2.0-dev"
	Commit = "abc1234"
	Built = "2026-02-27T10:00:00Z"

	got := versionString()
	want := "crib 0.2.0-dev (abc1234, 2026-02-27T10:00:00Z)"
	if got != want {
		t.Errorf("versionString() = %q, want %q", got, want)
	}
}

func TestVersionString_DevUnknownBuilt(t *testing.T) {
	origV, origC, origB := Version, Commit, Built
	defer func() { Version, Commit, Built = origV, origC, origB }()

	Version = "0.2.0-dev"
	Commit = "abc1234"
	Built = "unknown"

	got := versionString()
	want := "crib 0.2.0-dev (abc1234)"
	if got != want {
		t.Errorf("versionString() = %q, want %q", got, want)
	}
}

func TestVersionString_Release(t *testing.T) {
	origV, origC, origB := Version, Commit, Built
	defer func() { Version, Commit, Built = origV, origC, origB }()

	Version = "0.2.0"
	Commit = "abc1234"
	Built = "2026-02-27T10:00:00Z"

	got := versionString()
	want := "crib 0.2.0"
	if got != want {
		t.Errorf("versionString() = %q, want %q", got, want)
	}
}

func TestVersionString_UnknownCommit(t *testing.T) {
	origV, origC, origB := Version, Commit, Built
	defer func() { Version, Commit, Built = origV, origC, origB }()

	Version = "0.3.0-dev"
	Commit = "unknown"
	Built = "unknown"

	got := versionString()
	want := "crib 0.3.0-dev"
	if got != want {
		t.Errorf("versionString() = %q, want %q", got, want)
	}
}
