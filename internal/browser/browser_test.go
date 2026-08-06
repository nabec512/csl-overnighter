package browser

import (
	"context"
	"strings"
	"testing"
)

func TestDateValueMatches(t *testing.T) {
	parts := []string{"2026", "08", "06"}

	for _, tc := range []struct {
		name  string
		value string
		want  bool
	}{
		{name: "exact", value: "2026-08-06", want: true},
		{name: "unpadded month and day", value: "2026-8-6", want: true},
		{name: "other separators", value: "2026/08/06", want: true},

		// The states actually observed on the live form while the year
		// segment was being mangled — every one of these must read as "not
		// filled in correctly" so the retry loop keeps going.
		{name: "empty placeholders", value: "année-mois-jour", want: false},
		{name: "partial year, month placeholder", value: "202-mois-06", want: false},
		{name: "truncated year", value: "202-08-06", want: false},
		{name: "two-digit year", value: "26-08-06", want: false},
		{name: "scrambled year", value: "2620-08-06", want: false},

		{name: "empty", value: "", want: false},
		{name: "wrong day", value: "2026-08-07", want: false},
		{name: "wrong order", value: "06-08-2026", want: false},
		{name: "extra digit group", value: "2026-08-06 12:00", want: false},
		{name: "too few groups", value: "2026-08", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := dateValueMatches(tc.value, parts); got != tc.want {
				t.Errorf("dateValueMatches(%q, %v) = %v, want %v", tc.value, parts, got, tc.want)
			}
		})
	}
}

// TestFillDateRejectsMalformedStart covers the guard that replaced an
// unchecked parts[1]/parts[2] index, which used to panic inside the
// chromedp action rather than returning an error.
func TestFillDateRejectsMalformedStart(t *testing.T) {
	for _, start := range []string{"", "2026", "2026-08"} {
		action := fillDate("OPPStart", start, nil)

		// No browser is needed: the format check runs before anything
		// touches chromedp.
		err := action.Do(context.TODO())
		if err == nil {
			t.Errorf("fillDate(%q): expected an error, got nil", start)
			continue
		}
		if !strings.Contains(err.Error(), "YYYY-MM-DD") {
			t.Errorf("fillDate(%q): error should name the expected format, got: %v", start, err)
		}
	}
}

func TestOnlyDigits(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{in: "(514) 291-1170", want: "5142911170"},
		{in: "514.291.1170", want: "5142911170"},
		{in: "5142911170", want: "5142911170"},
		{in: "", want: ""},
		{in: "no digits here", want: ""},
	} {
		if got := onlyDigits(tc.in); got != tc.want {
			t.Errorf("onlyDigits(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
