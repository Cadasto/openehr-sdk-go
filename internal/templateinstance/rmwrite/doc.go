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
//     DV_DURATION, DV_BOOLEAN, DV_COUNT, DV_QUANTITY, DV_MULTIMEDIA,
//     DV_STATE, DV_GENERAL_TIME_SPECIFICATION and
//     DV_PERIODIC_TIME_SPECIFICATION, and CODE_PHRASE;
//   - DV_INTERVAL over each concrete ordered data value and over
//     DV_ORDERED itself;
//   - the parties PERSON, AGENT, GROUP, ORGANISATION and ROLE, and
//     CAPABILITY, ADDRESS, CONTACT, PARTY_IDENTITY and PARTY_RELATIONSHIP;
//   - the party proxies PARTY_IDENTIFIED, PARTY_RELATED and PARTY_SELF;
//   - the EHR-wide roots EHR_STATUS and FOLDER;
//   - PARTICIPATION and TERM_MAPPING;
//   - the references OBJECT_REF, PARTY_REF, LOCATABLE_REF and
//     ACCESS_GROUP_REF, the identifiers HIER_OBJECT_ID,
//     OBJECT_VERSION_ID, ARCHETYPE_ID, TEMPLATE_ID, TERMINOLOGY_ID and
//     GENERIC_ID, and the INSTRUCTION_DETAILS an ACTION holds.
//
// AppendMultiple adds a member to multi-valued attributes on
// COMPOSITION, EVENT_CONTEXT, SECTION, the five entries, ISM_TRANSITION,
// HISTORY, ITEM_TREE, ITEM_LIST, ITEM_TABLE, CLUSTER, the five parties
// (an ACTOR's languages and roles included), CONTACT, PARTY_IDENTIFIED,
// PARTY_RELATED, FOLDER, DV_TEXT, DV_CODED_TEXT and DV_PARAGRAPH.
//
// Each parent's writer lists the attributes it fills. An attribute takes a
// child of its RM type or of a class that descends from it, wherever the
// parent's Go field can hold one: a DV_TEXT attribute (every LOCATABLE
// `name`, ELEMENT `null_reason`, INSTRUCTION `narrative`, DV_PARAGRAPH
// `items`, PARTICIPATION `function`, ISM_TRANSITION `reason`, ACTOR
// `languages`) also takes a DV_CODED_TEXT, a DV_URI attribute (DV_TEXT
// `hyperlink`, DV_MULTIMEDIA `uri`) a DV_EHR_URI, EVENT_CONTEXT
// `health_care_facility` a PARTY_RELATED as well as a PARTY_IDENTIFIED,
// and an attribute of an abstract class, such as ELEMENT `value`
// (DATA_VALUE), PARTICIPATION `performer` (PARTY_PROXY) or OBJECT_REF `id`
// (OBJECT_ID), any concrete class of it. An OBJECT_REF attribute (an
// ENTRY's `workflow_id`, a CARE_ENTRY's `guideline_id`, a member of a
// FOLDER's `items`) takes any class of reference as given, and
// LOCATABLE_REF `id` is written into the UID_BASED_ID field LOCATABLE_REF
// declares, not the one it embeds from OBJECT_REF. An Integer attribute
// (DV_QUANTITY `precision`, DV_MULTIMEDIA `size`, INTERVAL_EVENT
// `sample_count`) takes a whole number of any Go integer shape, and a
// whole number outside the 32-bit range an RM Integer holds returns an
// ErrTypeMismatch that says so. A Character attribute (TERM_MAPPING
// `match`) takes a string or an rm.Character of one character, and one of
// any other length returns an ErrTypeMismatch that says so. Any other
// parent or attribute returns ErrUnknownAttribute, and a child of the
// wrong Go type returns ErrTypeMismatch. Table-driven tests in this
// package write through most of these parents, and check the DV_INTERVAL
// cases against the type registry, so an ordered data value the registry
// gains fails them until it has a case.
//
// Three more tests ask the writers about the type registry and the
// classes in openehr/rm/rminfo. The first two ask both writers about
// every class the type registry knows and every attribute name any class
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
// would drop the default silently. The third pins the classes the
// second cannot see: the registered classes a value below a LOCATABLE can
// be, found by following the declared type of each attribute from every
// concrete LOCATABLE, that neither writer has a case for. A class that
// becomes reachable, or loses its last case, fails it until the class gets
// a case or joins the pinned list on purpose.
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
