package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// logLevelEnv lets the log level be set without passing a flag every time,
// which is handy for a tool that ends up wrapped in a cron line or a shell
// alias. The --log-level flag wins if both are set.
const logLevelEnv = "CSL_LOG_LEVEL"

// parseLogLevel maps a human-written level name to a slog.Level. It accepts
// the four slog names case-insensitively, plus "warning" as an alias for
// "warn", and reports the valid set on anything else so a typo in a cron
// line produces an actionable error rather than a silent fallback.
func parseLogLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid log level %q (want one of: debug, info, warn, error)", s)
	}
}

// resolveLogLevel picks the effective level from the --log-level flag, the
// --verbose shorthand, and the CSL_LOG_LEVEL environment variable, in that
// order of precedence. flagSet reports whether --log-level was actually
// passed, so that an explicit "--log-level info" beats CSL_LOG_LEVEL=debug
// while an unpassed flag's default does not.
func resolveLogLevel(flagValue string, flagSet, verbose bool) (slog.Level, error) {
	if flagSet {
		return parseLogLevel(flagValue)
	}
	if verbose {
		return slog.LevelDebug, nil
	}
	if env := os.Getenv(logLevelEnv); env != "" {
		level, err := parseLogLevel(env)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", logLevelEnv, err)
		}
		return level, nil
	}
	return slog.LevelInfo, nil
}

// newLogger builds the handler used for all of the tool's logging. Logs go
// to stderr so that stdout carries only the human-facing result (a
// confirmation line, a screenshot path), keeping `csl-overnighter run ...
// > out.txt` and shell pipelines useful.
//
// At debug it's the standard text handler: that output exists to be
// grepped and correlated after the fact, so it keeps timestamps and
// levels. At info and above it's compactHandler, because those few lines
// are progress meant to be read by a person watching a run happen.
func newLogger(w io.Writer, level slog.Level) *slog.Logger {
	if level <= slog.LevelDebug {
		return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
	}
	return slog.New(&compactHandler{w: w, level: level})
}

// compactHandler prints one readable line per record: the message, then any
// attributes as key=value. Warnings and errors get a level prefix so they
// stand out from routine progress; info doesn't, since at info every line
// is info and the prefix is pure noise.
type compactHandler struct {
	w     io.Writer
	level slog.Level
	attrs []slog.Attr
}

func (h *compactHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level }

func (h *compactHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	if r.Level >= slog.LevelWarn {
		b.WriteString(r.Level.String())
		b.WriteString(": ")
	}
	b.WriteString(r.Message)

	writeAttr := func(a slog.Attr) {
		b.WriteByte(' ')
		b.WriteString(a.Key)
		b.WriteByte('=')
		fmt.Fprintf(&b, "%v", a.Value.Any())
	}
	for _, a := range h.attrs {
		writeAttr(a)
	}
	r.Attrs(func(a slog.Attr) bool {
		writeAttr(a)
		return true
	})

	b.WriteByte('\n')
	_, err := io.WriteString(h.w, b.String())
	return err
}

func (h *compactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &next
}

// WithGroup is unused by this tool; grouping would only complicate the
// one-line-per-record format, so groups are flattened away.
func (h *compactHandler) WithGroup(string) slog.Handler { return h }
