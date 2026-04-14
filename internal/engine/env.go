package engine

import (
	"maps"
	"strings"

	"github.com/fgrehm/crib/internal/config"
)

// parseEnvLines parses the output of the `env` command into a map.
// Each line is expected to be KEY=VALUE; lines without '=' are skipped.
// Values may contain '=' characters; only the first '=' is used as separator.
func parseEnvLines(output string) map[string]string {
	env := make(map[string]string)
	for line := range strings.SplitSeq(output, "\n") {
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
	return config.EnvMap()
}

// probedEnvSkip lists probed environment variables to exclude from the
// final env. These are session-specific or host-specific values that are
// meaningless inside containers and would override container defaults or
// confuse tool managers (mise, etc.) when injected into a new shell.
var probedEnvSkip = map[string]bool{
	"HOSTNAME":   true,
	"SHLVL":      true,
	"PWD":        true,
	"OLDPWD":     true,
	"_":          true,
	"MISE_SHELL": true,

	// Terminal colors and pager helpers.
	"LS_COLORS": true,
	"LSCOLORS":  true,
	"LESSCLOSE": true,
	"LESSOPEN":  true,

	// Terminal identity.
	"TERM_PROGRAM":         true,
	"TERM_PROGRAM_VERSION": true,
	"COLORTERM":            true,
	"VTE_VERSION":          true,

	// X11/Wayland display.
	"WINDOWID":        true,
	"DISPLAY":         true,
	"WAYLAND_DISPLAY": true,

	// Desktop session.
	"DESKTOP_SESSION":          true,
	"SESSION_MANAGER":          true,
	"XDG_SESSION_TYPE":         true,
	"XDG_SESSION_CLASS":        true,
	"XDG_SESSION_ID":           true,
	"XDG_CURRENT_DESKTOP":      true,
	"DBUS_SESSION_BUS_ADDRESS": true,
	"GPG_AGENT_INFO":           true,
}

// filterProbedEnv returns a copy of probed with session-specific and
// host-specific variables removed. Variables prefixed with __MISE_ are
// also excluded.
func filterProbedEnv(probed map[string]string) map[string]string {
	if len(probed) == 0 {
		return nil
	}

	result := make(map[string]string)
	for k, v := range probed {
		if probedEnvSkip[k] || strings.HasPrefix(k, "__MISE_") {
			continue
		}
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
	maps.Copy(cp, m)
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

	for d := range strings.SplitSeq(containerPATH, ":") {
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
		for d := range strings.SplitSeq(current, ":") {
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
