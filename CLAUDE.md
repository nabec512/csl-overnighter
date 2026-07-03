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

The form automation (`internal/browser`) is a stub. `browser.Submit` always
returns an error until the target form's URL and field mapping are known.
Everything else (profile CRUD, CLI plumbing) is implemented and working.

**Pending from the user before automation can be implemented:** the permit
form's URL and its field list (names/labels/types of each input, the submit
button, and what a success vs. failure response looks like). Once supplied,
implement navigation and field-filling in `internal/browser/browser.go`
using chromedp selectors, and update `profile.Profile` / the `profile save`
flags if specific fields deserve to be named instead of going through the
generic `--field key=value` mechanism.

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
internal/browser/      chromedp-driven form automation (stub — see Status)
```

**internal/profile** — `Profile` is `{Name, Fields map[string]string,
CreatedAt, UpdatedAt}`. `Fields` is intentionally a generic string map
rather than named struct fields, because the real form's field list isn't
known yet; `profile save` accepts them via repeatable `--field key=value`
flags (parsed by `ParseFieldFlags`). Profiles are stored as one JSON file
per profile, named `<name>.json`, in `Store.Dir`. `profile.DefaultDir()`
resolves this to `os.UserConfigDir()/csl-overnighter/profiles` (e.g.
`~/.config/csl-overnighter/profiles` on Linux). Files are written with
`0600` and the directory with `0700` — profile data (plate, address, etc.)
is sensitive personal info but is stored as plain JSON, not encrypted, per
the threat model of "local single-user tool."

**internal/cli** — `NewRootCmd` builds the cobra tree:
`csl-overnighter profile {save,list,show,delete}` and
`csl-overnighter run <profile-name>`. Each command opens a `profile.Store`
via `openStore()` (always at the default dir — there's currently no
`--profile-dir` override flag). `run` loads a profile and calls
`browser.Submit`.

**internal/browser** — `Submit(ctx, profile, Config) (*Result, error)` is
the one entrypoint the CLI calls. `Config` carries `Headful` (run with a
visible window — default is headless), `Timeout`, and `ScreenshotPath`.
`Result` carries `Success`, `ConfirmationText`, and `ScreenshotPath`. The
intent is that all form-specific logic (navigation, selectors, waiting for
elements, detecting success vs. failure) lives inside this package and
nowhere else — `internal/cli` should stay form-agnostic.

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
