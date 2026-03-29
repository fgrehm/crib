package cmd

import "testing"

func TestVersionString_Dev(t *testing.T) {
	origV, origC, origD := version, commit, date
	defer func() { version, commit, date = origV, origC, origD }()

	version = "0.2.0-dev"
	commit = "abc1234"
	date = "2026-02-27T10:00:00Z"

	got := versionString()
	want := "crib 0.2.0-dev (abc1234, 2026-02-27T10:00:00Z)"
	if got != want {
		t.Errorf("versionString() = %q, want %q", got, want)
	}
}

func TestVersionString_DevUnknownDate(t *testing.T) {
	origV, origC, origD := version, commit, date
	defer func() { version, commit, date = origV, origC, origD }()

	version = "0.2.0-dev"
	commit = "abc1234"
	date = "unknown"

	got := versionString()
	want := "crib 0.2.0-dev (abc1234)"
	if got != want {
		t.Errorf("versionString() = %q, want %q", got, want)
	}
}

func TestVersionString_Release(t *testing.T) {
	origV, origC, origD := version, commit, date
	defer func() { version, commit, date = origV, origC, origD }()

	version = "0.2.0"
	commit = "abc1234"
	date = "2026-02-27T10:00:00Z"

	got := versionString()
	want := "crib 0.2.0"
	if got != want {
		t.Errorf("versionString() = %q, want %q", got, want)
	}
}

func TestVersionString_UnknownCommit(t *testing.T) {
	origV, origC, origD := version, commit, date
	defer func() { version, commit, date = origV, origC, origD }()

	version = "0.3.0-dev"
	commit = "unknown"
	date = "unknown"

	got := versionString()
	want := "crib 0.3.0-dev"
	if got != want {
		t.Errorf("versionString() = %q, want %q", got, want)
	}
}
