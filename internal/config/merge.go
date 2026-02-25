package config

// MergeConfiguration merges a DevContainerConfig with image metadata entries
// to produce a MergedDevContainerConfig. Metadata entries come from features
// and the base image. The base config takes priority, then entries are applied
// in reverse order (first entry = highest priority after base).
func MergeConfiguration(config *DevContainerConfig, imageMetadata []*ImageMetadata) *MergedDevContainerConfig {
	// Build the full list: config metadata + image metadata entries.
	// Reverse for priority (first = highest priority after base config).
	entries := reverseSlice(imageMetadata)

	merged := &MergedDevContainerConfig{
		ImageContainer:      config.ImageContainer,
		ComposeContainer:    config.ComposeContainer,
		DockerfileContainer: config.DockerfileContainer,
		Origin:              config.Origin,
	}

	// Merge base config fields with metadata entries.
	mergeConfigBase(&merged.DevContainerConfigBase, &config.DevContainerConfigBase, entries)
	mergeNonComposeBase(&merged.NonComposeBase, &config.NonComposeBase, entries)
	mergeConfigProperties(&merged.MergedConfigProperties, &config.DevContainerActions, entries)

	return merged
}

func mergeConfigBase(dst *DevContainerConfigBase, base *DevContainerConfigBase, entries []*ImageMetadata) {
	dst.Name = base.Name
	dst.Features = base.Features
	dst.OverrideFeatureInstallOrder = base.OverrideFeatureInstallOrder
	dst.ShutdownAction = base.ShutdownAction
	dst.WaitFor = base.WaitFor
	dst.UserEnvProbe = base.UserEnvProbe
	dst.HostRequirements = base.HostRequirements
	dst.InitializeCommand = base.InitializeCommand
	dst.WorkspaceFolder = base.WorkspaceFolder

	// RemoteUser: base config wins, then first entry with a value.
	dst.RemoteUser = base.RemoteUser
	if dst.RemoteUser == "" {
		dst.RemoteUser = firstString(entries, func(e *ImageMetadata) string { return e.RemoteUser })
	}

	// OverrideCommand: base config wins, then first entry.
	dst.OverrideCommand = base.OverrideCommand
	if dst.OverrideCommand == nil {
		dst.OverrideCommand = some(entries, func(e *ImageMetadata) *bool { return e.OverrideCommand })
	}

	// UpdateRemoteUserUID: base config wins, then first entry.
	dst.UpdateRemoteUserUID = base.UpdateRemoteUserUID
	if dst.UpdateRemoteUserUID == nil {
		dst.UpdateRemoteUserUID = some(entries, func(e *ImageMetadata) *bool { return e.UpdateRemoteUserUID })
	}

	// RemoteEnv: merge maps, base config values override entries.
	dst.RemoteEnv = mergeMaps(entries, func(e *ImageMetadata) map[string]string { return e.RemoteEnv })
	for k, v := range base.RemoteEnv {
		if dst.RemoteEnv == nil {
			dst.RemoteEnv = make(map[string]string)
		}
		dst.RemoteEnv[k] = v
	}

	// ForwardPorts: union and deduplicate.
	dst.ForwardPorts = mergeForwardPorts(base.ForwardPorts, entries)

	// PortsAttributes: merge maps.
	dst.PortsAttributes = mergePortsAttributes(base.PortsAttributes, entries)
	dst.OtherPortsAttributes = base.OtherPortsAttributes
}

func mergeNonComposeBase(dst *NonComposeBase, base *NonComposeBase, entries []*ImageMetadata) {
	dst.AppPort = base.AppPort
	dst.RunArgs = base.RunArgs
	dst.WorkspaceMount = base.WorkspaceMount

	// ContainerUser: base wins, then first entry.
	dst.ContainerUser = base.ContainerUser
	if dst.ContainerUser == "" {
		dst.ContainerUser = firstString(entries, func(e *ImageMetadata) string { return e.ContainerUser })
	}

	// ContainerEnv: merge maps.
	dst.ContainerEnv = mergeMaps(entries, func(e *ImageMetadata) map[string]string { return e.ContainerEnv })
	for k, v := range base.ContainerEnv {
		if dst.ContainerEnv == nil {
			dst.ContainerEnv = make(map[string]string)
		}
		dst.ContainerEnv[k] = v
	}

	// Mounts: union by target path, base wins.
	dst.Mounts = mergeMounts(base.Mounts, entries)

	// Init: base wins.
	dst.Init = base.Init
	if dst.Init == nil {
		dst.Init = some(entries, func(e *ImageMetadata) *bool { return e.Init })
	}

	// Privileged: base wins.
	dst.Privileged = base.Privileged
	if dst.Privileged == nil {
		dst.Privileged = some(entries, func(e *ImageMetadata) *bool { return e.Privileged })
	}

	// CapAdd: union and deduplicate.
	dst.CapAdd = mergeStringSlices(base.CapAdd, entries, func(e *ImageMetadata) []string { return e.CapAdd })

	// SecurityOpt: union and deduplicate.
	dst.SecurityOpt = mergeStringSlices(base.SecurityOpt, entries, func(e *ImageMetadata) []string { return e.SecurityOpt })
}

func mergeConfigProperties(dst *MergedConfigProperties, actions *DevContainerActions, entries []*ImageMetadata) {
	// Collect lifecycle hooks from all entries.
	dst.OnCreateCommands = mergeLifecycleHooks(
		actions.OnCreateCommand,
		entries,
		func(e *ImageMetadata) LifecycleHook { return e.OnCreateCommand },
	)
	dst.UpdateContentCommands = mergeLifecycleHooks(
		actions.UpdateContentCommand,
		entries,
		func(e *ImageMetadata) LifecycleHook { return e.UpdateContentCommand },
	)
	dst.PostCreateCommands = mergeLifecycleHooks(
		actions.PostCreateCommand,
		entries,
		func(e *ImageMetadata) LifecycleHook { return e.PostCreateCommand },
	)
	dst.PostStartCommands = mergeLifecycleHooks(
		actions.PostStartCommand,
		entries,
		func(e *ImageMetadata) LifecycleHook { return e.PostStartCommand },
	)
	dst.PostAttachCommands = mergeLifecycleHooks(
		actions.PostAttachCommand,
		entries,
		func(e *ImageMetadata) LifecycleHook { return e.PostAttachCommand },
	)

	// Collect entrypoints.
	for _, e := range entries {
		if e.Entrypoint != "" {
			dst.Entrypoints = append(dst.Entrypoints, e.Entrypoint)
		}
	}
}

// --- Merge helpers ---

// some returns the first non-nil bool pointer from entries.
func some[T any](entries []T, get func(T) *bool) *bool {
	for _, e := range entries {
		if v := get(e); v != nil {
			return v
		}
	}
	return nil
}

// firstString returns the first non-empty string from entries.
func firstString[T any](entries []T, get func(T) string) string {
	for _, e := range entries {
		if v := get(e); v != "" {
			return v
		}
	}
	return ""
}

// mergeMaps merges string maps from entries. Later entries override earlier ones.
func mergeMaps[T any, V comparable](entries []T, get func(T) map[string]V) map[string]V {
	var result map[string]V
	for _, e := range entries {
		m := get(e)
		if len(m) == 0 {
			continue
		}
		if result == nil {
			result = make(map[string]V)
		}
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

// mergeLifecycleHooks collects non-empty lifecycle hooks from entries,
// appending the base config's hook at the end.
func mergeLifecycleHooks(base LifecycleHook, entries []*ImageMetadata, get func(*ImageMetadata) LifecycleHook) []LifecycleHook {
	var result []LifecycleHook
	for _, e := range entries {
		hook := get(e)
		if len(hook) > 0 {
			result = append(result, hook)
		}
	}
	if len(base) > 0 {
		result = append(result, base)
	}
	return result
}

// mergeMounts unions mounts by target path. Base mounts take priority.
func mergeMounts(baseMounts []Mount, entries []*ImageMetadata) []Mount {
	seen := make(map[string]bool)
	var result []Mount

	// Base mounts first (highest priority).
	for _, m := range baseMounts {
		if m.Target != "" && !seen[m.Target] {
			seen[m.Target] = true
			result = append(result, m)
		}
	}

	// Then entry mounts.
	for _, e := range entries {
		for _, m := range e.Mounts {
			if m.Target != "" && !seen[m.Target] {
				seen[m.Target] = true
				result = append(result, m)
			}
		}
	}

	return result
}

// mergeForwardPorts unions and deduplicates forward ports.
func mergeForwardPorts(basePorts StrIntArray, entries []*ImageMetadata) StrIntArray {
	seen := make(map[string]bool)
	var result StrIntArray

	for _, p := range basePorts {
		if !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}

	for _, e := range entries {
		for _, p := range e.ForwardPorts {
			if !seen[p] {
				seen[p] = true
				result = append(result, p)
			}
		}
	}

	return result
}

// mergePortsAttributes merges port attribute maps.
func mergePortsAttributes(base map[string]PortAttribute, entries []*ImageMetadata) map[string]PortAttribute {
	entryAttrs := mergeMaps(entries, func(e *ImageMetadata) map[string]PortAttribute { return e.PortsAttributes })

	if len(base) == 0 && len(entryAttrs) == 0 {
		return nil
	}

	result := make(map[string]PortAttribute)
	for k, v := range entryAttrs {
		result[k] = v
	}
	for k, v := range base {
		result[k] = v
	}
	return result
}

// mergeStringSlices unions and deduplicates string slices.
func mergeStringSlices(base []string, entries []*ImageMetadata, get func(*ImageMetadata) []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, s := range base {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}

	for _, e := range entries {
		for _, s := range get(e) {
			if !seen[s] {
				seen[s] = true
				result = append(result, s)
			}
		}
	}

	return result
}

func reverseSlice[T any](s []T) []T {
	if len(s) == 0 {
		return s
	}
	result := make([]T, len(s))
	for i, v := range s {
		result[len(s)-1-i] = v
	}
	return result
}
