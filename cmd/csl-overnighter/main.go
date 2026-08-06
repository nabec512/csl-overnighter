// Command csl-overnighter fills in and submits a town's overnight parking
// permit web form using a saved profile.
package main

import (
	"os"

	"github.com/nabec512/csl-overnighter/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
