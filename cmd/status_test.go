package cmd

import (
	"testing"

	"github.com/fgrehm/crib/internal/compose"
	"github.com/fgrehm/crib/internal/driver"
)

func TestFormatPorts_Empty(t *testing.T) {
	if got := formatPorts(nil); got != "" {
		t.Errorf("formatPorts(nil) = %q, want empty", got)
	}
}

func TestFormatPorts_Single(t *testing.T) {
	ports := []driver.PortBinding{
		{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"},
	}
	want := "8080->80/tcp"
	if got := formatPorts(ports); got != want {
		t.Errorf("formatPorts = %q, want %q", got, want)
	}
}

func TestFormatPorts_Multiple_Sorted(t *testing.T) {
	ports := []driver.PortBinding{
		{HostPort: 9090, ContainerPort: 3000, Protocol: "tcp"},
		{HostPort: 8080, ContainerPort: 8080, Protocol: "tcp"},
	}
	want := "8080->8080/tcp, 9090->3000/tcp"
	if got := formatPorts(ports); got != want {
		t.Errorf("formatPorts = %q, want %q", got, want)
	}
}

func TestFormatPorts_DefaultProtocol(t *testing.T) {
	ports := []driver.PortBinding{
		{HostPort: 3000, ContainerPort: 3000},
	}
	want := "3000->3000/tcp"
	if got := formatPorts(ports); got != want {
		t.Errorf("formatPorts = %q, want %q", got, want)
	}
}

func TestComposePortsToDriver(t *testing.T) {
	composePorts := []compose.PortBinding{
		{ContainerPort: 5432, HostPort: 5432, HostIP: "0.0.0.0", Protocol: "tcp"},
		{ContainerPort: 6379, HostPort: 6379, Protocol: "tcp"},
	}
	got := composePortsToDriver(composePorts)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ContainerPort != 5432 || got[0].HostPort != 5432 || got[0].HostIP != "0.0.0.0" {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].ContainerPort != 6379 || got[1].HostPort != 6379 {
		t.Errorf("got[1] = %+v", got[1])
	}
}
