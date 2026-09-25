// Package aql builds AQL queries and models AQL requests and results.
//
// It provides:
//
//   - A struct-builder ([NewBuilder]) and a verb-functions builder (Select,
//     From, …). Both produce identical AQL on the wire.
//   - AQL request and result models usable without an executor.
//   - Shared sentinels: ErrInvalidQuery, ErrPathResolution and
//     ErrEngineCapability (both execute-time), ErrSyntax (parse-time), and
//     ErrIncompleteAST (structured-AST residual after a clean parse).
//
// The builder spells the `standardPredicate` and `archetypePredicate` bracket
// alternatives in class position, plus the projection-position vocabulary the
// read side models, so a query with a version predicate, a standing class
// predicate or a typed projection does not need a hand-assembled string:
//
//	// … CONTAINS VERSION v[LATEST_VERSION]
//	Contains(Version("v", LatestVersion()))
//	// … CONTAINS VERSIONED_COMPOSITION vo[uid/value = $vo]
//	Contains(Class("VERSIONED_COMPOSITION", "vo").Predicated("uid/value", OpEq, Param("vo")))
//	// SELECT DISTINCT COUNT(*) AS n …
//	Select(CountStar().As("n")).Distinct()
//
// The version bracket has three shapes, [LatestVersion], [AllVersions] and
// [VersionCompare], carried onto the class by [Version]. The class bracket's
// one comparison is set by [Containment.Predicated]. The projection is built
// from [ColAs], [Star], [Count], [CountDistinct], [CountStar], [Fn], [Lit] and
// [SelectField.As], beside the clause-level [Builder.Distinct].
//
// The other class-bracket alternatives have no builder spelling: a node id, a
// name slot, a regex match, and an AND / OR `nodePredicate` junction. All are
// legal AQL that the parser structures, so read/write parity covers only the
// two alternatives named above. A query needing one of the others still needs
// hand-assembled text.
//
// [Col] splices its text verbatim. [Builder.Build] refuses a Col whose text
// splits the projection into more items, spills into another clause, or
// introduces a clause-level flag the builder never recorded (see the SELECT
// read-back check described below).
//
// The executor lives at openehr/client/query and wraps this package.
// Parsing and static lint of AQL strings live in the subpackages
// openehr/aql/parse (syntax → generated-type-free AST) and openehr/aql/lint
// (syntax, shape and template checks, plus RM-semantic containment and
// portability checks over openehr/aql/contain's containment relation);
// validation.ValidateAQL bridges lint into the shared validation Issue model.
// Programs that only need to construct AQL (no execution) can import
// openehr/aql alone.
//
// Three entry points answer three different questions, in widening order. None
// of them implies the next:
//
//   - [Builder.Build] answers the shape question: is this representable,
//     canonical AQL? It refuses a tree the grammar cannot carry, plus four
//     write-side rules of its own: a containment class node must carry an
//     alias (an ergonomic choice; the grammar does not require it); paging
//     must not be set on both the in-text and request-envelope channels; the
//     SELECT clause it emitted must read back as the projection it recorded,
//     which bounds [Col]'s verbatim splicing; and a standing predicate is
//     refused on a `VERSION`-spelled node, which has its own bracket
//     production and its own carrier ([Containment.Predicated]). Build does
//     not check the RM: a query whose classes can never contain one another
//     still builds and emits.
//   - [Builder.VerifyContainment] is the opt-in RM-semantics check: it walks
//     the builder's own FROM root and containment algebra and reports
//     containment findings. It takes a nilable containment relation; nil
//     selects the default relation, so a caller with no dialect overlay edges
//     passes nil. Build never calls it.
//   - lint.LintString covers the read side, for AQL that arrives as text rather
//     than being built here: the same containment checks over a parsed
//     document, plus the syntax, shape, param, template, and portability
//     layers a builder tree cannot pose. Both sides drive one rule engine, so
//     an equivalent query draws the same containment codes either way.
//
// The SDK grammar profile documents deltas from the official openEHR QUERY
// 1.1.0 grammar, notably SDK-AQL-001 (CONTAINS vs CONTAINS_STR) and
// SDK-AQL-002 (SELECT *), in resources/aql/grammar/DIVERGENCES.md. Parse
// success and lint-clean do not imply official-spec conformance.
package aql
