package engine

import (
	"crypto/sha256"
	"fmt"
	"os"
	"reflect"
	"sort"

	"github.com/fgrehm/crib/internal/config"
)

// configChangeKind classifies what changed between stored and current config.
type configChangeKind int

const (
	changeNone         configChangeKind = iota // Nothing changed.
	changeSafe                                 // Volumes, ports, env, mounts — container recreate is sufficient.
	changeNeedsRebuild                         // Image, Dockerfile, features — full rebuild required.
)

// detectConfigChange compares a stored config with a freshly parsed config
// and classifies the changes.
func detectConfigChange(stored, current *config.DevContainerConfig) configChangeKind {
	// Check image-affecting changes.
	if stored.Image != current.Image {
		return changeNeedsRebuild
	}
	if stored.Dockerfile != current.Dockerfile {
		return changeNeedsRebuild
	}
	if !buildOptsEqual(stored.Build, current.Build) {
		return changeNeedsRebuild
	}
	if !featuresEqual(stored.Features, current.Features) {
		return changeNeedsRebuild
	}

	// Check safe changes (container runtime config).
	if !stringMapsEqual(stored.ContainerEnv, current.ContainerEnv) {
		return changeSafe
	}
	// Note: RemoteEnv is intentionally not compared here. The stored config
	// includes probed environment values (from userEnvProbe) merged into
	// RemoteEnv during setup, which won't be present in a freshly parsed
	// config. Also, remoteEnv is injected at exec time via -e flags, so
	// changes don't require container recreation.
	if stored.ContainerUser != current.ContainerUser {
		return changeSafe
	}
	if stored.RemoteUser != current.RemoteUser {
		return changeSafe
	}
	if stored.WorkspaceMount != current.WorkspaceMount {
		return changeSafe
	}
	if stored.WorkspaceFolder != current.WorkspaceFolder {
		return changeSafe
	}
	if !mountsEqual(stored.Mounts, current.Mounts) {
		return changeSafe
	}
	if !strSlicesEqual(stored.RunArgs, current.RunArgs) {
		return changeSafe
	}
	if !strSlicesEqual([]string(stored.AppPort), []string(current.AppPort)) {
		return changeSafe
	}
	if !strSlicesEqual([]string(stored.ForwardPorts), []string(current.ForwardPorts)) {
		return changeSafe
	}
	if !boolPtrEqual(stored.Init, current.Init) {
		return changeSafe
	}
	if !boolPtrEqual(stored.Privileged, current.Privileged) {
		return changeSafe
	}
	if !strSlicesEqual(stored.CapAdd, current.CapAdd) {
		return changeSafe
	}
	if !strSlicesEqual(stored.SecurityOpt, current.SecurityOpt) {
		return changeSafe
	}
	if !boolPtrEqual(stored.OverrideCommand, current.OverrideCommand) {
		return changeSafe
	}

	// Check compose-specific safe changes.
	if !strSlicesEqual([]string(stored.DockerComposeFile), []string(current.DockerComposeFile)) {
		return changeSafe
	}
	if stored.Service != current.Service {
		return changeSafe
	}
	if !strSlicesEqual(stored.RunServices, current.RunServices) {
		return changeSafe
	}

	return changeNone
}

// --- comparison helpers ---

func stringMapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func strSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func boolPtrEqual(a, b *bool) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func mountsEqual(a, b []config.Mount) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func buildOptsEqual(a, b *config.ConfigBuildOptions) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Dockerfile != b.Dockerfile || a.Context != b.Context || a.Target != b.Target {
		return false
	}
	if !strSlicesEqual([]string(a.CacheFrom), []string(b.CacheFrom)) {
		return false
	}
	if !strSlicesEqual(a.Options, b.Options) {
		return false
	}
	// Compare args.
	if len(a.Args) != len(b.Args) {
		return false
	}
	for k, v := range a.Args {
		bv, ok := b.Args[k]
		if !ok {
			return false
		}
		if (v == nil) != (bv == nil) {
			return false
		}
		if v != nil && *v != *bv {
			return false
		}
	}
	return true
}

func featuresEqual(a, b map[string]any) bool {
	return reflect.DeepEqual(a, b)
}

// computeComposeFilesHash computes a short fingerprint (truncated SHA-256) of
// the contents of all compose files. This catches changes inside compose files
// (volumes, ports, env, etc.) that are invisible to detectConfigChange, which
// only compares devcontainer.json fields. The fingerprint is 16 hex characters
// (8 bytes), matching the truncation used by computeHookHash.
//
// Limitation: this is a raw content fingerprint, not a parsed comparison. Any
// change in the compose files (including build-affecting fields like "build:"
// or "image:") is classified as changeSafe (recreate without rebuild). If build
// config changed, the recreated container will still use the old image. Run
// "crib rebuild" to pick up compose build changes.
func computeComposeFilesHash(files []string) string {
	if len(files) == 0 {
		return ""
	}

	// Sort for stable ordering regardless of config order.
	sorted := make([]string, len(files))
	copy(sorted, files)
	sort.Strings(sorted)

	h := sha256.New()
	for _, f := range sorted {
		data, err := os.ReadFile(f)
		if err != nil {
			// Include the error so a missing file still changes the hash.
			h.Write([]byte("err:" + f + ":" + err.Error()))
			continue
		}
		// Include filename as separator to avoid collisions when file
		// contents are moved between files.
		h.Write([]byte(f))
		h.Write([]byte{0})
		h.Write(data)
	}
	return fmt.Sprintf("%x", h.Sum(nil)[:8])
}
