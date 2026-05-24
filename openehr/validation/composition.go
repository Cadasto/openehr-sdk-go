package validation

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
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

// describeRMType returns a string suitable for inclusion in Issue
// Detail messages identifying the concrete Go type of an RM value.
// Pointer types are unwrapped to their elem type name.
func describeRMType(v any) string {
	switch v.(type) {
	case nil:
		return "<nil>"
	case *rm.Composition, rm.Composition:
		return "COMPOSITION"
	case *rm.Observation, rm.Observation:
		return "OBSERVATION"
	case *rm.Evaluation, rm.Evaluation:
		return "EVALUATION"
	case *rm.Instruction, rm.Instruction:
		return "INSTRUCTION"
	case *rm.Action, rm.Action:
		return "ACTION"
	case *rm.AdminEntry, rm.AdminEntry:
		return "ADMIN_ENTRY"
	case *rm.GenericEntry, rm.GenericEntry:
		return "GENERIC_ENTRY"
	case *rm.Section, rm.Section:
		return "SECTION"
	case *rm.Activity, rm.Activity:
		return "ACTIVITY"
	case *rm.EventContext, rm.EventContext:
		return "EVENT_CONTEXT"
	case *rm.History[rm.ItemStructure], rm.History[rm.ItemStructure]:
		return "HISTORY"
	case *rm.PointEvent[rm.ItemStructure], rm.PointEvent[rm.ItemStructure]:
		return "POINT_EVENT"
	case *rm.IntervalEvent[rm.ItemStructure], rm.IntervalEvent[rm.ItemStructure]:
		return "INTERVAL_EVENT"
	case *rm.ItemTree, rm.ItemTree:
		return "ITEM_TREE"
	case *rm.ItemList, rm.ItemList:
		return "ITEM_LIST"
	case *rm.ItemSingle, rm.ItemSingle:
		return "ITEM_SINGLE"
	case *rm.ItemTable, rm.ItemTable:
		return "ITEM_TABLE"
	case *rm.Cluster, rm.Cluster:
		return "CLUSTER"
	case *rm.Element, rm.Element:
		return "ELEMENT"
	case *rm.DVCodedText, rm.DVCodedText:
		return "DV_CODED_TEXT"
	case *rm.DVText, rm.DVText:
		return "DV_TEXT"
	case *rm.CodePhrase, rm.CodePhrase:
		return "CODE_PHRASE"
	case *rm.DVQuantity, rm.DVQuantity:
		return "DV_QUANTITY"
	case *rm.DVCount, rm.DVCount:
		return "DV_COUNT"
	case *rm.DVBoolean, rm.DVBoolean:
		return "DV_BOOLEAN"
	case *rm.DVOrdinal, rm.DVOrdinal:
		return "DV_ORDINAL"
	case *rm.DVDate, rm.DVDate:
		return "DV_DATE"
	case *rm.DVTime, rm.DVTime:
		return "DV_TIME"
	case *rm.DVDateTime, rm.DVDateTime:
		return "DV_DATE_TIME"
	case *rm.DVDuration, rm.DVDuration:
		return "DV_DURATION"
	case *rm.DVURI, rm.DVURI:
		return "DV_URI"
	case *rm.DVEHRURI, rm.DVEHRURI:
		return "DV_EHR_URI"
	case *rm.DVIdentifier, rm.DVIdentifier:
		return "DV_IDENTIFIER"
	case *rm.DVMultimedia, rm.DVMultimedia:
		return "DV_MULTIMEDIA"
	case *rm.DVParsable, rm.DVParsable:
		return "DV_PARSABLE"
	case *rm.DVProportion, rm.DVProportion:
		return "DV_PROPORTION"
	}
	return fmt.Sprintf("%T", v)
}
