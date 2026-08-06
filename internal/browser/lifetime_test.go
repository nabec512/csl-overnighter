package browser

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// findChrome locates a browser binary the same way a developer would, and
// skips the test when there isn't one — CI (and the sandbox this was
// originally written in) has no Chrome, and these tests are diagnostics for
// humans rather than gatekeepers.
func findChrome(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser", "google-chrome-stable"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	t.Skip("no Chrome/Chromium binary found; skipping browser lifetime test")
	return ""
}

func newTestBrowser(t *testing.T, ctx context.Context) context.Context {
	t.Helper()
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.ExecPath(findChrome(t)))
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	t.Cleanup(func() {
		taskCancel()
		allocCancel()
	})
	return taskCtx
}

// TestDeadlineOnFirstRunKillsBrowser pins down the chromedp behaviour that
// made the original manual-fix handoff broken: because the browser process
// is allocated on the first Run with exec.CommandContext bound to that
// same context, running the first action under a deadline means the
// deadline tears down the whole browser. context.WithoutCancel cannot undo
// it. If this test ever starts failing, chromedp changed and the structure
// of Submit can be simplified.
func TestDeadlineOnFirstRunKillsBrowser(t *testing.T) {
	taskCtx := newTestBrowser(t, context.Background())

	// The mistake: first Run carries the deadline.
	timedCtx, cancel := context.WithTimeout(taskCtx, 2*time.Second)
	defer cancel()
	if err := chromedp.Run(timedCtx, chromedp.Navigate("about:blank")); err != nil {
		t.Fatalf("initial run should succeed: %v", err)
	}

	<-timedCtx.Done()

	// Detaching from the deadline does not bring the browser back.
	revived, revivedCancel := context.WithTimeout(context.WithoutCancel(timedCtx), 5*time.Second)
	defer revivedCancel()

	if err := chromedp.Run(revived, chromedp.Navigate("about:blank")); err == nil {
		t.Fatal("expected the browser to be dead after the deadline that allocated it expired, " +
			"but a WithoutCancel run succeeded — chromedp's allocation behaviour may have changed")
	} else {
		t.Logf("confirmed: browser unusable after allocating deadline expired: %v", err)
	}
}

// TestUntimedAllocationSurvivesPhaseDeadline is the fix Submit relies on:
// allocate the browser on an untimed context, then give each phase its own
// deadline. An expired phase deadline must leave the browser usable, so
// that the OnDateMismatch handoff can pause for a human and still read the
// field back afterward.
func TestUntimedAllocationSurvivesPhaseDeadline(t *testing.T) {
	taskCtx := newTestBrowser(t, context.Background())

	// Allocate on the untimed context, exactly as Submit does.
	if err := chromedp.Run(taskCtx); err != nil {
		t.Fatalf("start browser: %v", err)
	}

	// A phase whose deadline expires.
	phaseCtx, cancelPhase := context.WithTimeout(taskCtx, 1500*time.Millisecond)
	if err := chromedp.Run(phaseCtx, chromedp.Navigate("about:blank")); err != nil {
		t.Fatalf("phase run should succeed: %v", err)
	}
	<-phaseCtx.Done()
	cancelPhase()

	// The human-pause path: detach from the dead phase deadline and keep
	// driving the same browser.
	fixCtx, cancelFix := context.WithTimeout(context.WithoutCancel(phaseCtx), 15*time.Second)
	defer cancelFix()

	var title string
	if err := chromedp.Run(fixCtx,
		chromedp.Navigate("about:blank"),
		chromedp.Evaluate(`"still alive"`, &title),
	); err != nil {
		t.Fatalf("browser should have survived the expired phase deadline: %v", err)
	}
	if title != "still alive" {
		t.Fatalf("unexpected evaluate result: %q", title)
	}
}
