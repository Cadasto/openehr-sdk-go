// Package termgen generates the openehr/terminology tables from the pinned
// openEHR Terminology XML under resources/terminology/ (REQ-034).
//
// It is the terminology counterpart of internal/bmmgen: [Parse] decodes and
// validates the pin, [Render] emits one gofmt-clean openehr_gen.go, and [Run]
// either writes that file or — in verify mode — reports how it drifts from
// the pin without touching it. cmd/termgen is the CLI; `make termgen` and
// `make termgen-verify` are the entry points.
package termgen

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// terminologyName is the only terminology this generator accepts. The pin is
// the openEHR Foundation's own `openehr` terminology — the one the RM's
// invariants reference — so a document naming any other is a wrong file, not
// a variant to generate from.
const terminologyName = "openehr"

// idPattern is the openehr_id shape the generator accepts: lower-case ASCII
// words joined by underscores. It is what makes [GoName]'s mangling total.
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Terminology is one parsed openehr_terminology.xml: the release metadata
// plus every code set and group of the pin, in document order.
type Terminology struct {
	Name     string
	Language string
	Version  string
	Date     string
	CodeSets []CodeSetDef
	Groups   []GroupDef
}

// CodeSetDef is one <codeset> of the pin — a value set of bare codes with no
// rubrics, such as the normal statuses.
type CodeSetDef struct {
	ID    string
	Name  string
	Codes []string
}

// GroupDef is one <group> of the pin — a value set of coded concepts, each
// with an English rubric.
type GroupDef struct {
	ID       string
	Name     string
	Concepts []ConceptDef
}

// ConceptDef is one <concept> of a group: its code (the XML's id attribute)
// and its rubric.
type ConceptDef struct {
	Code   string
	Rubric string
}

// xmlTerminology mirrors the pin's element shape for encoding/xml; [Parse]
// converts it into the exported [Terminology]. The indirection earns its
// keep twice over: a <code value="N"/> carries its code in an attribute,
// which encoding/xml cannot decode straight into a []string field, and the
// XMLName field makes "this is not the terminology pin" a decode error.
type xmlTerminology struct {
	XMLName  xml.Name     `xml:"terminology"`
	Name     string       `xml:"name,attr"`
	Language string       `xml:"language,attr"`
	Version  string       `xml:"version,attr"`
	Date     string       `xml:"date,attr"`
	CodeSets []xmlCodeSet `xml:"codeset"`
	Groups   []xmlGroup   `xml:"group"`
}

type xmlCodeSet struct {
	ID    string    `xml:"openehr_id,attr"`
	Name  string    `xml:"name,attr"`
	Codes []xmlCode `xml:"code"`
}

type xmlCode struct {
	Value string `xml:"value,attr"`
}

type xmlGroup struct {
	ID       string       `xml:"openehr_id,attr"`
	Name     string       `xml:"name,attr"`
	Concepts []xmlConcept `xml:"concept"`
}

type xmlConcept struct {
	Code   string `xml:"id,attr"`
	Rubric string `xml:"rubric,attr"`
}

// Parse decodes openehr_terminology.xml and validates the invariants the
// generated tables rely on: it is the openEHR terminology, it names a
// release, every code and rubric of a group is present and unique within it,
// every code of a code set is present and unique within it, no table is
// empty, and every openehr_id is lower-case snake_case and mangles to a Go
// name no other id claims.
//
// Every breach comes back as an error naming the offending id — the pin is
// vendored from upstream, so a bad table is a version-bump surprise the
// maintainer has to read, not a programmer error to panic on.
func Parse(r io.Reader) (*Terminology, error) {
	var doc xmlTerminology
	dec := xml.NewDecoder(r)
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode the terminology XML: %w", err)
	}
	if doc.Name != terminologyName {
		return nil, fmt.Errorf("terminology name is %q, want %q — this is not the openEHR Terminology pin", doc.Name, terminologyName)
	}
	if doc.Version == "" {
		return nil, errors.New("terminology carries no version attribute — the generated tables must name the release they came from")
	}
	// encoding/xml stops at the first element, so a second <terminology> or any
	// trailing element would vanish silently past the sha256 pin. Require the
	// document to hold exactly one terminology root.
	if err := expectEOF(dec); err != nil {
		return nil, err
	}

	t := &Terminology{Name: doc.Name, Language: doc.Language, Version: doc.Version, Date: doc.Date}
	// taken maps each Go name already claimed to the openehr_id it was
	// mangled from, so one map catches a repeated id (in either kind) and a
	// mangling collision between two different ids.
	taken := make(map[string]string, len(doc.CodeSets)+len(doc.Groups))
	for _, cs := range doc.CodeSets {
		def, err := codeSetDef(cs)
		if err != nil {
			return nil, err
		}
		if err := claimGoName(taken, "code set", cs.ID); err != nil {
			return nil, err
		}
		t.CodeSets = append(t.CodeSets, def)
	}
	for _, g := range doc.Groups {
		def, err := groupDef(g)
		if err != nil {
			return nil, err
		}
		if err := claimGoName(taken, "group", g.ID); err != nil {
			return nil, err
		}
		t.Groups = append(t.Groups, def)
	}
	if len(t.Groups) == 0 || len(t.CodeSets) == 0 {
		return nil, fmt.Errorf("terminology has %d group(s) and %d code set(s) — the pin must carry at least one of each; a renamed or dropped table would otherwise generate a silently incomplete vocabulary", len(t.Groups), len(t.CodeSets))
	}
	return t, nil
}

// expectEOF reports an error when the decoder holds any further element after
// the terminology root — a well-formed pin carries exactly one. Trailing
// whitespace, comments and processing instructions are ignored.
func expectEOF(dec *xml.Decoder) error {
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read past the terminology root: %w", err)
		}
		if se, ok := tok.(xml.StartElement); ok {
			return fmt.Errorf("trailing <%s> after the terminology root — the pin must hold exactly one terminology element", se.Name.Local)
		}
	}
}

// codeSetDef validates one <codeset> and returns it as a [CodeSetDef].
func codeSetDef(cs xmlCodeSet) (CodeSetDef, error) {
	def := CodeSetDef{ID: cs.ID, Name: cs.Name}
	seen := make(map[string]bool, len(cs.Codes))
	for _, c := range cs.Codes {
		switch {
		case c.Value == "":
			return CodeSetDef{}, fmt.Errorf("code set %q has a code with no value attribute", cs.ID)
		case seen[c.Value]:
			return CodeSetDef{}, fmt.Errorf("code set %q repeats code %q — membership would be ambiguous", cs.ID, c.Value)
		}
		seen[c.Value] = true
		def.Codes = append(def.Codes, c.Value)
	}
	if len(def.Codes) == 0 {
		return CodeSetDef{}, fmt.Errorf("code set %q has no codes", cs.ID)
	}
	return def, nil
}

// groupDef validates one <group> and returns it as a [GroupDef]. Rubrics are
// keys as much as codes are: the accessor looks a code up by its rubric, so a
// group repeating one would make that lookup arbitrary.
func groupDef(g xmlGroup) (GroupDef, error) {
	def := GroupDef{ID: g.ID, Name: g.Name}
	codes := make(map[string]bool, len(g.Concepts))
	rubrics := make(map[string]bool, len(g.Concepts))
	for _, c := range g.Concepts {
		switch {
		case c.Code == "":
			return GroupDef{}, fmt.Errorf("group %q has a concept with no id attribute", g.ID)
		case c.Rubric == "":
			return GroupDef{}, fmt.Errorf("group %q: concept %q has no rubric attribute", g.ID, c.Code)
		case codes[c.Code]:
			return GroupDef{}, fmt.Errorf("group %q repeats code %q — its rubric would be ambiguous", g.ID, c.Code)
		case rubrics[c.Rubric]:
			return GroupDef{}, fmt.Errorf("group %q repeats rubric %q — the rubric-to-code lookup would be ambiguous", g.ID, c.Rubric)
		}
		codes[c.Code] = true
		rubrics[c.Rubric] = true
		// xmlConcept and ConceptDef carry the same two fields, so the
		// conversion is the whole mapping (and stops compiling if either
		// side gains one).
		def.Concepts = append(def.Concepts, ConceptDef(c))
	}
	if len(def.Concepts) == 0 {
		return GroupDef{}, fmt.Errorf("group %q has no concepts", g.ID)
	}
	return def, nil
}

// claimGoName validates one openehr_id and records the exported Go name it
// mangles to, refusing an id the generator cannot turn into a distinct
// package-level variable. kind ("group" / "code set") only colours the
// message about the id's shape; a repeat or a collision is reported by id,
// whichever kind the ids came from.
func claimGoName(taken map[string]string, kind, id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("%s openehr_id %q is not a lower-case snake_case name (want %s)", kind, id, idPattern)
	}
	name := GoName(id)
	if other, ok := taken[name]; ok {
		if other == id {
			return fmt.Errorf("openehr_id %q appears twice", id)
		}
		return fmt.Errorf("openehr_id %q and %q both mangle to the Go name %s", other, id, name)
	}
	taken[name] = id
	return nil
}

// GoName mangles an openehr_id into the exported Go identifier the generated
// tables use for it: audit_change_type becomes AuditChangeType. Empty parts
// (a doubled underscore) vanish, which is why [Parse] refuses two ids that
// mangle to the same name.
func GoName(id string) string {
	var b strings.Builder
	b.Grow(len(id))
	for part := range strings.SplitSeq(id, "_") {
		if part == "" {
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]))
		b.WriteString(part[1:])
	}
	return b.String()
}
