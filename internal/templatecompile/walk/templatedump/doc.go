// Package templatedump provides reference [walk.Visitor]
// implementations: a pretty-printer that renders a compiled OPT as
// an indented tree, and a path collector that accumulates every
// node's canonical AQL path.
//
// These exist primarily as documentation-by-example for the
// [walk] package — both visitors are short, exercise the
// pre-/post-order hooks, and demonstrate the typical Context
// usage. Once [internal/templatecompile] is promoted to a public
// surface (REQ-101 / REQ-102), worked examples under cmd/examples/
// can re-use these visitors directly instead of hand-rolling
// tree-traversal loops.
package templatedump
