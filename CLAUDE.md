# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A personal CLI tool that fills in and submits the user's town's overnight
on-street parking permit web form, using a headless (or headful) Chrome
browser driven by chromedp. The user saves one or more "profiles" (vehicle
+ household details) and then runs a profile to have the tool drive the
browser through the form and submit it, instead of doing it by hand every
night.

This is a single-user, local-only tool. There is no server component, no
multi-tenancy, and no auth beyond whatever the town's site itself requires.

## Status

Everything is implemented (profile CRUD, CLI plumbing, and form automation
in `internal/browser`).

**Verified against the live form** (2026-08-06, via `task dry:headless`):
every field fills correctly end-to-end — address, suite, name, the masked
phone, email, plate, all six ComboBoxes (make/model/color/country/state/
reason), the start date, and the night count — and `checkValidation` passes
with no `.k-invalid` fields. Confirmed stable across 5 consecutive runs
(`task flake`).

Still **unverified**: everything past the Submit click. `--dry-run` stops
before submitting, so `Result.Success` detection is still the best-effort
heuristic described in `internal/browser/browser.go` and has never been
compared against what the real success/failure page looks like. Check the
screenshot after any real submit rather than trusting `Success`.

The form renders in French with English subtitles; the date field displays
`yyyy-MM-dd` with French placeholders for unfilled segments
(`annee-mois-jour`), which is what `dateValueMatches` is written against.

The target form is a Kendo UI (Telerik) React SPA — the server returns an
empty `<div id="root">` shell, all fields render client-side, and most
inputs are Kendo widgets (ComboBox, MaskedTextBox, DatePicker,
NumericTextBox) rather than plain `<input>`/`<select>` elements, which is
why the filling logic in `internal/browser` is more involved than
click-and-type.

## Commands

There's a `Taskfile.yml` ([go-task](https://taskfile.dev)) wrapping the
common loops — it resolves the Go path itself, so it's the easiest way in:

```sh
task                       # list every task
task check                 # gofmt + go vet + offline tests (the bar below)
task test:live             # also run the tests that drive Chrome at the real form
task dry                   # fill the form in a visible browser, stop before Submit
task dry:headless          # same, no window — writes tmp/dry-headless.png
task flake RUNS=5          # repeat the dry-run to shake out flaky fields
task run -- --duration 2   # extra flags pass through to the CLI
```

Tests that need Chrome skip themselves when no binary is present, and the
ones that hit the live form are behind `-short` (so `task check` stays
offline and `task test:live` doesn't).

Vars are overridable per-invocation: `task dry PROFILE=laura
START=2026-08-09 CHROME=/usr/bin/google-chrome`.

Underneath, Go is at `/usr/local/go/bin/go` and is not on `PATH` by default
in this environment; prefix commands with `export
PATH=$PATH:/usr/local/go/bin` or otherwise reference it directly.

```sh
go build ./...                  # build everything
go run ./cmd/csl-overnighter ... # run without building a binary
go vet ./...                    # static checks
gofmt -l -w .                   # format (run before committing)
go test ./...                   # run all tests
go test ./internal/profile/...  # run tests for one package
go test ./... -run TestName     # run a single test by name
```

There is no separate lint config (no golangci-lint setup) — `go vet` +
`gofmt` is the bar.

## Architecture

```
cmd/csl-overnighter/   main.go — thin entrypoint, delegates to internal/cli
internal/cli/          cobra command tree (root, profile subcommands, run)
internal/profile/      Profile type + Store (JSON-file persistence)
internal/browser/      chromedp-driven form automation
```

**internal/profile** — `Profile` has named fields matching the real form
1:1 (`Address`, `Suite`, `FirstName`, `LastName`, `Phone`, `Email`,
`LicencePlate`, `VehicleMake`, `VehicleModel`, `VehicleColor`, `Country`,
`State`, `Reason`). `Start`/`Duration` are deliberately *not* profile
fields — they change every run, so they're flags on `run` instead (`--start`,
defaulting to today; `--duration`, defaulting to 1). `profile save` exposes
one flag per field (`--address`, `--first-name`, etc.) and overwrites the
whole profile each time it's called — there's no partial-update/patch
mode, so all required flags must be passed together. Profiles are stored as
one JSON file per profile, named `<name>.json`, in `Store.Dir`.
`profile.DefaultDir()` resolves this to
`os.UserConfigDir()/csl-overnighter/profiles` (e.g.
`~/.config/csl-overnighter/profiles` on Linux). Files are written with
`0600` and the directory with `0700` — profile data (plate, address, etc.)
is sensitive personal info but is stored as plain JSON, not encrypted, per
the threat model of "local single-user tool."

**internal/cli** — `NewRootCmd` builds the cobra tree:
`csl-overnighter profile {save,list,show,delete}` and
`csl-overnighter run <profile-name>`. Each command opens a `profile.Store`
via `openStore()` (always at the default dir — there's currently no
`--profile-dir` override flag). `run` validates `--start`/`--duration`,
loads a profile, and calls `browser.Submit`.

**internal/browser** — `Submit(ctx, profile, Config) (*Result, error)` is
the one entrypoint the CLI calls; all form-specific logic (navigation,
selectors, waiting for elements, detecting success vs. failure) lives here
and nowhere else. `Config` carries `Headful`, `Timeout`, `ScreenshotPath`,
`Start`, `Duration`, and `DryRun` (fills the form, screenshots, and stops
before clicking Submit). Three field-filling strategies are used depending
on widget type:
- **Plain/masked/numeric fields** (`typeInto`): focus, select existing
  text, then send real key events — required so the Kendo MaskedTextBox
  (phone) sees and formats each keystroke rather than getting a value set
  out from under it.
- **Kendo ComboBox fields** (`selectCombobox`): Address, Vehicle
  Make/Model/Color, Country, State, and Reason all require picking a listed
  option rather than accepting free text. This function types the target
  value to trigger the widget's live filter, waits for its popup listbox
  (`#<id>list`), and clicks whichever `[role="option"]` matches the value
  case-insensitively — erroring out with the list of available options if
  nothing matches (e.g. a typo, or a value not present in that field's
  list). Country is selected before State because State's option list is
  populated based on the chosen Country (cascading fields) — don't reorder
  those two.
- **The start-date field** (`fillDate` / `typeDate`): the Kendo DateInput
  is segmented (year/month/day) and only responds to keyboard navigation
  (Home, ArrowRight between segments), not text selection or typed
  separators. Two things make it work, both found by running against the
  live form — **don't undo either**:
  1. Digits are typed **one at a time** via selector-less
     `chromedp.KeyEvent`, never `SendKeys`. `SendKeys` re-focuses the node
     first (its `KeyEventNode` calls `dom.Focus`), which resets the caret
     to the first segment so month/day digits land in the year. And
     back-to-back keystrokes get dropped mid-render, so there's a
     `dateDigitSettle` pause after each digit.
  2. The field is cleared (End, Delete, Home) at the start of each attempt,
     so a retry doesn't type into a half-filled segment.

  Before this, the year segment came out truncated or scrambled (`202-`,
  `26-`, `2620-`) on most runs; after, 5/5 runs filled correctly on the
  first attempt. `fillDate` still verifies rather than trusting the typing:
  it reads the field back and retypes from scratch up to
  `maxDateFillAttempts` (3) times, comparing digit groups positionally
  (`dateValueMatches`). If it still doesn't match, `Config.OnDateMismatch`
  (set in `internal/cli/run.go`'s `onDateMismatch`) gets one chance to fix
  things before a final check: in `--headful` mode it pauses and prompts
  the operator to fix the field by hand in the visible browser window and
  press Enter; headless there's no window to fix, so it fails immediately
  with a message pointing at `--headful`. That handoff runs on a
  `context.WithoutCancel` context — a human at a keyboard will routinely
  outlast `--timeout`, and the fix must not be discarded when they don't.
  This only works in combination with the browser-lifetime rule below;
  `WithoutCancel` alone drops a deadline but cannot revive a browser a
  deadline already killed.

Before clicking Submit, `checkValidation` re-reads the DOM for any
remaining `.k-invalid`/`aria-invalid` fields and aborts rather than
submitting a form the site itself considers incomplete.

**Browser lifetime vs. timeouts — don't collapse these back together.**
chromedp allocates the Chrome process on the *first* `Run` against a
context, with `exec.CommandContext` bound to that same context. So putting
the run's deadline on the context used for the first `Run` means the
deadline kills the entire browser, not just the action that overran (their
`Run` doc says as much). `Submit` therefore:
1. starts Chrome with a bare `chromedp.Run(taskCtx)` on the **untimed**
   `taskCtx`, which owns the process; then
2. gives each phase (fill, validate, submit) its own deadline via the
   `deadline()` helper, a `context.WithTimeout` child of `taskCtx`.

`--timeout` is therefore per-phase, not whole-run. This is what lets the
date field's manual-fix pause outlast it. Both halves are pinned by tests
in `lifetime_test.go` — one asserts the broken shape really does kill the
browser, the other that the current shape survives an expired phase
deadline. If chromedp changes, those tests say so.

## Logging

`internal/cli/logging.go` owns log setup; `--log-level`
(debug|info|warn|error), `-v/--verbose` (= debug), and `CSL_LOG_LEVEL` feed
`resolveLogLevel`, in that precedence order. Logs go to **stderr** so
stdout carries only the human result (confirmation line, screenshot path)
and stays pipeable.

- **info** (default) — a few progress lines through `compactHandler`, which
  prints `message key=value` with no time/level/msg scaffolding, since
  those lines are read by a person watching a run. Warn/error get a level
  prefix so they stand out.
- **debug** — the standard `slog.TextHandler` with timestamps and levels,
  plus a line per field filled and per ComboBox option chosen (with how
  many options the filter offered, which is what tells you whether the
  site's lists moved). **Debug logs field values, including phone, email,
  address, and plate** — don't paste it into a bug report unedited.

The root command sets `SilenceUsage`/`SilenceErrors`: a failed run is
almost always a bad value or a moved form, and dumping the whole usage
block after the error just buries it.

## Design decisions already made (don't re-litigate without reason)

- **Storage:** local JSON files, one per profile, no DB.
- **Secrets:** no encryption — plain JSON relying on file permissions,
  since this holds personal info (plate, address) but not a password,
  under the assumption the form itself needs no login. If a future form
  variant requires login credentials, store *those* via an OS keychain
  library rather than adding them to the plain JSON profile.
- **Run mode:** headless by default; `--headful` opts into a visible
  browser window for debugging when selectors break.
- **Scheduling:** out of scope for the tool itself. The user runs it
  on-demand, or wires it into their own cron job later — no built-in
  daemon/scheduler command.
- **Browser driver:** chromedp (not go-rod).
- **CLI framework:** cobra (not stdlib `flag`).
