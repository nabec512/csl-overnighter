package cli

import (
	"bytes"
	"strings"
	"testing"
)

// runRoot executes the command tree with args, capturing everything it
// writes, and returns (output, error).
func runRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

// TestUsageShownForArgErrors covers the two halves of the SilenceUsage
// split: a malformed invocation should still explain how to use the
// command, while a failure from inside the command should not bury its
// error under a wall of usage text.
func TestUsageShownForArgErrors(t *testing.T) {
	t.Run("missing argument prints usage", func(t *testing.T) {
		out, err := runRoot(t, "run")
		if err == nil {
			t.Fatal("expected an error for a missing profile name")
		}
		if !strings.Contains(out, "Usage:") {
			t.Errorf("arg error should print usage, got: %q", out)
		}
	})

	t.Run("unknown flag prints usage", func(t *testing.T) {
		out, err := runRoot(t, "run", "someprofile", "--nope")
		if err == nil {
			t.Fatal("expected an error for an unknown flag")
		}
		if !strings.Contains(out, "Usage:") {
			t.Errorf("flag error should print usage, got: %q", out)
		}
	})

	t.Run("runtime failure does not print usage", func(t *testing.T) {
		// A bad --log-level fails inside PersistentPreRunE, i.e. after
		// cobra has accepted the invocation itself.
		out, err := runRoot(t, "run", "someprofile", "--log-level", "chatty")
		if err == nil {
			t.Fatal("expected an error for an invalid log level")
		}
		if strings.Contains(out, "Usage:") {
			t.Errorf("runtime error should not print usage, got: %q", out)
		}
	})
}

// TestValidationRejectsBadRunFlags checks the run command's own input
// checks fire before anything tries to start a browser.
func TestValidationRejectsBadRunFlags(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "duration too high", args: []string{"run", "p", "--duration", "4"}, want: "--duration"},
		{name: "duration too low", args: []string{"run", "p", "--duration", "0"}, want: "--duration"},
		{name: "malformed start", args: []string{"run", "p", "--start", "aug 6"}, want: "--start"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runRoot(t, tc.args...)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error should mention %s, got: %v", tc.want, err)
			}
		})
	}
}
