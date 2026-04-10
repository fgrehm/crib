// Package plugintest provides shared test helpers for plugin tests.
package plugintest

import "github.com/fgrehm/crib/internal/plugin"

// TestReq returns a PreContainerRunRequest with common test defaults.
func TestReq(workspaceDir, remoteUser string) *plugin.PreContainerRunRequest {
	return &plugin.PreContainerRunRequest{
		WorkspaceID:     "test-ws",
		WorkspaceDir:    workspaceDir,
		SourceDir:       "/home/user/project",
		Runtime:         "docker",
		ImageName:       "ubuntu:22.04",
		RemoteUser:      remoteUser,
		WorkspaceFolder: "/workspaces/project",
		ContainerName:   "crib-test-ws",
	}
}
