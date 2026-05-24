// Package walk provides a Visitor abstraction over the compiled
// OPT tree produced by [internal/templatecompile.Compile]. Future
// consumers — composition builder (REQ-101), validator (REQ-102),
// example generator, and serialisation walkers — share this
// abstraction so each does not reinvent depth-first traversal,
// pre/post-order hook semantics, or subtree pruning.
//
// REQ-100 follow-up Phase 5. Kept under internal/ alongside
// [internal/templatecompile] until REQ-101 / REQ-102 confirm the
// public API shape; the two will be promoted together.
//
// # Walk semantics
//
//   - Depth-first traversal of every [templatecompile.CompiledNode]
//     reachable via [templatecompile.CompiledNode.Attributes].
//   - Pre-order ([Visitor.PreHandle]) fires before any child is
//     visited; post-order ([Visitor.PostHandle]) fires after every
//     child has been visited.
//   - Returning [SkipSubtree] from PreHandle prunes the subtree:
//     no children are visited and PostHandle is NOT fired. Sibling
//     traversal continues.
//   - Any other non-nil error aborts the walk immediately and is
//     returned to the caller.
//   - Implicit attributes (rminfo-injected, no OPT-declared children)
//     are walked but contribute no recursion since their Children
//     slice is empty.
//   - *Slot leaves are visited once via PreHandle/PostHandle; their
//     opaque slot-fill semantics mean the walker does not descend
//     into Includes / Excludes assertions.
//
// # Out of scope (Phase 5)
//
//   - **WalkComposition** — lockstep walk over an OPT and an RM
//     instance. Deferred until REQ-101 lands the composition shape.
//   - **Choice handling** — sibling nodes representing an RM type
//     choice (e.g. DV_TEXT | DV_CODED_TEXT under the same path).
//     Deferred until the compiler surfaces a Choice group on
//     CompiledNode.
//   - **WalkUntilError / collect-all** — fail-fast Walk is the only
//     variant. Validators that want to collect every diagnostic can
//     accumulate inside the visitor and never return a non-nil err.
package walk
