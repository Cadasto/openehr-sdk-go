// bmmgen is the BMM-driven code generator CLI.
//
// Usage:
//
//	bmmgen [flags]
//	  -resources string   path to resources/bmm/ directory (default "./resources/bmm")
//	  -out string         output module root (default ".")
//	  -target string      comma-separated targets: "rm", "aom14", "all"
//	                      (default "all")
//	  -root string        legacy single-target override (Phase 2 compat);
//	                      overrides only the FIRST target's RootID
//	  -verify             do not write; instead diff against existing
//	                      files; exit 1 on drift
//	  -v                  verbose
//
// The generator reads the pinned BMM JSON files under resources/bmm/ and
// emits Go source under <out>/openehr/rm/ and/or
// <out>/openehr/aom/aom14/ depending on -target. See
// docs/plans/2026-05-15-bmm-codegen.md for the contract and
// internal/bmmgen for the implementation.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/bmmgen"
)

func main() {
	var (
		resources = flag.String("resources", "./resources/bmm", "path to resources/bmm/ directory")
		out       = flag.String("out", ".", "output module root")
		verify    = flag.Bool("verify", false, "verify; do not write; exit 1 on drift")
		rootID    = flag.String("root", "", "legacy: override the first target's BMM root id (without .bmm.json)")
		target    = flag.String("target", "all", "comma-separated targets: rm, aom14, all")
		verbose   = flag.Bool("v", false, "verbose diagnostic output")
	)
	flag.Parse()

	targets, err := selectTargets(*target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bmmgen:", err)
		os.Exit(2)
	}

	result, err := bmmgen.Run(bmmgen.Options{
		ResourcesDir: *resources,
		OutDir:       *out,
		Targets:      targets,
		RootID:       *rootID,
		Verify:       *verify,
		Verbose:      *verbose,
		Stderr:       os.Stderr,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "bmmgen:", err)
		os.Exit(2)
	}
	if *verbose {
		fmt.Fprintf(os.Stderr, "bmmgen: %d files, %d method stubs (%d TODO escapes)\n",
			len(result.Files), result.MethodStubsEmitted, result.MethodTodoEscapes)
		for _, tr := range result.PerTarget {
			fmt.Fprintf(os.Stderr, "  target %s: %d files, %d method stubs (%d TODO escapes)\n",
				tr.Target.GoPackage, len(tr.Files), tr.MethodStubsEmitted, tr.MethodTodoEscapes)
		}
		for _, n := range result.Notes {
			fmt.Fprintln(os.Stderr, "  note:", n)
		}
	}
	if *verify && len(result.Drifts) > 0 {
		fmt.Fprintf(os.Stderr, "bmmgen: drift detected in %d file(s):\n", len(result.Drifts))
		for _, d := range result.Drifts {
			if !d.Existing {
				fmt.Fprintf(os.Stderr, "  MISSING %s\n", d.Path)
				continue
			}
			fmt.Fprintf(os.Stderr, "  DIFFER  %s\n", d.Path)
			diff := unifiedDiff(d.Path, d.Got, d.Want)
			fmt.Fprintln(os.Stderr, indent("    ", diff))
		}
		os.Exit(1)
	}
}

// selectTargets parses the -target flag value into the corresponding
// [bmmgen.Target] slice. "all" expands to [bmmgen.DefaultTargets()].
func selectTargets(spec string) ([]bmmgen.Target, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" || spec == "all" {
		return bmmgen.DefaultTargets(), nil
	}
	var out []bmmgen.Target
	for _, name := range strings.Split(spec, ",") {
		name = strings.TrimSpace(name)
		switch name {
		case "rm":
			out = append(out, bmmgen.TargetRM)
		case "aom14":
			out = append(out, bmmgen.TargetAOM14)
		case "":
			continue
		default:
			return nil, fmt.Errorf("unknown -target %q (want rm, aom14, or all)", name)
		}
	}
	if len(out) == 0 {
		return bmmgen.DefaultTargets(), nil
	}
	return out, nil
}

// indent prefixes every non-empty line of s with prefix.
func indent(prefix, s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l == "" {
			continue
		}
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}
