package instanceprobes_test

import (
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	mrand "math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/internal/templateinstance/rmwrite"
	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

const (
	reasonRefusalString       = "refusal_string"
	reasonRefusalAttribute    = "refusal_attribute"
	reasonRefusalOther        = "refusal_other"
	reasonPlaceholderTime     = "placeholder_time"
	reasonPlaceholderLanguage = "placeholder_language"
	reasonOrdinalSymbol       = "ordinal_symbol"
	reasonRandomInterval      = "random_interval"
	reasonFloorCluster        = "floor_cluster"
	reasonFloorElement        = "floor_element"
	reasonFloorElementValue   = "floor_element_value"
	reasonFloorTemporal       = "floor_temporal"
	reasonHollowBody          = "hollow_body"
	reasonFloorAction         = "floor_action"
	reasonFloorCoded          = "floor_coded"
	reasonValidatorOther      = "validator_other"

	entryGenerate = "generate"
	entryBuilder  = "builder"
)

// ratchetNow is the clock every corpus call shares.
var ratchetNow = time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)

// ratchetSeeds are the PCG seeds RandomFill runs with. ExampleFill ignores
// its source, so it runs once. Three seeds keep the runtime flat while
// widening the random draw past the single (1, 2) stream.
var ratchetSeeds = [][2]uint64{{1, 2}, {3, 4}, {5, 6}}

// ratchetFailure is one axis of the corpus ratchet. The reason carries a
// locator, so two distinct failures on the same axis do not share a key.
type ratchetFailure struct {
	template string
	entry    string
	policy   string
	fill     string
	reason   string
}

// corpusCompileFailures names vendored OPTs whose ParseFile or Compile
// fails. An allowlisted name that compiles, or a compile failure that
// is not listed, fails the test.
var corpusCompileFailures = []string{
	"definition/body_weight",
}

// requiredBodies names the compiling vendored templates whose body holds
// at least one ELEMENT under Required, under every value fill and seed:
// the hollow-body floor under that policy. Required builds only what the
// template makes mandatory and gives an optional child no member of its
// own (REQ-107 Visit rule, What a visit writes), so a template that
// requires no ELEMENT gets a body with none by design: the other 63
// compiling COMPOSITION templates, whose ELEMENTs all sit below an
// optional attribute or child (an optional content, or a required entry
// whose items are optional), and Address.v2, which is ADDRESS-rooted, has
// no content attribute, and leaves its details optional. An optional
// content that only its cardinality asks a member of, at existence 0..1
// and cardinality 1..* over optional archetype roots, is one Required
// leaves out, as body_weight, clinical_notes.v0 and GECCO_Diagnose have
// it. A listed template that becomes hollow under Required fails the
// ratchet, and so does an unlisted one that stops being hollow, as a
// listed row that now passes does.
var requiredBodies = []string{
	"templates/TestPerson.v2",
	"templates/test_template_rename_node_2",
}

// mediumHollowBodies names the compiling vendored templates of which a root
// Generate returns under Medium, under some value fill or seed, holds no
// ELEMENT: the hollow-body floor under that policy. Medium gives a
// multi-valued attribute at most one optional archetype, its first optional
// archetype root in OPT order that needs no undescribed content (REQ-107
// What a visit writes), so a template whose kept optional archetype holds
// no ELEMENT gets a body with none by design: clinical_content_validation,
// whose content builds only that root, a SECTION.validation_section_test.v0
// with no items. A listed template that stops being hollow under Medium
// fails the ratchet, and so does an unlisted one that becomes hollow, as an
// unlisted row does.
var mediumHollowBodies = []string{
	"templates/clinical_content_validation",
}

// corpusRatchet is the failure set measured on this tree. Policy and fill
// are the diagnostic strings from instance.Policy and instance.ValueFill.
// RandomFill runs once per seed in ratchetSeeds, a fresh math/rand/v2 PCG
// source per call; a row is observed when any seed produces it.
// Each reason is category plus a stable locator (type, attribute, code
// and path, or JSON path).
//
// Large rows: under Large every optional child that needs no undescribed
// content gets its member, each use of one archetype, named or not,
// included (REQ-107 What a visit writes). So Large builds a member of each
// optional archetype use under an attribute, and reaches the TestPerson.v2
// identifier clusters, the clinical_content_validation EVALUATION and each
// of the four named SECTION.adhoc.v1 roots of Corona_Anamnese, whose roots
// all validate. Under Large no template's body is hollow: a hollow_body
// failure there fails the ratchet.
// The builder builds on Large when no WithPolicy is given, so its rows are
// Large rows.
//
// Medium rows: Medium gives an attribute at most one optional archetype,
// its first optional archetype root. The TestPerson.v2 identifier clusters
// and the clinical_content_validation EVALUATION come after the first
// optional root of their attribute, so Medium builds neither and has no row
// there. Under Medium the hollow-body floor is the mediumHollowBodies list
// instead of a row.
//
// Required rows: none. Required visits only the attributes the template
// makes mandatory and gives an optional child no member of its own
// (REQ-107 Visit rule, What a visit writes), so it reaches none of the
// optional archetypes the rows here fail in. Under Required the
// hollow-body floor is the requiredBodies list instead of a row.
//
// No slot_fill rows: family_history.v.1.2.3's slot no example can fill sits
// in the optional CLUSTER at0024, whose required items admit only slots, so
// no policy builds that CLUSTER (REQ-107 § Slot fills) and every entry
// returns a root; TestREQ107_FamilyHistoryLeavesItsUnfillableSlotOut pins it.
//
// floor_coded rows: TestPerson.v2 codes both its DV_MULTIMEDIA media_type
// nodes in terminology openEHR, one with the code list 425 to 429 and one
// with no code. REQ-107 § Exceptions and known gaps, "An OPT constraint that
// admits no value an RM rule accepts, such as an RM default that yields to
// the OPT (§ Precedence 1)": the generator keeps the openEHR code the OPT
// gives (the person_identifier_multimedia cluster's at0001) or the at0000
// placeholder in openEHR (the person_identifier_wid cluster's at0006) over
// the text/plain default, so Media_type_valid (REQ-112 Coded invariants)
// breaks.
var corpusRatchet = []ratchetFailure{
	{template: "templates/TestPerson.v2", entry: "generate", policy: "large", fill: "example", reason: "floor_coded:code_not_in_value_set:/details/items[openEHR-DEMOGRAPHIC-CLUSTER.person_identifier_multimedia.v2]/items[at0001]/value/media_type"},
	{template: "templates/TestPerson.v2", entry: "generate", policy: "large", fill: "example", reason: "floor_coded:code_not_in_value_set:/details/items[openEHR-DEMOGRAPHIC-CLUSTER.person_identifier_wid.v2]/items[at0006]/value/media_type"},
	{template: "templates/TestPerson.v2", entry: "generate", policy: "large", fill: "random", reason: "floor_coded:code_not_in_value_set:/details/items[openEHR-DEMOGRAPHIC-CLUSTER.person_identifier_multimedia.v2]/items[at0001]/value/media_type"},
	{template: "templates/TestPerson.v2", entry: "generate", policy: "large", fill: "random", reason: "floor_coded:code_not_in_value_set:/details/items[openEHR-DEMOGRAPHIC-CLUSTER.person_identifier_wid.v2]/items[at0006]/value/media_type"},
}

func TestREQ107_CorpusRatchet(t *testing.T) {
	// PROBE-027 — Generate, then ValidateRM and the template validator,
	// over every vendored OPT that compiles (REQ-107).
	ctx := t.Context()
	refs, err := fixtures.ListAllOPTs()
	if err != nil {
		t.Fatalf("ListAllOPTs: %v", err)
	}
	if len(refs) == 0 {
		t.Fatal("ListAllOPTs returned no templates")
	}

	var compileFails []string
	observed := make(map[ratchetFailure]struct{})
	// requiredRuns records, per template, whether every root Generate
	// returned under Required holds an ELEMENT: true while each does, false
	// once one is hollow. A template with no such root is absent.
	requiredRuns := make(map[string]bool)
	// mediumHollow records the templates of which a root Generate returned
	// under Medium holds no ELEMENT.
	mediumHollow := make(map[string]struct{})
	for _, ref := range refs {
		opt, err := template.ParseFile(ref.Path)
		if err != nil {
			compileFails = append(compileFails, ref.Name)
			continue
		}
		compiled, err := templatecompile.Compile(opt)
		if err != nil {
			compileFails = append(compileFails, ref.Name)
			continue
		}
		rootType := ""
		if compiled.Root() != nil {
			rootType = compiled.Root().RMTypeName()
		}
		compositionRoot := rootType == "COMPOSITION"

		for _, policy := range []instance.Policy{instance.Large, instance.Medium, instance.Required} {
			for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
				seeds := ratchetSeeds
				if fill == instance.ExampleFill {
					seeds = seeds[:1]
				}
				for _, seed := range seeds {
					opts := instance.Options{
						Policy:      policy,
						Language:    "en",
						Now:         ratchetNow,
						ValueFill:   fill,
						ValueSource: mrand.NewPCG(seed[0], seed[1]),
					}
					if compositionRoot {
						opts.Territory = "NL"
						opts.Composer = fixedComposer()
					}
					out, err := instance.Generate(ctx, compiled, opts)
					if err != nil {
						reason := generateReason(t, err)
						recordReason(observed, ref.Name, entryGenerate, policy, fill, reason)
						continue
					}
					reasons := outputReasons(t, out, compiled, compositionRoot, fill)
					_, hollow := reasons[reasonHollowBody+":/"]
					switch policy {
					case instance.Large:
						// A hollow body under Large stays a row, so it
						// fails the ratchet.
					case instance.Required:
						delete(reasons, reasonHollowBody+":/")
						body, seen := requiredRuns[ref.Name]
						requiredRuns[ref.Name] = !hollow && (body || !seen)
					case instance.Medium:
						delete(reasons, reasonHollowBody+":/")
						if hollow {
							mediumHollow[ref.Name] = struct{}{}
						}
					}
					recordReasons(observed, ref.Name, entryGenerate, policy, fill, reasons)
				}
			}
		}

		// REQ-101 is composition-only. A non-COMPOSITION root is not built.
		if !compositionRoot {
			continue
		}
		b, err := composition.NewBuilder(ctx, compiled,
			composition.WithLanguage("en"),
			composition.WithTerritory("NL"),
			composition.WithComposer(fixedComposer()),
			composition.WithNow(ratchetNow),
		)
		if err != nil {
			reason := generateReason(t, err)
			recordReason(observed, ref.Name, entryBuilder, instance.Large, instance.ExampleFill, reason)
			continue
		}
		comp, err := b.Build()
		if err != nil {
			reason := generateReason(t, err)
			recordReason(observed, ref.Name, entryBuilder, instance.Large, instance.ExampleFill, reason)
			continue
		}
		reasons := outputReasons(t, comp, compiled, true, instance.ExampleFill)
		recordReasons(observed, ref.Name, entryBuilder, instance.Large, instance.ExampleFill, reasons)
	}

	compileUnlisted, compileStale := diffStrings(setOf(compileFails), setOf(corpusCompileFailures))
	unlisted, stale := diffRatchet(observed, ratchetSet(t, corpusRatchet))
	bodies := make(map[string]struct{})
	for name, body := range requiredRuns {
		if body {
			bodies[name] = struct{}{}
		}
	}
	bodyUnlisted, nowHollow := diffStrings(bodies, setOf(requiredBodies))
	hollowUnlisted, nowHolds := diffStrings(mediumHollow, setOf(mediumHollowBodies))
	if len(compileUnlisted) == 0 && len(compileStale) == 0 && len(unlisted) == 0 && len(stale) == 0 &&
		len(bodyUnlisted) == 0 && len(nowHollow) == 0 && len(hollowUnlisted) == 0 && len(nowHolds) == 0 {
		return
	}

	var buf strings.Builder
	if len(compileUnlisted) > 0 || len(compileStale) > 0 {
		buf.WriteString("compile-failure allowlist mismatch\n")
		buf.WriteString("  unlisted: ")
		buf.WriteString(strings.Join(compileUnlisted, ", "))
		buf.WriteString("\n  now compiles: ")
		buf.WriteString(strings.Join(compileStale, ", "))
		buf.WriteByte('\n')
	}
	if len(bodyUnlisted) > 0 || len(nowHollow) > 0 {
		buf.WriteString("Required hollow-body list (requiredBodies) mismatch\n")
		buf.WriteString("  listed, now hollow under Required: ")
		buf.WriteString(strings.Join(nowHollow, ", "))
		buf.WriteString("\n  unlisted, now holds an ELEMENT under Required: ")
		buf.WriteString(strings.Join(bodyUnlisted, ", "))
		buf.WriteByte('\n')
	}
	if len(hollowUnlisted) > 0 || len(nowHolds) > 0 {
		buf.WriteString("Medium hollow-body list (mediumHollowBodies) mismatch\n")
		buf.WriteString("  unlisted, now hollow under Medium: ")
		buf.WriteString(strings.Join(hollowUnlisted, ", "))
		buf.WriteString("\n  listed, now holds an ELEMENT under Medium: ")
		buf.WriteString(strings.Join(nowHolds, ", "))
		buf.WriteByte('\n')
	}
	writeRows(&buf, "unlisted failures", unlisted)
	writeRows(&buf, "listed cases that now pass", stale)
	t.Fatalf("REQ-107 corpus ratchet\n%s", buf.String())
}

func fixedComposer() *rm.PartyIdentified {
	name := "Test Composer"
	return &rm.PartyIdentified{Name: &name}
}

func recordReason(dst map[ratchetFailure]struct{}, template, entry string, policy instance.Policy, fill instance.ValueFill, reason string) {
	dst[ratchetFailure{
		template: template,
		entry:    entry,
		policy:   policy.String(),
		fill:     fill.String(),
		reason:   reason,
	}] = struct{}{}
}

func recordReasons(dst map[ratchetFailure]struct{}, template, entry string, policy instance.Policy, fill instance.ValueFill, reasons map[string]struct{}) {
	for reason := range reasons {
		recordReason(dst, template, entry, policy, fill, reason)
	}
}

// outputReasons returns the census reasons for one generated root: the
// errors ValidateRM and the template validator report, and the body scan's
// placeholder and hollow-body findings. Under Required and Medium the caller
// takes the hollow-body finding out and checks it against requiredBodies or
// mediumHollowBodies instead.
func outputReasons(t *testing.T, root any, compiled *templatecompile.Compiled, compositionRoot bool, fill instance.ValueFill) map[string]struct{} {
	t.Helper()
	reasons := make(map[string]struct{})
	note := func(res validation.Result) {
		if res.OK {
			return
		}
		for _, iss := range res.Issues {
			if iss.Severity != validation.Error {
				continue
			}
			reasons[issueReason(iss, fill)] = struct{}{}
		}
	}
	note(validation.ValidateRM(root))
	if compositionRoot {
		comp, err := instance.AsComposition(root)
		if err != nil {
			reasons[generateReason(t, err)] = struct{}{}
			return reasons
		}
		note(validation.ValidateComposition(comp, compiled))
	} else {
		// REQ-110: non-COMPOSITION roots use the generic validator.
		note(validation.Validate(root, compiled))
	}
	scanned, err := bodyReasons(root)
	if err != nil {
		t.Fatalf("body scan: %v", err)
	}
	for reason := range scanned {
		reasons[reason] = struct{}{}
	}
	return reasons
}

func generateReason(t *testing.T, err error) string {
	t.Helper()
	switch {
	case errors.Is(err, rmwrite.ErrUnknownRMType):
		tok, ok := lastQuotedToken(err.Error())
		if !ok {
			t.Fatalf("unknown RM type without a quoted locator: %v", err)
		}
		if tok == "STRING" {
			return reasonRefusalString + ":" + tok
		}
		return reasonRefusalOther + ":" + tok
	case errors.Is(err, rmwrite.ErrUnknownAttribute):
		tok, ok := lastQuotedToken(err.Error())
		if !ok {
			t.Fatalf("unknown attribute without a quoted locator: %v", err)
		}
		return reasonRefusalAttribute + ":" + tok
	case errors.Is(err, rmwrite.ErrTypeMismatch):
		tok, ok := lastQuotedToken(err.Error())
		if !ok {
			t.Fatalf("type mismatch without a quoted locator: %v", err)
		}
		return reasonRefusalOther + ":type_mismatch:" + tok
	case errors.Is(err, instance.ErrSlotFillUnsupported):
		path, ok := trailingPath(err.Error())
		if !ok {
			t.Fatalf("slot fill without a path: %v", err)
		}
		return reasonRefusalOther + ":slot_fill:" + path
	case errors.Is(err, instance.ErrArchetypeIDMissing):
		locator, ok := archetypeIDMissingLocator(err.Error())
		if !ok {
			t.Fatalf("archetype id missing without a path: %v", err)
		}
		return reasonRefusalOther + ":archetype_id_missing:" + locator
	case errors.Is(err, instance.ErrTypeMismatch):
		return reasonRefusalOther + ":type_mismatch"
	default:
		t.Fatalf("unclassified generate error: %v", err)
		return ""
	}
}

// issueReason keys a validator finding by category, code, and path.
// The category is chosen from the code and the path's last attribute.
// The two REQ-112 rm_invariant rules that have no path signature of their
// own, the ELEMENT null-flavour rule and the temporal Value_valid rule, are
// told apart by the RM invariant name in the detail, so a regression of
// either keeps a precise key. A REQ-112 coded invariant has a code of its
// own, code_not_in_value_set, which keys it as floor_coded. Any other detail
// text is not read.
func issueReason(iss validation.Issue, fill instance.ValueFill) string {
	last := lastAttr(iss.Path)
	cat := reasonValidatorOther
	switch {
	case iss.Code == "code_not_in_value_set":
		cat = reasonFloorCoded
	case iss.Code == "rm_invariant" && strings.Contains(iss.Detail, "Inv_null_flavour_indicated"):
		cat = reasonFloorElementValue
	case iss.Code == "rm_invariant" && last == "value" && strings.Contains(iss.Detail, "Value_valid"):
		cat = reasonFloorTemporal
	case last == "symbol" && iss.Code == "required":
		cat = reasonOrdinalSymbol
	case fill == instance.RandomFill && iss.Code == "primitive_unit_unknown" && (last == "lower" || last == "upper"):
		cat = reasonRandomInterval
	case last == "items" && (iss.Code == "cardinality" || iss.Code == "required"):
		cat = reasonFloorCluster
	case iss.Code == "required" && (last == "name" || last == "archetype_node_id"):
		cat = reasonFloorElement
	case iss.Code == "required" && (last == "time" || last == "ism_transition"):
		cat = reasonFloorAction
	}
	return cat + ":" + iss.Code + ":" + iss.Path
}

// bodyReasons decodes root as canonical JSON and returns the placeholder
// findings plus a hollow_body finding when the body holds no ELEMENT. A body
// with no ELEMENT passes ValidateRM and the template validator vacuously, so
// the count is the coverage floor that keeps those two checks honest.
func bodyReasons(root any) (map[string]struct{}, error) {
	raw, err := canjson.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("canjson.Marshal: %w", err)
	}
	var tree any
	if err := jsonv2.Unmarshal(raw, &tree); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}
	if _, ok := tree.(map[string]any); !ok {
		return nil, fmt.Errorf("canonical JSON decoded as %T, want map[string]any", tree)
	}
	reasons := make(map[string]struct{})
	walkPlaceholder(tree, "", reasons)
	if countElements(tree) == 0 {
		reasons[reasonHollowBody+":/"] = struct{}{}
	}
	return reasons, nil
}

// countElements counts the nodes of a decoded canonical JSON tree whose
// _type is ELEMENT.
func countElements(v any) int {
	n := 0
	switch node := v.(type) {
	case map[string]any:
		if typ, _ := node["_type"].(string); typ == "ELEMENT" {
			n++
		}
		for _, child := range node {
			n += countElements(child)
		}
	case []any:
		for _, child := range node {
			n += countElements(child)
		}
	}
	return n
}

func walkPlaceholder(v any, path string, reasons map[string]struct{}) {
	switch n := v.(type) {
	case map[string]any:
		loc := path
		if loc == "" {
			loc = "/"
		}
		if typ, _ := n["_type"].(string); typ == "DV_DATE_TIME" && stringField(n, "value") == "example" {
			reasons[reasonPlaceholderTime+":"+loc] = struct{}{}
		}
		if attr := lastAttr(loc); (attr == "language" || attr == "encoding") && languageExample(n) {
			reasons[reasonPlaceholderLanguage+":"+loc] = struct{}{}
		}
		for k, child := range n {
			walkPlaceholder(child, joinKey(loc, k), reasons)
		}
	case []any:
		base := path
		if base == "" {
			base = "/"
		}
		for i, child := range n {
			walkPlaceholder(child, fmt.Sprintf("%s[%d]", base, i), reasons)
		}
	case string:
		if n != "example" {
			return
		}
		switch lastAttr(path) {
		case "time", "origin":
			reasons[reasonPlaceholderTime+":"+path] = struct{}{}
		case "language", "encoding":
			reasons[reasonPlaceholderLanguage+":"+path] = struct{}{}
		}
	}
}

// languageExample reports an ENTRY language or encoding whose code or
// value is exactly "example".
func languageExample(n map[string]any) bool {
	if stringField(n, "code_string") == "example" {
		return true
	}
	_, hasCode := n["code_string"]
	return !hasCode && stringField(n, "value") == "example"
}

func stringField(n map[string]any, key string) string {
	s, _ := n[key].(string)
	return s
}

func joinKey(parent, key string) string {
	if parent == "" || parent == "/" {
		return "/" + key
	}
	return parent + "/" + key
}

// lastQuotedToken returns the last double-quoted identifier in msg.
// The token is one run of letters, digits, and underscores, so a value
// such as "example.com" does not match.
func lastQuotedToken(msg string) (string, bool) {
	var last string
	found := false
	for i := 0; i < len(msg); i++ {
		if msg[i] != '"' {
			continue
		}
		j := i + 1
		for j < len(msg) && isTokenByte(msg[j]) {
			j++
		}
		if j > i+1 && j < len(msg) && msg[j] == '"' {
			last = msg[i+1 : j]
			found = true
			i = j
		}
	}
	return last, found
}

func isTokenByte(b byte) bool {
	return b == '_' || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}

// trailingPath returns the AQL path at the end of a slot-fill error.
// A name predicate may contain a space; a tab means the suffix is not a path.
func trailingPath(msg string) (string, bool) {
	const mark = ": /"
	i := strings.LastIndex(msg, mark)
	if i < 0 {
		return "", false
	}
	p := msg[i+len(mark)-1:]
	if p == "/" || strings.Contains(p, "\t") {
		return "", false
	}
	return p, true
}

// archetypeIDMissingLocator returns the locator an ErrArchetypeIDMissing
// error keys by. It reads the text after the sentinel's own: the OPT path
// from the first " at /", without the parenthesised note that may follow
// it. A path never ends in ")", and no note holds " (", so the last " (" of
// a message ending in ")" starts the note even when a name predicate in the
// path holds one. For a required attribute the template leaves without
// children ("<type> for <CLASS>.<attribute> at <path> (...)") the locator is
// the attribute's path, so that refusal keys apart from one at the node
// itself. A tab means the text is not a path.
func archetypeIDMissingLocator(msg string) (string, bool) {
	_, detail, ok := strings.Cut(msg, instance.ErrArchetypeIDMissing.Error()+": ")
	if !ok {
		return "", false
	}
	head, rest, ok := strings.Cut(detail, " at /")
	if !ok {
		return "", false
	}
	p := "/" + rest
	if strings.HasSuffix(p, ")") {
		if i := strings.LastIndex(p, " ("); i >= 0 {
			p = p[:i]
		}
	}
	if strings.Contains(p, "\t") {
		return "", false
	}
	if _, owner, isAttr := strings.Cut(head, " for "); isAttr {
		i := strings.LastIndex(owner, ".")
		if i < 0 || i == len(owner)-1 {
			return "", false
		}
		p = strings.TrimSuffix(p, "/") + "/" + owner[i+1:]
	}
	return p, true
}

func lastAttr(path string) string {
	path = strings.TrimSuffix(path, "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		path = path[i+1:]
	}
	if i := strings.IndexByte(path, '['); i >= 0 {
		path = path[:i]
	}
	return path
}

func (r ratchetFailure) line() string {
	return r.template + "\t" + r.entry + "\t" + r.policy + "\t" + r.fill + "\t" + r.reason
}

func ratchetSet(t *testing.T, rows []ratchetFailure) map[ratchetFailure]struct{} {
	t.Helper()
	out := make(map[ratchetFailure]struct{}, len(rows))
	for _, row := range rows {
		if _, ok := out[row]; ok {
			t.Errorf("duplicate ratchet row %s", row.line())
		}
		out[row] = struct{}{}
	}
	return out
}

func diffRatchet(got, want map[ratchetFailure]struct{}) (unlisted, stale []ratchetFailure) {
	for row := range got {
		if _, ok := want[row]; !ok {
			unlisted = append(unlisted, row)
		}
	}
	for row := range want {
		if _, ok := got[row]; !ok {
			stale = append(stale, row)
		}
	}
	slices.SortFunc(unlisted, cmpRatchet)
	slices.SortFunc(stale, cmpRatchet)
	return unlisted, stale
}

func cmpRatchet(a, b ratchetFailure) int {
	if c := strings.Compare(a.template, b.template); c != 0 {
		return c
	}
	if c := strings.Compare(a.entry, b.entry); c != 0 {
		return c
	}
	if c := strings.Compare(a.policy, b.policy); c != 0 {
		return c
	}
	if c := strings.Compare(a.fill, b.fill); c != 0 {
		return c
	}
	return strings.Compare(a.reason, b.reason)
}

func setOf(names []string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[name] = struct{}{}
	}
	return out
}

func diffStrings(got, want map[string]struct{}) (unlisted, stale []string) {
	for name := range got {
		if _, ok := want[name]; !ok {
			unlisted = append(unlisted, name)
		}
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			stale = append(stale, name)
		}
	}
	slices.Sort(unlisted)
	slices.Sort(stale)
	return unlisted, stale
}

func writeRows(buf *strings.Builder, title string, rows []ratchetFailure) {
	if len(rows) == 0 {
		return
	}
	buf.WriteString(title)
	buf.WriteString(":\n")
	for _, row := range rows {
		buf.WriteString("  ")
		buf.WriteString(row.line())
		buf.WriteByte('\n')
	}
}
