package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var doctorFixFlag bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check workspace health and diagnose issues",
	RunE: func(cmd *cobra.Command, args []string) error {
		u := newUI()

		eng, _, _, err := newEngine()
		if err != nil {
			return err
		}

		u.Header("Checking workspace health")

		result, err := eng.Doctor(cmd.Context(), doctorFixFlag)
		if err != nil {
			return err
		}

		// Report runtime and compose status.
		if result.RuntimeOK {
			u.Success("Container runtime is reachable")
		}
		if result.ComposeOK {
			u.Success("Docker Compose is available")
		}

		if len(result.Issues) == 0 {
			u.Success("No issues found")
			return nil
		}

		for _, issue := range result.Issues {
			prefix := "warning"
			if issue.Level == "error" {
				prefix = "error"
			}
			msg := fmt.Sprintf("[%s] %s: %s", prefix, issue.Check, issue.Description)
			if doctorFixFlag && issue.Fix != "" {
				msg += fmt.Sprintf(" (%s)", issue.Fix)
			}
			if issue.Level == "error" {
				u.Error(msg)
			} else {
				u.Dim(msg)
			}
		}

		if !doctorFixFlag {
			hasFixable := false
			for _, issue := range result.Issues {
				if issue.Fix != "" {
					hasFixable = true
					break
				}
			}
			if hasFixable {
				fmt.Println()
				u.Dim("Run 'crib doctor --fix' to auto-fix issues")
			}
		}

		return nil
	},
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorFixFlag, "fix", false, "auto-fix found issues")
}
