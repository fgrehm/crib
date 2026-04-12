package config

import "os"

// EnvMap returns the current process environment as a map.
func EnvMap() map[string]string {
	env := make(map[string]string, len(os.Environ()))
	for _, e := range os.Environ() {
		if k, v, ok := cutEnv(e); ok {
			env[k] = v
		}
	}
	return env
}

// cutEnv splits an environment variable string at the first '='.
func cutEnv(s string) (string, string, bool) {
	for i := range len(s) {
		if s[i] == '=' {
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}
