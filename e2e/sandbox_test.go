package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sandboxDevcontainerJSON uses a Debian-based image (bubblewrap requires
// apt-get) with the sandbox plugin configured for filesystem and network
// restrictions. remoteUser is "root" for a predictable home directory
// (/root), avoiding UID-sync edge cases where the container user's home
// ends up as "/".
const sandboxDevcontainerJSON = `{
	"name": "sandbox-e2e",
	"image": "debian:bookworm-slim",
	"overrideCommand": true,
	"remoteUser": "root",
	"customizations": {
		"crib": {
			"sandbox": {
				"denyRead": ["~/.secret-config"],
				"blockLocalNetwork": true,
				"aliases": ["cat"]
			}
		}
	}
}`

func setupSandboxProject(t *testing.T) (projectDir, cribHome string) {
	t.Helper()
	dir := t.TempDir()
	devDir := filepath.Join(dir, ".devcontainer")
	if err := os.MkdirAll(devDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devDir, "devcontainer.json"), []byte(sandboxDevcontainerJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	cribHome = t.TempDir()

	t.Cleanup(func() {
		cmd := cribCmd(dir, cribHome, "remove", "--force")
		_ = cmd.Run()
	})

	mustRunCrib(t, dir, cribHome, "up")

	return dir, cribHome
}

func TestE2ESandboxInstallsAndGeneratesWrapper(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}

	projectDir, cribHome := setupSandboxProject(t)

	// bubblewrap should be installed.
	mustRunCrib(t, projectDir, cribHome, "exec", "--", "sh", "-c", "command -v bwrap")

	// Sandbox wrapper script should exist and be executable.
	mustRunCrib(t, projectDir, cribHome, "exec", "--", "test", "-x", "/root/.local/bin/sandbox")

	// Sandbox wrapper should contain bwrap invocation and key policy elements.
	out := mustRunCrib(t, projectDir, cribHome, "exec", "--",
		"sh", "-c", "cat /root/.local/bin/sandbox")
	if !strings.Contains(out, "exec bwrap") {
		t.Errorf("sandbox wrapper missing 'exec bwrap', got:\n%s", out)
	}
	if !strings.Contains(out, "--ro-bind / /") {
		t.Errorf("sandbox wrapper missing '--ro-bind / /', got:\n%s", out)
	}
	if !strings.Contains(out, "--bind /tmp /tmp") {
		t.Errorf("sandbox wrapper missing writable /tmp bind, got:\n%s", out)
	}
}

func TestE2ESandboxAliasWrapper(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}

	projectDir, cribHome := setupSandboxProject(t)

	// Alias wrapper for "cat" should exist and be executable.
	mustRunCrib(t, projectDir, cribHome, "exec", "--", "test", "-x", "/root/.local/bin/cat")

	// Alias wrapper should contain the banner and reference the sandbox.
	out := mustRunCrib(t, projectDir, cribHome, "exec", "--",
		"sh", "-c", "cat /root/.local/bin/cat")
	if !strings.Contains(out, "[crib sandbox]") {
		t.Errorf("alias wrapper missing '[crib sandbox]' banner, got:\n%s", out)
	}
	if !strings.Contains(out, "/root/.local/bin/sandbox") {
		t.Errorf("alias wrapper missing sandbox path, got:\n%s", out)
	}
}

func TestE2ESandboxFilesystemIsolation(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}

	projectDir, cribHome := setupSandboxProject(t)

	// Verify the wrapper has the deny rule for the configured path.
	out := mustRunCrib(t, projectDir, cribHome, "exec", "--",
		"sh", "-c", "cat /root/.local/bin/sandbox")
	if !strings.Contains(out, "--tmpfs '/root/.secret-config'") {
		t.Errorf("sandbox wrapper missing tmpfs for denyRead path, got:\n%s", out)
	}

	// Create a file at the denied path so we can test read blocking.
	mustRunCrib(t, projectDir, cribHome, "exec", "--",
		"sh", "-c", "mkdir -p /root/.secret-config && echo top-secret > /root/.secret-config/creds")

	// Verify the file is readable without the sandbox.
	out = mustRunCrib(t, projectDir, cribHome, "exec", "--",
		"sh", "-c", "cat /root/.secret-config/creds")
	if !strings.Contains(out, "top-secret") {
		t.Fatalf("file should be readable without sandbox, got: %s", out)
	}

	// Through the sandbox, the denied path should be masked by tmpfs.
	// bwrap may fail if user namespaces are restricted (Ubuntu 24.04
	// AppArmor, some Docker-in-Docker setups), so skip gracefully.
	sandboxOut, err := runCrib(t, projectDir, cribHome, "exec", "--",
		"sh", "-c", "/root/.local/bin/sandbox cat /root/.secret-config/creds 2>&1")
	if err != nil {
		if strings.Contains(sandboxOut, "Operation not permitted") ||
			strings.Contains(sandboxOut, "Permission denied") ||
			strings.Contains(sandboxOut, "bwrap:") {
			t.Skipf("bwrap not usable in this environment (user namespaces restricted?): %s", strings.TrimSpace(sandboxOut))
		}
		// bwrap ran but cat couldn't find the file (tmpfs masked it).
		if !strings.Contains(sandboxOut, "No such file") {
			t.Errorf("expected 'No such file' for denied path, got: %s", sandboxOut)
		}
		return
	}
	// If the command succeeded, the output must not contain the secret.
	if strings.Contains(sandboxOut, "top-secret") {
		t.Error("sandbox failed to block read access to denied path")
	}
}

func TestE2ESandboxNetworkRules(t *testing.T) {
	if !hasRuntime() {
		t.Fatal("container runtime not available or not working (docker or podman required)")
	}

	projectDir, cribHome := setupSandboxProject(t)

	// Network rules are applied at post-create time (not per sandbox
	// invocation), so they should already be in effect. Install iptables
	// to query the CRIB_SANDBOX chain.
	_, err := runCrib(t, projectDir, cribHome, "exec", "--",
		"sh", "-c", "apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq iptables >/dev/null 2>&1")
	if err != nil {
		t.Skip("could not install iptables in container (insufficient privileges?)")
	}

	// OUTPUT should jump to CRIB_SANDBOX.
	outputOut := mustRunCrib(t, projectDir, cribHome, "exec", "--",
		"sh", "-c", "iptables -L OUTPUT -n 2>/dev/null")
	if !strings.Contains(outputOut, "CRIB_SANDBOX") {
		t.Errorf("expected OUTPUT chain to jump to CRIB_SANDBOX:\n%s", outputOut)
	}

	// CRIB_SANDBOX should contain the actual block rules.
	rulesOut := mustRunCrib(t, projectDir, cribHome, "exec", "--",
		"sh", "-c", "iptables -L CRIB_SANDBOX -n 2>/dev/null")
	for _, cidr := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
		if !strings.Contains(rulesOut, cidr) {
			t.Errorf("expected iptables rule for %s in CRIB_SANDBOX chain:\n%s", cidr, rulesOut)
		}
	}

	// Cloud metadata endpoint (link-local).
	if !strings.Contains(rulesOut, "169.254.0.0/16") {
		t.Errorf("expected iptables rule for 169.254.0.0/16 (cloud metadata) in CRIB_SANDBOX chain:\n%s", rulesOut)
	}
}
