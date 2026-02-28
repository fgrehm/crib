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

func TestFormatComposePorts_Empty(t *testing.T) {
	if got := formatComposePorts(nil); got != "" {
		t.Errorf("formatComposePorts(nil) = %q, want empty", got)
	}
}

func TestFormatComposePorts_Single(t *testing.T) {
	ports := []compose.PortBinding{
		{HostPort: 5432, ContainerPort: 5432, Protocol: "tcp"},
	}
	want := "5432->5432/tcp"
	if got := formatComposePorts(ports); got != want {
		t.Errorf("formatComposePorts = %q, want %q", got, want)
	}
}

func TestFormatComposePorts_DefaultProtocol(t *testing.T) {
	ports := []compose.PortBinding{
		{HostPort: 3000, ContainerPort: 3000},
	}
	want := "3000->3000/tcp"
	if got := formatComposePorts(ports); got != want {
		t.Errorf("formatComposePorts = %q, want %q", got, want)
	}
}
