package main

import (
	"fmt"
	"strings"
)

// unifiedDiff produces a small unified-style diff between got and
// want. The implementation is intentionally minimal — line-by-line
// LCS would balloon the cmd binary without third-party deps, and
// for drift-detection purposes the consumer only needs to see *that*
// the file differs and roughly *where*. The output is suitable for
// piping into a terminal and reviewing.
func unifiedDiff(path string, got, want []byte) string {
	var b strings.Builder
	fmt.Fprintf(&b, "--- %s (on disk)\n", path)
	fmt.Fprintf(&b, "+++ %s (regenerated)\n", path)
	gotLines := strings.Split(string(got), "\n")
	wantLines := strings.Split(string(want), "\n")
	max := len(gotLines)
	if len(wantLines) > max {
		max = len(wantLines)
	}
	context := 3
	lastDiff := -100
	for i := 0; i < max; i++ {
		var gl, wl string
		if i < len(gotLines) {
			gl = gotLines[i]
		}
		if i < len(wantLines) {
			wl = wantLines[i]
		}
		if gl == wl {
			if i-lastDiff <= context {
				fmt.Fprintf(&b, " %s\n", gl)
			}
			continue
		}
		// Emit a small header when transitioning from equal to
		// different so the reader sees the line number.
		if i-lastDiff > context+1 {
			fmt.Fprintf(&b, "@@ line %d @@\n", i+1)
		}
		if i < len(gotLines) {
			fmt.Fprintf(&b, "-%s\n", gl)
		}
		if i < len(wantLines) {
			fmt.Fprintf(&b, "+%s\n", wl)
		}
		lastDiff = i
	}
	return b.String()
}
