package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectName_Default(t *testing.T) {
	t.Setenv("COMPOSE_PROJECT_NAME", "")
	got := ProjectName("myproject")
	if got != "crib-myproject" {
		t.Errorf("ProjectName(%q) = %q, want %q", "myproject", got, "crib-myproject")
	}
}

func TestProjectName_EnvOverride(t *testing.T) {
	t.Setenv("COMPOSE_PROJECT_NAME", "custom-name")
	got := ProjectName("myproject")
	if got != "custom-name" {
		t.Errorf("ProjectName(%q) = %q, want %q", "myproject", got, "custom-name")
	}
}

func TestProjectArgs_NoFiles(t *testing.T) {
	args := projectArgs("myproj", nil)
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}
	if args[0] != "--project-name" || args[1] != "myproj" {
		t.Errorf("unexpected args: %v", args)
	}
}

func TestParseComposePsJSON_WithPublishers(t *testing.T) {
	// Simulates the JSON output from `docker compose ps --format json`.
	input := `[
		{
			"Labels": {"com.docker.compose.service": "web"},
			"State": "running",
			"Publishers": [
				{"URL": "0.0.0.0", "TargetPort": 80, "PublishedPort": 8080, "Protocol": "tcp"},
				{"URL": "0.0.0.0", "TargetPort": 443, "PublishedPort": 8443, "Protocol": "tcp"}
			]
		},
		{
			"Labels": {"com.docker.compose.service": "db"},
			"State": "running",
			"Publishers": [
				{"URL": "0.0.0.0", "TargetPort": 5432, "PublishedPort": 5432, "Protocol": "tcp"},
				{"URL": "", "TargetPort": 5432, "PublishedPort": 0, "Protocol": "tcp"}
			]
		}
	]`

	// Use the same parsing logic as ListServiceStatuses.
	var containers []struct {
		Labels     map[string]string `json:"Labels"`
		State      string            `json:"State"`
		Publishers []struct {
			URL           string `json:"URL"`
			TargetPort    int    `json:"TargetPort"`
			PublishedPort int    `json:"PublishedPort"`
			Protocol      string `json:"Protocol"`
		} `json:"Publishers"`
	}
	if err := json.Unmarshal([]byte(input), &containers); err != nil {
		t.Fatal(err)
	}

	var statuses []ServiceStatus
	for _, c := range containers {
		svc := c.Labels[ServiceLabel]
		if svc == "" {
			continue
		}
		ss := ServiceStatus{Service: svc, State: c.State}
		for _, p := range c.Publishers {
			if p.PublishedPort == 0 {
				continue
			}
			ss.Ports = append(ss.Ports, PortBinding{
				ContainerPort: p.TargetPort,
				HostPort:      p.PublishedPort,
				HostIP:        p.URL,
				Protocol:      p.Protocol,
			})
		}
		statuses = append(statuses, ss)
	}

	if len(statuses) != 2 {
		t.Fatalf("len = %d, want 2", len(statuses))
	}

	// Web service: 2 published ports.
	web := statuses[0]
	if web.Service != "web" || len(web.Ports) != 2 {
		t.Errorf("web = %+v", web)
	}
	if web.Ports[0].HostPort != 8080 || web.Ports[0].ContainerPort != 80 {
		t.Errorf("web.Ports[0] = %+v", web.Ports[0])
	}

	// DB service: 1 published port (PublishedPort=0 filtered out).
	db := statuses[1]
	if db.Service != "db" || len(db.Ports) != 1 {
		t.Errorf("db = %+v", db)
	}
	if db.Ports[0].HostPort != 5432 || db.Ports[0].ContainerPort != 5432 {
		t.Errorf("db.Ports[0] = %+v", db.Ports[0])
	}
}

func TestParseComposePsJSON_NullPublishers(t *testing.T) {
	input := `[{"Labels": {"com.docker.compose.service": "app"}, "State": "running", "Publishers": null}]`

	var containers []struct {
		Labels     map[string]string `json:"Labels"`
		State      string            `json:"State"`
		Publishers []struct {
			URL           string `json:"URL"`
			TargetPort    int    `json:"TargetPort"`
			PublishedPort int    `json:"PublishedPort"`
			Protocol      string `json:"Protocol"`
		} `json:"Publishers"`
	}
	if err := json.Unmarshal([]byte(input), &containers); err != nil {
		t.Fatal(err)
	}

	if len(containers) != 1 || len(containers[0].Publishers) != 0 {
		t.Errorf("expected 1 container with no publishers, got %+v", containers)
	}
}

func TestProjectArgs_WithFiles(t *testing.T) {
	args := projectArgs("myproj", []string{"a.yml", "b.yml"})
	expected := []string{"--project-name", "myproj", "-f", "a.yml", "-f", "b.yml"}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, want := range expected {
		if args[i] != want {
			t.Errorf("args[%d] = %q, want %q", i, args[i], want)
		}
	}
}

// fakeJSONHelper creates a Helper whose base command is a shell script that
// prints fixed JSON output (ignoring all arguments). This lets unit tests
// verify JSON parsing and service matching without a real container runtime.
func fakeJSONHelper(t *testing.T, jsonOutput string) *Helper {
	t.Helper()
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "fake-compose")
	// printf is used instead of echo to avoid shell interpretation of the JSON.
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s' '%s'\n", jsonOutput)
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return &Helper{
		baseCommand: "/bin/sh",
		argsPrefix:  []string{scriptPath},
		logger:      slog.Default(),
	}
}

func TestFindServiceContainerID_MatchesService(t *testing.T) {
	h := fakeJSONHelper(t, `[
		{"Id":"aaa111","Labels":{"com.docker.compose.service":"postgres"}},
		{"Id":"bbb222","Labels":{"com.docker.compose.service":"rails-app"}},
		{"Id":"ccc333","Labels":{"com.docker.compose.service":"chrome"}}
	]`)

	id, err := h.FindServiceContainerID(context.Background(), "myproj", nil, "rails-app", nil)
	if err != nil {
		t.Fatalf("FindServiceContainerID: %v", err)
	}
	if id != "bbb222" {
		t.Errorf("got %q, want %q", id, "bbb222")
	}
}

func TestFindServiceContainerID_NotFound(t *testing.T) {
	h := fakeJSONHelper(t, `[
		{"Id":"aaa111","Labels":{"com.docker.compose.service":"postgres"}}
	]`)

	id, err := h.FindServiceContainerID(context.Background(), "myproj", nil, "rails-app", nil)
	if err != nil {
		t.Fatalf("FindServiceContainerID: %v", err)
	}
	if id != "" {
		t.Errorf("expected empty string for missing service, got %q", id)
	}
}

func TestFindServiceContainerID_DockerUppercaseID(t *testing.T) {
	// Docker Compose uses "ID" (uppercase). Go's json.Unmarshal matches
	// case-insensitively, so our "Id" tag should match both.
	h := fakeJSONHelper(t, `[
		{"ID":"docker123","Labels":{"com.docker.compose.service":"web"}}
	]`)

	id, err := h.FindServiceContainerID(context.Background(), "myproj", nil, "web", nil)
	if err != nil {
		t.Fatalf("FindServiceContainerID: %v", err)
	}
	if id != "docker123" {
		t.Errorf("got %q, want %q", id, "docker123")
	}
}
