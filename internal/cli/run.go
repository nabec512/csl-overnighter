package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
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
				OnDateMismatch: onDateMismatch(headful),
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
	cmd.Flags().DurationVar(&timeout, "timeout", 90*time.Second, "timeout for each phase of the run (page load, filling, submit)")
	cmd.Flags().StringVar(&screenshotPath, "screenshot", "", "path to save a screenshot of the final page state")
	cmd.Flags().StringVar(&start, "start", "", "first night the permit is valid for, format YYYY-MM-DD (default: today)")
	cmd.Flags().IntVar(&duration, "duration", 1, "number of consecutive nights requested (1, 2, or 3)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "fill in the form and stop before clicking Submit")
	cmd.Flags().StringVar(&chromePath, "chrome-path", "", "path to the Chrome/Chromium binary, if it's not installed somewhere chromedp looks by default")

	return cmd
}

// onDateMismatch builds the hook browser.Config.OnDateMismatch calls if the
// start-date field still doesn't hold the requested value after fillDate's
// own retries. In --headful mode, where the operator can actually see and
// click into the browser window, it pauses and asks them to fix the field
// by hand before letting the run continue. In headless mode there's no
// window to fix, so it just fails with a pointer to --headful instead of
// leaving the run hanging on a prompt nobody can act on.
func onDateMismatch(headful bool) func(ctx context.Context, fieldID, expected, actual string) error {
	return func(ctx context.Context, fieldID, expected, actual string) error {
		if !headful {
			return fmt.Errorf("field %s would not fill in automatically after several attempts (wanted %s, got %q); rerun with --headful to fix it by hand in the browser window", fieldID, expected, actual)
		}

		fmt.Printf("\nThe %s field didn't fill in correctly after several attempts (wanted %s, browser currently shows %q).\n", fieldID, expected, actual)
		fmt.Println("Fix it by hand in the browser window, then press Enter here to continue...")
		if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
			return fmt.Errorf("read confirmation for manual date fix: %w", err)
		}
		return nil
	}
}
