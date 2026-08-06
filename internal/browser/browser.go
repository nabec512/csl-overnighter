// Package browser drives headless (or headful) Chrome via chromedp to fill
// in and submit the Cote Saint-Luc overnight parking permit form at
// https://cotesaintluc-publicform.icosolutions.com/publicforms/2.
package browser

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strconv"
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

	// ChromePath, if set, overrides chromedp's built-in browser discovery
	// with an explicit path to the Chrome/Chromium binary. Needed when the
	// browser isn't installed in one of the couple of default locations
	// chromedp checks (e.g. Chrome installed under ~/Applications instead
	// of /Applications on macOS).
	ChromePath string

	// OnDateMismatch is called if the OPPStart date field still doesn't
	// hold the requested date after fillDate's built-in retries (see
	// maxDateFillAttempts) — typically because the Kendo DateInput's
	// keyboard-segment navigation raced a page re-render. It's given the
	// field id, the requested date, and what the field actually shows, and
	// gets one chance to fix things (e.g. by pausing so a human can edit
	// the field by hand in a --headful browser window) before fillDate
	// re-checks the field once more and gives up for good if it still
	// doesn't match. If nil, a mismatch fails the run immediately.
	OnDateMismatch func(ctx context.Context, fieldID, expected, actual string) error
}

func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Bool("headful", c.Headful),
		slog.String("timeout", c.Timeout.String()),
		slog.String("screenshot_path", c.ScreenshotPath),
		slog.String("start", c.Start),
		slog.Int("duration", c.Duration),
		slog.Bool("dry_run", c.DryRun),
		slog.String("chrome_path", c.ChromePath),
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
	if cfg.ChromePath != "" {
		opts = append(opts, chromedp.ExecPath(cfg.ChromePath))
	}
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	taskCtx, taskCancel := chromedp.NewContext(allocCtx)

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 90 * time.Second
	}

	// closeBrowser tears down the Chrome process. On error in --headful
	// mode we deliberately skip this so the window stays open for the
	// user to inspect what the page actually looks like.
	closeBrowser := func() {
		taskCancel()
		allocCancel()
	}

	// Start Chrome on the UNTIMED taskCtx. chromedp allocates the browser
	// on the first Run against a context, with exec.CommandContext bound to
	// that same context — so running the first action under a deadline
	// makes the deadline kill the whole browser, not just that action
	// (chromedp's own Run doc warns about exactly this). Everything below
	// therefore takes its deadline from a child context via `deadline`,
	// leaving taskCtx owning the process. That's what lets the
	// OnDateMismatch handoff pause for a human for as long as it needs
	// without the window disappearing mid-edit.
	started := time.Now()
	if err := chromedp.Run(taskCtx); err != nil {
		closeBrowser()
		return nil, fmt.Errorf("start browser: %w", err)
	}
	slog.DebugContext(ctx, "browser started",
		slog.String("took", time.Since(started).Round(time.Millisecond).String()),
		slog.Bool("headful", cfg.Headful))

	// deadline derives a bounded context for one phase of the run from the
	// browser-owning taskCtx.
	deadline := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(taskCtx, timeout)
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
		fillDate("OPPStart", cfg.Start, cfg.OnDateMismatch),
		typeInto("OPPDuration", fmt.Sprintf("%d", cfg.Duration)),
	}

	slog.InfoContext(ctx, "filling form", slog.String("url", formURL), slog.String("start", cfg.Start), slog.Int("nights", cfg.Duration))

	fillCtx, cancelFill := deadline()
	err := chromedp.Run(fillCtx, fill)
	cancelFill()
	if err != nil {
		path, _ := screenshot(taskCtx, cfg.ScreenshotPath)
		return fail(fmt.Errorf("fill form (screenshot: %s): %w", path, err))
	}
	slog.InfoContext(ctx, "form filled", slog.String("took", time.Since(started).Round(time.Millisecond).String()))

	validateCtx, cancelValidate := deadline()
	err = checkValidation(validateCtx)
	cancelValidate()
	if err != nil {
		path, _ := screenshot(taskCtx, cfg.ScreenshotPath)
		return fail(fmt.Errorf("%w (screenshot: %s)", err, path))
	}
	slog.DebugContext(ctx, "client-side validation clean")

	if cfg.DryRun {
		path, _ := screenshot(taskCtx, cfg.ScreenshotPath)
		slog.InfoContext(ctx, "dry run: stopping before submit", slog.String("screenshot", path))
		closeBrowser()
		return &Result{ScreenshotPath: path}, nil
	}

	slog.InfoContext(ctx, "submitting form")
	submitCtx, cancelSubmit := deadline()
	err = chromedp.Run(submitCtx, chromedp.Click(`.publicForm-submit button`, chromedp.ByQuery))
	cancelSubmit()
	if err != nil {
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

	// Log both inputs to that heuristic, not just the verdict — when it
	// gets this wrong (and it's unvalidated, so it will), knowing which
	// half disagreed is the whole diagnosis.
	slog.InfoContext(ctx, "submit result",
		slog.Bool("success", result.Success),
		slog.Bool("form_gone", formGone),
		slog.Any("invalid_fields", stillInvalid),
		slog.String("screenshot", result.ScreenshotPath))

	var confirmation string
	_ = chromedp.Run(taskCtx, chromedp.Evaluate(
		`(document.body.innerText.match(/(confirmation|success|succ[eè]s)[^\n]*/i) || [''])[0]`,
		&confirmation,
	))
	result.ConfirmationText = strings.TrimSpace(confirmation)

	// Leave the window open for inspection if it looks like the submit
	// didn't succeed; otherwise there's nothing left to check, close it.
	if !cfg.Headful || result.Success {
		closeBrowser()
	}

	return result, nil
}

// typeInto focuses the field, selects any existing text, and types value
// via real key events so masked/segmented Kendo widgets (phone, date,
// numeric) see and format each keystroke as a user would produce it.
func typeInto(id, value string) chromedp.Action {
	sel := "#" + id
	return chromedp.ActionFunc(func(ctx context.Context) error {
		if err := chromedp.Run(ctx,
			chromedp.Focus(sel, chromedp.ByID),
			chromedp.Evaluate(fmt.Sprintf(`document.getElementById(%q).select()`, id), nil),
			chromedp.SendKeys(sel, value, chromedp.ByID),
		); err != nil {
			// Name the field in the error. Without this a broken selector
			// surfaces as a bare "could not find node", with nothing to say
			// which of the dozen fields it was.
			return fmt.Errorf("type into %s: %w", id, err)
		}
		slog.DebugContext(ctx, "field typed", slog.String("id", id), slog.String("value", value))
		return nil
	})
}

// dateDigitSettle is how long fillDate waits after each individual
// keystroke. CDP's dispatchKeyEvent returns as soon as the event is
// dispatched, not after the page's (React/Kendo) event handlers have
// finished re-rendering, so keystrokes sent back-to-back get dropped by
// the widget mid-render. Observed against the live form: typing the year
// as one four-character SendKeys landed 2 or 3 of the 4 digits, or the
// right count in the wrong order ("2620"), varying run to run. Typing one
// digit at a time with a pause between them is what makes this reliable.
const dateDigitSettle = 120 * time.Millisecond

// dateSegmentSettle is the longer pause used around segment boundaries
// (clicking in, Home, ArrowRight), where the widget does more work than
// for a single digit.
const dateSegmentSettle = 300 * time.Millisecond

// maxDateFillAttempts bounds how many times fillDate retypes the date
// before giving up (or handing off to onMismatch, if set).
const maxDateFillAttempts = 3

// dateDigitGroups pulls the numeric segments out of the DateInput's
// displayed value. The live form renders French placeholders for unfilled
// segments in a yyyy-MM-dd layout ("annee-mois-jour", e.g. a half-typed
// value looks like "202-mois-06"), so matching digit runs and comparing
// them positionally — rather than parsing the whole string as a date —
// lets fillDate tell "not filled in yet" from "filled in wrong".
var dateDigitGroups = regexp.MustCompile(`\d+`)

// fillDate fills the Kendo DateInput inside the OPPStart date picker and
// verifies the result before moving on. Its value is segmented
// (year/month/day, format yyyy-MM-dd) and only responds to keyboard
// navigation, not text selection or typed separators: the widget does NOT
// auto-advance to the next segment once one is filled, so each segment
// must be typed and then explicitly advanced past with ArrowRight.
//
// Each segment's digits are typed one at a time via KeyEvent rather than
// in one SendKeys call, for two reasons found by running this against the
// live form: SendKeys re-focuses the input before dispatching (chromedp's
// KeyEventNode calls dom.Focus), which resets the caret back to the first
// segment and lands the month/day digits in the year; and back-to-back
// keystrokes get dropped mid-render (see dateDigitSettle).
//
// Even so, the typing is racy enough to be worth verifying, so fillDate
// reads the field back afterward and retypes it from scratch (up to
// maxDateFillAttempts times) until the value matches what was requested.
// If it still doesn't match, onMismatch — if non-nil — gets one chance to
// fix things (e.g. pause for a human to edit the field by hand in a
// --headful window) before a final check. That handoff and re-check run on
// a context detached from the overall run deadline, since a human at a
// keyboard will routinely take longer than the remaining timeout. A nil
// onMismatch, or one whose fix still doesn't take, fails the run with the
// requested vs. actual value so the operator knows exactly what's wrong
// instead of a generic "invalid field" error.
func fillDate(id, isoDate string, onMismatch func(ctx context.Context, fieldID, expected, actual string) error) chromedp.Action {
	sel := "#" + id

	return chromedp.ActionFunc(func(ctx context.Context) error {
		parts := strings.SplitN(isoDate, "-", 3) // [yyyy, MM, dd]
		if len(parts) != 3 {
			return fmt.Errorf("field %s: start date %q is not formatted YYYY-MM-DD", id, isoDate)
		}

		var actual string
		var lastErr error
		for attempt := 1; attempt <= maxDateFillAttempts; attempt++ {
			if err := chromedp.Run(ctx, typeDate(sel, parts)); err != nil {
				// Out of time or the page went away — no point retyping,
				// but still give onMismatch its shot below.
				lastErr = fmt.Errorf("type date into %s (attempt %d/%d): %w", id, attempt, maxDateFillAttempts, err)
				break
			}

			if err := chromedp.Run(ctx, chromedp.Value(sel, &actual, chromedp.ByID)); err != nil {
				lastErr = fmt.Errorf("read back %s: %w", id, err)
				break
			}
			if dateValueMatches(actual, parts) {
				slog.DebugContext(ctx, "date field filled", slog.String("id", id), slog.Int("attempt", attempt), slog.String("value", actual))
				return nil
			}
			slog.DebugContext(ctx, "date field mismatch, retrying", slog.String("id", id), slog.Int("attempt", attempt), slog.String("got", actual), slog.Any("want_parts", parts))
		}

		// A driver-level failure (deadline exceeded, target crashed, page
		// navigated away) is not a value mismatch, and handing it to
		// onMismatch would tell the operator to go fix a field by hand in
		// a browser that may not even be there. Report it as itself.
		if lastErr != nil {
			return lastErr
		}

		if onMismatch == nil {
			return fmt.Errorf("field %s: could not fill in date %s automatically after %d attempts (field shows %q)", id, isoDate, maxDateFillAttempts, actual)
		}

		// Detach from the phase deadline: the operator is about to be asked
		// to edit the field by hand, which takes as long as it takes. This
		// only works because Submit starts Chrome on an untimed context —
		// WithoutCancel drops the deadline but cannot revive a browser that
		// a deadline already killed. See the `deadline` helper in Submit.
		fixCtx := context.WithoutCancel(ctx)
		if err := onMismatch(fixCtx, id, isoDate, actual); err != nil {
			return err
		}

		verifyCtx, cancel := context.WithTimeout(fixCtx, 15*time.Second)
		defer cancel()
		if err := chromedp.Run(verifyCtx, chromedp.Value(sel, &actual, chromedp.ByID)); err != nil {
			return fmt.Errorf("read back %s after manual fix: %w", id, err)
		}
		if dateValueMatches(actual, parts) {
			slog.DebugContext(ctx, "date field fixed by hand", slog.String("id", id), slog.String("value", actual))
			return nil
		}
		return fmt.Errorf("field %s: still shows %q, not %s, after %d automatic attempts and a manual fix", id, actual, isoDate, maxDateFillAttempts)
	})
}

// typeDate clicks into the date field and types each segment digit by
// digit, advancing between segments with ArrowRight. It deliberately uses
// selector-less KeyEvent calls so nothing re-focuses (and thereby resets
// the caret on) the input mid-sequence — see fillDate's comment.
func typeDate(sel string, parts []string) chromedp.Action {
	acts := []chromedp.Action{
		// The action before this one selects an option in the Reason
		// ComboBox, whose popup is a body-level portal that animates
		// closed. Click dispatches a real mouse event at the date field's
		// coordinates, so clicking while that popup is still on top lands
		// on the popup instead and the caret never enters the DateInput.
		// Wait for it to get out of the way.
		chromedp.Sleep(dateSegmentSettle),
		chromedp.Click(sel, chromedp.ByID),
		chromedp.Sleep(dateSegmentSettle),
	}
	// Clear each segment so a retry starts from a known-empty field rather
	// than relying on typing to overwrite whatever the failed attempt left
	// behind. Delete only clears the segment the caret is in, so this walks
	// all three; pressing it once (even after End) measurably clears
	// nothing — verified against the live form, where the value was
	// unchanged afterward, versus "année-mois-jour" with the walk.
	acts = append(acts, chromedp.KeyEvent(kb.Home))
	for range parts {
		acts = append(acts,
			chromedp.KeyEvent(kb.Delete),
			chromedp.Sleep(dateDigitSettle),
			chromedp.KeyEvent(kb.ArrowRight),
		)
	}
	acts = append(acts, chromedp.KeyEvent(kb.Home), chromedp.Sleep(dateSegmentSettle))
	for i, part := range parts {
		if i > 0 {
			acts = append(acts, chromedp.KeyEvent(kb.ArrowRight), chromedp.Sleep(dateSegmentSettle))
		}
		for _, digit := range part {
			acts = append(acts, chromedp.KeyEvent(string(digit)), chromedp.Sleep(dateDigitSettle))
		}
	}
	return chromedp.Tasks(acts)
}

// dateValueMatches reports whether value's numeric segments equal parts
// ([yyyy, MM, dd]) by value, ignoring separators, locale ordering
// assumptions, and zero-padding differences (e.g. "8" vs "08") — it
// requires exactly three digit groups and each to match its corresponding
// wanted part by position, since fillDate always types year, then month,
// then day, in that order.
func dateValueMatches(value string, parts []string) bool {
	groups := dateDigitGroups.FindAllString(value, -1)
	if len(groups) != len(parts) {
		return false
	}
	for i, want := range parts {
		wantN, err := strconv.Atoi(want)
		if err != nil {
			return false
		}
		gotN, err := strconv.Atoi(groups[i])
		if err != nil {
			return false
		}
		if wantN != gotN {
			return false
		}
	}
	return true
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
		// Log which of how many options was picked: when the site adds or
		// reorders entries, "matched 1 of 40" vs "1 of 1" is what tells you
		// whether the filter behaved as expected.
		slog.DebugContext(ctx, "combobox option selected",
			slog.String("id", id), slog.String("value", value),
			slog.Int("option_index", idx), slog.Int("options_offered", len(texts)))
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
		slog.WarnContext(ctx, "form reports invalid fields", slog.Any("fields", invalid))
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
