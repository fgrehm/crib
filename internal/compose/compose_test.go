package compose

import (
	"encoding/json"
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
