// Package cstringvalue picks the string the instance generator writes under a
// C_STRING constraint, so that the generator and the Simplified Formats
// decoder, which completes an ACTIVITY's action_archetype_id from the
// template, write the same value for the same constraint.
//
// It imports only the standard library and the template's primitive
// constraints.
package cstringvalue

import (
	"cmp"
	"errors"
	"fmt"
	"regexp/syntax"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// AnyActionArchetype is the RM default of ACTIVITY.action_archetype_id, a
// pattern in // delimiters that matches the id of any archetype.
const AnyActionArchetype = "/.*/"

// ErrNoValue reports a C_STRING that no string [Pick] builds satisfies: no
// list member passes the pattern, the pattern matches nothing, or the
// pattern does not compile.
var ErrNoValue = errors.New("no string satisfies the C_STRING constraint")

// Pick returns a string cs accepts: example when cs accepts it, else the
// first list member cs accepts, else, where cs lists no string or its
// list is open (AOM 1.4 list_open, which names values without limiting the
// string to them), a string the pattern's syntax builds. That is the shortest build where it
// is not empty; where it is empty, which an attribute such as
// DV_TEXT.value refuses, a non-empty build with one repetition of the
// first optional or starred part that yields a character. Where cs refuses
// that build, as a word boundary in the pattern can, Pick takes the
// shortest non-empty string of a wider, bounded search that cs accepts
// (the one [First] makes), and keeps the empty string only where cs
// accepts no non-empty string of it. Acceptance is
// [constraints.CString.Validate], which matches a pattern against the
// whole string, so a value that only contains a match is never chosen.
// Pick returns an error when example is not a string, and [ErrNoValue]
// when none of these is accepted.
func Pick(cs constraints.CString, example any) (string, error) {
	s, ok := example.(string)
	if !ok {
		return "", fmt.Errorf("C_STRING example value is %T, want string", example)
	}
	accepts := func(v string) bool { return len(cs.Validate(v)) == 0 }
	if accepts(s) {
		return s, nil
	}
	for _, member := range cs.List {
		if accepts(member) {
			return member, nil
		}
	}
	if (len(cs.List) == 0 || cs.ListOpen) && cs.Pattern != "" {
		if v, ok := patternPick(cs.Pattern, accepts); ok {
			return v, nil
		}
	}
	return "", ErrNoValue
}

// First returns the first string that both cs and keep accept: a list
// member, in list order, or, where cs lists none or its list is open, a
// string its pattern builds, shortest first, from the bounded search
// [Pick] makes when its first builds are refused. ok is false when there
// is none, as for a constraint with no pattern and a closed or empty list.
func First(cs constraints.CString, keep func(string) bool) (string, bool) {
	accepts := func(v string) bool { return keep(v) && len(cs.Validate(v)) == 0 }
	for _, member := range cs.List {
		if accepts(member) {
			return member, true
		}
	}
	if len(cs.List) > 0 && !cs.ListOpen || cs.Pattern == "" {
		return "", false
	}
	re, ok := parse(cs.Pattern)
	if !ok {
		return "", false
	}
	for _, c := range candidates(re) {
		if accepts(c) {
			return c, true
		}
	}
	return "", false
}

// ActionArchetypeID returns the action_archetype_id of an ACTIVITY whose
// template constrains that attribute by cs: [AnyActionArchetype] where cs
// lists no string and accepts it, as a constraint with neither a pattern
// nor a list does, and otherwise what [Pick] returns for cs and example. The
// RM default comes before the example there, because a string such as the
// example, which a pattern like .* also accepts, names no action
// archetype. A list gives the values itself, so cs with a list gets what
// Pick returns, even where the list holds the RM default as well. A
// template that does not constrain the attribute gives [AnyActionArchetype]
// too.
func ActionArchetypeID(cs constraints.CString, example any) (string, error) {
	if len(cs.List) == 0 && len(cs.Validate(AnyActionArchetype)) == 0 {
		return AnyActionArchetype, nil
	}
	return Pick(cs, example)
}

// parse parses pattern as Perl syntax and simplifies it, which rewrites
// every counted repetition. ok is false when the pattern does not parse.
func parse(pattern string) (*syntax.Regexp, bool) {
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return nil, false
	}
	return re.Simplify(), true
}

// patternPick returns the string [Pick] takes from the pattern of a
// C_STRING with no list or an open one, judged by accepts. ok is false when the
// pattern does not parse, matches nothing, or accepts no string the search
// builds, the empty string included.
func patternPick(pattern string, accepts func(string) bool) (string, bool) {
	re, ok := parse(pattern)
	if !ok {
		return "", false
	}
	var b strings.Builder
	if !writeShortest(&b, re) {
		return "", false
	}
	shortest := b.String()
	if shortest != "" && accepts(shortest) {
		return shortest, true
	}
	if shortest == "" {
		var n strings.Builder
		if writeNonEmpty(&n, re) && accepts(n.String()) {
			return n.String(), true
		}
	}
	for _, c := range candidates(re) {
		if c != "" && accepts(c) {
			return c, true
		}
	}
	if accepts("") {
		return "", true
	}
	return "", false
}

// maxCandidates bounds the strings the search builds for a pattern, and
// for each part of it, so a long pattern costs at most that many
// acceptance checks.
const maxCandidates = 256

// candidates returns up to maxCandidates strings re builds, shortest
// first: each optional part absent and present, each starred part zero to
// two times and each plus part once or twice, every alternative, and for a
// class or a dot a word character and one that is not, so that a word
// boundary the first choice breaks can hold for another.
func candidates(re *syntax.Regexp) []string {
	out := build(re)
	slices.SortStableFunc(out, func(a, b string) int {
		return cmp.Compare(utf8.RuneCountInString(a), utf8.RuneCountInString(b))
	})
	return out
}

// build returns the strings candidates sorts, in the order re's parts give
// them.
func build(re *syntax.Regexp) []string {
	switch re.Op {
	case syntax.OpEmptyMatch,
		syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText, syntax.OpEndText,
		syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		// Zero width; the acceptance check decides whether a boundary
		// holds.
		return []string{""}
	case syntax.OpLiteral:
		return []string{string(re.Rune)}
	case syntax.OpCharClass:
		return classSamples(re.Rune)
	case syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		return []string{"a", "-"}
	case syntax.OpCapture:
		return build(re.Sub[0])
	case syntax.OpQuest:
		return union([]string{""}, build(re.Sub[0]))
	case syntax.OpStar:
		sub := build(re.Sub[0])
		return union(union([]string{""}, sub), product(sub, sub))
	case syntax.OpPlus:
		sub := build(re.Sub[0])
		return union(sub, product(sub, sub))
	case syntax.OpConcat:
		out := []string{""}
		for _, sub := range re.Sub {
			out = product(out, build(sub))
		}
		return out
	case syntax.OpAlternate:
		var out []string
		for _, sub := range re.Sub {
			out = union(out, build(sub))
		}
		return out
	case syntax.OpNoMatch:
		// The pattern admits no string.
		return nil
	case syntax.OpRepeat:
		// Simplify rewrites every counted repetition, so none reaches
		// here.
		return nil
	default:
		return nil
	}
}

// union returns a followed by the strings of b it does not hold, at most
// maxCandidates in all.
func union(a, b []string) []string {
	out := slices.Clip(a)
	for _, s := range b {
		if len(out) == maxCandidates {
			break
		}
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// product returns each string of a followed by each of b, without
// repeats, at most maxCandidates in all.
func product(a, b []string) []string {
	var out []string
	for _, x := range a {
		for _, y := range b {
			if len(out) == maxCandidates {
				return out
			}
			if s := x + y; !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
	}
	return out
}

// writeNonEmpty writes to b the string writeShortest writes for re when
// that string is not empty, and otherwise a non-empty string built the
// same way except at the first part that yields a character. It reports
// false, and writes nothing, when re builds no non-empty string.
func writeNonEmpty(b *strings.Builder, re *syntax.Regexp) bool {
	switch re.Op {
	case syntax.OpLiteral, syntax.OpCharClass, syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		// One character or more, or no string at all for an empty class.
		var part strings.Builder
		if !writeShortest(&part, re) || part.Len() == 0 {
			return false
		}
		b.WriteString(part.String())
		return true
	case syntax.OpCapture, syntax.OpPlus, syntax.OpStar, syntax.OpQuest:
		// One repetition, which is also the minimum of a plus.
		return writeNonEmpty(b, re.Sub[0])
	case syntax.OpConcat:
		var short strings.Builder
		if !writeShortest(&short, re) {
			return false
		}
		if short.Len() > 0 {
			b.WriteString(short.String())
			return true
		}
		// Every part's shortest string is empty, so the first part that
		// yields a character gives the whole string.
		for _, sub := range re.Sub {
			var part strings.Builder
			if writeNonEmpty(&part, sub) {
				b.WriteString(part.String())
				return true
			}
		}
		return false
	case syntax.OpAlternate:
		for _, sub := range re.Sub {
			var alt strings.Builder
			if writeNonEmpty(&alt, sub) {
				b.WriteString(alt.String())
				return true
			}
		}
		return false
	case syntax.OpEmptyMatch, syntax.OpNoMatch,
		syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText, syntax.OpEndText,
		syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		// Zero width, or no match at all.
		return false
	case syntax.OpRepeat:
		// Simplify rewrites every counted repetition, x{0} into an empty
		// match, so none reaches here.
		return false
	default:
		return false
	}
}

func writeShortest(b *strings.Builder, re *syntax.Regexp) bool {
	switch re.Op {
	case syntax.OpEmptyMatch, syntax.OpStar, syntax.OpQuest,
		syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText, syntax.OpEndText,
		syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		// Zero width, or zero repetitions. The caller checks the whole
		// string against the pattern, so a boundary that does not hold
		// there is caught.
		return true
	case syntax.OpLiteral:
		for _, r := range re.Rune {
			b.WriteRune(r)
		}
		return true
	case syntax.OpCharClass:
		r, ok := classRune(re.Rune)
		if ok {
			b.WriteRune(r)
		}
		return ok
	case syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		b.WriteByte('a')
		return true
	case syntax.OpCapture, syntax.OpPlus:
		return writeShortest(b, re.Sub[0])
	case syntax.OpRepeat:
		for range re.Min {
			if !writeShortest(b, re.Sub[0]) {
				return false
			}
		}
		return true
	case syntax.OpConcat:
		for _, sub := range re.Sub {
			if !writeShortest(b, sub) {
				return false
			}
		}
		return true
	case syntax.OpAlternate:
		for _, sub := range re.Sub {
			var alt strings.Builder
			if writeShortest(&alt, sub) {
				b.WriteString(alt.String())
				return true
			}
		}
		return false
	case syntax.OpNoMatch:
		// The pattern admits no string.
		return false
	default:
		return false
	}
}

// classRune picks one character of a character class given as rune
// ranges: a letter or digit when the class has one, else the first
// printable ASCII character, else the class's first character.
func classRune(ranges []rune) (rune, bool) {
	if len(ranges) < 2 {
		return 0, false
	}
	for _, r := range "aA0" {
		if inClass(ranges, r) {
			return r, true
		}
	}
	if r, ok := firstInClass(ranges, func(rune) bool { return true }); ok {
		return r, true
	}
	return ranges[0], true
}

// classSamples returns the characters of a class given as rune ranges
// that the search tries: the one classRune picks, then the first ASCII
// word character and the first printable ASCII character that is not one,
// where the class holds them.
func classSamples(ranges []rune) []string {
	first, ok := classRune(ranges)
	if !ok {
		return nil
	}
	out := []string{string(first)}
	for _, word := range []bool{true, false} {
		if r, ok := firstInClass(ranges, func(r rune) bool { return isWordRune(r) == word }); ok {
			out = union(out, []string{string(r)})
		}
	}
	return out
}

// firstInClass returns the first printable ASCII character other than
// the space that the class given as rune ranges holds and keep accepts.
func firstInClass(ranges []rune, keep func(rune) bool) (rune, bool) {
	for r := rune(0x21); r < 0x7f; r++ {
		if keep(r) && inClass(ranges, r) {
			return r, true
		}
	}
	return 0, false
}

// inClass reports whether the class given as rune ranges holds r.
func inClass(ranges []rune, r rune) bool {
	for i := 0; i+1 < len(ranges); i += 2 {
		if ranges[i] <= r && r <= ranges[i+1] {
			return true
		}
	}
	return false
}

// isWordRune reports whether r is an ASCII word character, the set \b
// and \B read: a letter, a digit or the underscore.
func isWordRune(r rune) bool {
	return r == '_' || '0' <= r && r <= '9' || 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z'
}
