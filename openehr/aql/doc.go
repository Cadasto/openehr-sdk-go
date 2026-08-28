// Package aql provides:
//
//   - A struct-builder AQL builder and a verb-functions builder — both
//     produce identical AQL on the wire.
//   - AQL request and result models usable without an executor.
//   - Shared sentinels: ErrInvalidQuery, ErrPathResolution and
//     ErrEngineCapability (both execute-time), ErrSyntax (parse-time), and
//     ErrIncompleteAST (structured-AST residual after a clean parse).
//
// Since REQ-163 the builder spells the `standardPredicate` and
// `archetypePredicate` bracket alternatives in class position, plus the whole
// projection-position vocabulary the read side models, so a query with a version
// predicate, a standing class predicate or a typed projection no longer needs a
// hand-assembled string:
//
//	// … CONTAINS VERSION v[LATEST_VERSION]
//	Contains(Version("v", LatestVersion()))
//	// … CONTAINS VERSIONED_COMPOSITION vo[uid/value = $vo]
//	Contains(Class("VERSIONED_COMPOSITION", "vo").Predicated("uid/value", OpEq, Param("vo")))
//	// SELECT DISTINCT COUNT(*) AS n …
//	Select(CountStar().As("n")).Distinct()
//
// The version bracket's three shapes are [LatestVersion], [AllVersions] and
// [VersionCompare], carried onto the class by [Version]; the class bracket's one
// comparison by [Containment.Predicated]; the projection by [ColAs], [Star],
// [Count], [CountDistinct], [CountStar], [Fn], [Lit] and [SelectField.As],
// beside the clause-level [Builder.Distinct].
//
// The class bracket's OTHER alternatives stay unspelled and are out of scope for
// REQ-163 — a node id, a name slot, a regex match, an AND / OR `nodePredicate`
// junction. All are legal AQL the read side already structures, so the
// read/write parity this package claims is bounded by the two alternatives named
// above; a query needing one of the rest still needs hand-assembled text.
//
// [Col] keeps its signature and its verbatim splicing, and stays the legacy
// route. What DID change with REQ-163 is one previously-accepted behaviour,
// classified semver-minor: a Col whose text splits the projection into more
// items, spills into another clause, or introduces a clause-level flag the
// builder never recorded now fails [Builder.Build] where it used to emit. See
// the SELECT verification below, and the CHANGELOG entry that calls it out.
//
// The executor lives at openehr/client/query and wraps this package.
// Parsing and static lint of AQL strings live in the building-block subpackages
// openehr/aql/parse (syntax → generated-type-free AST) and openehr/aql/lint
// (REQ-109's syntax, shape and template layers, plus REQ-161's RM-semantic
// containment and portability checks over openehr/aql/contain's containment
// relation); validation.ValidateAQL bridges lint into the shared validation
// Issue model. Programs that only need to construct AQL (no execution) can
// import openehr/aql alone.
//
// Three entry points, three different questions, in widening order — none of
// them implies the next:
//
//   - [Builder.Build] answers the SHAPE question: is this representable,
//     canonical AQL? It refuses a tree the grammar cannot carry — plus four
//     write-side rules of its own, each documented where it lives: a
//     containment class node must carry an alias (an ergonomic choice, not a
//     grammar rule — [Containment.validateTree]); paging must not be set on
//     both the in-text and request-envelope channels, which never reaches the
//     emitted text at all (`validatePaging`, REQ-117); the SELECT clause it
//     emitted must read back as the projection it recorded, which is what bounds
//     [Col]'s verbatim splicing (`verifySelectClause`, REQ-163); and a standing
//     predicate is refused on a `VERSION`-spelled node, which has its own bracket
//     production and its own carrier ([Containment.Predicated], REQ-163) —
//     legal AQL routed loudly rather than silently. What Build does NOT ask is
//     the RM question: a query whose classes can never contain one another still
//     builds and emits.
//   - [Builder.VerifyContainment] is the opt-in RM-semantics gate (REQ-162): it
//     walks the builder's own FROM root and containment algebra and reports the
//     REQ-161 containment findings. It takes a nilable REQ-160 containment
//     relation — nil is the default relation, so a caller with no dialect
//     overlay edges passes nil and supplies nothing. Opt-in means opt-in —
//     Build never calls it.
//   - lint.LintString covers the read side, for AQL that arrives as text rather
//     than being built here: the same containment checks over a parsed document,
//     plus the syntax, shape, param, template, and portability layers a
//     builder tree cannot pose. Both sides drive one rule engine, so an
//     equivalent query draws the same containment codes either way.
//
// The SDK grammar profile (ADR 0007) documents deltas from official QUERY
// 1.1.0 — notably SDK-AQL-001 (CONTAINS vs CONTAINS_STR) and SDK-AQL-002
// (SELECT *) — in resources/aql/grammar/DIVERGENCES.md. Parse success and
// lint-clean do not imply official-spec conformance.
package aql
