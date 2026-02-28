package engine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
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
func (e *Engine) setupContainer(ctx context.Context, ws *workspace.Workspace, cfg *config.DevContainerConfig, containerID, workspaceFolder, remoteUser string) error {
	// Resolve ${containerEnv:VAR} in remoteEnv by probing the container environment.
	if len(cfg.RemoteEnv) > 0 {
		e.resolveRemoteEnv(ctx, ws.ID, containerID, cfg)
	}

	// Sync container user UID/GID with host before chowning.
	// uidsSynced is true when UIDs are confirmed to match (either already did, or were synced),
	// meaning chownWorkspace is not needed for bind mounts (rootless podman limitation).
	uidsSynced := false
	if remoteUser != "" && remoteUser != "root" {
		var err error
		uidsSynced, err = e.syncRemoteUserUID(ctx, ws.ID, containerID, remoteUser, cfg)
		if err != nil {
			e.logger.Warn("failed to sync remote user UID/GID", "error", err)
		}
	}

	// Chown workspace directory to remote user, unless UIDs are already in sync.
	// When UIDs match, bind-mount files are already accessible and chown would fail
	// on rootless Podman (no CAP_CHOWN over bind-mounted files).
	if remoteUser != "" && remoteUser != "root" && !uidsSynced {
		if err := e.chownWorkspace(ctx, ws.ID, containerID, workspaceFolder, remoteUser); err != nil {
			e.logger.Warn("failed to chown workspace", "error", err)
		}
	}

	// Save the original config remoteEnv before merging with probed env.
	// We need this to re-merge after hooks, since hooks may install tools
	// that change PATH and other vars.
	configRemoteEnv := copyStringMap(cfg.RemoteEnv)

	// Pre-hook environment probe: captures PATH and other vars from shell
	// profile files (e.g. mise, rbenv, nvm) so lifecycle hooks have the
	// user's full environment.
	probedEnv := e.probeUserEnv(ctx, ws.ID, containerID, remoteUser, cfg.UserEnvProbe)
	if hookEnv := mergeEnv(probedEnv, configRemoteEnv); len(hookEnv) > 0 {
		cfg.RemoteEnv = hookEnv
	}

	// Run lifecycle hooks.
	runner := &lifecycleRunner{
		driver:      e.driver,
		store:       e.store,
		workspaceID: ws.ID,
		containerID: containerID,
		remoteUser:  remoteUser,
		remoteEnv:   cfg.RemoteEnv,
		logger:      e.logger,
		stdout:      e.stdout,
		stderr:      e.stderr,
		progress:    e.progress,
		verbose:     e.verbose,
	}

	hookErr := runner.runLifecycleHooks(ctx, cfg, workspaceFolder)

	// Post-hook environment probe: re-captures the environment to pick up
	// any changes from lifecycle hooks (e.g. tools installed via mise, nvm).
	// This is what gets persisted for crib shell/exec.
	postProbe := e.probeUserEnv(ctx, ws.ID, containerID, remoteUser, cfg.UserEnvProbe)
	if finalEnv := mergeEnv(postProbe, configRemoteEnv); len(finalEnv) > 0 {
		cfg.RemoteEnv = finalEnv
	}

	return hookErr
}

// resolveRemoteEnv resolves ${containerEnv:VAR} references in cfg.RemoteEnv by
// probing the container's runtime environment. Updates cfg.RemoteEnv in place.
// Per the devcontainer spec, remoteEnv is injected by the tool (not written to
// /etc/environment) and ${containerEnv:VAR} is only valid in remoteEnv.
func (e *Engine) resolveRemoteEnv(ctx context.Context, workspaceID, containerID string, cfg *config.DevContainerConfig) {
	var buf bytes.Buffer
	if err := e.driver.ExecContainer(ctx, workspaceID, containerID, []string{"env"}, nil, &buf, io.Discard, nil, ""); err != nil {
		e.logger.Warn("failed to probe container environment for remoteEnv resolution", "error", err)
		return
	}

	containerEnv := parseEnvLines(buf.String())
	resolved, err := config.SubstituteContainerEnv(containerEnv, cfg)
	if err != nil {
		e.logger.Warn("failed to resolve remoteEnv container variables", "error", err)
		return
	}
	cfg.RemoteEnv = resolved.RemoteEnv
}

// syncRemoteUserUID synchronizes the container user's UID/GID with the host user.
// This prevents permission mismatches on bind mounts, especially with rootless Podman.
// Returns true when UIDs are confirmed to be in sync (already matched or successfully synced),
// meaning chownWorkspace is not needed. Returns false when sync was skipped or failed.
func (e *Engine) syncRemoteUserUID(ctx context.Context, workspaceID, containerID, remoteUser string, cfg *config.DevContainerConfig) (bool, error) {
	// Guard: skip if explicitly disabled.
	if cfg.UpdateRemoteUserUID != nil && !*cfg.UpdateRemoteUserUID {
		return false, nil
	}

	// Guard: only on Linux.
	if runtime.GOOS != "linux" {
		return false, nil
	}

	// Guard: skip if remoteUser is empty or root.
	if remoteUser == "" || remoteUser == "root" {
		return false, nil
	}

	// Get host UID/GID.
	hostUID := os.Getuid()
	hostGID := os.Getgid()

	// Probe image user's current UID/GID.
	imageUID, err := e.execGetUserID(ctx, workspaceID, containerID, remoteUser, "u")
	if err != nil {
		e.logger.Warn("failed to probe container user UID", "user", remoteUser, "error", err)
		return false, nil // Non-fatal.
	}

	imageGID, err := e.execGetUserID(ctx, workspaceID, containerID, remoteUser, "g")
	if err != nil {
		e.logger.Warn("failed to probe container user GID", "user", remoteUser, "error", err)
		return false, nil // Non-fatal.
	}

	// If UIDs and GIDs already match, no sync needed and chown is unnecessary.
	if imageUID == hostUID && imageGID == hostGID {
		return true, nil
	}

	// Get the user's primary group name.
	groupName, err := e.execGetGroupName(ctx, workspaceID, containerID, remoteUser)
	if err != nil {
		e.logger.Warn("failed to get user group name", "user", remoteUser, "error", err)
		return false, nil // Non-fatal.
	}

	syncOK := true

	// Sync GID first if it differs.
	if imageGID != hostGID {
		// If the target GID is already in use by a different group, move that group out of the
		// way first. This happens on images like ubuntu:24.04 where standard groups may occupy
		// common GIDs (e.g., the "ubuntu" group at GID 1000).
		if conflict, err := e.execFindGroupByGID(ctx, workspaceID, containerID, hostGID); err == nil && conflict != "" && conflict != groupName {
			if freeGID, err := e.execFindFreeGID(ctx, workspaceID, containerID); err == nil {
				moveCmd := []string{"groupmod", "-g", strconv.Itoa(freeGID), conflict}
				var moveStderr bytes.Buffer
				if err := e.driver.ExecContainer(ctx, workspaceID, containerID, moveCmd, nil, io.Discard, &moveStderr, nil, "root"); err != nil {
					e.logger.Warn("failed to move conflicting group", "group", conflict, "error", err, "stderr", moveStderr.String())
				}
			}
		}

		cmd := []string{"groupmod", "-g", strconv.Itoa(hostGID), groupName}
		var stderr bytes.Buffer
		if err := e.driver.ExecContainer(ctx, workspaceID, containerID, cmd, nil, io.Discard, &stderr, nil, "root"); err != nil {
			e.logger.Warn("failed to sync group GID", "group", groupName, "gid", hostGID, "error", err, "stderr", stderr.String())
			syncOK = false
		}
	}

	// Sync UID if it differs.
	if imageUID != hostUID {
		// If the target UID is already in use by a different user, move that user out of the
		// way first. This happens on images like ubuntu:24.04 where the "ubuntu" user occupies
		// UID 1000, preventing usermod from assigning the same UID to the dev user.
		if conflict, err := e.execFindUserByUID(ctx, workspaceID, containerID, hostUID); err == nil && conflict != "" && conflict != remoteUser {
			if freeUID, err := e.execFindFreeUID(ctx, workspaceID, containerID); err == nil {
				moveCmd := []string{"usermod", "-u", strconv.Itoa(freeUID), conflict}
				var moveStderr bytes.Buffer
				if err := e.driver.ExecContainer(ctx, workspaceID, containerID, moveCmd, nil, io.Discard, &moveStderr, nil, "root"); err != nil {
					e.logger.Warn("failed to move conflicting user", "user", conflict, "error", err, "stderr", moveStderr.String())
				}
			}
		}

		cmd := []string{"usermod", "-u", strconv.Itoa(hostUID), remoteUser}
		var stderr bytes.Buffer
		if err := e.driver.ExecContainer(ctx, workspaceID, containerID, cmd, nil, io.Discard, &stderr, nil, "root"); err != nil {
			e.logger.Warn("failed to sync user UID", "user", remoteUser, "uid", hostUID, "error", err, "stderr", stderr.String())
			syncOK = false
		}
	}

	// Re-own files under the home directory that changed.
	homeDir := "/home/" + remoteUser
	if imageUID != hostUID {
		cmd := []string{"find", homeDir, "-user", strconv.Itoa(imageUID), "-exec", "chown", "-h", strconv.Itoa(hostUID), "{}", "+"}
		var stderr bytes.Buffer
		if err := e.driver.ExecContainer(ctx, workspaceID, containerID, cmd, nil, io.Discard, &stderr, nil, "root"); err != nil {
			e.logger.Warn("failed to re-own files after UID sync", "user", remoteUser, "dir", homeDir, "error", err, "stderr", stderr.String())
		}
	}

	if imageGID != hostGID {
		cmd := []string{"find", homeDir, "-group", strconv.Itoa(imageGID), "-exec", "chgrp", "-h", strconv.Itoa(hostGID), "{}", "+"}
		var stderr bytes.Buffer
		if err := e.driver.ExecContainer(ctx, workspaceID, containerID, cmd, nil, io.Discard, &stderr, nil, "root"); err != nil {
			e.logger.Warn("failed to re-own files after GID sync", "user", remoteUser, "dir", homeDir, "error", err, "stderr", stderr.String())
		}
	}

	return syncOK, nil
}

// execGetUserID runs `id -<flag> <user>` and returns the numeric ID.
func (e *Engine) execGetUserID(ctx context.Context, workspaceID, containerID, remoteUser, flag string) (int, error) {
	cmd := []string{"id", "-" + flag, remoteUser}
	var stdout, stderr bytes.Buffer
	if err := e.driver.ExecContainer(ctx, workspaceID, containerID, cmd, nil, &stdout, &stderr, nil, ""); err != nil {
		return 0, fmt.Errorf("id -%s %s: %w: %s", flag, remoteUser, err, stderr.String())
	}

	idStr := strings.TrimSpace(stdout.String())
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return 0, fmt.Errorf("parsing id output: %w", err)
	}
	return id, nil
}

// execGetGroupName runs `id -gn <user>` and returns the primary group name.
func (e *Engine) execGetGroupName(ctx context.Context, workspaceID, containerID, remoteUser string) (string, error) {
	cmd := []string{"id", "-gn", remoteUser}
	var stdout, stderr bytes.Buffer
	if err := e.driver.ExecContainer(ctx, workspaceID, containerID, cmd, nil, &stdout, &stderr, nil, ""); err != nil {
		return "", fmt.Errorf("id -gn %s: %w: %s", remoteUser, err, stderr.String())
	}

	groupName := strings.TrimSpace(stdout.String())
	if groupName == "" {
		return "", fmt.Errorf("empty group name from id -gn")
	}
	return groupName, nil
}

// execFindUserByUID returns the username that owns the given UID, or "" if none.
func (e *Engine) execFindUserByUID(ctx context.Context, workspaceID, containerID string, uid int) (string, error) {
	cmd := []string{"getent", "passwd", strconv.Itoa(uid)}
	var stdout bytes.Buffer
	if err := e.driver.ExecContainer(ctx, workspaceID, containerID, cmd, nil, &stdout, io.Discard, nil, ""); err != nil {
		return "", nil // getent exits non-zero when not found; treat as "not found"
	}
	// Output: "username:x:uid:gid:comment:home:shell"
	name := strings.SplitN(strings.TrimSpace(stdout.String()), ":", 2)[0]
	return name, nil
}

// execFindGroupByGID returns the group name that owns the given GID, or "" if none.
func (e *Engine) execFindGroupByGID(ctx context.Context, workspaceID, containerID string, gid int) (string, error) {
	cmd := []string{"getent", "group", strconv.Itoa(gid)}
	var stdout bytes.Buffer
	if err := e.driver.ExecContainer(ctx, workspaceID, containerID, cmd, nil, &stdout, io.Discard, nil, ""); err != nil {
		return "", nil // getent exits non-zero when not found; treat as "not found"
	}
	// Output: "groupname:x:gid:members"
	name := strings.SplitN(strings.TrimSpace(stdout.String()), ":", 2)[0]
	return name, nil
}

// execFindFreeUID returns the lowest unused UID above all current UIDs in /etc/passwd.
func (e *Engine) execFindFreeUID(ctx context.Context, workspaceID, containerID string) (int, error) {
	cmd := []string{"awk", "-F:", "BEGIN{max=0}{if($3+0>max)max=$3}END{print max+1}", "/etc/passwd"}
	var stdout, stderr bytes.Buffer
	if err := e.driver.ExecContainer(ctx, workspaceID, containerID, cmd, nil, &stdout, &stderr, nil, ""); err != nil {
		return 0, fmt.Errorf("awk /etc/passwd: %w: %s", err, stderr.String())
	}
	return strconv.Atoi(strings.TrimSpace(stdout.String()))
}

// execFindFreeGID returns the lowest unused GID above all current GIDs in /etc/group.
func (e *Engine) execFindFreeGID(ctx context.Context, workspaceID, containerID string) (int, error) {
	cmd := []string{"awk", "-F:", "BEGIN{max=0}{if($3+0>max)max=$3}END{print max+1}", "/etc/group"}
	var stdout, stderr bytes.Buffer
	if err := e.driver.ExecContainer(ctx, workspaceID, containerID, cmd, nil, &stdout, &stderr, nil, ""); err != nil {
		return 0, fmt.Errorf("awk /etc/group: %w: %s", err, stderr.String())
	}
	return strconv.Atoi(strings.TrimSpace(stdout.String()))
}

// chownWorkspace changes ownership of the workspace folder to the remote user.
// The trailing colon preserves the existing group ownership.
func (e *Engine) chownWorkspace(ctx context.Context, workspaceID, containerID, workspaceFolder, remoteUser string) error {
	cmd := []string{"chown", "-R", remoteUser + ":", workspaceFolder}
	var stderr bytes.Buffer
	if err := e.driver.ExecContainer(ctx, workspaceID, containerID, cmd, nil, io.Discard, &stderr, nil, "root"); err != nil {
		return fmt.Errorf("chowning workspace: %w: %s", err, stderr.String())
	}
	return nil
}

// probeUserEnv probes the container user's environment using the shell type
// specified by userEnvProbe. Returns the probed environment variables, or nil
// if probing is skipped or fails.
func (e *Engine) probeUserEnv(ctx context.Context, workspaceID, containerID, remoteUser, userEnvProbe string) map[string]string {
	probe := userEnvProbe
	if probe == "" {
		probe = "loginInteractiveShell" // spec default
	}
	if probe == "none" {
		return nil
	}

	shell := e.detectUserShell(ctx, workspaceID, containerID, remoteUser)

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
	if err := e.driver.ExecContainer(ctx, workspaceID, containerID, shellArgs, nil, &stdout, io.Discard, nil, remoteUser); err != nil {
		e.logger.Warn("userEnvProbe failed", "probe", probe, "shell", shell, "error", err)
		return nil
	}

	return parseEnvLines(stdout.String())
}

// detectUserShell determines the remote user's login shell by parsing
// the output of getent passwd. Falls back to common shells if detection fails.
func (e *Engine) detectUserShell(ctx context.Context, workspaceID, containerID, remoteUser string) string {
	var stdout bytes.Buffer
	cmd := []string{"getent", "passwd", remoteUser}
	if err := e.driver.ExecContainer(ctx, workspaceID, containerID, cmd, nil, &stdout, io.Discard, nil, ""); err != nil {
		e.logger.Debug("getent passwd failed, trying shell fallbacks", "user", remoteUser, "error", err)
		return e.detectShellFallback(ctx, workspaceID, containerID)
	}

	// Format: username:x:uid:gid:comment:home:shell
	parts := strings.Split(strings.TrimSpace(stdout.String()), ":")
	if len(parts) >= 7 && parts[6] != "" {
		shell := parts[6]
		if err := e.driver.ExecContainer(ctx, workspaceID, containerID, []string{"test", "-x", shell}, nil, io.Discard, io.Discard, nil, ""); err == nil {
			return shell
		}
		e.logger.Debug("user shell not executable, trying fallbacks", "shell", shell)
	}

	return e.detectShellFallback(ctx, workspaceID, containerID)
}

// detectShellFallback tries common shells in preference order.
func (e *Engine) detectShellFallback(ctx context.Context, workspaceID, containerID string) string {
	for _, shell := range []string{"/bin/bash", "/bin/sh"} {
		if err := e.driver.ExecContainer(ctx, workspaceID, containerID, []string{"test", "-x", shell}, nil, io.Discard, io.Discard, nil, ""); err == nil {
			return shell
		}
	}
	return "/bin/sh"
}
