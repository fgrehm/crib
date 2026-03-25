package engine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/fgrehm/crib/internal/config"
	"github.com/fgrehm/crib/internal/workspace"
)

// setupContainer performs post-creation container setup via docker exec.
// This includes:
//   - Resolving ${containerEnv:VAR} references in remoteEnv
//   - Synchronizing the container user's UID/GID with the host
//   - Chowning the workspace directory to the remote user
//   - Running lifecycle hooks
//
// imageMetadata contains feature metadata for merging lifecycle hooks. When
// non-nil, feature hooks are dispatched before user hooks at each stage.
//
// Returns the final merged environment produced by the EnvBuilder. Callers
// should assign it to cfg.RemoteEnv for persistence; setupContainer itself
// does not mutate cfg.RemoteEnv.
func (e *Engine) setupContainer(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, cc containerContext, envb *EnvBuilder, imageMetadata []*config.ImageMetadata) (map[string]string, error) {
	// Resolve ${containerEnv:VAR} in remoteEnv by probing the container environment.
	// Also captures the container's base PATH for later merging.
	var containerPATH string
	resolvedConfigEnv := cfg.RemoteEnv
	if len(cfg.RemoteEnv) > 0 {
		resolvedConfigEnv, containerPATH = e.resolveRemoteEnv(ctx, cc, cfg)
	}

	// Sync container user UID/GID with host before chowning.
	// uidsSynced is true when UIDs are confirmed to match (either already did, or were synced),
	// meaning chownWorkspace is not needed for bind mounts (rootless podman limitation).
	uidsSynced := false
	if cc.remoteUser != "" && cc.remoteUser != "root" {
		var err error
		uidsSynced, err = e.syncRemoteUserUID(ctx, cc, cfg)
		if err != nil {
			e.logger.Warn("failed to sync remote user UID/GID", "error", err)
		}
	}

	// Chown workspace directory to remote user, unless UIDs are already in sync.
	// When UIDs match, bind-mount files are already accessible and chown would fail
	// on rootless Podman (no CAP_CHOWN over bind-mounted files).
	if cc.remoteUser != "" && cc.remoteUser != "root" && !uidsSynced {
		if err := e.chownWorkspace(ctx, cc); err != nil {
			e.logger.Warn("failed to chown workspace", "error", err)
		}
	}

	// Update the builder's configEnv after resolveRemoteEnv has resolved
	// ${containerEnv:VAR} references.
	envb.SetConfigEnv(resolvedConfigEnv)

	// If resolveRemoteEnv didn't run (no remoteEnv), capture the container's
	// base PATH separately. Login shells on Debian reset PATH via /etc/profile,
	// dropping entries that Docker images add via ENV (e.g. /usr/local/bundle/bin
	// in ruby images). We merge these back after probing.
	// Skip entirely when userEnvProbe is "none" since no login shell runs.
	if containerPATH == "" && cfg.UserEnvProbe != "none" {
		containerPATH = e.probeContainerPATH(ctx, cc)
	}
	envb.SetContainerPATH(containerPATH)

	// Pre-hook environment probe: captures PATH and other vars from shell
	// profile files (e.g. mise, rbenv, nvm) so lifecycle hooks have the
	// user's full environment.
	probedEnv := e.probeUserEnv(ctx, cc, cfg.UserEnvProbe)
	envb.SetProbed(probedEnv)
	preHookEnv := envb.Build()

	// Build hook set: merge feature hooks with user hooks when metadata is available.
	var hooks *hookSet
	if len(imageMetadata) > 0 {
		merged := config.MergeConfiguration(cfg, imageMetadata)
		hooks = hookSetFromMerged(merged)
	} else {
		hooks = hookSetFromConfig(cfg)
	}

	// Run lifecycle hooks with the pre-hook merged environment.
	runner := e.newLifecycleRunner(ws, cc, preHookEnv)
	hookErr := runner.runLifecycleHooks(ctx, hooks, cc.workspaceFolder)

	// Post-hook environment probe: re-captures the environment to pick up
	// any changes from lifecycle hooks (e.g. tools installed via mise, nvm).
	// This is what gets persisted for crib shell/exec.
	postProbe := e.probeUserEnv(ctx, cc, cfg.UserEnvProbe)
	envb.SetProbed(postProbe)

	return envb.Build(), hookErr
}

// resolveRemoteEnv resolves ${containerEnv:VAR} references in cfg.RemoteEnv by
// probing the container's runtime environment. Returns the resolved env map and
// the container's base PATH for use in PATH preservation.
// Per the devcontainer spec, remoteEnv is injected by the tool (not written to
// /etc/environment) and ${containerEnv:VAR} is only valid in remoteEnv.
func (e *Engine) resolveRemoteEnv(ctx context.Context, cc containerContext, cfg *config.DevContainerConfig) (map[string]string, string) {
	var buf bytes.Buffer
	if err := e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID, []string{"env"}, nil, &buf, io.Discard, nil, ""); err != nil {
		e.logger.Warn("failed to probe container environment for remoteEnv resolution", "error", err)
		return cfg.RemoteEnv, ""
	}

	containerEnv := parseEnvLines(buf.String())
	resolved, err := config.SubstituteContainerEnv(containerEnv, cfg)
	if err != nil {
		e.logger.Warn("failed to resolve remoteEnv container variables", "error", err)
		return cfg.RemoteEnv, containerEnv["PATH"]
	}

	resolvedEnv := resolved.RemoteEnv

	// Resolve bare ${VAR} references (e.g. ${PATH}) as container env lookups.
	// Many devcontainer.json files use ${PATH} instead of ${containerEnv:PATH}
	// in remoteEnv. Since exec -e doesn't do shell expansion, we must resolve
	// these ourselves.
	resolveBareVarRefs(resolvedEnv, containerEnv)

	return resolvedEnv, containerEnv["PATH"]
}

// bareVarRe matches ${VARNAME} where VARNAME contains no colons (i.e. not
// a namespaced reference like ${containerEnv:PATH} or ${localEnv:HOME}).
var bareVarRe = regexp.MustCompile(`\$\{([^:}]+)\}`)

// resolveBareVarRefs replaces bare ${VAR} references in env values with the
// corresponding value from containerEnv. This handles the common pattern of
// writing ${PATH} in remoteEnv instead of ${containerEnv:PATH}.
func resolveBareVarRefs(env map[string]string, containerEnv map[string]string) {
	for k, v := range env {
		env[k] = bareVarRe.ReplaceAllStringFunc(v, func(match string) string {
			name := match[2 : len(match)-1]
			if val, ok := containerEnv[name]; ok {
				return val
			}
			return match
		})
	}
}

// syncRemoteUserUID synchronizes the container user's UID/GID with the host user.
// This prevents permission mismatches on bind mounts, especially with rootless Podman.
// Returns true when UIDs are confirmed to be in sync (already matched or successfully synced),
// meaning chownWorkspace is not needed. Returns false when sync was skipped or failed.
func (e *Engine) syncRemoteUserUID(ctx context.Context, cc containerContext, cfg *config.DevContainerConfig) (bool, error) {
	// Guard: skip if explicitly disabled.
	if cfg.UpdateRemoteUserUID != nil && !*cfg.UpdateRemoteUserUID {
		return false, nil
	}

	// Guard: only on Linux.
	if runtime.GOOS != "linux" {
		return false, nil
	}

	// Guard: skip if remoteUser is empty or root.
	if cc.remoteUser == "" || cc.remoteUser == "root" {
		return false, nil
	}

	// Get host UID/GID.
	hostUID := os.Getuid()
	hostGID := os.Getgid()

	// Probe image user's current UID/GID.
	imageUID, err := e.execGetUserID(ctx, cc, "u")
	if err != nil {
		e.logger.Warn("failed to probe container user UID", "user", cc.remoteUser, "error", err)
		return false, nil // Non-fatal.
	}

	imageGID, err := e.execGetUserID(ctx, cc, "g")
	if err != nil {
		e.logger.Warn("failed to probe container user GID", "user", cc.remoteUser, "error", err)
		return false, nil // Non-fatal.
	}

	// If UIDs and GIDs already match, no sync needed and chown is unnecessary.
	if imageUID == hostUID && imageGID == hostGID {
		return true, nil
	}

	// Get the user's primary group name.
	groupName, err := e.execGetGroupName(ctx, cc)
	if err != nil {
		e.logger.Warn("failed to get user group name", "user", cc.remoteUser, "error", err)
		return false, nil // Non-fatal.
	}

	syncOK := true

	// Sync GID first if it differs.
	if imageGID != hostGID {
		// If the target GID is already in use by a different group, move that group out of the
		// way first. This happens on images like ubuntu:24.04 where standard groups may occupy
		// common GIDs (e.g., the "ubuntu" group at GID 1000).
		if conflict, err := e.execFindGroupByGID(ctx, cc, hostGID); err == nil && conflict != "" && conflict != groupName {
			if freeGID, err := e.execFindFreeGID(ctx, cc); err == nil {
				moveCmd := []string{"groupmod", "-g", strconv.Itoa(freeGID), conflict}
				var moveStderr bytes.Buffer
				if err := e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID, moveCmd, nil, io.Discard, &moveStderr, nil, "root"); err != nil {
					e.logger.Warn("failed to move conflicting group", "group", conflict, "error", err, "stderr", moveStderr.String())
				}
			}
		}

		cmd := []string{"groupmod", "-g", strconv.Itoa(hostGID), groupName}
		var stderr bytes.Buffer
		if err := e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID, cmd, nil, io.Discard, &stderr, nil, "root"); err != nil {
			e.logger.Warn("failed to sync group GID", "group", groupName, "gid", hostGID, "error", err, "stderr", stderr.String())
			syncOK = false
		}
	}

	// Sync UID if it differs.
	if imageUID != hostUID {
		// If the target UID is already in use by a different user, move that user out of the
		// way first. This happens on images like ubuntu:24.04 where the "ubuntu" user occupies
		// UID 1000, preventing usermod from assigning the same UID to the dev user.
		if conflict, err := e.execFindUserByUID(ctx, cc, hostUID); err == nil && conflict != "" && conflict != cc.remoteUser {
			if freeUID, err := e.execFindFreeUID(ctx, cc); err == nil {
				moveCmd := []string{"usermod", "-u", strconv.Itoa(freeUID), conflict}
				var moveStderr bytes.Buffer
				if err := e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID, moveCmd, nil, io.Discard, &moveStderr, nil, "root"); err != nil {
					e.logger.Warn("failed to move conflicting user", "user", conflict, "error", err, "stderr", moveStderr.String())
				}
			}
		}

		cmd := []string{"usermod", "-u", strconv.Itoa(hostUID), cc.remoteUser}
		var stderr bytes.Buffer
		if err := e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID, cmd, nil, io.Discard, &stderr, nil, "root"); err != nil {
			e.logger.Warn("failed to sync user UID", "user", cc.remoteUser, "uid", hostUID, "error", err, "stderr", stderr.String())
			syncOK = false
		}
	}

	// Re-own files under the home directory that changed.
	homeDir := "/home/" + cc.remoteUser
	if imageUID != hostUID {
		cmd := []string{"find", homeDir, "-user", strconv.Itoa(imageUID), "-exec", "chown", "-h", strconv.Itoa(hostUID), "{}", "+"}
		var stderr bytes.Buffer
		if err := e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID, cmd, nil, io.Discard, &stderr, nil, "root"); err != nil {
			e.logger.Warn("failed to re-own files after UID sync", "user", cc.remoteUser, "dir", homeDir, "error", err, "stderr", stderr.String())
		}
	}

	if imageGID != hostGID {
		cmd := []string{"find", homeDir, "-group", strconv.Itoa(imageGID), "-exec", "chgrp", "-h", strconv.Itoa(hostGID), "{}", "+"}
		var stderr bytes.Buffer
		if err := e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID, cmd, nil, io.Discard, &stderr, nil, "root"); err != nil {
			e.logger.Warn("failed to re-own files after GID sync", "user", cc.remoteUser, "dir", homeDir, "error", err, "stderr", stderr.String())
		}
	}

	return syncOK, nil
}

// execGetUserID runs `id -<flag> <user>` and returns the numeric ID.
func (e *Engine) execGetUserID(ctx context.Context, cc containerContext, flag string) (int, error) {
	cmd := []string{"id", "-" + flag, cc.remoteUser}
	var stdout, stderr bytes.Buffer
	if err := e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID, cmd, nil, &stdout, &stderr, nil, ""); err != nil {
		return 0, fmt.Errorf("id -%s %s: %w: %s", flag, cc.remoteUser, err, stderr.String())
	}

	idStr := strings.TrimSpace(stdout.String())
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return 0, fmt.Errorf("parsing id output: %w", err)
	}
	return id, nil
}

// execGetGroupName runs `id -gn <user>` and returns the primary group name.
func (e *Engine) execGetGroupName(ctx context.Context, cc containerContext) (string, error) {
	cmd := []string{"id", "-gn", cc.remoteUser}
	var stdout, stderr bytes.Buffer
	if err := e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID, cmd, nil, &stdout, &stderr, nil, ""); err != nil {
		return "", fmt.Errorf("id -gn %s: %w: %s", cc.remoteUser, err, stderr.String())
	}

	groupName := strings.TrimSpace(stdout.String())
	if groupName == "" {
		return "", fmt.Errorf("empty group name from id -gn")
	}
	return groupName, nil
}

// execFindUserByUID returns the username that owns the given UID, or "" if none.
func (e *Engine) execFindUserByUID(ctx context.Context, cc containerContext, uid int) (string, error) {
	cmd := []string{"getent", "passwd", strconv.Itoa(uid)}
	var stdout bytes.Buffer
	if err := e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID, cmd, nil, &stdout, io.Discard, nil, ""); err != nil {
		return "", nil // getent exits non-zero when not found; treat as "not found"
	}
	// Output: "username:x:uid:gid:comment:home:shell"
	name := strings.SplitN(strings.TrimSpace(stdout.String()), ":", 2)[0]
	return name, nil
}

// execFindGroupByGID returns the group name that owns the given GID, or "" if none.
func (e *Engine) execFindGroupByGID(ctx context.Context, cc containerContext, gid int) (string, error) {
	cmd := []string{"getent", "group", strconv.Itoa(gid)}
	var stdout bytes.Buffer
	if err := e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID, cmd, nil, &stdout, io.Discard, nil, ""); err != nil {
		return "", nil // getent exits non-zero when not found; treat as "not found"
	}
	// Output: "groupname:x:gid:members"
	name := strings.SplitN(strings.TrimSpace(stdout.String()), ":", 2)[0]
	return name, nil
}

// execFindFreeUID returns the lowest unused UID above all current UIDs in /etc/passwd.
func (e *Engine) execFindFreeUID(ctx context.Context, cc containerContext) (int, error) {
	cmd := []string{"awk", "-F:", "BEGIN{max=0}{if($3+0>max)max=$3}END{print max+1}", "/etc/passwd"}
	var stdout, stderr bytes.Buffer
	if err := e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID, cmd, nil, &stdout, &stderr, nil, ""); err != nil {
		return 0, fmt.Errorf("awk /etc/passwd: %w: %s", err, stderr.String())
	}
	return strconv.Atoi(strings.TrimSpace(stdout.String()))
}

// execFindFreeGID returns the lowest unused GID above all current GIDs in /etc/group.
func (e *Engine) execFindFreeGID(ctx context.Context, cc containerContext) (int, error) {
	cmd := []string{"awk", "-F:", "BEGIN{max=0}{if($3+0>max)max=$3}END{print max+1}", "/etc/group"}
	var stdout, stderr bytes.Buffer
	if err := e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID, cmd, nil, &stdout, &stderr, nil, ""); err != nil {
		return 0, fmt.Errorf("awk /etc/group: %w: %s", err, stderr.String())
	}
	return strconv.Atoi(strings.TrimSpace(stdout.String()))
}

// chownWorkspace changes ownership of the workspace folder to the remote user.
// The trailing colon preserves the existing group ownership.
func (e *Engine) chownWorkspace(ctx context.Context, cc containerContext) error {
	cmd := []string{"chown", "-R", cc.remoteUser + ":", cc.workspaceFolder}
	var stderr bytes.Buffer
	if err := e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID, cmd, nil, io.Discard, &stderr, nil, "root"); err != nil {
		return fmt.Errorf("chowning workspace: %w: %s", err, stderr.String())
	}
	return nil
}

// probeContainerPATH returns the container's base PATH without shell
// interpretation. This captures PATH entries set by the Docker image (ENV
// directive) before a login shell's /etc/profile can reset them.
func (e *Engine) probeContainerPATH(ctx context.Context, cc containerContext) string {
	var stdout bytes.Buffer
	if err := e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID, []string{"printenv", "PATH"}, nil, &stdout, io.Discard, nil, ""); err != nil {
		e.logger.Debug("failed to probe container PATH", "error", err)
		return ""
	}
	return strings.TrimSpace(stdout.String())
}

// probeUserEnv probes the container user's environment using the shell type
// specified by userEnvProbe. Returns the probed environment variables, or nil
// if probing is skipped or fails.
func (e *Engine) probeUserEnv(ctx context.Context, cc containerContext, userEnvProbe string) map[string]string {
	probe := userEnvProbe
	if probe == "" {
		probe = "loginInteractiveShell" // spec default
	}
	if probe == "none" {
		return nil
	}

	shell := e.detectUserShell(ctx, cc)

	var shellArgs []string
	switch probe {
	case "loginShell":
		shellArgs = []string{shell, "-l", "-c", "env"}
	case "interactiveShell":
		shellArgs = []string{shell, "-i", "-c", "env"}
	case "loginInteractiveShell":
		shellArgs = []string{shell, "-l", "-i", "-c", "env"}
	default:
		e.logger.Warn("unknown userEnvProbe value, using loginInteractiveShell", "value", probe)
		shellArgs = []string{shell, "-l", "-i", "-c", "env"}
	}

	e.logger.Debug("probing user environment", "probe", probe, "shell", shell)

	var stdout bytes.Buffer
	if err := e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID, shellArgs, nil, &stdout, io.Discard, nil, cc.remoteUser); err != nil {
		e.logger.Warn("userEnvProbe failed", "probe", probe, "shell", shell, "error", err)
		return nil
	}

	return parseEnvLines(stdout.String())
}

// detectUserShell determines the remote user's login shell by parsing
// the output of getent passwd. Falls back to common shells if detection fails.
func (e *Engine) detectUserShell(ctx context.Context, cc containerContext) string {
	var stdout bytes.Buffer
	cmd := []string{"getent", "passwd", cc.remoteUser}
	if err := e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID, cmd, nil, &stdout, io.Discard, nil, ""); err != nil {
		e.logger.Debug("getent passwd failed, trying shell fallbacks", "user", cc.remoteUser, "error", err)
		return e.detectShellFallback(ctx, cc)
	}

	// Format: username:x:uid:gid:comment:home:shell
	parts := strings.Split(strings.TrimSpace(stdout.String()), ":")
	if len(parts) >= 7 && parts[6] != "" {
		shell := parts[6]
		if err := e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID, []string{"test", "-x", shell}, nil, io.Discard, io.Discard, nil, ""); err == nil {
			return shell
		}
		e.logger.Debug("user shell not executable, trying fallbacks", "shell", shell)
	}

	return e.detectShellFallback(ctx, cc)
}

// detectShellFallback tries common shells in preference order.
func (e *Engine) detectShellFallback(ctx context.Context, cc containerContext) string {
	for _, shell := range []string{"/bin/bash", "/bin/sh"} {
		if err := e.driver.ExecContainer(ctx, cc.workspaceID, cc.containerID, []string{"test", "-x", shell}, nil, io.Discard, io.Discard, nil, ""); err == nil {
			return shell
		}
	}
	return "/bin/sh"
}
