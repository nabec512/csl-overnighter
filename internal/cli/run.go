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
	var start string
	var duration int
	var dryRun bool
	var chromePath string

	cmd := &cobra.Command{
		Use:   "run <profile-name>",
		Short: "Fill in and submit the permit form using a saved profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if duration < 1 || duration > 3 {
				return fmt.Errorf("--duration must be 1, 2, or 3 (got %d)", duration)
			}
			if start == "" {
				start = time.Now().Format("2006-01-02")
			} else if _, err := time.Parse("2006-01-02", start); err != nil {
				return fmt.Errorf("--start must be formatted YYYY-MM-DD (got %q)", start)
			}

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
				Start:          start,
				Duration:       duration,
				DryRun:         dryRun,
				ChromePath:     chromePath,
			}

			result, err := browser.Submit(context.Background(), p, cfg)
			if err != nil {
				return err
			}

			switch {
			case dryRun:
				fmt.Println("Dry run: form filled in but not submitted.")
			case result.Success:
				fmt.Println("Permit request submitted successfully.")
				if result.ConfirmationText != "" {
					fmt.Println("Confirmation:", result.ConfirmationText)
				}
			default:
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
	cmd.Flags().StringVar(&start, "start", "", "first night the permit is valid for, format YYYY-MM-DD (default: today)")
	cmd.Flags().IntVar(&duration, "duration", 1, "number of consecutive nights requested (1, 2, or 3)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "fill in the form and stop before clicking Submit")
	cmd.Flags().StringVar(&chromePath, "chrome-path", "", "path to the Chrome/Chromium binary, if it's not installed somewhere chromedp looks by default")

	return cmd
}
