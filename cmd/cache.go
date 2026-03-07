package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fgrehm/crib/internal/driver/oci"
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
			wsID, err := inferWorkspaceID()
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
				_, provider := parseVolumeName(v.Name)
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
			wsID, err := inferWorkspaceID()
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
				_, provider := parseVolumeName(v.Name)
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
			if !stdinIsTerminal() {
				return fmt.Errorf("--all requires confirmation; use --force to skip (stdin is not a terminal)")
			}
			fmt.Fprintf(os.Stderr, "This will remove %d cache volume(s) from ALL workspaces:\n", len(toRemove))
			for _, name := range toRemove {
				fmt.Fprintf(os.Stderr, "  %s\n", name)
			}
			fmt.Fprint(os.Stderr, "Continue? [y/N] ")
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "y" && answer != "yes" {
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

// parseVolumeName extracts workspace ID and provider from a cache volume name.
// Volume names follow the pattern "crib-cache-{wsID}-{provider}", but compose
// workspaces may have a "{project}_" prefix (e.g. "crib-web_crib-cache-web-apt").
// All provider names are single words (no hyphens), so the provider is the
// segment after the last hyphen in the suffix after "crib-cache-".
func parseVolumeName(name string) (workspaceID, provider string) {
	// Strip compose project prefix if present (everything before "crib-cache-").
	if i := strings.Index(name, packagecache.GlobalVolumePrefix); i > 0 {
		name = name[i:]
	}
	suffix := strings.TrimPrefix(name, packagecache.GlobalVolumePrefix)
	if i := strings.LastIndex(suffix, "-"); i >= 0 {
		return suffix[:i], suffix[i+1:]
	}
	return suffix, ""
}

// inferWorkspaceID derives a workspace ID from the current directory (or --dir / --config flags)
// without requiring workspace state to exist. It first tries the normal devcontainer resolution
// (which walks up to find .devcontainer/), and falls back to slugifying the directory name if
// no devcontainer config is found. This allows cache commands to work even if the project was
// deleted or was never set up with crib.
func inferWorkspaceID() (string, error) {
	switch {
	case configDirFlag != "":
		rr, err := workspace.ResolveConfigDir(configDirFlag)
		if err == nil {
			return rr.WorkspaceID, nil
		}
		// Only fall back when the config doesn't exist (project deleted
		// or never set up). Surface real errors (permissions, I/O).
		if !errors.Is(err, workspace.ErrNoDevContainer) {
			return "", err
		}
		absDir, err := filepath.Abs(configDirFlag)
		if err != nil {
			return "", fmt.Errorf("resolving config dir: %w", err)
		}
		return workspace.Slugify(filepath.Base(filepath.Dir(absDir))), nil
	case dirFlag != "":
		rr, err := workspace.Resolve(dirFlag)
		if err == nil {
			return rr.WorkspaceID, nil
		}
		if !errors.Is(err, workspace.ErrNoDevContainer) {
			return "", err
		}
		absDir, err := filepath.Abs(dirFlag)
		if err != nil {
			return "", fmt.Errorf("resolving dir: %w", err)
		}
		return workspace.Slugify(filepath.Base(absDir)), nil
	default:
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("getting working directory: %w", err)
		}
		rr, err := workspace.Resolve(cwd)
		if err == nil {
			return rr.WorkspaceID, nil
		}
		if !errors.Is(err, workspace.ErrNoDevContainer) {
			return "", err
		}
		return workspace.Slugify(filepath.Base(cwd)), nil
	}
}

func init() {
	cacheListCmd.Flags().BoolVar(&cacheListAllFlag, "all", false, "list cache volumes for all workspaces")
	cacheCleanCmd.Flags().BoolVar(&cacheCleanAllFlag, "all", false, "remove cache volumes for all workspaces")
	cacheCleanCmd.Flags().BoolVarP(&cacheCleanForceFlag, "force", "f", false, "skip confirmation prompt for --all")
	cacheCmd.AddCommand(cacheListCmd)
	cacheCmd.AddCommand(cacheCleanCmd)
}
