package main

import (
	"reflect"
	"testing"
)

// The bug this guards against shipped in v1.0.0: flag.Parse stops at the first
// positional argument, so `search "denim shirt" -limit 1 -text` searched for
// the literal string `denim shirt -limit 1 -text` and ignored both flags.
func TestSplitArgsAcceptsFlagsInAnyPosition(t *testing.T) {
	booleans := map[string]bool{"text": true, "stream": true}

	for _, testCase := range []struct {
		name       string
		in         []string
		wantFlags  []string
		wantValues []string
	}{
		{
			name:       "flags after the query",
			in:         []string{"denim shirt", "-limit", "1", "-text"},
			wantFlags:  []string{"-limit", "1", "-text"},
			wantValues: []string{"denim shirt"},
		},
		{
			name:       "flags before the query",
			in:         []string{"-limit", "1", "-text", "denim shirt"},
			wantFlags:  []string{"-limit", "1", "-text"},
			wantValues: []string{"denim shirt"},
		},
		{
			name:       "equals form takes no following token",
			in:         []string{"-limit=2", "coat"},
			wantFlags:  []string{"-limit=2"},
			wantValues: []string{"coat"},
		},
		{
			name:       "double dash ends flag parsing",
			in:         []string{"-text", "--", "-not-a-flag"},
			wantFlags:  []string{"-text"},
			wantValues: []string{"-not-a-flag"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			flags, values := splitArgs(testCase.in, booleans)
			if !reflect.DeepEqual(flags, testCase.wantFlags) {
				t.Errorf("flags = %q, want %q", flags, testCase.wantFlags)
			}
			if !reflect.DeepEqual(values, testCase.wantValues) {
				t.Errorf("positional = %q, want %q", values, testCase.wantValues)
			}
		})
	}
}
