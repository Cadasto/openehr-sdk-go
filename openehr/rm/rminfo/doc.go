// Package rminfo answers "what does the openEHR Reference Model
// declare about RM class X?": required attributes, attribute RM
// types, multi-cardinality flags, the universe of known class names,
// and the class graph over them (abstractness, immediate parents,
// transitive ancestors, conformance, the concrete classes an abstract
// class denotes, and the class each attribute is declared on).
//
// The attribute questions are on the [Lookup] interface; the class-graph
// questions are on the optional [Hierarchy] capability interface, which
// [Default] implements and consumers reach by type assertion. Why a name is
// not in the class universe is answered by the optional [AbsenceReporter],
// reached the same way.
//
// The data is generated from the pinned BMM schemas under resources/bmm/
// and lives in two generated tables: lookup_gen.go for the class
// universe, absence_gen.go for the declared names outside it. The
// hand-written surface is the [Lookup], [Hierarchy], [AttributeLister]
// and [AbsenceReporter] interfaces, the [ClassMeta] and [AttrMeta] data
// types, and the [Default], [New] and [NewWithAbsence] accessors. There
// is no runtime BMM dependency: the generated tables are plain Go
// literals.
//
// Typical uses are implicit attribute injection when compiling a
// template, and composition-builder or validator code that needs to
// enumerate the RM-mandatory fields an OPT does not model explicitly
// (e.g. COMPOSITION.category, COMPOSITION.language). The class graph
// serves code that reasons about the RM rather than about one template:
// AQL class-expression expansion and CONTAINS conformance, polymorphic
// slot fit, and BMM-faithful re-serialisation.
//
// The package uses only the standard library and does no init-time work
// beyond map literals, so it is safe to import from any SDK sub-package.
package rminfo
