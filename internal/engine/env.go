package engine

import (
	"os"
	"strings"
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
// Session-specific variables (HOSTNAME, SHLVL, PWD, _) are excluded from
// the probed set to avoid overriding container runtime defaults.
func mergeEnv(probed map[string]string, remoteEnv map[string]string) map[string]string {
	if len(probed) == 0 && len(remoteEnv) == 0 {
		return nil
	}

	skip := map[string]bool{
		"HOSTNAME": true,
		"SHLVL":    true,
		"PWD":      true,
		"_":        true,
	}

	result := make(map[string]string)
	for k, v := range probed {
		if !skip[k] {
			result[k] = v
		}
	}
	for k, v := range remoteEnv {
		result[k] = v
	}
	return result
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
