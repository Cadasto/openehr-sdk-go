// Package rminfo answers "what does the openEHR Reference Model
// declare about RM class X?" — required attributes, attribute RM
// types, multi-cardinality flags, the universe of known class names,
// and (REQ-048) the class graph over them: abstractness, immediate
// parents, transitive ancestors, conformance, the concrete classes an
// abstract class denotes, and the class each attribute is declared on.
//
// The attribute questions are on the [Lookup] interface; the class-graph
// questions are on the optional [Hierarchy] capability interface, which
// [Default] implements and consumers reach by type assertion. Why a name is
// NOT in the class universe (REQ-049) is on the optional [AbsenceReporter],
// reached the same way.
//
// The data is generated from the pinned BMM under resources/bmm/
// via internal/bmmgen and lives in lookup_gen.go; the hand-written
// surface is the [Lookup], [Hierarchy], [AttributeLister] and
// [AbsenceReporter] interfaces, the [ClassMeta] and [AttrMeta] data
// types, and the [Default], [New] and [NewWithAbsence] accessors.
// No runtime BMM dependency — generated tables are pure Go strings.
//
// Consumed by [internal/templatecompile] (REQ-100 follow-up Phase 4)
// for implicit attribute injection on the compiled template, and
// by composition-builder / validator code that needs to enumerate
// the RM-mandatory fields the OPT does not model explicitly (e.g.
// COMPOSITION.category, COMPOSITION.language). The class graph serves
// consumers reasoning about the RM rather than about one template —
// AQL class-expression expansion and CONTAINS conformance,
// polymorphic slot fit, and BMM-faithful re-serialisation.
//
// Building-block weight: stdlib-only, single internal data table,
// no init-time work beyond a map literal. Safe to import from any
// SDK sub-package.
package rminfo
