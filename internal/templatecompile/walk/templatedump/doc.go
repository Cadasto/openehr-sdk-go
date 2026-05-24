// Package templatedump provides reference [walk.Visitor]
// implementations: a pretty-printer that renders a compiled OPT as
// an indented tree, and a path collector that accumulates every
// node's canonical AQL path.
//
// These exist primarily as documentation-by-example for the
// [walk] package — both visitors are short, exercise the
// pre-/post-order hooks, and demonstrate the typical Context
// usage. Worked examples (e.g. cmd/examples/opt-parse) re-use them
// instead of hand-rolling tree-traversal loops.
package templatedump
