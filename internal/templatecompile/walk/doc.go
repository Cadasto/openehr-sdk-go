// Package walk provides a Visitor abstraction over the compiled
// OPT tree produced by [internal/templatecompile.Compile]. Consumers
// such as the composition builder, validator, example generator and
// serialisation walkers share it, so none of them reimplements
// depth-first traversal, pre/post-order hook semantics or subtree
// pruning.
//
// The package is internal, like [internal/templatecompile].
//
// # Walk semantics
//
//   - Depth-first traversal of every [templatecompile.CompiledNode]
//     reachable via [templatecompile.CompiledNode.Attributes] +
//     [templatecompile.CompiledAttribute.Children]. The walker visits
//     nodes, not attributes: a [templatecompile.CompiledAttribute]
//     is never passed to [Visitor.PreHandle] / [Visitor.PostHandle].
//     Visitors that need to enumerate attributes (including the
//     implicit rminfo-injected ones) must call ctx.Node().Attributes()
//     themselves in PreHandle.
//   - Pre-order ([Visitor.PreHandle]) fires before any child is
//     visited; post-order ([Visitor.PostHandle]) fires after every
//     child has been visited.
//   - Returning [SkipSubtree] from PreHandle prunes the subtree:
//     no children are visited and PostHandle is not fired. Sibling
//     traversal continues. [SkipSubtree] returned from PostHandle has
//     no special meaning and aborts the walk like any other non-nil
//     error.
//   - Any other non-nil error from either hook aborts the walk
//     immediately and is returned to the caller.
//   - Implicit attributes (rminfo-injected) have an empty Children
//     slice; the walker therefore performs no recursion through
//     them. Visitors that want to surface implicit attribute *names*
//     (e.g. for skeleton building) read them via ctx.Node().Attributes()
//     and filter on CompiledAttribute.Implicit().
//   - *Slot leaves are visited once via PreHandle/PostHandle; their
//     opaque slot-fill semantics mean the walker does not descend
//     into Includes / Excludes assertions.
//
// # Not supported
//
//   - A lockstep walk over an OPT and an RM instance.
//   - Choice handling for sibling nodes representing an RM type
//     choice (e.g. DV_TEXT | DV_CODED_TEXT under the same path).
//     The compiler does not surface a choice group on CompiledNode.
//   - A collect-all walk: fail-fast Walk is the only variant.
//     Validators that want to collect every diagnostic can
//     accumulate inside the visitor and never return a non-nil err.
package walk
