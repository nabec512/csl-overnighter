package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/nabec512/csl-overnighter/internal/browser"
)

func newRunCmd() *cobra.Command {
	var headful bool
	var timeout time.Duration
	var screenshotPath string

	cmd := &cobra.Command{
		Use:   "run <profile-name>",
		Short: "Fill in and submit the permit form using a saved profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}

			p, err := store.Load(args[0])
			if err != nil {
				return err
			}

			cfg := browser.Config{
				Headful:        headful,
				Timeout:        timeout,
				ScreenshotPath: screenshotPath,
			}

			result, err := browser.Submit(context.Background(), p, cfg)
			if err != nil {
				return err
			}

			if result.Success {
				fmt.Println("Permit request submitted successfully.")
				if result.ConfirmationText != "" {
					fmt.Println("Confirmation:", result.ConfirmationText)
				}
			} else {
				fmt.Println("Permit request did not appear to succeed; check the screenshot.")
			}
			if result.ScreenshotPath != "" {
				fmt.Println("Screenshot saved to:", result.ScreenshotPath)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&headful, "headful", false, "show the browser window instead of running headless")
	cmd.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "overall timeout for the run")
	cmd.Flags().StringVar(&screenshotPath, "screenshot", "", "path to save a screenshot of the final page state")

	return cmd
}
