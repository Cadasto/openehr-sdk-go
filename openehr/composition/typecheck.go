package composition

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// checkRMType verifies v's Go concrete type matches the RM type the
// compiled node declares. Returns nil on match, ErrTypeMismatch
// (wrapped with diagnostic context) otherwise. Closed type switch —
// REQ-024, no reflection.
//
// The table covers the DV primitives the builder Set helpers
// produce; non-primitive paths (CLUSTER, ELEMENT, OBSERVATION) are
// admissible via the catch-all "*rm.<RMType>" mapping checked against
// the compiled-node's RMTypeName so the table stays maintainable as
// new RM types come in.
func checkRMType(node *templatecompile.CompiledNode, v any) error {
	want := node.RMTypeName()
	got, ok := goConcreteRMType(v)
	if !ok {
		return fmt.Errorf("%w: value type %T is not a recognised RM concrete", ErrTypeMismatch, v)
	}
	if got != want {
		return fmt.Errorf("%w: value type %T (RM %q) does not match path RM type %q",
			ErrTypeMismatch, v, got, want)
	}
	return nil
}

// goConcreteRMType maps an RM Go concrete value back to its RM class
// name. Mirrors the typereg constructor table but in the reverse
// direction so checkRMType can compare against CompiledNode.RMTypeName
// without a reflection hop. Returns ("", false) for types that are
// not in the closed RM concrete set.
//
// Adding a new RM concrete to this switch is the same exercise as
// adding it to rmwrite and typereg — keep all three in lock-step.
func goConcreteRMType(v any) (string, bool) {
	switch v.(type) {
	case *rm.Composition, rm.Composition:
		return "COMPOSITION", true
	case *rm.Observation, rm.Observation:
		return "OBSERVATION", true
	case *rm.Evaluation, rm.Evaluation:
		return "EVALUATION", true
	case *rm.Instruction, rm.Instruction:
		return "INSTRUCTION", true
	case *rm.Action, rm.Action:
		return "ACTION", true
	case *rm.AdminEntry, rm.AdminEntry:
		return "ADMIN_ENTRY", true
	case *rm.GenericEntry, rm.GenericEntry:
		return "GENERIC_ENTRY", true
	case *rm.Section, rm.Section:
		return "SECTION", true
	case *rm.Activity, rm.Activity:
		return "ACTIVITY", true
	case *rm.EventContext, rm.EventContext:
		return "EVENT_CONTEXT", true
	case *rm.History[rm.ItemStructure], rm.History[rm.ItemStructure]:
		return "HISTORY", true
	case *rm.PointEvent[rm.ItemStructure], rm.PointEvent[rm.ItemStructure]:
		return "POINT_EVENT", true
	case *rm.IntervalEvent[rm.ItemStructure], rm.IntervalEvent[rm.ItemStructure]:
		return "INTERVAL_EVENT", true
	case *rm.ItemTree, rm.ItemTree:
		return "ITEM_TREE", true
	case *rm.ItemList, rm.ItemList:
		return "ITEM_LIST", true
	case *rm.ItemSingle, rm.ItemSingle:
		return "ITEM_SINGLE", true
	case *rm.ItemTable, rm.ItemTable:
		return "ITEM_TABLE", true
	case *rm.Cluster, rm.Cluster:
		return "CLUSTER", true
	case *rm.Element, rm.Element:
		return "ELEMENT", true
	case *rm.DVText, rm.DVText:
		return "DV_TEXT", true
	case *rm.DVCodedText, rm.DVCodedText:
		return "DV_CODED_TEXT", true
	case *rm.DVQuantity, rm.DVQuantity:
		return "DV_QUANTITY", true
	case *rm.DVCount, rm.DVCount:
		return "DV_COUNT", true
	case *rm.DVBoolean, rm.DVBoolean:
		return "DV_BOOLEAN", true
	case *rm.DVDate, rm.DVDate:
		return "DV_DATE", true
	case *rm.DVTime, rm.DVTime:
		return "DV_TIME", true
	case *rm.DVDateTime, rm.DVDateTime:
		return "DV_DATE_TIME", true
	case *rm.DVDuration, rm.DVDuration:
		return "DV_DURATION", true
	case *rm.DVOrdinal, rm.DVOrdinal:
		return "DV_ORDINAL", true
	case *rm.DVProportion, rm.DVProportion:
		return "DV_PROPORTION", true
	case *rm.DVURI, rm.DVURI:
		return "DV_URI", true
	case *rm.DVEHRURI, rm.DVEHRURI:
		return "DV_EHR_URI", true
	case *rm.DVIdentifier, rm.DVIdentifier:
		return "DV_IDENTIFIER", true
	case *rm.DVMultimedia, rm.DVMultimedia:
		return "DV_MULTIMEDIA", true
	case *rm.DVParsable, rm.DVParsable:
		return "DV_PARSABLE", true
	case *rm.CodePhrase, rm.CodePhrase:
		return "CODE_PHRASE", true
	}
	return "", false
}
