// Package constraints carries the typed primitive constraint values
// attached to leaf OPT nodes. It is a separate sub-package of
// [github.com/cadasto/openehr-sdk-go/openehr/template] so the wire
// decoder, the template compiler, and downstream validators can all
// share one closed-set primitive vocabulary without pulling in the
// rest of the template surface.
//
// The taxonomy mirrors the ADL 1.4 OPT XSD primitive `xsi:type`
// values one-to-one:
//
//   - C_BOOLEAN       → [CBoolean]
//   - C_INTEGER       → [CInteger]
//   - C_REAL          → [CReal]
//   - C_STRING        → [CString]
//   - C_DATE          → [CDate]
//   - C_TIME          → [CTime]
//   - C_DATE_TIME     → [CDateTime]
//   - C_DURATION      → [CDuration]
//   - C_CODE_PHRASE   → [CodePhrase]
//   - C_DV_QUANTITY   → [DvQuantity]
//   - C_DV_ORDINAL    → [CDvOrdinal]
//
// Every type implements the sealed [PrimitiveConstraint] interface,
// so validators can pattern-match the closed set:
//
//	switch p := node.PrimitiveConstraint().(type) {
//	case constraints.DvQuantity: ...
//	case constraints.CodePhrase: ...
//	case nil:
//	    // not a primitive leaf
//	}
//
// Each constraint exposes Validate(value any) []Violation. A nil
// (empty) result means the value satisfies every clause; a non-empty
// slice lists every clause that failed. Validators are pure functions.
// They do not consult terminology services, time zones, or any
// external state, so the result depends only on the constraint
// values and the input.
//
// The package is stdlib-only, so primitive constraint shapes are usable
// from any consumer (composition builder, validator, codegen) without
// pulling in transport, auth, or rm types.
package constraints
