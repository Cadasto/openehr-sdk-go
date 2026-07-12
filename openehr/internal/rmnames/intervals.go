// Package rmnames carries the wire-parameterised RM type names that
// the bare registry names (rm.RMTypeName, ADR 0013) deliberately do
// not encode. ITS-JSON names generic instantiations with their bound
// (`DV_INTERVAL<DV_QUANTITY>`); the registry registers and reverses
// the bare class name (`DV_INTERVAL`). Validation diagnostics and
// builder type-checks need the parameterised form — this package is
// its single canonical home (previously two divergent hand-written
// switches; one had drifted three instantiations behind).
package rmnames

import "github.com/cadasto/openehr-sdk-go/openehr/rm"

// TypedIntervalName returns the ITS-JSON parameterised RM type name
// for a typed DV_INTERVAL instantiation, in value or pointer form.
// The closed set is the DVOrdered concrete closure — keep in lock-step
// with the generator's DVOrdered descendant enumeration (a new
// DV_ORDERED descendant in the BMM adds an instantiation here).
// The bare DVInterval[DVOrdered] form is NOT parameterised — it maps
// to plain "DV_INTERVAL" via rm.RMTypeName and returns ("", false)
// here. REQ-024: no reflection.
func TypedIntervalName(v any) (string, bool) {
	switch v.(type) {
	case *rm.DVInterval[rm.DVCount], rm.DVInterval[rm.DVCount]:
		return "DV_INTERVAL<DV_COUNT>", true
	case *rm.DVInterval[rm.DVDate], rm.DVInterval[rm.DVDate]:
		return "DV_INTERVAL<DV_DATE>", true
	case *rm.DVInterval[rm.DVDateTime], rm.DVInterval[rm.DVDateTime]:
		return "DV_INTERVAL<DV_DATE_TIME>", true
	case *rm.DVInterval[rm.DVDuration], rm.DVInterval[rm.DVDuration]:
		return "DV_INTERVAL<DV_DURATION>", true
	case *rm.DVInterval[rm.DVOrdinal], rm.DVInterval[rm.DVOrdinal]:
		return "DV_INTERVAL<DV_ORDINAL>", true
	case *rm.DVInterval[rm.DVProportion], rm.DVInterval[rm.DVProportion]:
		return "DV_INTERVAL<DV_PROPORTION>", true
	case *rm.DVInterval[rm.DVQuantity], rm.DVInterval[rm.DVQuantity]:
		return "DV_INTERVAL<DV_QUANTITY>", true
	case *rm.DVInterval[rm.DVScale], rm.DVInterval[rm.DVScale]:
		return "DV_INTERVAL<DV_SCALE>", true
	case *rm.DVInterval[rm.DVTime], rm.DVInterval[rm.DVTime]:
		return "DV_INTERVAL<DV_TIME>", true
	}
	return "", false
}
