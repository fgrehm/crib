package cmd

import (
	"fmt"
	"os"

	"github.com/fgrehm/crib/internal/engine"
	"github.com/fgrehm/crib/internal/ui"
	"github.com/fgrehm/crib/internal/workspace"
	"github.com/spf13/cobra"
)

var (
	pruneAllFlag   bool
	pruneForceFlag bool
)

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove stale and orphan workspace images",
	Long: `Remove stale and orphan crib-managed images.

By default, prunes images for the current workspace only.
Use --all to prune images across all workspaces (including orphans).`,
	Args: noArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		u := newUI()

		eng, _, _, err := newEngine()
		if err != nil {
			return err
		}

		opts := engine.PruneOptions{DryRun: true}
		if !pruneAllFlag {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
			wsID, err := workspace.InferID(configDirFlag, dirFlag, cwd)
			if err != nil {
				return err
			}
			opts.WorkspaceID = wsID
		}

		// Dry run to show what would be removed.
		preview, err := eng.PruneImages(cmd.Context(), opts)
		if err != nil {
			return err
		}

		if len(preview.Removed) == 0 {
			u.Dim("No stale images to remove")
			return nil
		}

		var totalSize int64
		for _, img := range preview.Removed {
			label := "stale"
			if img.Orphan {
				label = "orphan"
			}
			fmt.Fprintf(os.Stderr, "  %s (%s, %s)\n", img.Reference, label, ui.FormatBytes(img.Size))
			totalSize += img.Size
		}
		fmt.Fprintf(os.Stderr, "\n%d image(s), %s total\n", len(preview.Removed), ui.FormatBytes(totalSize))

		if !pruneForceFlag {
			confirmed, err := confirmPrompt("pruning requires confirmation")
			if err != nil {
				return err
			}
			if !confirmed {
				u.Dim("Aborted")
				return nil
			}
		}

		// Actual removal.
		opts.DryRun = false
		result, err := eng.PruneImages(cmd.Context(), opts)
		if err != nil {
			return err
		}

		for _, img := range result.Removed {
			u.Success("Removed " + img.Reference)
		}
		for _, e := range result.Errors {
			u.Dim(fmt.Sprintf("  warning: %s: %v", e.Reference, e.Err))
		}

		return nil
	},
}

func init() {
	pruneCmd.Flags().BoolVar(&pruneAllFlag, "all", false, "prune images across all workspaces")
	pruneCmd.Flags().BoolVarP(&pruneForceFlag, "force", "f", false, "skip confirmation prompt")
}
