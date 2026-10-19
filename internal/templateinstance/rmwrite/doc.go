// Package rmwrite writes RM attribute values by name without
// reflection. It is the inverse of the generated attribute access in
// internal/rmaccess: where rmaccess answers "give me the RM value at
// attribute `name` on this parent", rmwrite answers "attach `child` to
// attribute `name` on this parent".
//
// The API is intentionally small:
//
//	EnsureSingle(parent any, attrName string, child any) error
//	AppendMultiple(parent any, attrName string, child any) error
//	NewRM(rmTypeName string) (any, error)
//
// All three are pure structural setters. Dispatch is a closed type
// switch on the Go concrete type of `parent`, with no reflection.
//
// # Scope
//
// rmwrite is a low-level setter: it binds one RM value into the one
// named slot a caller asks for on a parent RM value. Of the LOCATABLE
// bookkeeping it writes only `name`, and only when asked; it never
// stamps `archetype_node_id`, `uid` or `archetype_details`, which are
// the caller's (`openehr/instance/`) responsibility. Higher-level
// identity, terminology, and template-id wiring live in the instance
// generator above.
//
// # Coverage
//
// EnsureSingle writes single-valued attributes on these parents:
//
//   - COMPOSITION, EVENT_CONTEXT, SECTION and GENERIC_ENTRY;
//   - the entries OBSERVATION, EVALUATION, INSTRUCTION, ACTION and
//     ADMIN_ENTRY, and the ACTIVITY and ISM_TRANSITION they hold;
//   - HISTORY, POINT_EVENT and INTERVAL_EVENT;
//   - the item structures ITEM_TREE, ITEM_LIST, ITEM_SINGLE and
//     ITEM_TABLE, and CLUSTER and ELEMENT;
//   - the data values DV_TEXT, DV_CODED_TEXT, DV_URI, DV_EHR_URI,
//     DV_PARSABLE, DV_IDENTIFIER, DV_DATE, DV_TIME, DV_DATE_TIME,
//     DV_DURATION, DV_BOOLEAN, DV_COUNT, DV_QUANTITY and DV_MULTIMEDIA,
//     and CODE_PHRASE;
//   - DV_INTERVAL over each concrete ordered data value and over
//     DV_ORDERED itself;
//   - the parties PERSON, AGENT, GROUP, ORGANISATION and ROLE, and
//     CAPABILITY, ADDRESS and PARTY_IDENTITY;
//   - PARTY_RELATED, of which it writes only relationship.
//
// AppendMultiple adds a member to multi-valued attributes on
// COMPOSITION, SECTION, INSTRUCTION, HISTORY, ITEM_TREE, ITEM_LIST,
// ITEM_TABLE, CLUSTER, the five parties and CONTACT.
//
// Each parent's writer lists the attributes it fills. An attribute takes
// a child of its RM type or of a class that descends from it, wherever
// the parent's Go field can hold one: a DV_TEXT attribute (every `name`,
// ELEMENT `null_reason`, INSTRUCTION `narrative`) also takes a
// DV_CODED_TEXT, EVENT_CONTEXT `health_care_facility` a PARTY_RELATED as
// well as a PARTY_IDENTIFIED, and an attribute of an abstract class, such
// as ELEMENT `value` (DATA_VALUE), any concrete class of it. Any other
// parent or attribute returns ErrUnknownAttribute, and a child of the
// wrong Go type returns ErrTypeMismatch. Table-driven tests in this package
// write through most of these parents, and check the DV_INTERVAL cases
// against the type registry, so an ordered data value the registry
// gains fails them until it has a case.
//
// Two more tests ask both writers about every class the type registry
// knows and every attribute name any class in openehr/rm/rminfo
// declares, with a child no case accepts. The first checks that each
// writer case names an attribute its class declares, in the right
// shape: EnsureSingle for a single-valued attribute, AppendMultiple for
// a list. The second pins, for each class the writers write into, the
// declared attributes no case fills. It leaves out the LOCATABLE
// attributes other than name: the instance generator stamps
// archetype_node_id, uid and archetype_details itself; links and
// feeder_audit are optional in the RM, and nothing fills them. An
// attribute a BMM update adds, or a case that is removed or
// renamed, fails it until the attribute gets a case or joins the
// pinned list on purpose. The instance generator drops the error when
// it fills an RM-required default, so without this pin a missing case
// would drop the default silently.
//
// # Dependencies
//
// This package imports only the standard library, openehr/rm, and
// openehr/rm/typereg. It does not import openehr/template,
// internal/templatecompile, openehr/validation, or any wire / auth /
// transport layer. openehr/instance and openehr/composition import it,
// and so do the tests of testkit/probes/instance, which sort generator
// failures by its errors. Adding a new RM type means adding a case,
// and a writer for its attributes, to EnsureSingle, and to
// AppendMultiple when the type has a multi-valued attribute to fill.
// NewRM needs no change: it finds the type in the typereg registry,
// which the generated RM code fills. The read side is generated.
package rmwrite
