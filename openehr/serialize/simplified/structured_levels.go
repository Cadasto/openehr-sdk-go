package simplified

import (
	"maps"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// One rule judges the members of a STRUCTURED body at every level (REQ-170):
// a member whose name the FLAT grammar does not read at that level is set
// aside and reported by its member path, whatever the JSON shape of its value;
// a name the grammar reads keeps the shape refusals. A [memberLevel] is what
// the grammar reads among the members of one object, and [memberLevel.judge]
// applies the rule to one name, giving the level its elements stand for.
//
// The levels mirror the FLAT decode's routing: the Web Template's nodes, its
// leaves and their sub-path grammars (an interval's bounds, a party's
// sub-structure), the underscore families an owner carries and the grammar
// inside each, the metadata spellings below the root, and the reference
// spellings the decode recognises and refuses. The tables below are built from
// the ones the FLAT decode reads, so the two cannot name different sub-paths.

// levelKind says how a [memberLevel] judges a name.
type levelKind int

const (
	// levelOpaque reads every name and judges nothing below it: the content
	// is the FLAT decode's to refuse (a deferred family, PARTICIPATION.time,
	// FEEDER_AUDIT_DETAILS.other_details, an ISM transition's `_reason`), or a
	// body converted without a template.
	levelOpaque levelKind = iota
	// levelNode is an object at a Web Template node, a leaf included.
	levelNode
	// levelMetadata is an object at a composition-metadata spelling the Web
	// Template carries no node for (a `context` the template leaves out).
	levelMetadata
	// levelGrammar is an object below a leaf or inside an underscore family:
	// it reads the sub-path names in sub, and its suffixes.
	levelGrammar
)

// memberLevel is what the FLAT grammar reads among the members of one
// STRUCTURED object.
type memberLevel struct {
	kind levelKind
	// node is the Web Template node a levelNode object stands for.
	node *webtemplate.Node
	// rel is the path of ids below the template root that reaches a levelNode
	// or levelMetadata object, the form metadata spellings are listed in.
	rel string
	// sub names the sub-path members a levelGrammar object reads, each with
	// the level its elements stand for. A nil sub reads suffixes only.
	sub map[string]memberLevel
}

// opaqueLevel reads every name below it.
var opaqueLevel = memberLevel{kind: levelOpaque}

// suffixesOnly reads no sub-path name: only the suffix members.
var suffixesOnly = memberLevel{kind: levelGrammar}

// grammarOf is a levelGrammar reading each of names with suffixes only below
// it.
func grammarOf(names ...map[string]bool) memberLevel {
	sub := make(map[string]memberLevel)
	for _, set := range names {
		for name := range set {
			sub[name] = suffixesOnly
		}
	}
	return memberLevel{kind: levelGrammar, sub: sub}
}

// with returns l reading name as level too.
func (l memberLevel) with(name string, level memberLevel) memberLevel {
	sub := make(map[string]memberLevel, len(l.sub)+1)
	maps.Copy(sub, l.sub)
	sub[name] = level
	return memberLevel{kind: levelGrammar, sub: sub}
}

var (
	// partyLevel is the party grammar ([partyTails]): `relationship` and the
	// nested `_identifier` list, each in its own suffix form.
	partyLevel = grammarOf(partySubTails, partyListTails)
	// participationLevel is PARTICIPATION's grammar: the performer's
	// `relationship`, and `time`, which the decode refuses in every spelling.
	participationLevel = grammarOf(partySubTails).with(participationTimeSuffix, opaqueLevel)
	// feederDetailsLevel is FEEDER_AUDIT_DETAILS: its three parties, and
	// `other_details`, which the decode refuses.
	feederDetailsLevel = func() memberLevel {
		l := memberLevel{kind: levelGrammar, sub: map[string]memberLevel{feederAuditOtherDetails: opaqueLevel}}
		for seg := range feederAuditDetailsParties {
			l.sub[seg] = partyLevel
		}
		return l
	}()
	// feederLevel is FEEDER_AUDIT: the two identifier lists, the two details
	// objects and the two spellings of original_content.
	feederLevel = func() memberLevel {
		l := memberLevel{kind: levelGrammar, sub: map[string]memberLevel{}}
		for seg := range feederAuditItemIDLists {
			l.sub[seg] = suffixesOnly
		}
		for seg := range feederAuditDetailSegs {
			l.sub[seg] = feederDetailsLevel
		}
		for seg := range feederAuditContentSegs {
			l.sub[seg] = suffixesOnly
		}
		return l
	}()
	// familyLevels is the grammar inside each underscore family that has
	// sub-paths. Every other family the decode carries reads its suffixes
	// only.
	familyLevels = map[string]memberLevel{
		"_health_care_facility":   partyLevel,
		"_provider":               partyLevel,
		"_participation":          participationLevel,
		"_other_participation":    participationLevel,
		"_normal_range":           grammarOf(intervalSubTails),
		"_other_reference_ranges": grammarOf(refRangeSubTails),
		"_mapping":                grammarOf(mappingSubTails),
		"_feeder_audit":           feederLevel,
	}
	// ismReferenceLevel is the reference's in-context `ism_transition` node,
	// which the decode refuses wherever it reads one of its leaves or its
	// `_reason` family; any other name below it is unmatched.
	ismReferenceLevel = grammarOf(ismLeaves).with(ismReasonFamily, opaqueLevel)
)

// nodeLevel is the level of an object at the Web Template node node, reached
// by the ids rel below the root.
func nodeLevel(node *webtemplate.Node, rel string) memberLevel {
	return memberLevel{kind: levelNode, node: node, rel: rel}
}

// familyLevel is the level inside the underscore family the decode reads at
// an owner of RM class kind holding leaf ("" unless the owner is a collapsed
// ELEMENT). read is false where the family names nothing there.
func familyLevel(kind, leaf, name string) (memberLevel, bool) {
	if rmattrDeferredAt(kind, name) {
		return opaqueLevel, true
	}
	if _, _, read := rmattrFamilyOf(kind, leaf, name); !read {
		return memberLevel{}, false
	}
	if l, ok := familyLevels[name]; ok {
		return l, true
	}
	return suffixesOnly, true
}

// memberBase is the part of a member name that the grammar judges: the first
// `/`-separated segment, and of it the id before a `:` or `|`.
func memberBase(name string) string {
	base, _, _ := strings.Cut(name, "/")
	if cut := strings.IndexAny(base, ":|"); cut >= 0 {
		base = base[:cut]
	}
	return base
}

// judge applies the one rule to member name of an object at level l: it
// returns the level the member's elements stand for, and read false where
// the FLAT grammar reads no such name here. Suffix members are not judged
// here; the FLAT decode judges the key each makes.
//
// A name that spells more than one FLAT piece (a `:`, `|` or `/` in it) is
// judged by its base ([memberBase]). Where the base reads nothing the member
// is set aside like any other. Where the base reads something the member is
// flattened as written, the FLAT decode judges the key it spells, and a value
// that is not an array keeps the shape refusal; nothing below it is judged.
func (l memberLevel) judge(name string) (memberLevel, bool) {
	if base := memberBase(name); base != name {
		if _, read := l.judge(base); !read {
			return memberLevel{}, false
		}
		return opaqueLevel, true
	}
	switch l.kind {
	case levelNode:
		if len(l.node.Children) == 0 {
			return l.judgeLeaf(name)
		}
		return l.judgeNode(name)
	case levelGrammar:
		below, read := l.sub[name]
		return below, read
	case levelMetadata:
		return l.judgeMetadata(name)
	case levelOpaque:
	}
	return opaqueLevel, true
}

// judgeNode judges a member of an object at a Web Template node with
// children: a child node, an underscore family the node carries, a metadata
// spelling below the root, and a reference spelling the decode refuses
// ([refuseInContextKeys]).
func (l memberLevel) judgeNode(name string) (memberLevel, bool) {
	rel := strings.TrimPrefix(l.rel+"/"+name, "/")
	if child := childByID(l.node, name); child != nil {
		return nodeLevel(child, rel), true
	}
	rmType := nodeRMType(l.node)
	if strings.HasPrefix(name, "_") {
		if rmType == "ISM_TRANSITION" && name == ismReasonFamily {
			return opaqueLevel, true
		}
		return familyLevel(rmType, "", name)
	}
	switch {
	case metadataSegPaths[rel]:
		return memberLevel{kind: levelMetadata, rel: rel}, true
	case rmType == "ACTION" && name == ismReferenceSeg:
		return ismReferenceLevel, true
	case inContextAttrs[rmType].leaf[name], rmType == "ISM_TRANSITION" && ismLeaves[name]:
		return suffixesOnly, true
	}
	return memberLevel{}, false
}

// judgeLeaf judges a member of an element of a Web Template leaf: a sub-path
// of the leaf's grammar (an interval's `lower` and `upper`, a party's
// `relationship` and `_identifier`) or an underscore family the leaf's owner
// carries. A collapsed ELEMENT leaf owns the families of its ELEMENT and, for
// a value family, of the datatype it holds (REQ-140); a leaf on any other
// attribute owns none, as [rmattrOwnerAt] resolves no owner for it. An
// unparameterised DV_INTERVAL reads no sub-path.
func (l memberLevel) judgeLeaf(name string) (memberLevel, bool) {
	rmType := nodeRMType(l.node)
	switch {
	case isIntervalLeafType(rmType) && intervalSubTails[name]:
		return suffixesOnly, true
	case isPartyLeafType(rmType):
		below, read := partyLevel.sub[name]
		return below, read
	case !strings.HasPrefix(name, "_") || !strings.HasSuffix(bareAQLPath(l.node.AQLPath), "/value"):
		return memberLevel{}, false
	}
	return familyLevel("ELEMENT", rmType, name)
}

// judgeMetadata judges a member of an object at a metadata spelling the Web
// Template has no node for: a longer metadata spelling, and below `context`,
// which the decode resolves as the EVENT_CONTEXT without the Web Template
// ([rmattrOwnerAt]), the families an EVENT_CONTEXT carries.
func (l memberLevel) judgeMetadata(name string) (memberLevel, bool) {
	rel := l.rel + "/" + name
	switch {
	case metadataSegPaths[rel]:
		return memberLevel{kind: levelMetadata, rel: rel}, true
	case l.rel == "context" && strings.HasPrefix(name, "_"):
		return familyLevel("EVENT_CONTEXT", "", name)
	}
	return memberLevel{}, false
}
