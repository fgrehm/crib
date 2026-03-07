package engine

import "github.com/fgrehm/crib/internal/plugin"

// EnvBuilder accumulates environment variables from multiple sources,
// each with defined precedence. Layers are merged bottom-up: higher
// layers override lower layers for the same key.
type EnvBuilder struct {
	probed        map[string]string // from userEnvProbe or restored stored env (lowest)
	containerPATH string            // Docker image ENV PATH entries
	pluginEnv     map[string]string // plugin Env responses
	pluginPrepend []string          // plugin PathPrepend dirs
	configEnv     map[string]string // devcontainer.json remoteEnv, resolved (highest)
}

// NewEnvBuilder creates a builder seeded with the devcontainer.json remoteEnv.
// The caller must not mutate configRemoteEnv after this call; use SetConfigEnv
// to replace the layer with a fresh copy when it changes (e.g. after
// resolveRemoteEnv resolves ${containerEnv:VAR} references).
func NewEnvBuilder(configRemoteEnv map[string]string) *EnvBuilder {
	return &EnvBuilder{
		configEnv: configRemoteEnv,
	}
}

// SetProbed sets the probed environment (from userEnvProbe).
// Replaces any previously set probed env. The builder takes ownership
// of env; the caller must not mutate it afterward.
func (b *EnvBuilder) SetProbed(env map[string]string) {
	b.probed = env
}

// RestoreFrom loads a previously saved env as the probed layer.
// Used by restart paths that skip setupContainer. The stored env is the
// output of a previous Build() (all layers merged, already filtered),
// so re-filtering in Build() is a no-op for skip-list vars but harmless.
func (b *EnvBuilder) RestoreFrom(storedEnv map[string]string) {
	b.probed = copyStringMap(storedEnv)
}

// SetContainerPATH records the container's base PATH for preservation.
func (b *EnvBuilder) SetContainerPATH(path string) {
	b.containerPATH = path
}

// SetConfigEnv updates the configEnv layer. Called after resolveRemoteEnv
// resolves ${containerEnv:VAR} references.
func (b *EnvBuilder) SetConfigEnv(env map[string]string) {
	b.configEnv = copyStringMap(env)
}

// AddPluginResponse merges a plugin response's Env and PathPrepend into
// the builder. Safe to call with nil.
func (b *EnvBuilder) AddPluginResponse(resp *plugin.PreContainerRunResponse) {
	if resp == nil {
		return
	}
	b.AddPluginEnv(resp.Env)
	b.AddPluginPathPrepend(resp.PathPrepend)
}

// AddPluginEnv merges plugin Env vars into the plugin layer.
func (b *EnvBuilder) AddPluginEnv(env map[string]string) {
	if len(env) == 0 {
		return
	}
	if b.pluginEnv == nil {
		b.pluginEnv = make(map[string]string, len(env))
	}
	for k, v := range env {
		b.pluginEnv[k] = v
	}
}

// AddPluginPathPrepend appends plugin PathPrepend dirs.
func (b *EnvBuilder) AddPluginPathPrepend(dirs []string) {
	b.pluginPrepend = append(b.pluginPrepend, dirs...)
}

// Build merges all layers and returns the final env map.
// Precedence (lowest to highest):
//  1. probed env (filtered via filterProbedEnv skip list)
//  2. container base PATH (append missing dirs)
//  3. plugin Env (overrides probed)
//  4. devcontainer.json remoteEnv (overrides everything for non-PATH keys)
//  5. plugin PathPrepend (prepended to PATH)
func (b *EnvBuilder) Build() map[string]string {
	if len(b.probed) == 0 && b.containerPATH == "" && len(b.pluginEnv) == 0 && len(b.configEnv) == 0 && len(b.pluginPrepend) == 0 {
		return nil
	}

	// Start with filtered probed env.
	result := filterProbedEnv(b.probed)
	if result == nil {
		result = make(map[string]string)
	}

	// Preserve container base PATH entries.
	preserveContainerPATH(result, b.containerPATH)

	// Plugin Env (overrides probed).
	for k, v := range b.pluginEnv {
		result[k] = v
	}

	// Config remoteEnv (overrides everything).
	for k, v := range b.configEnv {
		result[k] = v
	}

	// Plugin PathPrepend (prepend to whatever PATH we have).
	prependToPath(result, b.pluginPrepend)

	if len(result) == 0 {
		return nil
	}
	return result
}
