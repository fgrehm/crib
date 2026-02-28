package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/fgrehm/crib/internal/driver"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:     "status",
	Aliases: []string{"ps"},
	Short:   "Show the status of the current workspace container",
	RunE: func(cmd *cobra.Command, args []string) error {
		u := newUI()

		eng, _, store, err := newEngine()
		if err != nil {
			return err
		}

		ws, err := currentWorkspace(store, false)
		if err != nil {
			return err
		}

		result, err := eng.Status(cmd.Context(), ws)
		if err != nil {
			return err
		}

		u.Dim(versionString())
		u.Header(ws.ID)
		fmt.Printf("%-12s%s\n", "source", ws.Source)

		if result.Container == nil {
			fmt.Printf("%-12s%s\n", "status", u.StatusColor("no container"))
			return nil
		}

		fmt.Printf("%-12s%s\n", "container", shortID(result.Container.ID))
		fmt.Printf("%-12s%s\n", "status", u.StatusColor(result.Container.State.Status))

		if ports := formatPorts(result.Container.Ports); ports != "" {
			fmt.Printf("%-12s%s\n", "ports", ports)
		}

		if len(result.Services) > 0 {
			fmt.Println("services")
			for _, svc := range result.Services {
				state := u.StatusColor(svc.State)
				if ports := formatPorts(composePortsToDriver(svc.Ports)); ports != "" {
					state += "  " + ports
				}
				u.Keyval(svc.Service, state)
			}
		}

		return nil
	},
}

// formatPorts formats port bindings into a compact display string.
// Example: "8080->8080/tcp, 9090->3000/tcp"
func formatPorts(ports []driver.PortBinding) string {
	if len(ports) == 0 {
		return ""
	}
	sorted := make([]driver.PortBinding, len(ports))
	copy(sorted, ports)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].HostPort != sorted[j].HostPort {
			return sorted[i].HostPort < sorted[j].HostPort
		}
		return sorted[i].ContainerPort < sorted[j].ContainerPort
	})
	parts := make([]string, len(sorted))
	for i, p := range sorted {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		parts[i] = fmt.Sprintf("%d->%d/%s", p.HostPort, p.ContainerPort, proto)
	}
	return strings.Join(parts, ", ")
}
