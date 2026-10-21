package rmwrite

import (
	"fmt"
	"unicode/utf8"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// writeParticipationSingle writes a PARTICIPATION's function, mode,
// performer and time. The function, a DV_TEXT or its descendant
// DV_CODED_TEXT, and the performer, any PARTY_PROXY, are stored as given.
func writeParticipationSingle(p *rm.Participation, attr string, child any) error {
	switch attr {
	case "function":
		return assignTextAsGiven(child, func(v rm.DVTextLike) { p.Function = v }, attr)
	case "mode":
		return assignVia(child, func(v rm.DVCodedText) { p.Mode = &v }, attr, "DV_CODED_TEXT")
	case "performer":
		v, ok := child.(rm.PartyProxy)
		if !ok || rm.IsTypedNil(v) {
			return mismatch(attr, child, "PARTY_PROXY")
		}
		p.Performer = v
		return nil
	case "time":
		return assignVia(child, func(v rm.DVInterval[rm.DVDateTime]) { p.Time = &v }, attr, "DV_INTERVAL<DV_DATE_TIME>")
	}
	return fmt.Errorf("%w: *rm.Participation has no single attr %q", ErrUnknownAttribute, attr)
}

// appendParticipation appends a PARTICIPATION, given as a value or as a
// pointer, to list: an EVENT_CONTEXT's participations or an ENTRY's
// other_participations.
func appendParticipation(list *[]rm.Participation, attr string, child any) error {
	return assignVia(child, func(v rm.Participation) { *list = append(*list, v) }, attr, "PARTICIPATION")
}

// writeTermMappingSingle writes a TERM_MAPPING's match, purpose and target.
func writeTermMappingSingle(m *rm.TermMapping, attr string, child any) error {
	switch attr {
	case "match":
		return writeCharacter(&m.Match, attr, child)
	case "purpose":
		return assignVia(child, func(v rm.DVCodedText) { m.Purpose = &v }, attr, "DV_CODED_TEXT")
	case "target":
		return assignVia(child, func(v rm.CodePhrase) { m.Target = v }, attr, "CODE_PHRASE")
	}
	return fmt.Errorf("%w: *rm.TermMapping has no single attr %q", ErrUnknownAttribute, attr)
}

// writeCharacter stores a Character attribute from a string or an
// rm.Character. A value that is not exactly one character is no RM
// Character, so it is a type mismatch too, and its message says that the
// length is what fails. The message carries the attribute, never the
// value.
func writeCharacter(dst *rm.Character, attr string, child any) error {
	var s string
	switch v := child.(type) {
	case string:
		s = v
	case rm.Character:
		s = string(v)
	default:
		return mismatch(attr, child, "Character")
	}
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) != 1 {
		return fmt.Errorf("%w: attr %q takes a Character and the string is not one character", ErrTypeMismatch, attr)
	}
	*dst = rm.Character(s)
	return nil
}

// assignTextAsGiven stores a DV_TEXT or its descendant DV_CODED_TEXT, as
// given, by value or by pointer. A nil child or a nil pointer is a type
// mismatch.
func assignTextAsGiven(child any, set func(rm.DVTextLike), attr string) error {
	v, ok := child.(rm.DVTextLike)
	if !ok || rm.IsTypedNil(v) {
		return mismatch(attr, child, "DV_TEXT")
	}
	set(v)
	return nil
}
