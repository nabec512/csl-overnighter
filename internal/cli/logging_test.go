package cli

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLogLevel(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    slog.Level
		wantErr bool
	}{
		{in: "debug", want: slog.LevelDebug},
		{in: "info", want: slog.LevelInfo},
		{in: "warn", want: slog.LevelWarn},
		{in: "warning", want: slog.LevelWarn},
		{in: "error", want: slog.LevelError},
		{in: "DEBUG", want: slog.LevelDebug},
		{in: "  Info  ", want: slog.LevelInfo},
		{in: "", wantErr: true},
		{in: "verbose", wantErr: true},
		{in: "trace", wantErr: true},
	} {
		got, err := parseLogLevel(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseLogLevel(%q) = %v, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseLogLevel(%q): unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseLogLevel(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestResolveLogLevel(t *testing.T) {
	for _, tc := range []struct {
		name      string
		flagValue string
		flagSet   bool
		verbose   bool
		env       string
		want      slog.Level
		wantErr   bool
	}{
		{name: "default is info", flagValue: "info", want: slog.LevelInfo},
		{name: "verbose means debug", flagValue: "info", verbose: true, want: slog.LevelDebug},
		{name: "explicit flag wins over verbose", flagValue: "warn", flagSet: true, verbose: true, want: slog.LevelWarn},
		{name: "env applies when flag unset", flagValue: "info", env: "error", want: slog.LevelError},
		{name: "explicit flag beats env", flagValue: "info", flagSet: true, env: "debug", want: slog.LevelInfo},
		{name: "verbose beats env", flagValue: "info", verbose: true, env: "error", want: slog.LevelDebug},
		{name: "bad env is an error", flagValue: "info", env: "chatty", wantErr: true},
		{name: "bad flag is an error", flagValue: "chatty", flagSet: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(logLevelEnv, tc.env)

			got, err := resolveLogLevel(tc.flagValue, tc.flagSet, tc.verbose)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveLogLevel = %v, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("resolveLogLevel = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestNewLoggerFiltersByLevel is the property that matters most: at the
// default level a run must not spew the debug detail that used to be
// unconditional.
func TestNewLoggerFiltersByLevel(t *testing.T) {
	var buf bytes.Buffer
	log := newLogger(&buf, slog.LevelInfo)

	log.Debug("field typed", "id", "OPPPhone", "value", "5145551234")
	log.Info("form filled")

	out := buf.String()
	if strings.Contains(out, "5145551234") {
		t.Errorf("debug record leaked at info level: %q", out)
	}
	if !strings.Contains(out, "form filled") {
		t.Errorf("info record missing: %q", out)
	}
}

// TestNewLoggerFormatPerLevel covers the readability split: info output is
// for a human watching a run, so it's one plain line with no time= /
// level= / msg= scaffolding, while debug output keeps the full text
// handler for after-the-fact grepping.
func TestNewLoggerFormatPerLevel(t *testing.T) {
	var info, debug bytes.Buffer

	newLogger(&info, slog.LevelInfo).Info("submitting form", "nights", 2)
	newLogger(&debug, slog.LevelDebug).Debug("field typed", "id", "OPPPhone")

	infoOut, debugOut := info.String(), debug.String()

	if got, want := infoOut, "submitting form nights=2\n"; got != want {
		t.Errorf("info output = %q, want %q", got, want)
	}
	if !strings.Contains(debugOut, "level=DEBUG") || !strings.Contains(debugOut, "time=") {
		t.Errorf("debug output should keep time/level prefixes: %q", debugOut)
	}
}

// TestCompactHandlerMarksWarnings checks that a warning is visually
// distinct from routine progress — at info level everything is info, so
// only warn/error carry a prefix.
func TestCompactHandlerMarksWarnings(t *testing.T) {
	var buf bytes.Buffer
	log := newLogger(&buf, slog.LevelInfo)

	log.Info("form filled")
	log.Warn("form reports invalid fields", "fields", []string{"OPPStart"})

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), buf.String())
	}
	if strings.HasPrefix(lines[0], "INFO") {
		t.Errorf("info line should not be prefixed: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "WARN: ") {
		t.Errorf("warn line should be prefixed: %q", lines[1])
	}
	if !strings.Contains(lines[1], "OPPStart") {
		t.Errorf("warn line lost its attrs: %q", lines[1])
	}
}

// TestCompactHandlerWithAttrs guards the slog.Handler contract: WithAttrs
// must not mutate the receiver's attrs, or loggers derived from a shared
// parent bleed attributes into each other.
func TestCompactHandlerWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	base := newLogger(&buf, slog.LevelInfo)

	a := base.With("run", "a")
	b := base.With("run", "b")

	a.Info("one")
	b.Info("two")
	base.Info("three")

	out := buf.String()
	want := "one run=a\ntwo run=b\nthree\n"
	if out != want {
		t.Errorf("With() leaked attrs between loggers:\ngot  %q\nwant %q", out, want)
	}
}
