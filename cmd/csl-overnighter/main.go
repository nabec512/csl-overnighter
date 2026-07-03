// Command csl-overnighter fills in and submits a town's overnight parking
// permit web form using a saved profile.
package main

import (
	"os"

	"github.com/nabec512/csl-overnighter/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
