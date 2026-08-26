package rminfo_test

// PROBE-098 — the compiled-in absence table accounts for the class universe's
// negative space (REQ-049).
//
// The expectation is the SAME independent reduction PROBE-094 builds
// (probe_094_test.go: the pinned schemas read through the openehr/bmm loader,
// REQ-045, with the generator's exclusion lists restated as literals).
// exclusionKindOf is the one derivation both probes share; this file maps its
// answer onto the shipped AbsenceReason and compares. internal/bmmgen is never
// imported — comparing the table against the walk that produced it would pass
// on a generator whose walk is itself wrong, which is what these arms exist to
// catch.
//
// WHAT THE SHIPPED SURFACE CAN WITNESS. The generated table is unexported, so
// the probe asks about names rather than enumerating keys. Every claim REQ-049
// makes is reachable that way except one: a stored entry whose name is also a
// SHIPPED universe member, which (*lookup).AbsenceReason masks by design —
// membership is answered first, and absence_test.go's
// TestUniverseWinsOverAnAbsenceEntry pins that masking as intended behaviour.
// That one claim is held where the keys are visible instead:
// internal/bmmgen's TestRMInfoAbsenceAccountsForThePinnedBMM asserts the
// universe and the table partition the declared names, key by key, and
// `make codegen-verify` holds the committed file to what the generator emits.
// Saying so here is the difference between an arm that is honest about its
// reach and one that reads as more than it proves.

import (
	"maps"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// shippedReasonFor maps the reduction's own taxonomy onto the shipped enum.
// This function IS the comparison: everything to the left of it is derived from
// the raw BMM, everything to the right is what rminfo ships.
//
// isAnAnswer=false for the three kinds that are not exclusions. For a name the
// pinned schemas DECLARE and the universe omits, those are not answers at all:
// REQ-049 § Generated, accounted, computed makes an unaccounted declared name a
// generation failure, never an *undeclared* reading — that failure mode is what
// keeps a widened package skip from silently reclassifying a real openEHR class
// as a name nobody declared.
func shippedReasonFor(kind exclusionKind) (reason rminfo.AbsenceReason, isAnAnswer bool) {
	switch kind {
	case inTheUniverse:
		return rminfo.AbsenceNone, true
	case excludedAsPrimitive:
		return rminfo.AbsencePrimitive, true
	case excludedAsEnumeration:
		return rminfo.AbsenceEnumeration, true
	case excludedByClassName:
		return rminfo.AbsenceExcludedClass, true
	case excludedByPackagePrefix:
		return rminfo.AbsenceExcludedPackage, true
	case notDeclaredAtAll, unaccountedFor:
		return rminfo.AbsenceNone, false
	}
	// A kind added to exclusionKind without a mapping lands here. Reporting
	// isAnAnswer=false makes that a named failure at the call site rather
	// than a silent *none* — the fail-closed posture REQ-049 requires of
	// anyone switching on this taxonomy.
	return rminfo.AbsenceNone, false
}

// declaredNames is the reduction's declared set — `class_definitions` ∪
// `primitive_types`, which is exactly what bmmReduction.classOf keys (see its
// field comment). Both maps, because REQ-049 § Generated, accounted, computed
// names both as the accounting's input: a primitive_types-only name such as
// `Interval` is declared, omitted, and must carry a reason.
//
// Sorted, so a failure list reads the same on every run.
func declaredNames(r *bmmReduction) []string {
	return slices.Sorted(maps.Keys(r.classOf))
}

// absenceReasonOrder is the reporting order for census output and pinned
// expectations. It exists so a failure message and a log line list the reasons
// the same way every run — the enum's numeric values carry no contract.
var absenceReasonOrder = []rminfo.AbsenceReason{
	rminfo.AbsenceNone,
	rminfo.AbsencePrimitive,
	rminfo.AbsenceEnumeration,
	rminfo.AbsenceExcludedClass,
	rminfo.AbsenceExcludedPackage,
}

// syntheticUndeclared are names no pinned schema mentions at all — the shapes a
// caller actually supplies by mistake. The empty string is deliberate: REQ-049
// requires it to report *undeclared* rather than the zero-valued *none* an
// inverted membership test would give.
var syntheticUndeclared = []string{
	"",
	"NEVER_AN_RM_CLASS",
	"composition",                // a real class, wrong case
	"EXTRACT_",                   // a near-miss of a real table key
	"org.openehr.rm.ehr_extract", // a package path, not a class name
}

// TestProbe098AbsenceAccountsForTheNegativeSpace — arm (a). Accounting, both
// directions, over every name the pinned schemas declare: the universe's
// members must report *none* and every omission must report the reason the
// reduction derives for it, under REQ-049's fixed precedence.
//
// Walking the DECLARED set rather than only the omissions is what makes the
// second direction bite. A table entry for a name the reduction places in the
// universe is caught here — the shipped answer would be that stored reason
// where the reduction expects *none* — unless the shipped universe also
// contains the name, the one masked case the file header accounts for.
func TestProbe098AbsenceAccountsForTheNegativeSpace(t *testing.T) {
	r := reducePinnedBMM(t)
	rep := reporter(t, rminfo.Default)

	census := map[rminfo.AbsenceReason]int{}
	for _, name := range declaredNames(r) {
		want, isAnAnswer := shippedReasonFor(r.exclusionKindOf(name))
		if !isAnAnswer {
			reason, _ := r.exclusionReason(name)
			t.Errorf("declared name %q is out of the universe but %s — REQ-049 makes that a "+
				"generation failure naming the class, never an *undeclared* answer", name, reason)
			continue
		}
		if got := rep.AbsenceReason(name); got != want {
			t.Errorf("AbsenceReason(%s) = %v, the reduction derives %v", name, got, want)
		}
		census[want]++
	}

	// The census is what keeps the walk above from being one-directional.
	// The stored table's keys are unreachable, so an expectation that
	// quietly shrinks — a name dropped from the reduction, which would leave
	// its table entry simply unasked-about — has to fail on the SHAPE of the
	// walk instead. Pinned per reason rather than as one total, so a name
	// moving between two rules cannot cancel out.
	//
	// Where the excluded-package count comes from, measured rather than
	// assumed: all 30 sit under org.openehr.rm.ehr_extract. The other three
	// skipped prefixes contribute NOTHING to this reason — a nearer rule
	// (primitive, or the named-class set) claims every name they hold — which
	// is why a package skip widening to a foundation package would show up
	// here as a count nobody expected rather than as no change at all.
	wantCensus := map[rminfo.AbsenceReason]int{
		rminfo.AbsencePrimitive:       29, // the § Primitive type mapping table, entire
		rminfo.AbsenceEnumeration:     3,  // PROPORTION_KIND, VALIDITY_KIND, VERSION_STATUS
		rminfo.AbsenceExcludedClass:   12, // the 11 belt-and-braces names + Comparable
		rminfo.AbsenceExcludedPackage: 30, // all of ehr_extract; see below
	}
	absent := 0
	for _, reason := range absenceReasonOrder {
		if reason == rminfo.AbsenceNone {
			continue
		}
		absent += census[reason]
		if census[reason] != wantCensus[reason] {
			t.Errorf("%v: the reduction derives it for %d declared names, want %d",
				reason, census[reason], wantCensus[reason])
		}
	}
	// The two halves must partition the declared names — no name in both, no
	// declared name in neither (REQ-049 § Generated, accounted, computed).
	if census[rminfo.AbsenceNone] != len(r.universe) {
		t.Errorf("%d declared names report none, but the reduction's universe holds %d",
			census[rminfo.AbsenceNone], len(r.universe))
	}
	if census[rminfo.AbsenceNone]+absent != len(r.classOf) {
		t.Errorf("universe %d + absent %d != %d declared names",
			census[rminfo.AbsenceNone], absent, len(r.classOf))
	}
	for _, reason := range absenceReasonOrder {
		t.Logf("%-16v %d declared names", reason, census[reason])
	}
}

// TestProbe098NoOverlapWithTheUniverse — arm (b). Membership and absence answer
// the same question, so the two must never disagree: every KnownRMTypes member
// reports *none*, and every declared name reporting *none* is a KnownRMTypes
// member (REQ-049 § Negative-space consistency).
//
// That is the half of arm (b) the shipped answers can witness. Its other two
// claims are about the stored table's KEYS — that it holds no universe member
// and no *undeclared* entry — and the surface answers neither: a universe
// member's entry is masked by the membership check, and a stored *undeclared* is
// indistinguishable from the computed fallback. The first is held key by key by
// internal/bmmgen's TestRMInfoAbsenceAccountsForThePinnedBMM (see the file
// header); the second is pressed from the answering side in arm (c), which
// sweeps the names a stray entry could plausibly have been keyed on.
func TestProbe098NoOverlapWithTheUniverse(t *testing.T) {
	r := reducePinnedBMM(t)
	rep := reporter(t, rminfo.Default)

	shipped := rminfo.Default.KnownRMTypes()
	if len(shipped) == 0 {
		t.Fatal("KnownRMTypes is empty — the arm would be vacuous")
	}
	member := make(map[string]bool, len(shipped))
	for _, name := range shipped {
		member[name] = true
		if got := rep.AbsenceReason(name); got != rminfo.AbsenceNone {
			t.Errorf("KnownRMTypes lists %s but AbsenceReason(%s) = %v, want none — "+
				"membership and absence disagree", name, name, got)
		}
	}
	for _, name := range declaredNames(r) {
		if rep.AbsenceReason(name) != rminfo.AbsenceNone {
			continue
		}
		if !member[name] {
			t.Errorf("AbsenceReason(%s) = none but KnownRMTypes does not list it — "+
				"a name absent from the universe reported as a member of it", name)
		}
	}
	t.Logf("%d universe members, all reporting none", len(shipped))
}

// TestProbe098UndeclaredIsComputedNotStored — arm (c). A name no pinned schema
// declares reports *undeclared*, with no table entry behind it.
//
// The witnesses come from two places. The derived ones are names the schemas
// MENTION without declaring — the sharpest available, because a generator can
// only store a name it read from the schemas, so this is where a stray entry
// would come from. The synthetic ones cover the shapes a caller supplies: a
// name that does not exist, a near-miss of a real table key, a wrong-case
// spelling, a package path where a class name belongs, and the empty string.
func TestProbe098UndeclaredIsComputedNotStored(t *testing.T) {
	r := reducePinnedBMM(t)
	rep := reporter(t, rminfo.Default)

	mentioned := referencedButNotDeclared(r)
	if len(mentioned) == 0 {
		t.Fatal("the pinned schemas mention no undeclared name — the derived half is vacuous")
	}
	for _, name := range slices.Concat(mentioned, syntheticUndeclared) {
		if r.classOf[name] != nil {
			t.Errorf("%q is declared by the pinned schemas — a wrong witness for this arm", name)
			continue
		}
		if got := rep.AbsenceReason(name); got != rminfo.AbsenceUndeclared {
			t.Errorf("AbsenceReason(%q) = %v, want undeclared — the table stores a name no "+
				"schema declares, or the fallback is not reached", name, got)
		}
	}
	t.Logf("mentioned but not declared: %v", mentioned)
}

// TestProbe098PrecedenceIsFixed — arm (d). REQ-049's order — primitive, then
// enumeration, then excluded class, then excluded package — puts what a name IS
// ahead of why it was SKIPPED, so a redundant restatement in the generator's
// exclusion lists cannot decide the reported reason.
//
// Two of the three adjacent ranks have witnesses in the pinned schemas, and
// both are pinned BY NAME rather than counted: a substitution inside either set
// would keep a count intact and go unnoticed. The enumeration rank has no
// witness at all here — no pinned enumeration matches a second rule — and that
// gap is covered where it can be: internal/bmmgen's
// TestRMInfoAbsenceReasonsUnderTheFixedPrecedence puts a synthetic enumeration
// into the named-class exclusion set and pins the enumeration answer.
func TestProbe098PrecedenceIsFixed(t *testing.T) {
	r := reducePinnedBMM(t)
	rep := reporter(t, rminfo.Default)

	// The witness the conformance entry names: a primitive_types entry that
	// internal/bmmgen's named-class exclusion list ALSO restates (see
	// probeExcludedClasses's note on belt and braces). Two rules match;
	// primitive must win.
	const witness = "Ordered"
	if !r.primitive[witness] {
		t.Fatalf("%s is not a primitive_types entry — the two-rule witness is gone", witness)
	}
	if !probeExcludedClasses[witness] {
		t.Fatalf("%s is no longer in the restated exclusion set — only one rule matches it now", witness)
	}
	if got := rep.AbsenceReason(witness); got != rminfo.AbsencePrimitive {
		t.Errorf("AbsenceReason(%s) = %v, want primitive — the exclusion list's restatement "+
			"must not decide the reason", witness, got)
	}

	// Primitive over excluded class, for the whole set that matches both.
	var primitiveAndNamed []string
	for _, name := range declaredNames(r) {
		if r.primitive[name] && probeExcludedClasses[name] {
			primitiveAndNamed = append(primitiveAndNamed, name)
		}
	}
	wantPrimitiveAndNamed := []string{"Container", "Numeric", "Ordered", "Ordered_Numeric"}
	if !slices.Equal(primitiveAndNamed, wantPrimitiveAndNamed) {
		t.Errorf("names matching primitive AND the exclusion set = %v, want %v",
			primitiveAndNamed, wantPrimitiveAndNamed)
	}
	for _, name := range primitiveAndNamed {
		if got := rep.AbsenceReason(name); got != rminfo.AbsencePrimitive {
			t.Errorf("AbsenceReason(%s) = %v, want primitive", name, got)
		}
	}

	// Excluded class over excluded package. TUPLE is the representative of
	// this rank, NOT of the one above it: it is a class_definitions entry the
	// exclusion set names and the functional package skips wholesale, and no
	// primitive rule touches it.
	var namedAndPackaged []string
	for _, name := range declaredNames(r) {
		if r.primitive[name] || r.enumeration[name] {
			continue
		}
		if probeExcludedClasses[name] && excludedPackage(r.packageOf[name]) {
			namedAndPackaged = append(namedAndPackaged, name)
		}
	}
	wantNamedAndPackaged := []string{
		"Env", "FUNCTION", "Locale", "Math", "PROCEDURE", "Quantity_converter",
		"ROUTINE", "Statistical_evaluator", "TUPLE", "TUPLE1", "TUPLE2",
	}
	if !slices.Equal(namedAndPackaged, wantNamedAndPackaged) {
		t.Errorf("names matching the exclusion set AND an excluded package = %v, want %v",
			namedAndPackaged, wantNamedAndPackaged)
	}
	if !slices.Contains(namedAndPackaged, "TUPLE") {
		t.Error("TUPLE no longer matches both rules — this rank has lost its named witness")
	}
	for _, name := range namedAndPackaged {
		if got := rep.AbsenceReason(name); got != rminfo.AbsenceExcludedClass {
			t.Errorf("AbsenceReason(%s) = %v, want excluded class", name, got)
		}
	}

	// The enumeration rank's gap, asserted rather than assumed: if a pinned
	// enumeration ever does match a second rule, this arm must gain a
	// witness for it instead of leaving the rank untested.
	for _, name := range declaredNames(r) {
		if !r.enumeration[name] {
			continue
		}
		if probeExcludedClasses[name] || excludedPackage(r.packageOf[name]) {
			t.Errorf("enumeration %s now matches a second rule — pin it as this arm's "+
				"witness for the enumeration rank", name)
		}
	}
}

// TestProbe098AbsentNamesAreUnknownEverywhere — arm (e). Consistency with the
// negative space: a name reporting any absence reason reports not-known on
// every Lookup and Hierarchy question (REQ-049 § Negative-space consistency).
// PROBE-094 arm (e) holds the converse for universe members.
//
// Every out-of-universe name the other four arms exercise is swept here — all
// 74 declared-but-omitted names, plus both sets of undeclared witnesses —
// because the requirement is a property of the whole negative space, not of a
// sample of it.
func TestProbe098AbsentNamesAreUnknownEverywhere(t *testing.T) {
	r := reducePinnedBMM(t)
	rep := reporter(t, rminfo.Default)
	h := hier(t)
	lister, ok := rminfo.Default.(rminfo.AttributeLister)
	if !ok {
		t.Fatal("rminfo.Default does not implement AttributeLister")
	}
	shipped := rminfo.Default.KnownRMTypes()

	var exercised []string
	for _, name := range declaredNames(r) {
		if r.exclusionKindOf(name) != inTheUniverse {
			exercised = append(exercised, name)
		}
	}
	exercised = slices.Concat(exercised, referencedButNotDeclared(r), syntheticUndeclared)
	if len(exercised) == 0 {
		t.Fatal("no out-of-universe name to exercise — the arm is vacuous")
	}

	for _, name := range exercised {
		if got := rep.AbsenceReason(name); got == rminfo.AbsenceNone {
			t.Errorf("AbsenceReason(%q) = none for a name outside the universe", name)
			continue
		}
		// Lookup: the flattened attribute answers.
		if attrs := rminfo.Default.RequiredAttributes(name); attrs != nil {
			t.Errorf("RequiredAttributes(%q) = %v, want nil", name, attrs)
		}
		if attrs := lister.AttributeNames(name); attrs != nil {
			t.Errorf("AttributeNames(%q) = %v, want nil", name, attrs)
		}
		for _, attr := range []string{"name", "value"} {
			if rmType, known := rminfo.Default.AttributeRMType(name, attr); known {
				t.Errorf("AttributeRMType(%q, %s) = (%q, true), want not-known", name, attr, rmType)
			}
			if container, known := rminfo.Default.IsContainer(name, attr); known {
				t.Errorf("IsContainer(%q, %s) = (%t, true), want not-known", name, attr, container)
			}
			if site, ok := h.DeclaredOn(name, attr); ok {
				t.Errorf("DeclaredOn(%q, %s) = (%q, true), want not-found", name, attr, site)
			}
		}
		if slices.Contains(shipped, name) {
			t.Errorf("KnownRMTypes lists %q, which reports an absence reason", name)
		}
		// Hierarchy: the class-graph answers.
		if _, known := h.IsAbstract(name); known {
			t.Errorf("IsAbstract(%q): known=true for a name outside the universe", name)
		}
		if parents, known := h.Parents(name); known || parents != nil {
			t.Errorf("Parents(%q) = (%v, %t), want (nil, false)", name, parents, known)
		}
		if ancestors, known := h.Ancestors(name); known || ancestors != nil {
			t.Errorf("Ancestors(%q) = (%v, %t), want (nil, false)", name, ancestors, known)
		}
		if descendants, known := h.ConcreteDescendants(name); known || descendants != nil {
			t.Errorf("ConcreteDescendants(%q) = (%v, %t), want (nil, false)", name, descendants, known)
		}
		if _, known := h.ConformsTo(name, "LOCATABLE"); known {
			t.Errorf("ConformsTo(%q, LOCATABLE): known=true", name)
		}
		if _, known := h.ConformsTo("COMPOSITION", name); known {
			t.Errorf("ConformsTo(COMPOSITION, %q): known=true", name)
		}
	}
	t.Logf("swept %d out-of-universe names on every Lookup and Hierarchy question", len(exercised))
}

// referencedButNotDeclared collects names the pinned schemas MENTION — as an
// ancestor, or as the type of a property — without declaring them in either
// `class_definitions` or `primitive_types`. Today that is one name — the open
// generic parameter `T`; whatever it is tomorrow, it is the set of names a
// generator could plausibly have read and stored, which is what makes these the
// sharpest *undeclared* witnesses available.
//
// Function signatures are deliberately not walked: ancestors and property types
// are the references the universe rule and the absence table are derived from,
// and widening the walk would pull in names no absence rule ever sees.
func referencedButNotDeclared(r *bmmReduction) []string {
	mentioned := map[string]bool{}
	for _, cls := range r.classOf {
		for _, anc := range cls.Ancestors() {
			mentioned[anc] = true
		}
		for _, prop := range bmmProperties(cls) {
			collectPropertyTypeNames(prop, mentioned)
		}
	}
	var out []string
	for name := range mentioned {
		if r.classOf[name] == nil {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// collectPropertyTypeNames records every type NAME a property's declaration
// names — no type mapping, so this stays independent of REQ-043 rather than
// restating it.
//
// All four Property variants are handled; the interface's marker method is
// unexported, so no fifth can arrive from outside openehr/bmm, and a fifth
// added inside it would show up as witnesses this walk stopped finding — which
// the callers' vacuity checks make loud.
func collectPropertyTypeNames(prop bmm.Property, out map[string]bool) {
	switch p := prop.(type) {
	case *bmm.SingleProperty:
		out[p.TypeName] = true
	case *bmm.SinglePropertyOpen:
		out[p.TypeName] = true
	case *bmm.ContainerProperty:
		if p.TypeDef != nil {
			collectTypeNames(p.TypeDef, out)
		}
	case *bmm.GenericProperty:
		if p.TypeDef != nil {
			collectTypeNames(p.TypeDef, out)
		}
	}
}

// collectTypeNames records the names in a type expression, nested generics and
// containers included. A missing inner type is skipped rather than dereferenced:
// this walk gathers witnesses, and a malformed schema is REQ-047's business, not
// a reason for the probe to panic.
func collectTypeNames(typ bmm.Type, out map[string]bool) {
	if typ == nil {
		return
	}
	switch t := typ.(type) {
	case *bmm.SimpleType:
		out[t.TypeName] = true
	case *bmm.GenericType:
		out[t.RootType] = true
		for _, param := range t.GenericParameters {
			collectTypeNames(param, out)
		}
		for _, param := range t.GenericParameterDefs {
			collectTypeNames(param, out)
		}
	case *bmm.ContainerType:
		out[t.ContainerType] = true
		collectTypeNames(t.TypeDef, out)
	}
}
