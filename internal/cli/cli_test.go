package cli

import (
	"flag"
	"reflect"
	"testing"
)

func TestParseFlagsPermutesInterleavedArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantGeo  string
		wantLim  int
		wantDup  bool
		wantArgs []string
	}{
		{
			name:     "flags before positionals",
			args:     []string{"--geo", "paris", "--limit", "3", "eiffel tower"},
			wantGeo:  "paris",
			wantLim:  3,
			wantArgs: []string{"eiffel tower"},
		},
		{
			name:     "flags after positionals",
			args:     []string{"eiffel tower", "--geo", "paris", "--limit", "3"},
			wantGeo:  "paris",
			wantLim:  3,
			wantArgs: []string{"eiffel tower"},
		},
		{
			name:     "interleaved",
			args:     []string{"--limit", "3", "eiffel tower", "--geo", "paris"},
			wantGeo:  "paris",
			wantLim:  3,
			wantArgs: []string{"eiffel tower"},
		},
		{
			name:     "equals form",
			args:     []string{"eiffel", "--geo=paris"},
			wantGeo:  "paris",
			wantLim:  10,
			wantArgs: []string{"eiffel"},
		},
		{
			name:     "bool flag does not swallow the next token",
			args:     []string{"louvre", "--allow-duplicates", "--geo", "paris"},
			wantGeo:  "paris",
			wantLim:  10,
			wantDup:  true,
			wantArgs: []string{"louvre"},
		},
		{
			name:     "double dash ends flag parsing",
			args:     []string{"--geo", "paris", "--", "--not-a-flag"},
			wantGeo:  "paris",
			wantLim:  10,
			wantArgs: []string{"--not-a-flag"},
		},
		{
			name:     "multiple positionals preserve order",
			args:     []string{"GET", "/api/user", "--geo", "x"},
			wantGeo:  "x",
			wantLim:  10,
			wantArgs: []string{"GET", "/api/user"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			geo := fs.String("geo", "", "")
			limit := fs.Int("limit", 10, "")
			dup := fs.Bool("allow-duplicates", false, "")

			if err := parseFlags(fs, tc.args); err != nil {
				t.Fatalf("parseFlags(%q) returned error: %v", tc.args, err)
			}
			if *geo != tc.wantGeo {
				t.Errorf("geo = %q, want %q", *geo, tc.wantGeo)
			}
			if *limit != tc.wantLim {
				t.Errorf("limit = %d, want %d", *limit, tc.wantLim)
			}
			if *dup != tc.wantDup {
				t.Errorf("allow-duplicates = %v, want %v", *dup, tc.wantDup)
			}
			if !reflect.DeepEqual(fs.Args(), tc.wantArgs) {
				t.Errorf("positionals = %q, want %q", fs.Args(), tc.wantArgs)
			}
		})
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"/api/user", "/api/user"},
		{"api/user", "/api/user"},
		// Git Bash rewrites leading-slash args into Windows paths.
		{"C:/Program Files/Git/api/user", "/api/user"},
		{`C:\Program Files\Git\api\tripPlans`, "/api/tripPlans"},
		{"/api/tripPlans/abc/sections", "/api/tripPlans/abc/sections"},
	}
	for _, tc := range tests {
		if got := normalizePath(tc.in); got != tc.want {
			t.Errorf("normalizePath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseNear(t *testing.T) {
	lat, lng, err := parseNear("48.8566, 2.3522")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lat != 48.8566 || lng != 2.3522 {
		t.Errorf("got (%v, %v), want (48.8566, 2.3522)", lat, lng)
	}

	for _, bad := range []string{"48.8566", "", "a,b", "1,2,3"} {
		if _, _, err := parseNear(bad); err == nil {
			t.Errorf("parseNear(%q) should have failed", bad)
		}
	}
}
