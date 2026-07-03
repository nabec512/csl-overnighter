// Package cli wires up the cobra command tree for csl-overnighter.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nabec512/csl-overnighter/internal/profile"
)

// NewRootCmd builds the top-level "csl-overnighter" command.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "csl-overnighter",
		Short: "Fill and submit your town's overnight parking permit form",
	}

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

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}
