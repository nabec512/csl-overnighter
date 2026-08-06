// Package cli wires up the cobra command tree for csl-overnighter.
package cli

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/nabec512/csl-overnighter/internal/profile"
)

// NewRootCmd builds the top-level "csl-overnighter" command.
func NewRootCmd() *cobra.Command {
	var logLevel string
	var verbose bool

	root := &cobra.Command{
		Use:   "csl-overnighter",
		Short: "Fill and submit your town's overnight parking permit form",

		// Execute prints the error itself, so cobra shouldn't print it too.
		SilenceErrors: true,

		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Suppress the usage block from here on. Cobra parses flags and
			// validates args *before* PersistentPreRunE, so those errors
			// still get usage printed (which is what you want for a
			// mistyped command), while everything after — a missing
			// profile, a form that moved, a field that wouldn't fill — is a
			// runtime failure where the usage dump only buries the error.
			cmd.SilenceUsage = true

			level, err := resolveLogLevel(logLevel, cmd.Flags().Changed("log-level"), verbose)
			if err != nil {
				return err
			}
			slog.SetDefault(newLogger(cmd.ErrOrStderr(), level))
			return nil
		},
	}

	root.PersistentFlags().StringVar(&logLevel, "log-level", "info",
		"log verbosity: debug, info, warn, or error (env: "+logLevelEnv+")")
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false,
		"shorthand for --log-level debug; logs every field value, including personal details")

	root.AddCommand(newProfileCmd())
	root.AddCommand(newRunCmd())

	return root
}

func openStore() (*profile.Store, error) {
	dir, err := profile.DefaultDir()
	if err != nil {
		return nil, err
	}
	return profile.NewStore(dir)
}

// Execute runs the root command, reporting any error on stderr. It returns
// the process exit code so main stays a one-liner.
func Execute() int {
	if err := NewRootCmd().Execute(); err != nil {
		// PersistentPreRunE may not have run (e.g. flag parse failure), so
		// don't assume the configured logger exists yet.
		if _, printErr := os.Stderr.WriteString("Error: " + err.Error() + "\n"); printErr != nil {
			return 1
		}
		return 1
	}
	return 0
}
