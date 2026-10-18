package bmmtype

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
)

// Split, Class and Substitute take shortcuts for the names they meet most.
// The functions below are the plain implementation those shortcuts stand in
// for, copied from before the shortcuts were added: Split's full parse,
// Class through Split, and Substitute through Split and the formal
// parameter table. They use the package's own wellFormed and
// formalParameters, so a later change to either still has to be matched by
// the shortcuts.

func referenceSplit(name string) (class string, params []string, ok bool) {
	name = strings.TrimSpace(name)
	if !wellFormed(name) {
		return "", nil, false
	}
	open := strings.IndexByte(name, '<')
	if open < 0 {
		return name, nil, true
	}
	inner := name[open+1 : len(name)-1]
	depth, start := 0, 0
	for i := range len(inner) {
		switch inner[i] {
		case '<':
			depth++
		case '>':
			depth--
		case ',':
			if depth == 0 {
				params = append(params, strings.TrimSpace(inner[start:i]))
				start = i + 1
			}
		}
	}
	params = append(params, strings.TrimSpace(inner[start:]))
	return strings.TrimSpace(name[:open]), params, true
}

func referenceClass(name string) string {
	class, _, ok := referenceSplit(name)
	if !ok {
		return name
	}
	return class
}

func referenceSubstitute(owner, declared string) string {
	class, params, ok := referenceSplit(owner)
	if !ok || len(params) == 0 {
		return declared
	}
	formals := formalParameters[class]
	if len(formals) != len(params) {
		return declared
	}
	if i := slices.Index(formals, declared); i >= 0 {
		return params[i]
	}
	return declared
}

// edgeNames are type names at the edges of the shortcuts: plain names that
// take them, and names one character away from plain that must not.
var edgeNames = []string{
	// Plain names, which take the shortcut.
	"A", "a", "_", "9", "T", "DV_TEXT", "Point_interval", "abc_123",
	// Plain names with white space around them.
	" DV_TEXT", "DV_TEXT ", "\tDV_TEXT\n", "\rDV_TEXT\f", "\u00a0DV_TEXT", "DV_TEXT\u0085", "\u3000DV_TEXT\u3000",
	// Empty and blank.
	"", " ", "\t", "\u00a0", "\u0085",
	// A character inside the name that is not a letter, digit or underscore.
	"DV INTERVAL", "DV\tTEXT", "DV\u00a0TEXT", "DV-TEXT", "DV.TEXT", "Ä", "DV_TEXTÄ",
	"A\u200bB", "A\x00B", "\xff", "A\xffB", "A,B",
	// Generic names.
	"DV_INTERVAL<DV_QUANTITY>", "Hash<String, String>", "A<B<C>>",
	"FUNCTION<TUPLE2<Integer, Real>, Boolean>", " DV_INTERVAL < DV_COUNT > ", "A <B>", "A<B> ",
	"\u00a0DV_INTERVAL<\u0085DV_COUNT\v>\u00a0", "DV INTERVAL<DV_COUNT>", "DV_INTERVAL<DV QUANTITY>",
	"DV_INTERVAL<DV_QUANTITY, DV_COUNT>", "DV_TEXT<DV_QUANTITY>", "DV_INTERVAL<DV_INTERVAL<DV_COUNT>>",
	"POINT_EVENT<ITEM_TREE>", "Point_interval<Integer>",
	// Brackets that do not balance, a part that is missing, or text after
	// the closing bracket.
	"<>", "<", ">", "A<", "A>", "A<>", "A< >", "<A>", "A<B", "A<B>>", "A<B<C>", "A<B>C",
	"A<B><C>", "A<B<C> D>", "A<,B>", "A<B,>", "A<\v>", "DV_INTERVAL<DV_QUANTITY>junk",
}

// declaredNames are the declared types Substitute is compared on: every
// formal parameter name in the table, and names that are close to one but
// are not.
func declaredNames() []string {
	names := []string{"", "U", "t", " T", "T ", "T<X>", "Boolean", "DV_TEXT", "DV_QUANTITY"}
	for formals := range maps.Values(formalParameters) {
		names = append(names, formals...)
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// ownerNames are the owners Substitute is compared on: the edge names, and
// every generic class in the table spelled bare, with as many actual
// parameters as it has formal ones, and with one too many.
func ownerNames() []string {
	names := slices.Clone(edgeNames)
	for _, class := range slices.Sorted(maps.Keys(formalParameters)) {
		var actuals []string
		for i := range formalParameters[class] {
			actuals = append(actuals, fmt.Sprintf("P%d", i))
		}
		names = append(names,
			class,
			class+"<"+strings.Join(actuals, ", ")+">",
			class+"<"+strings.Join(append(actuals, "EXTRA"), ", ")+">",
		)
	}
	return names
}

// splitDiff describes how Split differs from referenceSplit on name, or is
// empty when they agree.
func splitDiff(name string) string {
	class, params, ok := Split(name)
	wantClass, wantParams, wantOK := referenceSplit(name)
	if class == wantClass && slices.Equal(params, wantParams) && (params == nil) == (wantParams == nil) && ok == wantOK {
		return ""
	}
	return fmt.Sprintf("Split(%q) = (%q, %#v, %v), reference gives (%q, %#v, %v)",
		name, class, params, ok, wantClass, wantParams, wantOK)
}

// classDiff describes how Class differs from referenceClass on name, or is
// empty when they agree.
func classDiff(name string) string {
	got, want := Class(name), referenceClass(name)
	if got == want {
		return ""
	}
	return fmt.Sprintf("Class(%q) = %q, reference gives %q", name, got, want)
}

// substituteDiff describes how Substitute differs from referenceSubstitute
// on owner and declared, or is empty when they agree.
func substituteDiff(owner, declared string) string {
	got, want := Substitute(owner, declared), referenceSubstitute(owner, declared)
	if got == want {
		return ""
	}
	return fmt.Sprintf("Substitute(%q, %q) = %q, reference gives %q", owner, declared, got, want)
}

func TestSplitMatchesReference(t *testing.T) {
	for _, name := range edgeNames {
		if d := splitDiff(name); d != "" {
			t.Error(d)
		}
	}
}

func TestClassMatchesReference(t *testing.T) {
	for _, name := range edgeNames {
		if d := classDiff(name); d != "" {
			t.Error(d)
		}
	}
}

func TestSubstituteMatchesReference(t *testing.T) {
	for _, owner := range ownerNames() {
		for _, declared := range declaredNames() {
			if d := substituteDiff(owner, declared); d != "" {
				t.Error(d)
			}
		}
	}
}

// FuzzClassMatchesReference also holds Split to its reference, since Class
// and Split share the shortcut for a plain name.
func FuzzClassMatchesReference(f *testing.F) {
	for _, name := range edgeNames {
		f.Add(name)
	}
	f.Fuzz(func(t *testing.T, name string) {
		if d := classDiff(name); d != "" {
			t.Error(d)
		}
		if d := splitDiff(name); d != "" {
			t.Error(d)
		}
	})
}

func FuzzSubstituteMatchesReference(f *testing.F) {
	for _, owner := range ownerNames() {
		for _, declared := range declaredNames() {
			f.Add(owner, declared)
		}
	}
	f.Fuzz(func(t *testing.T, owner, declared string) {
		if d := substituteDiff(owner, declared); d != "" {
			t.Error(d)
		}
	})
}
