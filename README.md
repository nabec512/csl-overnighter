# csl-overnighter

CLI that fills in and submits the Cote Saint-Luc overnight parking permit
form using headless Chrome (chromedp).

Form: https://cotesaintluc-publicform.icosolutions.com/publicforms/2

## Install

Prebuilt binaries are published on the [Releases](https://github.com/nabec512/csl-overnighter/releases)
page. To download and install the right one for your machine automatically
(macOS/Linux) into `~/Desktop/CSL-Permit/csl-overnighter`:

```sh
curl -fsSL https://raw.githubusercontent.com/nabec512/csl-overnighter/refs/heads/master/install.sh | bash
```

## Build

```sh
go build -o bin/csl-overnighter ./cmd/csl-overnighter
```

Or, with [go-task](https://taskfile.dev):

```sh
task            # list available tasks
task build      # build into bin/
task check      # gofmt + go vet + go test
task dry        # fill the form in a visible browser, stop before Submit
```

## Usage

Save a profile:

```sh
csl-overnighter profile save driveway \
  --address "123 happy drive, Montreal" \
  --first-name Jane --last-name Doe \
  --phone 5145551234 --email jane@example.com \
  --plate ABC1234 --make Toyota --model Corolla --color Grey \
  --country Canada --state Quebec --reason "No driveway"
```

Manage profiles:

```sh
csl-overnighter profile list
csl-overnighter profile show driveway
csl-overnighter profile delete driveway
```

Run a profile (submits tonight's permit for 1 night by default):

```sh
csl-overnighter run driveway
csl-overnighter run driveway --start 2026-07-20 --duration 2
csl-overnighter run driveway --headful --dry-run --screenshot out.png
```

`--headful` shows the browser; `--dry-run` fills the form and stops before
clicking Submit; `--screenshot` saves the final page state to a PNG.

## Logging

Progress goes to stderr, results to stdout. By default you get a few lines:

```
filling form url=https://... start=2026-08-06 nights=1
form filled took=9.0s
```

Turn it up when something breaks — `-v` logs every field and dropdown
choice (including personal details, so don't paste it around):

```sh
csl-overnighter run driveway -v          # everything
csl-overnighter run driveway --log-level error   # near-silent, for cron
CSL_LOG_LEVEL=error csl-overnighter run driveway # same, via env
```

See `CLAUDE.md` for architecture notes.
