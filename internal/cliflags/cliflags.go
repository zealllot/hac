package cliflags

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

type Flags struct {
	Format  string
	Timeout time.Duration
}

func Parse(subcommand string, args []string) (Flags, []string, error) {
	return ParseWith(subcommand, args, nil)
}

// ParseWith is the Parse variant that lets a subcommand register its own flags.
// `setup` is invoked with the FlagSet before parsing, e.g.:
//
//	var createCategory bool
//	flags, rest, err := cliflags.ParseWith("deploy", args, func(fs *flag.FlagSet) {
//	    fs.BoolVar(&createCategory, "create-category", false, "auto-create missing HA category")
//	})
func ParseWith(subcommand string, args []string, setup func(*flag.FlagSet)) (Flags, []string, error) {
	flagArgs, positionals := splitArgs(args)

	fs := flag.NewFlagSet(subcommand, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	format := fs.String("format", "json", "json or table")
	timeout := fs.Duration("timeout", 30*time.Second, "HA request timeout")
	if setup != nil {
		setup(fs)
	}

	if err := fs.Parse(flagArgs); err != nil {
		return Flags{}, nil, err
	}
	if *format != "json" && *format != "table" {
		return Flags{}, nil, fmt.Errorf("invalid --format %q: must be json or table", *format)
	}
	positionals = append(positionals, fs.Args()...)
	return Flags{Format: *format, Timeout: *timeout}, positionals, nil
}

// splitArgs partitions args into flag tokens (consumed by flag.FlagSet.Parse)
// and positional tokens. Flag tokens are anything starting with "-" plus the
// next arg if the flag did not embed its value via "=".
func splitArgs(args []string) (flagArgs, positionals []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			positionals = append(positionals, a)
			continue
		}
		flagArgs = append(flagArgs, a)
		// If form is "--name=value", value is embedded — nothing more to pull.
		if strings.Contains(a, "=") {
			continue
		}
		// Otherwise, the next arg is the value, unless it looks like another flag.
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			i++
			flagArgs = append(flagArgs, args[i])
		}
	}
	return
}
