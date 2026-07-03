// Package browser drives headless (or headful) Chrome via chromedp to fill
// in and submit the town's overnight parking permit form.
package browser

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/nabec512/csl-overnighter/internal/profile"
)

// Config controls how the browser automation runs.
type Config struct {
	// Headful, when true, shows the browser window instead of running
	// headless. Useful when the form changes and selectors need
	// re-diagnosing.
	Headful bool

	// Timeout bounds the whole submit operation.
	Timeout time.Duration

	// ScreenshotPath, if set, saves a full-page PNG screenshot of the
	// final page state (success or failure) to this path.
	ScreenshotPath string
}

// Result describes the outcome of a form submission attempt.
type Result struct {
	Success          bool
	ConfirmationText string
	ScreenshotPath   string
}

// Submit fills in and submits the permit form using the given profile's
// fields, returning the outcome.
//
// TODO: this is a placeholder. The actual navigation, field selectors, and
// submit/confirmation logic depend on the live form and will be implemented
// once the URL and field mapping are available (see CLAUDE.md).
func Submit(ctx context.Context, p *profile.Profile, cfg Config) (*Result, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("headless", !cfg.Headful))
	allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
	defer cancel()

	taskCtx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	taskCtx, cancel = context.WithTimeout(taskCtx, timeout)
	defer cancel()

	_ = taskCtx
	return nil, fmt.Errorf("form automation not yet implemented: waiting on target URL and field mapping for profile %q", p.Name)
}
