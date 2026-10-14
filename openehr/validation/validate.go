package validation

import (
	"slices"

	"github.com/cadasto/openehr-sdk-go/internal/rmaccess"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// Validate validates an in-memory archetypeable RM root against a
// compiled OPT and returns every issue in one pass.
//
// It is the generic form of [ValidateComposition]: the compiled OPT
// drives the walk and `root` is the value source. `root` must be one of
// the RM LOCATABLE concretes the walk recognises: the COMPOSITION content
// closed set plus the demographic PARTY hierarchy (PERSON / ORGANISATION
// / GROUP / AGENT / ROLE and the archetypeable sub-components ADDRESS /
// CONTACT / PARTY_IDENTITY / PARTY_RELATIONSHIP / CAPABILITY) and the EHR
// IM roots FOLDER / EHR_STATUS. A root whose concrete RM type does not
// match the OPT root surfaces as an rm_type_mismatch at "/", not a silent
// pass.
//
// The walk applies the Reference Model rules ([ValidateRM]) as it goes, on
// every node the template admits, the nodes it does not constrain included,
// and returns their findings in the same Result, in the order the walk
// found them. At each attribute it asks the template first: content the
// template refuses, an attribute it prohibits or an item that binds to none
// of its nodes, is reported once and not entered, so no Reference Model
// rule reports anything from inside it. A defect both the template and the
// Reference Model see is reported once: a node the template requires and
// the instance lacks is the template's `required`, and an empty list the
// template counts is reported by the template alone. A value held from two
// places is validated at each, so a defect in it is reported at each path.
// No option leaves the Reference Model rules out; [ValidateRM] applies them
// alone.
//
// Where a template reuses one archetype under several names, an item is
// matched to the template node whose name it carries; an item whose name
// the template does not give goes to such a node that leaves the name
// open, or else to the first of them, whose name check then reports it.
// The item's name decides this match only and appears in no issue path or
// message.
//
// Returns a [Result] whose Issues slice is never nil. A nil root or nil
// compiled template yields a single guard issue (nil_root /
// nil_template). [Result.Coverage] lists what the call did not check.
//
// Validate behaves as a [Validator] built with no options, apart from a nil
// compiled template: Validate reports it, where [Validator.Validate] applies
// the Reference Model rules alone.
func Validate(root any, c *templatecompile.Compiled) Result {
	if root == nil || rmaccess.IsTypedNilPointer(root) {
		return nilRootResult()
	}
	if c == nil {
		return resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_template",
			Detail:   "validation called with a nil compiled template argument",
			Severity: Error,
		}})
	}
	var v Validator
	return v.Validate(root, c)
}

// nilRootResult is the guard a template-driven validation returns for a nil
// root.
func nilRootResult() Result {
	return resultFromIssues([]Issue{{
		Path:     "",
		Code:     "nil_root",
		Detail:   "Validate called with a nil RM root argument",
		Severity: Error,
	}})
}

// issueAt is what makes two issues the same finding: the code, at the path.
type issueAt struct {
	code, path string
}

// withoutRepeats returns the template walk's issues, in walk order, followed
// by each of the floor's issues, in walk order, that does not repeat a
// template issue's code at its path. Both walkers write one path per node,
// so an exact match is one defect seen twice; nothing is stripped from a
// path before comparing, which would merge siblings. Neither list is
// deduplicated against itself: the template walk may report one code twice
// at one path for two clauses, and both stay.
func withoutRepeats(template, floor []Issue) []Issue {
	switch {
	case len(floor) == 0:
		return template
	case len(template) == 0:
		return floor
	}
	seen := make(map[issueAt]struct{}, len(template))
	for _, i := range template {
		seen[issueAt{code: i.Code, path: i.Path}] = struct{}{}
	}
	out := slices.Grow(template, len(floor))
	for _, i := range floor {
		if _, repeat := seen[issueAt{code: i.Code, path: i.Path}]; repeat {
			continue
		}
		out = append(out, i)
	}
	return out
}

// ValidateDemographic validates an in-memory demographic PARTY (PERSON,
// ORGANISATION, GROUP, AGENT or ROLE) against a compiled OPT, the
// Reference Model rules included, as [Validate] describes.
// It guards the nil party (yielding nil_party), then delegates to
// [Validate]. The PARTY sub-components (ADDRESS, CONTACT, PARTY_IDENTITY,
// PARTY_RELATIONSHIP, CAPABILITY) are validated in place as the walk
// descends, or as roots in their own right via [Validate].
//
// Because rm.Party is an interface, the guard catches both a bare-nil
// interface and a typed-nil concrete behind it (e.g. a (*rm.Person)(nil)
// argument). Both yield nil_party rather than the generic nil_root.
func ValidateDemographic(party rm.Party, c *templatecompile.Compiled) Result {
	if party == nil || rmaccess.IsTypedNilPointer(party) {
		return resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_party",
			Detail:   "ValidateDemographic called with a nil rm.Party argument",
			Severity: Error,
		}})
	}
	return Validate(party, c)
}

// ValidateFolder validates an in-memory FOLDER (a directory tree root or
// sub-folder) against a compiled OPT, the Reference Model rules included,
// as [Validate] describes. A nil folder yields nil_folder.
func ValidateFolder(folder *rm.Folder, c *templatecompile.Compiled) Result {
	if folder == nil {
		return resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_folder",
			Detail:   "ValidateFolder called with a nil *rm.Folder argument",
			Severity: Error,
		}})
	}
	return Validate(folder, c)
}

// ValidateEHRStatus validates an in-memory EHR_STATUS against a compiled
// OPT, the Reference Model rules included, as [Validate] describes. A nil
// status yields nil_ehr_status.
func ValidateEHRStatus(status *rm.EHRStatus, c *templatecompile.Compiled) Result {
	if status == nil {
		return resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_ehr_status",
			Detail:   "ValidateEHRStatus called with a nil *rm.EHRStatus argument",
			Severity: Error,
		}})
	}
	return Validate(status, c)
}
