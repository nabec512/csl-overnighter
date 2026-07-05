# csl-overnighter

CLI that fills in and submits the Cote Saint-Luc overnight parking permit
form using headless Chrome (chromedp).

Form: https://cotesaintluc-publicform.icosolutions.com/publicforms/2

## Build

```sh
go build -o bin/csl-overnighter ./cmd/csl-overnighter
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

See `CLAUDE.md` for architecture notes.
