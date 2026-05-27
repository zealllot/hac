package cliflags_test

import (
	"flag"
	"reflect"
	"testing"
	"time"

	"github.com/zealllot/hac/internal/cliflags"
)

func TestParseWith_subcommandSpecificFlag(t *testing.T) {
	var createCategory bool
	_, rest, err := cliflags.ParseWith("deploy",
		[]string{"automations/x/y.yaml", "--create-category"},
		func(fs *flag.FlagSet) {
			fs.BoolVar(&createCategory, "create-category", false, "")
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !createCategory {
		t.Errorf("createCategory not set by --create-category flag")
	}
	if len(rest) != 1 || rest[0] != "automations/x/y.yaml" {
		t.Errorf("rest = %v, want [automations/x/y.yaml]", rest)
	}
}

func TestParse_invalidFormatErrors(t *testing.T) {
	_, _, err := cliflags.Parse("state", []string{"--format=xml", "light.x"})
	if err == nil {
		t.Fatalf("expected error for --format=xml")
	}
}

func TestParse(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		wantFormat  string
		wantTimeout time.Duration
		wantRest    []string
	}{
		{
			name:        "defaults",
			args:        []string{"light.x"},
			wantFormat:  "json",
			wantTimeout: 30 * time.Second,
			wantRest:    []string{"light.x"},
		},
		{
			name:        "format=table before positional",
			args:        []string{"--format=table", "light.x"},
			wantFormat:  "table",
			wantTimeout: 30 * time.Second,
			wantRest:    []string{"light.x"},
		},
		{
			name:        "timeout=5s before positional",
			args:        []string{"--timeout=5s", "light.x"},
			wantFormat:  "json",
			wantTimeout: 5 * time.Second,
			wantRest:    []string{"light.x"},
		},
		{
			name:        "multiple positionals preserved",
			args:        []string{"--format=table", "light.a", "light.b"},
			wantFormat:  "table",
			wantTimeout: 30 * time.Second,
			wantRest:    []string{"light.a", "light.b"},
		},
		{
			name:        "format=table after positional",
			args:        []string{"light.x", "--format=table"},
			wantFormat:  "table",
			wantTimeout: 30 * time.Second,
			wantRest:    []string{"light.x"},
		},
		{
			name:        "flag interspersed between positionals",
			args:        []string{"light.a", "--timeout=5s", "light.b"},
			wantFormat:  "json",
			wantTimeout: 5 * time.Second,
			wantRest:    []string{"light.a", "light.b"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, rest, err := cliflags.Parse("state", tc.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Format != tc.wantFormat {
				t.Errorf("Format = %q, want %q", got.Format, tc.wantFormat)
			}
			if got.Timeout != tc.wantTimeout {
				t.Errorf("Timeout = %v, want %v", got.Timeout, tc.wantTimeout)
			}
			if !reflect.DeepEqual(rest, tc.wantRest) {
				t.Errorf("rest = %v, want %v", rest, tc.wantRest)
			}
		})
	}
}
