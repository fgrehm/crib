package feature

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// PrepareContext creates the feature installation directory within the build
// context. It copies feature folders as numbered directories (0, 1, 2, ...),
// writes environment files, and generates wrapper installation scripts.
// Returns the path to the features folder within the context.
func PrepareContext(contextPath string, features []*FeatureSet, containerUser, remoteUser string) (string, error) {
	featuresDir := filepath.Join(contextPath, ContextFeatureFolder)

	// Clean up any existing features directory.
	if err := os.RemoveAll(featuresDir); err != nil {
		return "", fmt.Errorf("removing existing features dir: %w", err)
	}

	if err := os.MkdirAll(featuresDir, 0o755); err != nil {
		return "", fmt.Errorf("creating features dir: %w", err)
	}

	// Write builtin env file with container/remote user info.
	builtinEnv := fmt.Sprintf(
		"_CONTAINER_USER=%q\n_REMOTE_USER=%q\n_CONTAINER_USER_HOME=%q\n_REMOTE_USER_HOME=%q\n",
		containerUser, remoteUser,
		userHome(containerUser), userHome(remoteUser),
	)
	builtinEnvPath := filepath.Join(featuresDir, builtinEnvFile)
	if err := os.WriteFile(builtinEnvPath, []byte(builtinEnv), 0o644); err != nil {
		return "", fmt.Errorf("writing builtin env: %w", err)
	}

	// Copy each feature and generate its installation files.
	for i, f := range features {
		destDir := filepath.Join(featuresDir, fmt.Sprintf("%d", i))
		if err := copyDir(f.Folder, destDir); err != nil {
			return "", fmt.Errorf("copying feature %q: %w", f.ConfigID, err)
		}

		// Write per-feature env file with option variables.
		envVars := FeatureEnvVars(f.Config, f.Options)
		envContent := strings.Join(envVars, "\n")
		if envContent != "" {
			envContent += "\n"
		}
		envPath := filepath.Join(destDir, featureEnvFile)
		if err := os.WriteFile(envPath, []byte(envContent), 0o644); err != nil {
			return "", fmt.Errorf("writing feature env for %q: %w", f.ConfigID, err)
		}

		// Write wrapper installation script.
		script := installWrapperScript(f.ConfigID, f.Config, envVars)
		scriptPath := filepath.Join(destDir, "devcontainer-features-install.sh")
		if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
			return "", fmt.Errorf("writing install script for %q: %w", f.ConfigID, err)
		}
	}

	return featuresDir, nil
}

// installWrapperScript generates a shell script that sources environment
// files and runs the feature's install.sh.
func installWrapperScript(featureID string, fc *FeatureConfig, envVars []string) string {
	var b strings.Builder

	b.WriteString("#!/bin/sh\n")
	b.WriteString("set -e\n")
	b.WriteString("\n")

	// Feature metadata.
	fmt.Fprintf(&b, "echo 'Feature: %s'\n", escapeQuotes(featureID))
	if fc.Name != "" {
		fmt.Fprintf(&b, "echo 'Name: %s'\n", escapeQuotes(fc.Name))
	}
	if fc.Version != "" {
		fmt.Fprintf(&b, "echo 'Version: %s'\n", escapeQuotes(fc.Version))
	}
	b.WriteString("\n")

	// Source builtin env.
	b.WriteString(". /tmp/build-features/devcontainer-features.builtin.env\n")
	b.WriteString("\n")

	// Export feature-specific env vars.
	for _, envVar := range envVars {
		fmt.Fprintf(&b, "export %s\n", envVar)
	}
	if len(envVars) > 0 {
		b.WriteString("\n")
	}

	// Run the feature install script. The entrypoint field in
	// devcontainer-feature.json is the container entrypoint (not the
	// install script), so the install script is always install.sh.
	b.WriteString("cd \"$(dirname \"$0\")\"\n")
	b.WriteString("chmod +x ./install.sh\n")
	b.WriteString("./install.sh\n")

	return b.String()
}

// escapeQuotes escapes single quotes for safe inclusion in shell strings.
func escapeQuotes(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}

// copyDir recursively copies a directory tree from src to dst.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dst, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0o755)
		}

		return copyFile(path, destPath)
	})
}

// copyFile copies a single file, preserving permissions.
func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}

	if _, err = io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// userHome returns the expected home directory for a user.
func userHome(user string) string {
	if user == "" || user == "root" {
		return "/root"
	}
	return "/home/" + user
}
