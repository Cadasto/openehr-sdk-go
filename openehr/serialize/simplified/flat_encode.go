package simplified

// REQ-053 — FLAT encode: project an *rm.Composition into the FLAT map by
// walking the Web Template tree and resolving each node's value against the
// composition via openehr/rm/rmpath. The Web Template already carries the
// level-removed id-chain (FLAT path) and each node's canonical aqlPath, so
// the walk needs no separate flattening engine.

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// MarshalFlat encodes comp as FLAT JSON using wt (REQ-053).
func MarshalFlat(comp *rm.Composition, wt *webtemplate.WebTemplate) ([]byte, error) {
	m, err := encodeFlat(comp, wt)
	if err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

// encodeFlat builds the FLAT path->value map. The root COMPOSITION is the
// resolution root; its FLAT path is the tree id.
func encodeFlat(comp *rm.Composition, wt *webtemplate.WebTemplate) (map[string]any, error) {
	if wt == nil || wt.Tree == nil {
		return nil, ErrNoTemplate
	}
	if comp == nil {
		return nil, ErrNilComposition
	}
	out := make(map[string]any)
	if err := emitContext(out, comp); err != nil {
		return nil, err
	}
	root := wt.Tree
	// Reused siblings (REQ-116) share one bare path, and resolution below
	// keys on the bare spelling — emitNode refuses to emit through them.
	ambiguous := ambiguousBarePaths(wt)
	for _, ch := range root.Children {
		if err := emitNode(out, ch, root.ID, comp, root.AQLPath, ambiguous); err != nil {
			return nil, err
		}
	}
	// REQ-140 — the root COMPOSITION's own underscore attributes (it is a
	// LOCATABLE the node walk never visits, because the walk starts at its
	// children), and the EVENT_CONTEXT optionals under the real `context`
	// segment (ADR 0016). The context prefix is spelled here rather than taken
	// from a Web Template node because those attributes are RM-optional and a
	// template need not constrain `context` at all — decode resolves the same
	// prefix the same way (rmattrOwnerAt).
	if err := rmattrEncode(comp, root.ID, out); err != nil {
		return nil, err
	}
	if err := rmattrEncode(comp.Context, root.ID+"/context", out); err != nil {
		return nil, err
	}
	return out, nil
}

// emitContext writes composition-level metadata under the ctx/ prefix (REQ-053):
// the mandatory language and territory code strings, the composer, the context
// start time, and the context setting (ctx/setting|code + |value — the sixth
// respelled field; ADR 0015's left-open emission gap, closed 2026-08-05). The
// ctx/ short forms for participations, health-care facility, workflow ids, and
// the composer external reference are deferred — see deviations.md. (category
// is not a ctx/ field at all: it is a template-constrained leaf and rides its
// own Web Template path.)
//
// A composer the ctx/ short forms cannot carry (PARTY_RELATED, or a
// PARTY_IDENTIFIED without a name) is an error, not an omission: a decode of
// the composer-less output under WithTemplate would default the composer to
// PARTY_SELF — a silent type substitution (see deviations.md). A setting the
// ctx/setting pair cannot carry is refused on the same grounds
// (emitContextSetting).
func emitContext(out map[string]any, comp *rm.Composition) error {
	if comp.Language.CodeString != "" {
		out["ctx/language"] = comp.Language.CodeString
	}
	if comp.Territory.CodeString != "" {
		out["ctx/territory"] = comp.Territory.CodeString
	}
	switch c := comp.Composer.(type) {
	case nil:
		// Absent composer: nothing to represent, nothing substituted.
	case *rm.PartySelf:
		if c != nil {
			out["ctx/composer_self"] = true
		}
	case rm.PartySelf:
		out["ctx/composer_self"] = true
	case *rm.PartyIdentified:
		if c != nil {
			if c.Name == nil || *c.Name == "" {
				return fmt.Errorf("%w: composer PARTY_IDENTIFIED without a name is not representable as ctx/composer_name", ErrUnsupportedDatatype)
			}
			out["ctx/composer_name"] = *c.Name
		}
	case rm.PartyIdentified:
		if c.Name == nil || *c.Name == "" {
			return fmt.Errorf("%w: composer PARTY_IDENTIFIED without a name is not representable as ctx/composer_name", ErrUnsupportedDatatype)
		}
		out["ctx/composer_name"] = *c.Name
	default:
		return fmt.Errorf("%w: composer %T is not representable in the ctx/ short forms", ErrUnsupportedDatatype, comp.Composer)
	}
	if comp.Context != nil {
		if comp.Context.StartTime.Value != "" {
			out["ctx/time"] = comp.Context.StartTime.Value
		}
		if err := emitContextSetting(out, comp.Context.Setting); err != nil {
			return err
		}
	}
	return nil
}

// emitContextSetting writes EVENT_CONTEXT.setting as ctx/setting|code +
// ctx/setting|value (REQ-053, amended 2026-08-05). The pair carries exactly an
// openehr-coded code+value — the terminology is implied, not written — so a
// populated setting outside that shape is ErrUnsupportedDatatype naming
// ctx/setting rather than an omission: omitting it would let a WithTemplate
// decode substitute the 238|other care default silently (the same stance as
// the composer PARTY_RELATED rule). That refuses a non-openehr terminology
// (including an empty one — rebuilding it as openehr would re-terminologise
// the value), extras beyond code+value (mappings, formatting, a preferred
// term, …), and a value carried without a defining code.
//
// The all-zero value writes nothing: setting is a non-pointer DV_CODED_TEXT on
// EVENT_CONTEXT, so "unset" and "zero" coincide (the CODE_PHRASE all-zero
// precedent) — an unconditional emit would put blank setting keys on every
// composition decoded through the ctx/ forms.
func emitContextSetting(out map[string]any, s rm.DVCodedText) error {
	extras := len(s.Mappings) > 0 || s.Formatting != nil || s.Hyperlink != nil ||
		s.Language != nil || s.Encoding != nil || s.DefiningCode.PreferredTerm != nil
	if s.DefiningCode.CodeString == "" {
		if s.Value == "" && !extras && s.DefiningCode.TerminologyID.Value == "" {
			return nil // all-zero: absent, nothing substituted
		}
		return fmt.Errorf("%w: ctx/setting requires a coded value, but EVENT_CONTEXT.setting carries no defining code beside value %q", ErrUnsupportedDatatype, s.Value)
	}
	if s.DefiningCode.TerminologyID.Value != "openehr" {
		return fmt.Errorf("%w: ctx/setting implies the openehr terminology, but EVENT_CONTEXT.setting is coded in %q", ErrUnsupportedDatatype, s.DefiningCode.TerminologyID.Value)
	}
	if extras {
		return fmt.Errorf("%w: ctx/setting carries only code and value, but EVENT_CONTEXT.setting has extras (mappings, formatting, a preferred term, …)", ErrUnsupportedDatatype)
	}
	// A code with no rubric is refused rather than emitted with an empty
	// |value: parseCtx refuses that pair on the way back in, so emitting it
	// would produce a body this codec's own decode rejects. DV_TEXT.value is
	// RM-mandatory anyway, so the value is a defect at the source — the mirror
	// of the decode-side rule, and the same stance as value-without-code above.
	if s.Value == "" {
		return fmt.Errorf("%w: ctx/setting requires the code's rubric, but EVENT_CONTEXT.setting carries code %q with an empty value", ErrUnsupportedDatatype, s.DefiningCode.CodeString)
	}
	out["ctx/setting|code"] = s.DefiningCode.CodeString
	out["ctx/setting|value"] = s.Value
	return nil
}

// emitNode resolves node against resolveRoot (whose canonical path is
// resolveRootAql) and writes FLAT entries under flatPrefix. A repeating node
// enumerates its instances and stamps a zero-based :index; a container
// recurses into its children; a value leaf maps its datatype to suffix keys.
// Absent optional nodes resolve to nothing and are silently skipped.
//
// ambiguous holds the bare spellings claimed by several reused siblings
// (REQ-116). Resolution strips the name predicate, so for such a node the
// same RM instance answers to *every* sibling — with data present, one
// sibling's values would be emitted under all sibling ids, silently
// misattributed. emitNode refuses instead; a node in the ambiguous set that
// resolves to nothing is still skipped like any absent optional, so
// compositions that do not touch the reused region keep encoding.
func emitNode(out map[string]any, node *webtemplate.Node, flatPrefix string, resolveRoot rm.Locatable, resolveRootAql string, ambiguous map[string]bool) error {
	isContainer := len(node.Children) > 0
	// A value leaf normally carries input descriptors, but the Web Template emits
	// none for some datatypes (DV_URI, DV_MULTIMEDIA, DV_PARSABLE, the in-context
	// CODE_PHRASE pair, …); any childless node of a value leaf type is still a
	// value leaf and must be emitted (bare/suffixed or |raw), not dropped.
	isLeaf := !isContainer && (len(node.Inputs) > 0 || isValueLeafType(node.RMType))
	if !isContainer && !isLeaf {
		return nil // structural node carrying neither children nor value inputs
	}
	// Resolution against the RM instance keys on archetype_node_id, so the
	// REQ-116 name predicate the Web Template now carries is dropped here.
	// rmpath *does* honour a `node,'name'` predicate, which would filter
	// instances by their runtime LOCATABLE.name — an over-constraint: the
	// template pins the name a node is *modelled* with, while encoding must
	// emit whatever instance sits at that node id. Leaving it in silently
	// drops values whose stored name differs, breaking the FLAT round-trip.
	// The prefix trim runs first: both paths are the Web Template's
	// predicated spelling, so they only line up before stripping.
	relPath := bareAQLPath(strings.TrimPrefix(node.AQLPath, resolveRootAql))

	if node.Max != 1 {
		vals, err := rmpath.ItemsAtPath(resolveRoot, relPath)
		if err != nil {
			return skipNotFound(err, relPath)
		}
		if len(vals) > 0 {
			if err := refuseReusedSibling(node, ambiguous); err != nil {
				return err
			}
		}
		// The :index counts emitted instances, not RM list positions: an
		// instance whose subtree contributes no FLAT keys is omitted without
		// consuming an index. Stamping by list position would leave a sparse
		// sequence (":0",":2") that the decoder rejects as phantom gap-fill,
		// breaking the round-trip on valid compositions (see deviations.md).
		idx := 0
		for _, v := range vals {
			sub := make(map[string]any)
			if err := emitValue(sub, node, flatPrefix+"/"+node.ID+":"+strconv.Itoa(idx), v, isContainer, resolveRoot, resolveRootAql, ambiguous); err != nil {
				return err
			}
			if len(sub) == 0 {
				continue
			}
			maps.Copy(out, sub)
			idx++
		}
		return nil
	}
	v, err := rmpath.ItemAtPath(resolveRoot, relPath)
	if err != nil {
		return skipNotFound(err, relPath)
	}
	if err := refuseReusedSibling(node, ambiguous); err != nil {
		return err
	}
	return emitValue(out, node, flatPrefix+"/"+node.ID, v, isContainer, resolveRoot, resolveRootAql, ambiguous)
}

// refuseReusedSibling fails encoding when node is one of several reused
// siblings (its bare path is claimed more than once) and data resolved at it:
// the instance cannot be attributed to this sibling id versus the others, so
// emitting would alias one sibling's data under every sibling's FLAT id. The
// wrapped sentinel is rmpath.ErrPathAmbiguous — the ambiguity is real, it
// just lives across template nodes rather than across RM instances.
func refuseReusedSibling(node *webtemplate.Node, ambiguous map[string]bool) error {
	if node.NodeID == "" {
		return nil
	}
	bare := bareAQLPath(node.AQLPath)
	if !ambiguous[bare] {
		return nil
	}
	return fmt.Errorf("simplified: FLAT id %q is one of several reused siblings sharing the path %q — not yet encodable (see deviations.md): %w", node.ID, bare, rmpath.ErrPathAmbiguous)
}

// skipNotFound treats an absent optional node (ErrPathNotFound) as a no-op,
// but surfaces real faults — a malformed path (ErrPathSyntax) or a Max==1
// node that resolves to multiple items (ErrPathAmbiguous) — rather than
// silently dropping data.
func skipNotFound(err error, relPath string) error {
	if errors.Is(err, rmpath.ErrPathNotFound) {
		return nil
	}
	return fmt.Errorf("simplified: resolve %q: %w", relPath, err)
}

// emitValue recurses into a container instance or maps a leaf value.
// ancestorRoot/ancestorAql are the enclosing Locatable resolution root against
// which node was resolved; they carry through when a container is not itself
// Locatable (e.g. EVENT_CONTEXT), so its children resolve from that ancestor by
// their full relative paths rather than being dropped.
func emitValue(out map[string]any, node *webtemplate.Node, flatPath string, v any, isContainer bool, ancestorRoot rm.Locatable, ancestorAql string, ambiguous map[string]bool) error {
	if isContainer {
		root, rootAql := ancestorRoot, ancestorAql
		loc, isLocatable := v.(rm.Locatable)
		if isLocatable {
			// A Locatable container becomes the new resolution root for its
			// children (each repeatable instance is resolved independently).
			root, rootAql = loc, node.AQLPath
		}
		for _, ch := range node.Children {
			if err := emitNode(out, ch, flatPath, root, rootAql, ambiguous); err != nil {
				return err
			}
		}
		// REQ-140 — after the node's own emission, its underscore attributes.
		// Only a LOCATABLE owns any: EVENT_CONTEXT is handled from the root (its
		// FLAT prefix is fixed, template node or not), and a non-Locatable
		// container (ISM_TRANSITION) has no underscore attribute in the grammar.
		if isLocatable {
			return rmattrEncode(loc, flatPath, out)
		}
		return nil
	}
	if err := leafToFlat(out, flatPath, v, node.RMType, leafListOpen(node)); err != nil {
		return err
	}
	return emitLeafOwnerRMAttrs(out, node, flatPath, ancestorRoot, ancestorAql)
}

// emitLeafOwnerRMAttrs writes the REQ-140 underscore attributes of the LOCATABLE
// a collapsed leaf hides. The Web Template folds ELEMENT.value into its leaf
// node, so `<leaf>/_uid` and `<leaf>/_link:N` belong to the ELEMENT one
// attribute up — the one owner the node walk cannot reach, since the walk
// resolves the leaf's own `…/value` path.
//
// Keyed on the canonical trailing `/value` attribute *and* on the resolved
// parent actually being an ELEMENT. Every other childless leaf sits directly on
// a non-LOCATABLE attribute of an enclosing node (`context/start_time`, ENTRY
// `language`, an ISM_TRANSITION member, ACTIVITY `timing`), and that enclosing
// node emits its own attributes at its own FLAT path — so emitting here too
// would double-spell them.
func emitLeafOwnerRMAttrs(out map[string]any, node *webtemplate.Node, flatPath string, ancestorRoot rm.Locatable, ancestorAql string) error {
	rel := bareAQLPath(strings.TrimPrefix(node.AQLPath, ancestorAql))
	ownerRel, isElementValue := strings.CutSuffix(rel, "/value")
	if !isElementValue || ownerRel == "" {
		return nil
	}
	owner, err := rmpath.ItemAtPath(ancestorRoot, ownerRel)
	if err != nil {
		return skipNotFound(err, ownerRel)
	}
	if _, isElement := as[rm.Element](owner); !isElement {
		return nil
	}
	return rmattrEncode(owner, flatPath, out)
}
