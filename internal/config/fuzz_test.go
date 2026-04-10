package config

import "testing"

func FuzzParseBytes(f *testing.F) {
	f.Add([]byte(`{"image": "ubuntu:22.04"}`))
	f.Add([]byte(`{"dockerFile": "Dockerfile", "context": "."}`))
	f.Add([]byte(`{"dockerComposeFile": "docker-compose.yml", "service": "app"}`))
	f.Add([]byte(`// comment
{"image": "node:20", "remoteUser": "node"}`))
	f.Add([]byte(`{"image": "alpine", "mounts": ["type=bind,src=/tmp,dst=/tmp"]}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`not json at all`))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		// ParseBytes must not panic on any input.
		_, _ = ParseBytes(data)
	})
}

func FuzzSubstituteString(f *testing.F) {
	f.Add("${localWorkspaceFolder}/src")
	f.Add("${containerWorkspaceFolder}")
	f.Add("${localEnv:HOME}")
	f.Add("${localEnv:MISSING:default-value}")
	f.Add("${containerEnv:PATH}")
	f.Add("${devcontainerId}")
	f.Add("${localWorkspaceFolderBasename}")
	f.Add("no variables here")
	f.Add("${unknown}")
	f.Add("nested ${localEnv:${nope}}")
	f.Add("")

	ctx := &SubstitutionContext{
		DevContainerID:           "test-id",
		LocalWorkspaceFolder:     "/home/user/project",
		ContainerWorkspaceFolder: "/workspaces/project",
		Env:                      map[string]string{"HOME": "/home/user", "PATH": "/usr/bin"},
	}

	f.Fuzz(func(t *testing.T, s string) {
		// SubstituteString must not panic on any input.
		_ = SubstituteString(ctx, s)
	})
}

func FuzzParseMount(f *testing.F) {
	f.Add("type=bind,src=/tmp,dst=/tmp")
	f.Add("type=volume,source=mydata,target=/data")
	f.Add("type=bind,source=${localWorkspaceFolder},target=/workspace")
	f.Add("")
	f.Add("no-equals-here")
	f.Add("dst=/only-target")

	f.Fuzz(func(t *testing.T, s string) {
		// ParseMount must not panic on any input.
		_, _ = ParseMount(s)
	})
}
