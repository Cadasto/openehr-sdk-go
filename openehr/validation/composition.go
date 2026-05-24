package validation

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation/rmread"
)

// ValidateComposition validates an in-memory RM Composition
// against a compiled OPT and returns every issue in one pass —
// REQ-102.
//
// The walk is template-driven (see package doc): the compiled OPT
// drives traversal, the composition is the value source. Path
// strings come from CompiledNode.AQLPath() — composition-supplied
// predicates never form lookup keys.
//
// Returns a [Result] whose Issues slice is never nil (zero-length
// allocation when no issues fire).
func ValidateComposition(comp *rm.Composition, c *templatecompile.Compiled) Result {
	if comp == nil {
		return resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_composition",
			Detail:   "ValidateComposition called with a nil *rm.Composition argument",
			Severity: Error,
		}})
	}
	if c == nil {
		return resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_template",
			Detail:   "ValidateComposition called with a nil compiled template argument",
			Severity: Error,
		}})
	}
	if c.Root() == nil {
		return resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_template",
			Detail:   "compiled template has no root node",
			Severity: Error,
		}})
	}

	w := newWalker(c)
	w.walkNode(c.Root(), comp, "/")
	return resultFromIssues(w.issues)
}

// walker carries the issue accumulator and any per-call state for
// a single ValidateComposition invocation. Constructed via
// newWalker so callers do not see the zero value (which has a nil
// issues slice — never returned to callers).
type walker struct {
	compiled *templatecompile.Compiled
	issues   []Issue
}

func newWalker(c *templatecompile.Compiled) *walker {
	// Pre-allocate a non-nil issues slice so resultFromIssues
	// returns a non-nil slice on the OK path.
	return &walker{compiled: c, issues: []Issue{}}
}

// emit appends one Issue to the walker's accumulator. Small helper
// kept for symmetry with later code that may want to deduplicate
// or sort issues; right now it is a direct append.
func (w *walker) emit(i Issue) {
	w.issues = append(w.issues, i)
}

// joinPath glues a parent AQL path with a child segment. Mirrors
// the compile-side pathSegment convention: root path is "/" and
// child deltas already begin with "/" (e.g. "/category",
// "/content[at0001]"). Centralised here so future tweaks to path
// construction (e.g. predicate normalisation) have one home.
func joinPath(parent, segment string) string {
	if parent == "/" {
		return segment
	}
	return parent + segment
}

// rmTypeInfo is the single source of truth for "what RM class is
// this Go value and (when LOCATABLE) what is its archetype_node_id".
// All v2 walker code routes through this function — describeRMType
// and locatableArchetypeNodeID are thin wrappers. Adding a new RM
// type means editing one switch.
//
// Returns ("", "", false) for Go types outside the v2 closed set.
// Pointer + value receivers both enumerated because JSON decoding
// yields pointers while builders may yield values (REQ-024 — no
// reflection).
func rmTypeInfo(v any) (rmType string, archetypeNodeID string, ok bool) {
	if v == nil || rmread.IsTypedNilPointer(v) {
		return "", "", false
	}
	switch x := v.(type) {

	// LOCATABLE concretes — carry archetype_node_id.
	case *rm.Composition:
		return "COMPOSITION", x.ArchetypeNodeID, true
	case rm.Composition:
		return "COMPOSITION", x.ArchetypeNodeID, true
	case *rm.Observation:
		return "OBSERVATION", x.ArchetypeNodeID, true
	case rm.Observation:
		return "OBSERVATION", x.ArchetypeNodeID, true
	case *rm.Evaluation:
		return "EVALUATION", x.ArchetypeNodeID, true
	case rm.Evaluation:
		return "EVALUATION", x.ArchetypeNodeID, true
	case *rm.Instruction:
		return "INSTRUCTION", x.ArchetypeNodeID, true
	case rm.Instruction:
		return "INSTRUCTION", x.ArchetypeNodeID, true
	case *rm.Action:
		return "ACTION", x.ArchetypeNodeID, true
	case rm.Action:
		return "ACTION", x.ArchetypeNodeID, true
	case *rm.AdminEntry:
		return "ADMIN_ENTRY", x.ArchetypeNodeID, true
	case rm.AdminEntry:
		return "ADMIN_ENTRY", x.ArchetypeNodeID, true
	case *rm.GenericEntry:
		return "GENERIC_ENTRY", x.ArchetypeNodeID, true
	case rm.GenericEntry:
		return "GENERIC_ENTRY", x.ArchetypeNodeID, true
	case *rm.Section:
		return "SECTION", x.ArchetypeNodeID, true
	case rm.Section:
		return "SECTION", x.ArchetypeNodeID, true
	case *rm.Activity:
		return "ACTIVITY", x.ArchetypeNodeID, true
	case rm.Activity:
		return "ACTIVITY", x.ArchetypeNodeID, true
	case *rm.History[rm.ItemStructure]:
		return "HISTORY", x.ArchetypeNodeID, true
	case rm.History[rm.ItemStructure]:
		return "HISTORY", x.ArchetypeNodeID, true
	case *rm.PointEvent[rm.ItemStructure]:
		return "POINT_EVENT", x.ArchetypeNodeID, true
	case rm.PointEvent[rm.ItemStructure]:
		return "POINT_EVENT", x.ArchetypeNodeID, true
	case *rm.IntervalEvent[rm.ItemStructure]:
		return "INTERVAL_EVENT", x.ArchetypeNodeID, true
	case rm.IntervalEvent[rm.ItemStructure]:
		return "INTERVAL_EVENT", x.ArchetypeNodeID, true
	case *rm.ItemTree:
		return "ITEM_TREE", x.ArchetypeNodeID, true
	case rm.ItemTree:
		return "ITEM_TREE", x.ArchetypeNodeID, true
	case *rm.ItemList:
		return "ITEM_LIST", x.ArchetypeNodeID, true
	case rm.ItemList:
		return "ITEM_LIST", x.ArchetypeNodeID, true
	case *rm.ItemSingle:
		return "ITEM_SINGLE", x.ArchetypeNodeID, true
	case rm.ItemSingle:
		return "ITEM_SINGLE", x.ArchetypeNodeID, true
	case *rm.ItemTable:
		return "ITEM_TABLE", x.ArchetypeNodeID, true
	case rm.ItemTable:
		return "ITEM_TABLE", x.ArchetypeNodeID, true
	case *rm.Cluster:
		return "CLUSTER", x.ArchetypeNodeID, true
	case rm.Cluster:
		return "CLUSTER", x.ArchetypeNodeID, true
	case *rm.Element:
		return "ELEMENT", x.ArchetypeNodeID, true
	case rm.Element:
		return "ELEMENT", x.ArchetypeNodeID, true

	// EVENT_CONTEXT — has archetype-like nature but no archetype_node_id field.
	case *rm.EventContext, rm.EventContext:
		return "EVENT_CONTEXT", "", true

	// DataValue subtypes — no archetype_node_id.
	case *rm.DVCodedText, rm.DVCodedText:
		return "DV_CODED_TEXT", "", true
	case *rm.DVText, rm.DVText:
		return "DV_TEXT", "", true
	case *rm.CodePhrase, rm.CodePhrase:
		return "CODE_PHRASE", "", true
	case *rm.DVQuantity, rm.DVQuantity:
		return "DV_QUANTITY", "", true
	case *rm.DVCount, rm.DVCount:
		return "DV_COUNT", "", true
	case *rm.DVBoolean, rm.DVBoolean:
		return "DV_BOOLEAN", "", true
	case *rm.DVOrdinal, rm.DVOrdinal:
		return "DV_ORDINAL", "", true
	case *rm.DVDate, rm.DVDate:
		return "DV_DATE", "", true
	case *rm.DVTime, rm.DVTime:
		return "DV_TIME", "", true
	case *rm.DVDateTime, rm.DVDateTime:
		return "DV_DATE_TIME", "", true
	case *rm.DVDuration, rm.DVDuration:
		return "DV_DURATION", "", true
	case *rm.DVURI, rm.DVURI:
		return "DV_URI", "", true
	case *rm.DVEHRURI, rm.DVEHRURI:
		return "DV_EHR_URI", "", true
	case *rm.DVIdentifier, rm.DVIdentifier:
		return "DV_IDENTIFIER", "", true
	case *rm.DVMultimedia, rm.DVMultimedia:
		return "DV_MULTIMEDIA", "", true
	case *rm.DVParsable, rm.DVParsable:
		return "DV_PARSABLE", "", true
	case *rm.DVProportion, rm.DVProportion:
		return "DV_PROPORTION", "", true

	// PartyProxy concretes.
	case rm.PartySelf, *rm.PartySelf:
		return "PARTY_SELF", "", true
	case *rm.PartyIdentified, rm.PartyIdentified:
		return "PARTY_IDENTIFIED", "", true
	case *rm.PartyRelated, rm.PartyRelated:
		return "PARTY_RELATED", "", true
	}
	return "", "", false
}

// describeRMType returns a string suitable for inclusion in Issue
// Detail messages identifying the concrete Go type of an RM value.
// Pointer types are unwrapped to their elem type name.
func describeRMType(v any) string {
	if v == nil {
		return "<nil>"
	}
	if name, _, ok := rmTypeInfo(v); ok {
		return name
	}
	return fmt.Sprintf("%T", v)
}
