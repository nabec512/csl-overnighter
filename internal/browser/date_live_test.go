package browser

import (
	"context"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// TestFillDateOverwritesExistingValue exercises the retry path against the
// real form. Attempt 1 always meets an empty date field, so the clearing
// logic in typeDate — the thing every retry depends on — is never
// exercised by a normal successful run. Filling the field twice with
// different dates reproduces exactly that situation: the second call types
// into a fully populated field and must end up with the new date, not a
// blend of the two.
//
// This hits the live site, so it's skipped under -short (and when there's
// no Chrome). `go test ./... -short` stays offline; `task test:live` runs
// it.
func TestFillDateOverwritesExistingValue(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live-form test in short mode")
	}
	taskCtx := newTestBrowser(t, context.Background())

	if err := chromedp.Run(taskCtx); err != nil {
		t.Fatalf("start browser: %v", err)
	}

	loadCtx, cancelLoad := context.WithTimeout(taskCtx, 60*time.Second)
	defer cancelLoad()
	if err := chromedp.Run(loadCtx,
		chromedp.Navigate(formURL),
		chromedp.WaitVisible(`#OPPStart`, chromedp.ByID),
	); err != nil {
		t.Fatalf("load form: %v", err)
	}

	fill := func(t *testing.T, date string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(taskCtx, 60*time.Second)
		defer cancel()
		if err := fillDate("OPPStart", date, nil).Do(ctx); err != nil {
			t.Fatalf("fillDate(%s): %v", date, err)
		}
		var got string
		if err := chromedp.Run(ctx, chromedp.Value(`#OPPStart`, &got, chromedp.ByID)); err != nil {
			t.Fatalf("read back after %s: %v", date, err)
		}
		return got
	}

	if got := fill(t, "2026-08-06"); got != "2026-08-06" {
		t.Fatalf("first fill: got %q, want 2026-08-06", got)
	}

	// The retry case: type a different date over a populated field.
	if got := fill(t, "2027-12-31"); got != "2027-12-31" {
		t.Fatalf("overwrite of a populated date field: got %q, want 2027-12-31 "+
			"(the clearing step in typeDate is not clearing all segments)", got)
	}

	// And back to a date with fewer digits per segment, which is where a
	// partial clear would most obviously leave leftovers behind.
	if got := fill(t, "2026-01-02"); got != "2026-01-02" {
		t.Fatalf("overwrite with single-digit month/day: got %q, want 2026-01-02", got)
	}
}
