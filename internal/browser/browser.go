// Package browser drives headless (or headful) Chrome via chromedp to fill
// in and submit the Cote Saint-Luc overnight parking permit form at
// https://cotesaintluc-publicform.icosolutions.com/publicforms/2.
package browser

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"

	"github.com/nabec512/csl-overnighter/internal/profile"
)

const formURL = "https://cotesaintluc-publicform.icosolutions.com/publicforms/2"

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

	// Start is the first night the permit is valid for, formatted
	// "2006-01-02".
	Start string

	// Duration is the number of consecutive nights requested. The form
	// only accepts 1, 2, or 3.
	Duration int

	// DryRun fills in the form and stops before clicking Submit.
	DryRun bool
}

func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Bool("headful", c.Headful),
		slog.String("timeout", c.Timeout.String()),
		slog.String("screenshot_path", c.ScreenshotPath),
		slog.String("start", c.Start),
		slog.Int("duration", c.Duration),
		slog.Bool("dry_run", c.DryRun),
	)
}

// Result describes the outcome of a form submission attempt.
type Result struct {
	Success          bool
	ConfirmationText string
	ScreenshotPath   string
}

// Submit fills in and submits the permit form using the given profile's
// fields, returning the outcome.
func Submit(ctx context.Context, p *profile.Profile, cfg Config) (*Result, error) {
	slog.DebugContext(ctx, "config values", slog.Any("config", cfg.LogValue()))

	if err := p.Validate(); err != nil {
		return nil, err
	}
	if cfg.Duration < 1 || cfg.Duration > 3 {
		return nil, fmt.Errorf("duration must be 1, 2, or 3 nights, got %d", cfg.Duration)
	}

	phoneDigits := onlyDigits(p.Phone)
	if len(phoneDigits) != 10 {
		return nil, fmt.Errorf("phone number must have 10 digits, got %q", p.Phone)
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("headless", !cfg.Headful))
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	taskCtx, taskCancel := chromedp.NewContext(allocCtx)

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 90 * time.Second
	}
	taskCtx, timeoutCancel := context.WithTimeout(taskCtx, timeout)
	defer timeoutCancel()

	// closeBrowser tears down the Chrome process. On error in --headful
	// mode we deliberately skip this so the window stays open for the
	// user to inspect what the page actually looks like.
	closeBrowser := func() {
		taskCancel()
		allocCancel()
	}
	fail := func(err error) (*Result, error) {
		if cfg.Headful {
			return nil, fmt.Errorf("%w (browser window left open for inspection; close it manually)", err)
		}
		closeBrowser()
		return nil, err
	}

	fill := chromedp.Tasks{
		chromedp.Navigate(formURL),
		chromedp.WaitVisible(`#OPPFirstName`, chromedp.ByID),

		selectCombobox("OPPAddressSelector", p.Address),
		typeIntoIfSet("OPPSuiteNumber", p.Suite),
		typeInto("OPPFirstName", p.FirstName),
		typeInto("OPPLastName", p.LastName),
		typeInto("OPPPhone", phoneDigits),
		typeInto("OPPEmail", p.Email),
		typeInto("OPPLicencePlate", p.LicencePlate),
		selectCombobox("OPPVehicleMake", p.VehicleMake),
		selectCombobox("OPPVehicleModel", p.VehicleModel),
		selectCombobox("OPPVehicleColor", p.VehicleColor),
		selectCombobox("OPPCountry", p.Country),
		// The State list is populated based on the chosen Country; give
		// it a moment to refresh before opening it.
		chromedp.Sleep(300 * time.Millisecond),
		selectCombobox("OPPState", p.State),
		selectCombobox("OPPReason", p.Reason),
		fillDate(taskCtx, "OPPStart", cfg.Start),
		typeInto("OPPDuration", fmt.Sprintf("%d", cfg.Duration)),
	}

	if err := chromedp.Run(taskCtx, fill); err != nil {
		path, _ := screenshot(taskCtx, cfg.ScreenshotPath)
		return fail(fmt.Errorf("fill form (screenshot: %s): %w", path, err))
	}

	if err := checkValidation(taskCtx); err != nil {
		path, _ := screenshot(taskCtx, cfg.ScreenshotPath)
		return fail(fmt.Errorf("%w (screenshot: %s)", err, path))
	}

	if cfg.DryRun {
		path, _ := screenshot(taskCtx, cfg.ScreenshotPath)
		closeBrowser()
		return &Result{ScreenshotPath: path}, nil
	}

	if err := chromedp.Run(taskCtx, chromedp.Click(`.publicForm-submit button`, chromedp.ByQuery)); err != nil {
		return fail(fmt.Errorf("click submit: %w", err))
	}

	// Give the page a moment to process the submission and render a
	// result before we look at it.
	_ = chromedp.Run(taskCtx, chromedp.Sleep(2*time.Second))

	result := &Result{}
	result.ScreenshotPath, _ = screenshot(taskCtx, cfg.ScreenshotPath)

	var stillInvalid []string
	_ = chromedp.Run(taskCtx, chromedp.Evaluate(invalidFieldsJS, &stillInvalid))

	var formGone bool
	_ = chromedp.Run(taskCtx, chromedp.Evaluate(`document.querySelector('.publicForm-submit') === null`, &formGone))

	// Best-effort success signal: no client-side validation errors
	// remain, and the form itself is no longer on the page. This has not
	// been verified against a real submission's result page; treat
	// Result.Success with suspicion until confirmed, and always check
	// the screenshot.
	result.Success = len(stillInvalid) == 0 && formGone

	var confirmation string
	_ = chromedp.Run(taskCtx, chromedp.Evaluate(
		`(document.body.innerText.match(/(confirmation|success|succ[eè]s)[^\n]*/i) || [''])[0]`,
		&confirmation,
	))
	result.ConfirmationText = strings.TrimSpace(confirmation)

	// Leave the window open for inspection if it looks like the submit
	// didn't succeed; otherwise there's nothing left to check, close it.
	if !(cfg.Headful && !result.Success) {
		closeBrowser()
	}

	return result, nil
}

// typeInto focuses the field, selects any existing text, and types value
// via real key events so masked/segmented Kendo widgets (phone, date,
// numeric) see and format each keystroke as a user would produce it.
func typeInto(id, value string) chromedp.Action {
	sel := "#" + id
	return chromedp.Tasks{
		chromedp.Focus(sel, chromedp.ByID),
		chromedp.Evaluate(fmt.Sprintf(`document.getElementById(%q).select()`, id), nil),
		chromedp.SendKeys(sel, value, chromedp.ByID),
	}
}

// dateSegmentSettle is how long fillDate waits after each keystroke group
// before moving on. CDP's dispatchKeyEvent returns as soon as the event is
// dispatched, not after the page's (React/Kendo) event handlers have
// finished re-rendering, so firing ArrowRight immediately after SendKeys
// races the segment's own update and truncates it inconsistently (1 or 2
// of the 4 year digits landing, run to run). The sleep gives the widget
// time to finish processing each segment before we move past it.
const dateSegmentSettle = 500 * time.Millisecond

// fillDate fills the Kendo DateInput inside the OPPStart date picker. Its
// value is segmented (year/month/day, format yyyy-MM-dd) and only responds
// to keyboard navigation, not text selection or typed separators: the
// widget does NOT auto-advance to the next segment once one is filled, so
// each segment must be typed and then explicitly advanced past with
// ArrowRight.
func fillDate(ctx context.Context, id, isoDate string) chromedp.Action {
	sel := "#" + id
	parts := strings.SplitN(isoDate, "-", 3) // [yyyy, MM, dd]
	slog.DebugContext(ctx, "date parts", slog.Any("parts", parts))
	return chromedp.Tasks{
		chromedp.Sleep(dateSegmentSettle),
		chromedp.Click(sel, chromedp.ByID),
		chromedp.KeyEvent(kb.Home),
		chromedp.Sleep(dateSegmentSettle),
		chromedp.SendKeys(sel, parts[0], chromedp.ByID),
		chromedp.Sleep(dateSegmentSettle),
		chromedp.KeyEvent(kb.ArrowRight),
		chromedp.Sleep(dateSegmentSettle),
		chromedp.SendKeys(sel, parts[1], chromedp.ByID),
		chromedp.Sleep(dateSegmentSettle),
		chromedp.KeyEvent(kb.ArrowRight),
		chromedp.Sleep(dateSegmentSettle),
		chromedp.SendKeys(sel, parts[2], chromedp.ByID),
		chromedp.Sleep(dateSegmentSettle),
	}
}

// typeIntoIfSet is typeInto but a no-op when value is empty, for optional
// fields (e.g. suite number) that shouldn't touch an already-empty input.
func typeIntoIfSet(id, value string) chromedp.Action {
	if strings.TrimSpace(value) == "" {
		return chromedp.ActionFunc(func(context.Context) error { return nil })
	}
	return typeInto(id, value)
}

// selectCombobox types value into a Kendo ComboBox field (id) to trigger
// its live filtering, then clicks the option in its popup listbox
// (#<id>list) whose text matches value exactly (case-insensitive). This is
// required for fields like Vehicle Make/Model/Country/State/Reason, whose
// submitted value must come from selecting a listed option rather than
// free text.
func selectCombobox(id, value string) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("no value provided for required field %s", id)
		}

		inputSel := "#" + id
		listSel := "#" + id + "list"

		if err := chromedp.Run(ctx,
			chromedp.Focus(inputSel, chromedp.ByID),
			chromedp.Evaluate(fmt.Sprintf(`document.getElementById(%q).select()`, id), nil),
			chromedp.SendKeys(inputSel, value, chromedp.ByID),
			chromedp.WaitVisible(listSel, chromedp.ByID),
		); err != nil {
			return fmt.Errorf("type %q into %s: %w", value, id, err)
		}

		var texts []string
		optionsJS := fmt.Sprintf(`Array.from(document.querySelectorAll(%q)).map(el => el.textContent.trim())`, listSel+` [role="option"]`)
		if err := chromedp.Run(ctx, chromedp.Evaluate(optionsJS, &texts)); err != nil {
			return fmt.Errorf("read options for %s: %w", id, err)
		}

		idx := -1
		for i, t := range texts {
			if strings.EqualFold(t, value) {
				idx = i
				break
			}
		}
		if idx == -1 {
			return fmt.Errorf("field %s: no exact match for %q; available options: %s", id, value, strings.Join(texts, ", "))
		}

		optSel := fmt.Sprintf(`%s [role="option"]:nth-of-type(%d)`, listSel, idx+1)
		if err := chromedp.Run(ctx, chromedp.Click(optSel, chromedp.ByQuery)); err != nil {
			return fmt.Errorf("click option for %s: %w", id, err)
		}
		return nil
	})
}

// invalidFieldsJS collects the ids of inputs still failing client-side
// validation (Kendo marks either the input or its wrapper with k-invalid,
// depending on widget type).
const invalidFieldsJS = `(() => {
	const ids = new Set();
	document.querySelectorAll('.k-invalid, [aria-invalid="true"]').forEach(el => {
		const input = el.matches('input') ? el : el.querySelector('input[id]');
		ids.add(input ? input.id : el.className);
	});
	return Array.from(ids);
})()`

func checkValidation(ctx context.Context) error {
	var invalid []string
	if err := chromedp.Run(ctx, chromedp.Evaluate(invalidFieldsJS, &invalid)); err != nil {
		return fmt.Errorf("check form validation: %w", err)
	}
	if len(invalid) > 0 {
		return fmt.Errorf("form still has invalid fields after filling: %s", strings.Join(invalid, ", "))
	}
	return nil
}

func screenshot(ctx context.Context, path string) (string, error) {
	if path == "" {
		return "", nil
	}
	var buf []byte
	if err := chromedp.Run(ctx, chromedp.FullScreenshot(&buf, 90)); err != nil {
		return "", fmt.Errorf("capture screenshot: %w", err)
	}
	if err := os.WriteFile(path, buf, 0o600); err != nil {
		return "", fmt.Errorf("write screenshot to %s: %w", path, err)
	}
	return path, nil
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
