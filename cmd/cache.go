package cmd

import (
	"fmt"
	"strings"

	"github.com/fgrehm/crib/internal/driver/oci"
	"github.com/fgrehm/crib/internal/plugin/packagecache"
	"github.com/fgrehm/crib/internal/workspace"
	"github.com/spf13/cobra"
)

var cacheListAllFlag bool
var cacheCleanAllFlag bool

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage package cache volumes",
}

var cacheListCmd = &cobra.Command{
	Use:   "list",
	Short: "List package cache volumes",
	RunE: func(cmd *cobra.Command, args []string) error {
		u := newUI()

		d, err := oci.NewOCIDriver(logger)
		if err != nil {
			return err
		}

		var filter string
		if cacheListAllFlag {
			filter = packagecache.GlobalVolumePrefix
		} else {
			store, err := workspace.NewStore()
			if err != nil {
				return err
			}
			ws, err := currentWorkspace(store, false)
			if err != nil {
				return err
			}
			filter = packagecache.VolumePrefix(ws.ID)
		}

		volumes, err := d.ListVolumes(cmd.Context(), filter)
		if err != nil {
			return err
		}

		if len(volumes) == 0 {
			u.Dim("No cache volumes found")
			return nil
		}

		if cacheListAllFlag {
			headers := []string{"VOLUME", "WORKSPACE", "PROVIDER", "SIZE"}
			var rows [][]string
			for _, v := range volumes {
				ws, provider := parseVolumeName(v.Name)
				size := v.Size
				if size == "" {
					size = "-"
				}
				rows = append(rows, []string{v.Name, ws, provider, size})
			}
			u.Table(headers, rows)
		} else {
			headers := []string{"VOLUME", "PROVIDER", "SIZE"}
			var rows [][]string
			for _, v := range volumes {
				provider := strings.TrimPrefix(v.Name, filter)
				size := v.Size
				if size == "" {
					size = "-"
				}
				rows = append(rows, []string{v.Name, provider, size})
			}
			u.Table(headers, rows)
		}

		return nil
	},
}

var cacheCleanCmd = &cobra.Command{
	Use:   "clean [providers...]",
	Short: "Remove package cache volumes",
	RunE: func(cmd *cobra.Command, args []string) error {
		u := newUI()

		d, err := oci.NewOCIDriver(logger)
		if err != nil {
			return err
		}

		var filter string
		if cacheCleanAllFlag {
			filter = packagecache.GlobalVolumePrefix
		} else {
			store, err := workspace.NewStore()
			if err != nil {
				return err
			}
			ws, err := currentWorkspace(store, false)
			if err != nil {
				return err
			}
			filter = packagecache.VolumePrefix(ws.ID)
		}

		volumes, err := d.ListVolumes(cmd.Context(), filter)
		if err != nil {
			return err
		}

		if len(volumes) == 0 {
			u.Dim("No cache volumes to clean")
			return nil
		}

		// Filter by specific providers if given.
		if len(args) > 0 {
			wanted := make(map[string]bool, len(args))
			for _, a := range args {
				wanted[a] = true
			}
			var filtered []string
			for _, v := range volumes {
				_, provider := parseVolumeName(v.Name)
				if wanted[provider] {
					filtered = append(filtered, v.Name)
				}
			}
			if len(filtered) == 0 {
				u.Dim("No matching cache volumes found")
				return nil
			}
			for _, name := range filtered {
				if err := d.RemoveVolume(cmd.Context(), name); err != nil {
					u.Dim(fmt.Sprintf("  warning: %s: %v", name, err))
					continue
				}
				u.Success("Removed " + name)
			}
			return nil
		}

		for _, v := range volumes {
			if err := d.RemoveVolume(cmd.Context(), v.Name); err != nil {
				u.Dim(fmt.Sprintf("  warning: %s: %v", v.Name, err))
				continue
			}
			u.Success("Removed " + v.Name)
		}

		return nil
	},
}

// parseVolumeName extracts workspace ID and provider from a cache volume name.
// Volume names follow the pattern "crib-cache-{wsID}-{provider}".
// All provider names are single words (no hyphens), so the provider is the
// segment after the last hyphen in the suffix after "crib-cache-".
func parseVolumeName(name string) (workspaceID, provider string) {
	suffix := strings.TrimPrefix(name, packagecache.GlobalVolumePrefix)
	if i := strings.LastIndex(suffix, "-"); i >= 0 {
		return suffix[:i], suffix[i+1:]
	}
	return suffix, ""
}

func init() {
	cacheListCmd.Flags().BoolVar(&cacheListAllFlag, "all", false, "list cache volumes for all workspaces")
	cacheCleanCmd.Flags().BoolVar(&cacheCleanAllFlag, "all", false, "remove cache volumes for all workspaces")
	cacheCmd.AddCommand(cacheListCmd)
	cacheCmd.AddCommand(cacheCleanCmd)
}
