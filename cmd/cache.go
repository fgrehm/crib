package cmd

import (
	"fmt"
	"os"

	"github.com/fgrehm/crib/internal/driver/oci"
	"github.com/fgrehm/crib/internal/feature"
	"github.com/fgrehm/crib/internal/plugin/packagecache"
	"github.com/fgrehm/crib/internal/workspace"
	"github.com/spf13/cobra"
)

var cacheListAllFlag bool
var cacheCleanAllFlag bool
var cacheCleanForceFlag bool

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage package cache volumes",
}

var cacheListCmd = &cobra.Command{
	Use:   "list",
	Short: "List package cache volumes",
	Args:  noArgs,
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
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
			wsID, err := workspace.InferID(configDirFlag, dirFlag, cwd)
			if err != nil {
				return err
			}
			filter = packagecache.VolumePrefix(wsID)
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
				ws, provider := packagecache.ParseVolumeName(v.Name)
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
				_, provider := packagecache.ParseVolumeName(v.Name)
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
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
			wsID, err := workspace.InferID(configDirFlag, dirFlag, cwd)
			if err != nil {
				return err
			}
			filter = packagecache.VolumePrefix(wsID)
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
		var toRemove []string
		if len(args) > 0 {
			wanted := make(map[string]bool, len(args))
			for _, a := range args {
				wanted[a] = true
			}
			for _, v := range volumes {
				_, provider := packagecache.ParseVolumeName(v.Name)
				if wanted[provider] {
					toRemove = append(toRemove, v.Name)
				}
			}
			if len(toRemove) == 0 {
				u.Dim("No matching cache volumes found")
				return nil
			}
		} else {
			for _, v := range volumes {
				toRemove = append(toRemove, v.Name)
			}
		}

		// Prompt for confirmation when --all is used (affects other projects).
		if cacheCleanAllFlag && !cacheCleanForceFlag {
			fmt.Fprintf(os.Stderr, "This will remove %d cache volume(s) from ALL workspaces:\n", len(toRemove))
			for _, name := range toRemove {
				fmt.Fprintf(os.Stderr, "  %s\n", name)
			}
			confirmed, err := confirmPrompt("--all requires confirmation")
			if err != nil {
				return err
			}
			if !confirmed {
				u.Dim("Aborted")
				return nil
			}
		}

		for _, name := range toRemove {
			if err := d.RemoveVolume(cmd.Context(), name); err != nil {
				u.Dim(fmt.Sprintf("  warning: %s: %v", name, err))
				continue
			}
			u.Success("Removed " + name)
		}

		return nil
	},
}

var cacheCleanFeaturesCmd = &cobra.Command{
	Use:   "clean-features",
	Short: "Remove cached DevContainer Features",
	Args:  noArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		u := newUI()

		fc, err := feature.NewFeatureCache()
		if err != nil {
			return err
		}

		if fc.IsEmpty() {
			u.Dim("Feature cache is already empty")
			return nil
		}

		if err := fc.Clean(); err != nil {
			return fmt.Errorf("cleaning feature cache: %w", err)
		}

		u.Success("Feature cache cleared")
		return nil
	},
}

func init() {
	cacheListCmd.Flags().BoolVar(&cacheListAllFlag, "all", false, "list cache volumes for all workspaces")
	cacheCleanCmd.Flags().BoolVar(&cacheCleanAllFlag, "all", false, "remove cache volumes for all workspaces")
	cacheCleanCmd.Flags().BoolVarP(&cacheCleanForceFlag, "force", "f", false, "skip confirmation prompt for --all")
	cacheCmd.AddCommand(cacheListCmd)
	cacheCmd.AddCommand(cacheCleanCmd)
	cacheCmd.AddCommand(cacheCleanFeaturesCmd)
}
