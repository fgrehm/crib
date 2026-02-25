package config

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var variableRegexp = regexp.MustCompile(`\$\{(.*?)\}`)

// SubstitutionContext holds the values available for variable substitution.
type SubstitutionContext struct {
	DevContainerID           string
	LocalWorkspaceFolder     string
	ContainerWorkspaceFolder string
	Env                      map[string]string
}

// Substitute applies variable substitution to the given config.
// Supported variables:
//   - ${localEnv:VAR}, ${localEnv:VAR:default}
//   - ${containerEnv:VAR}, ${containerEnv:VAR:default}
//   - ${localWorkspaceFolder}, ${localWorkspaceFolderBasename}
//   - ${containerWorkspaceFolder}, ${containerWorkspaceFolderBasename}
//   - ${devcontainerId}
func Substitute(ctx *SubstitutionContext, config *DevContainerConfig) (*DevContainerConfig, error) {
	replacer := func(match, variable string, args []string) string {
		return replaceWithContext(ctx, match, variable, args)
	}
	return substituteConfig(config, replacer)
}

// SubstituteContainerEnv substitutes only ${containerEnv:VAR} variables
// using the provided container environment variables.
func SubstituteContainerEnv(containerEnv map[string]string, config *DevContainerConfig) (*DevContainerConfig, error) {
	replacer := func(match, variable string, args []string) string {
		if !strings.EqualFold(variable, "containerEnv") {
			return match
		}
		return lookupEnv(containerEnv, args, match)
	}
	return substituteConfig(config, replacer)
}

type replaceFunc func(match, variable string, args []string) string

func substituteConfig(config *DevContainerConfig, replacer replaceFunc) (*DevContainerConfig, error) {
	// Marshal to generic map for recursive substitution.
	data, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshaling config: %w", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("unmarshaling to map: %w", err)
	}

	substituted := substituteValue(raw, replacer)

	data, err = json.Marshal(substituted)
	if err != nil {
		return nil, fmt.Errorf("marshaling substituted config: %w", err)
	}

	var result DevContainerConfig
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unmarshaling substituted config: %w", err)
	}

	// Preserve the Origin field (not serialized in JSON).
	result.Origin = config.Origin
	return &result, nil
}

func substituteValue(val any, replacer replaceFunc) any {
	switch v := val.(type) {
	case string:
		return resolveString(v, replacer)
	case map[string]any:
		result := make(map[string]any, len(v))
		for k, vv := range v {
			result[k] = substituteValue(vv, replacer)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, vv := range v {
			result[i] = substituteValue(vv, replacer)
		}
		return result
	default:
		return val
	}
}

func resolveString(s string, replacer replaceFunc) string {
	return variableRegexp.ReplaceAllStringFunc(s, func(match string) string {
		// Strip ${ and }.
		inner := match[2 : len(match)-1]

		// Split on colons: variable:arg1:arg2
		parts := strings.SplitN(inner, ":", 3)
		variable := parts[0]
		args := parts[1:]

		return replacer(match, variable, args)
	})
}

func replaceWithContext(ctx *SubstitutionContext, match, variable string, args []string) string {
	switch variable {
	case "devcontainerId":
		return ctx.DevContainerID

	case "localWorkspaceFolder":
		return ctx.LocalWorkspaceFolder

	case "localWorkspaceFolderBasename":
		return filepath.Base(ctx.LocalWorkspaceFolder)

	case "containerWorkspaceFolder":
		return ctx.ContainerWorkspaceFolder

	case "containerWorkspaceFolderBasename":
		return filepath.Base(ctx.ContainerWorkspaceFolder)

	case "localEnv", "env":
		return lookupEnv(ctx.Env, args, match)

	case "containerEnv":
		// containerEnv is not available during initial substitution,
		// leave the variable as-is for later resolution.
		return match

	default:
		// Unknown variables are left as-is.
		return match
	}
}

func lookupEnv(env map[string]string, args []string, match string) string {
	if len(args) == 0 {
		return match
	}

	varName := args[0]
	if val, ok := env[varName]; ok {
		return val
	}

	// Use default value if provided.
	if len(args) >= 2 {
		return args[1]
	}

	// No value and no default: empty string.
	return ""
}
