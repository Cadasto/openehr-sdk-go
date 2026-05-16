// bmmdiff is a small CLI that compares two BMM JSON files and prints
// a human-readable "what changed" summary. Distinct from `git diff`
// because it understands the BMM structure — added/removed classes,
// per-class property additions/removals/changes, cardinality changes,
// function changes, primitive additions/removals.
//
// Usage:
//
//	bmmdiff <old.bmm.json> <new.bmm.json>
//
// Exit code is always 0 (this is an inspection tool, not a CI gate).
// Stderr carries usage errors only; the diff is written to stdout.
//
// Example:
//
//	bmmdiff resources/bmm/openehr_am_1.4.0.bmm.json \
//	         resources/bmm/openehr_am_2.4.0.bmm.json
//
// The diff text is plain ASCII; pipe it into a CHANGELOG draft or a
// PR review. For a suggested CHANGELOG bullet, see
// `internal/bmmdiff.SuggestChangelogEntry`.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cadasto/openehr-sdk-go/internal/bmmdiff"
	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

func main() {
	suggest := flag.Bool("suggest-changelog", false,
		"append a suggested CHANGELOG bullet at the end of the report")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: bmmdiff [flags] <old.bmm.json> <new.bmm.json>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(2)
	}
	oldPath := flag.Arg(0)
	newPath := flag.Arg(1)

	oldS, err := loadFile(oldPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bmmdiff: read %s: %v\n", oldPath, err)
		os.Exit(2)
	}
	newS, err := loadFile(newPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bmmdiff: read %s: %v\n", newPath, err)
		os.Exit(2)
	}

	report := bmmdiff.Diff(oldS, newS)
	fmt.Print(bmmdiff.Format(report))
	if *suggest && report.HasChanges() {
		if entry := bmmdiff.SuggestChangelogEntry(report); entry != "" {
			fmt.Println()
			fmt.Println("Suggested CHANGELOG entry:")
			fmt.Println("  " + entry)
		}
	}
}

func loadFile(path string) (*bmm.Schema, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return bmm.Load(f)
}
