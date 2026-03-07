package engine

import (
	"os"
	"strings"

	"github.com/fgrehm/crib/internal/config"
)

// parseEnvLines parses the output of the `env` command into a map.
// Each line is expected to be KEY=VALUE; lines without '=' are skipped.
// Values may contain '=' characters; only the first '=' is used as separator.
func parseEnvLines(output string) map[string]string {
	env := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok || k == "" {
			continue
		}
		env[k] = v
	}
	return env
}

// envMap returns the current process environment as a map.
func envMap() map[string]string {
	env := make(map[string]string)
	for _, e := range os.Environ() {
		if k, v, ok := strings.Cut(e, "="); ok {
			env[k] = v
		}
	}
	return env
}

// mergeEnv merges probed environment variables with remoteEnv.
// remoteEnv values take precedence over probed values on conflicts.
// Session-specific variables and tool-manager internal state are excluded
// from the probed set to avoid overriding container runtime defaults or
// confusing tool managers (mise, etc.) when injected into a new shell.
func mergeEnv(probed map[string]string, remoteEnv map[string]string) map[string]string {
	if len(probed) == 0 && len(remoteEnv) == 0 {
		return nil
	}

	skip := map[string]bool{
		"HOSTNAME":   true,
		"SHLVL":      true,
		"PWD":        true,
		"OLDPWD":     true,
		"_":          true,
		"MISE_SHELL": true,
	}

	result := make(map[string]string)
	for k, v := range probed {
		if skip[k] || strings.HasPrefix(k, "__MISE_") {
			continue
		}
		result[k] = v
	}
	for k, v := range remoteEnv {
		result[k] = v
	}
	return result
}

// copyStringMap returns a shallow copy of a string map, or nil if the input is nil.
func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	cp := make(map[string]string, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

// preserveContainerPATH merges container-base PATH entries into the env map's
// PATH. Login shells on Debian reset PATH via /etc/profile, dropping entries
// that Docker images add via ENV (e.g. /usr/local/bundle/bin in ruby images,
// /usr/local/go/bin in golang images). This appends any missing container PATH
// entries after the probed ones so they remain accessible.
func preserveContainerPATH(env map[string]string, containerPATH string) {
	if env == nil || containerPATH == "" {
		return
	}
	probedPATH, ok := env["PATH"]
	if !ok {
		return
	}

	dirs := strings.Split(probedPATH, ":")
	seen := make(map[string]bool, len(dirs))
	for _, d := range dirs {
		seen[d] = true
	}

	for _, d := range strings.Split(containerPATH, ":") {
		if d != "" && !seen[d] {
			dirs = append(dirs, d)
			seen[d] = true
		}
	}

	env["PATH"] = strings.Join(dirs, ":")
}

// prependToPath prepends the given directories to the PATH in env, skipping
// any that are already present. This is used to inject plugin-requested PATH
// additions (e.g. ~/.bundle/bin for bundler) that work regardless of shell type.
func prependToPath(env map[string]string, dirs []string) {
	if env == nil || len(dirs) == 0 {
		return
	}
	current := env["PATH"]
	existing := make(map[string]bool)
	if current != "" {
		for _, d := range strings.Split(current, ":") {
			existing[d] = true
		}
	}

	var prepend []string
	for _, d := range dirs {
		if d != "" && !existing[d] {
			prepend = append(prepend, d)
			existing[d] = true
		}
	}
	if len(prepend) == 0 {
		return
	}
	if current != "" {
		env["PATH"] = strings.Join(prepend, ":") + ":" + current
	} else {
		env["PATH"] = strings.Join(prepend, ":")
	}
}

// mergeStoredRemoteEnv restores the stored probed environment into cfg.RemoteEnv.
// Restart paths that skip setupContainer (no env re-probe) must call this
// before saveResult so the probed PATH entries (mise, rbenv, nvm) persist.
//
// For PATH: uses the stored value as the base and prepends any dirs from
// cfg.RemoteEnv["PATH"] that aren't already present (fresh plugin PathPrepend).
// For all other vars: stored values fill in as fallbacks; values already in
// cfg.RemoteEnv (from devcontainer.json or plugins) take precedence.
func mergeStoredRemoteEnv(cfg *config.DevContainerConfig, storedEnv map[string]string) {
	if len(storedEnv) == 0 {
		return
	}
	if cfg.RemoteEnv == nil {
		cfg.RemoteEnv = make(map[string]string)
	}

	// Handle PATH specially: prepend any new dirs from cfg onto the stored PATH.
	if storedPath, ok := storedEnv["PATH"]; ok {
		if freshPath, hasFresh := cfg.RemoteEnv["PATH"]; hasFresh && freshPath != "" {
			// Prepend fresh dirs that aren't already in the stored PATH.
			storedDirs := strings.Split(storedPath, ":")
			storedSet := make(map[string]bool, len(storedDirs))
			for _, d := range storedDirs {
				storedSet[d] = true
			}
			var newDirs []string
			for _, d := range strings.Split(freshPath, ":") {
				if d != "" && !storedSet[d] {
					newDirs = append(newDirs, d)
				}
			}
			if len(newDirs) > 0 {
				cfg.RemoteEnv["PATH"] = strings.Join(newDirs, ":") + ":" + storedPath
			} else {
				cfg.RemoteEnv["PATH"] = storedPath
			}
		} else {
			cfg.RemoteEnv["PATH"] = storedPath
		}
	}

	// For all other vars, stored values are fallbacks.
	for k, v := range storedEnv {
		if k == "PATH" {
			continue
		}
		if _, exists := cfg.RemoteEnv[k]; !exists {
			cfg.RemoteEnv[k] = v
		}
	}
}

// applyPathPrepend initializes cfg.RemoteEnv if needed and prepends the given
// paths. Used by resume-hook callers that don't go through setupContainer.
func applyPathPrepend(cfg *config.DevContainerConfig, dirs []string) {
	if len(dirs) == 0 {
		return
	}
	if cfg.RemoteEnv == nil {
		cfg.RemoteEnv = make(map[string]string)
	}
	prependToPath(cfg.RemoteEnv, dirs)
}

// envSlice converts a map of env vars to KEY=VALUE strings for ExecContainer.
func envSlice(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	result := make([]string, 0, len(env))
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}
