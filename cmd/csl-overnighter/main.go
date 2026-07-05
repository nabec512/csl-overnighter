// Command csl-overnighter fills in and submits a town's overnight parking
// permit web form using a saved profile.
package main

import (
	"log/slog"
	"os"

	"github.com/nabec512/csl-overnighter/internal/cli"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))

	if err := cli.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
