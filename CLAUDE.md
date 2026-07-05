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
in `internal/browser`), but the form-filling code in `internal/browser.go`
has **not been run against the live form** — there's no Chrome/Chromium
binary in the dev sandbox this was written in, so it was written from a
captured DOM snapshot rather than iteratively tested. Treat the field
interaction logic (especially the Kendo ComboBox selection and the date
field) as unverified until someone runs `csl-overnighter run <profile>
--headful --dry-run --screenshot out.png` against the real form and checks
the result. `Result.Success` detection after a real (non-dry-run) submit is
a best-effort heuristic (see `internal/browser/browser.go`) and hasn't been
validated against what the real success/failure page looks like either.

The target form is a Kendo UI (Telerik) React SPA — the server returns an
empty `<div id="root">` shell, all fields render client-side, and most
inputs are Kendo widgets (ComboBox, MaskedTextBox, DatePicker,
NumericTextBox) rather than plain `<input>`/`<select>` elements, which is
why the filling logic in `internal/browser` is more involved than
click-and-type.

## Commands

Go is at `/usr/local/go/bin/go` and is not on `PATH` by default in this
environment; prefix commands with `export PATH=$PATH:/usr/local/go/bin` or
otherwise reference it directly.

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
before clicking Submit). Two field-filling strategies are used depending on
widget type:
- **Plain/masked/date/numeric fields** (`typeInto`): focus, select existing
  text, then send real key events — required so the Kendo MaskedTextBox
  (phone) and DatePicker (start date) see and format each keystroke rather
  than getting a value set out from under them.
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

Before clicking Submit, `checkValidation` re-reads the DOM for any
remaining `.k-invalid`/`aria-invalid` fields and aborts rather than
submitting a form the site itself considers incomplete.

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
