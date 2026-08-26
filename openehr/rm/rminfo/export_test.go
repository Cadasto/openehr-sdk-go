package rminfo

import (
	"maps"
)

// export_test.go re-exports the one internal PROBE-098 cannot assert its arms
// from the outside: the generated absence table's own entries.
//
// It lives here rather than in absence.go because the accessor
// [AbsenceReporter.AbsenceReason] answers membership FIRST — a name in the
// class universe reports [AbsenceNone] whatever the table says about it, which
// is the documented behaviour [NewWithAbsence] promises and absence_test.go's
// TestUniverseWinsOverAnAbsenceEntry pins. That masking is right for consumers
// and blinding for a conformance probe: REQ-049 § Generated, accounted,
// computed constrains which names the table may CONTAIN, and no sequence of
// AbsenceReason calls can see a masked entry. Exporting the table itself would
// hand consumers a second, unmasked answer to the membership question and
// invite exactly the disagreement the surface exists to prevent; a file named
// *_test.go is compiled into this package for `go test` alone, so PROBE-098
// gets the keys while the shipped surface stays [Lookup], [Hierarchy],
// [AttributeLister] and [AbsenceReporter].

// AbsenceTable returns a copy of the generated absence table — every stored
// class name with the one reason it is out of the class universe. A COPY,
// because the shipped table is package state that every [Default] answer reads:
// handing out the live map would let one test's mutation change another's
// answers.
func AbsenceTable() map[string]AbsenceReason { return maps.Clone(absenceData) }
