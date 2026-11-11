// Package cstringvalue picks the string the instance generator writes under a
// C_STRING constraint, so that the generator and the Simplified Formats
// decoder, which completes an ACTIVITY's action_archetype_id from the
// template, write the same value for the same constraint.
//
// It imports only the standard library and the template's primitive
// constraints.
package cstringvalue

import (
	"errors"
	"fmt"
	"regexp/syntax"
	"strings"

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
// first list member cs accepts, else, for a pattern-only constraint, the
// shortest string the pattern's syntax builds. Where that is the empty
// string, which an attribute such as DV_TEXT.value refuses, Pick takes a
// non-empty string the syntax builds instead, with one repetition of the
// first optional or starred part that yields a character, and keeps the
// empty string only where cs accepts no such string. Acceptance is
// [constraints.CString.Validate], which matches a pattern against the whole
// string, so a value that only contains a match is never chosen. Pick
// returns an error when example is not a string, and [ErrNoValue] when none
// of these is accepted.
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
	if len(cs.List) == 0 && cs.Pattern != "" {
		if m, built := patternExample(cs.Pattern); built {
			if m == "" {
				if n, ok := patternNonEmpty(cs.Pattern); ok && accepts(n) {
					return n, nil
				}
			}
			if accepts(m) {
				return m, nil
			}
		}
	}
	return "", ErrNoValue
}

// ActionArchetypeID returns the action_archetype_id of an ACTIVITY whose
// template constrains that attribute by cs: [AnyActionArchetype] where cs
// accepts it, as a constraint with neither a pattern nor a list does, and
// otherwise what [Pick] returns for cs and example. The RM default comes
// before a list member and the example, because a string such as the
// example, which a pattern like .* also accepts, names no action
// archetype. A template that does not constrain the attribute gives
// [AnyActionArchetype] too.
func ActionArchetypeID(cs constraints.CString, example any) (string, error) {
	if len(cs.Validate(AnyActionArchetype)) == 0 {
		return AnyActionArchetype, nil
	}
	return Pick(cs, example)
}

// patternExample builds the shortest string the regular expression
// pattern describes: no repetition beyond the minimum, the first
// alternative, and one plain character for a class or a dot. built is
// false when the pattern does not parse or matches nothing.
func patternExample(pattern string) (string, bool) {
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return "", false
	}
	var b strings.Builder
	if !writeShortest(&b, re.Simplify()) {
		return "", false
	}
	return b.String(), true
}

// patternNonEmpty builds a non-empty string the regular expression
// pattern describes: the string [patternExample] builds where that is not
// empty, and where it is, the same walk with one repetition of the first
// optional or starred part that yields a character, or the first
// alternative that yields one. built is false when the pattern does not
// parse or builds no non-empty string, as with x{0}.
func patternNonEmpty(pattern string) (string, bool) {
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return "", false
	}
	var b strings.Builder
	if !writeNonEmpty(&b, re.Simplify()) {
		return "", false
	}
	return b.String(), true
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
	in := func(r rune) bool {
		for i := 0; i+1 < len(ranges); i += 2 {
			if ranges[i] <= r && r <= ranges[i+1] {
				return true
			}
		}
		return false
	}
	for _, r := range "aA0" {
		if in(r) {
			return r, true
		}
	}
	for r := rune(0x21); r < 0x7f; r++ {
		if in(r) {
			return r, true
		}
	}
	return ranges[0], true
}
